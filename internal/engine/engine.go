// Package engine executes search provider chains described as a graph.
package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/logging"
	"github.com/liberide/serpent-seek/internal/providers"
	"github.com/liberide/serpent-seek/internal/store"
)

// Input is a search request submitted to the engine.
type Input struct {
	Query  string
	Count  int
	Client string
	// UserID is the authenticated owner.
	UserID string
}

// Output is the engine result.
type Output struct {
	Request *store.Request
	Rows    providers.Rows
}

// Engine runs chain graphs against the provider registry.
type Engine struct {
	store    store.Storage
	reg      *providers.Registry
	settings *config.Manager
	sink     Sink
	log      *logging.Logger

	limiter *semLimiter
	seq     atomic.Uint64
}

// New builds an Engine.
func New(st store.Storage, reg *providers.Registry, settings *config.Manager, sink Sink, log *logging.Logger) *Engine {
	limit := 32
	if cfg := settings.Config(); cfg != nil {
		limit = cfg.MaxConcurrency
	}
	return &Engine{
		store:    st,
		reg:      reg,
		settings: settings,
		sink:     sink,
		log:      log,
		limiter:  newSemLimiter(limit),
	}
}

// registry exposes the provider registry (used by handlers for /test).
func (e *Engine) Registry() *providers.Registry { return e.reg }

// TestResult is the outcome of a provider connectivity test.
type TestResult struct {
	Result providers.Result `json:"result"`
	TookMS int              `json:"took_ms"`
}

// TestProvider runs a tiny query against one provider instance using its stored
// configuration. The id is the provider instance id (providers.id).
func (e *Engine) TestProvider(ctx context.Context, id string) TestResult {
	start := time.Now()
	var inst *store.Provider
	if row, err := e.store.GetProvider(ctx, id); err == nil {
		inst = row
	}
	driver := id
	if inst != nil {
		driver = strings.ToLower(inst.Code)
	}
	provider, ok := e.reg.Get(driver)
	if !ok {
		return TestResult{Result: providers.Result{Kind: "api", Provider: id, Error: "unknown provider driver " + driver}}
	}
	params := e.buildParams(ctx, driver, inst, &store.ChainNode{Params: map[string]string{}})
	creds := e.buildCredentials(inst)
	testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if inst != nil && inst.ProxyID != "" {
		if proxy, perr := e.store.GetProxy(ctx, inst.ProxyID); perr == nil {
			if pc := e.proxyConfig(proxy); pc != nil {
				testCtx = providers.WithProxy(testCtx, pc)
			}
		}
	}
	r := provider.Search(testCtx, providers.Query{Text: "test", Count: 3, Extra: map[string]string{}}, creds, params)
	r.Provider = e.providerDisplay(id, inst)
	r.Error = e.log.Redact(r.Error)
	return TestResult{Result: r, TookMS: int(time.Since(start) / time.Millisecond)}
}

// runContext carries per-request execution state.
type runContext struct {
	req       *store.Request
	plan      *Plan
	providers map[string]*store.Provider
	proxies   map[string]*store.Proxy
	startedAt time.Time
	answerMu  sync.Mutex // guards req.Answer when branches run in parallel
}

// Execute runs a search synchronously and returns the persisted request.
func (e *Engine) Execute(ctx context.Context, in Input) (*Output, error) {
	rc, err := e.prepare(ctx, in)
	if err != nil {
		return nil, err
	}
	return e.run(ctx, rc)
}

// Start runs a search asynchronously, returning the request row immediately so
// the UI can subscribe to its live SSE stream.
func (e *Engine) Start(ctx context.Context, in Input) (*store.Request, error) {
	rc, err := e.prepare(ctx, in)
	if err != nil {
		return nil, err
	}
	// Snapshot the initial row before handing the live pointer to the
	// background runner. run/finalize mutate rc.req in place (status, results,
	// timings), so returning rc.req directly would let callers observe - and
	// race with - those writes.
	snapshot := *rc.req
	go func() {
		runCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if _, err := e.run(runCtx, rc); err != nil {
			e.log.Error(rc.req.RID, "", "async run failed: "+err.Error())
		}
	}()
	return &snapshot, nil
}

func (e *Engine) prepare(ctx context.Context, in Input) (*runContext, error) {
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return nil, errors.New("engine: empty query")
	}
	count := in.Count
	if count < 0 {
		count = 0
	}
	if count > 20 {
		count = 20
	}
	// count=0 means "return everything found" (providers are queried without a
	// num/pageSize limit). The global toggle forces that for all requests.
	if e.settings.GetBool(ctx, config.KeyIgnoreRequestCount) {
		count = 0
	}

	chain, err := e.store.GetActiveChain(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, errors.New("engine: no active chain configured")
		}
		return nil, err
	}
	plan, err := BuildPlan(chain)
	if err != nil {
		return nil, err
	}
	// A Join block can opt out of the requested result count: when it is set,
	// providers are queried without a limit and every unique result is returned,
	// exactly like the global IGNORE_REQUEST_COUNT toggle.
	if planIgnoresCount(plan) {
		count = 0
	}

	req := &store.Request{
		ID:            uuid.NewString(),
		RID:           e.nextRID(),
		Query:         query,
		Count:         count,
		Status:        "running",
		ChainID:       chain.ID,
		ChainSnapshot: plan.Snapshot(),
		UserID:        in.UserID,
		Client:        in.Client,
		CreatedAt:     store.Now(),
	}
	if err := e.sink.CreateRequest(ctx, req); err != nil {
		return nil, err
	}
	e.sink.PublishGlobal(EventRequestNew, map[string]any{
		"id":         req.ID,
		"rid":        req.RID,
		"query":      req.Query,
		"status":     req.Status,
		"created_at": req.CreatedAt,
	})

	// Snapshot provider configuration for the whole request.
	rows, err := e.store.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[string]*store.Provider{}
	for _, p := range rows {
		byID[p.ID] = p
	}
	proxyRows, err := e.store.ListProxies(ctx)
	if err != nil {
		return nil, err
	}
	proxiesByID := map[string]*store.Proxy{}
	for _, p := range proxyRows {
		proxiesByID[p.ID] = p
	}

	rc := &runContext{req: req, plan: plan, providers: byID, proxies: proxiesByID, startedAt: time.Now()}
	e.emitDeadProviders(ctx, rc)
	return rc, nil
}

// paramBool reads a boolean-ish node parameter ("1"/"true"/"yes"/"on").
func paramBool(params map[string]string, key string) bool {
	v, ok := params[key]
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// planIgnoresCount reports whether any Join block asked to ignore the request
// count (param ignore_count), so the whole request runs unlimited.
func planIgnoresCount(p *Plan) bool {
	for _, key := range p.order {
		n := p.nodes[key]
		if Kind(n) == store.NodeKindJoin && paramBool(n.Params, "ignore_count") {
			return true
		}
	}
	return false
}

// emitDeadProviders reports provider instances that cannot be used for this request.
func (e *Engine) emitDeadProviders(ctx context.Context, rc *runContext) {
	seen := map[string]bool{}
	for _, node := range rc.req.ChainSnapshot.Nodes {
		if seen[node.ProviderID] {
			continue
		}
		seen[node.ProviderID] = true
		inst := rc.providers[node.ProviderID]
		driver := node.ProviderID
		if inst != nil {
			driver = strings.ToLower(inst.Code)
		}
		display := e.providerDisplay(node.ProviderID, inst)
		if reason := e.instanceDeadReason(ctx, driver, inst); reason != "" {
			e.sink.Publish(rc.req.ID, EventProviderDead, ProviderDead{Provider: display, Reason: reason})
			e.log.Warn(rc.req.RID, display, "provider marked dead: "+deadReasonText(reason, driver))
		}
	}
}

// providerDisplay returns the user-facing label of a provider instance.
func (e *Engine) providerDisplay(id string, p *store.Provider) string {
	if p != nil {
		if strings.TrimSpace(p.Name) != "" {
			return p.Name
		}
		if p.Code != "" {
			return p.Code
		}
	}
	return id
}

// Dead-provider reason codes. The engine persists and emits stable codes; the
// UI translates them and the log line uses deadReasonText for readability.
const (
	deadDisabled       = "provider_disabled"
	deadSearxngURL     = "searxng_url_missing"
	deadServiceAccount = "service_account_missing"
	deadAPIKey         = "api_key_missing"
	deadMarked         = "provider_marked_dead"
	deadUnknownDriver  = "unknown_driver"
)

func (e *Engine) instanceDeadReason(ctx context.Context, driver string, p *store.Provider) string {
	if p != nil && !p.Enabled {
		return deadDisabled
	}
	// SearXNG is always external: it stays dead until an instance URL is set.
	if driver == "searxng" {
		base := ""
		if p != nil {
			base = p.BaseURL
		}
		if strings.TrimSpace(base) == "" {
			return deadSearxngURL
		}
	}
	if driver == "vertex_search" {
		// Vertex AI Search does not support API keys — it needs a service account.
		if strings.TrimSpace(e.buildCredentials(p)["service_account_json"]) == "" {
			return deadServiceAccount
		}
		return ""
	}
	if providerRequiresKey(driver) {
		creds := e.buildCredentials(p)
		if strings.TrimSpace(creds["api_key"]) == "" {
			return deadAPIKey
		}
	}
	return ""
}

// deadReasonText renders a human-readable reason for logs. driver is only used
// by the unknown-driver case.
func deadReasonText(code, driver string) string {
	switch code {
	case deadDisabled:
		return "provider is disabled"
	case deadSearxngURL:
		return "external searxng url is not configured"
	case deadServiceAccount:
		return "missing service account json"
	case deadAPIKey:
		return "missing api key"
	case deadMarked:
		return "provider marked dead"
	case deadUnknownDriver:
		return "unknown provider driver " + driver
	default:
		if strings.HasPrefix(code, deadUnknownDriver+":") {
			return "unknown provider driver " + strings.TrimPrefix(code, deadUnknownDriver+":")
		}
		return code
	}
}

// run walks the chain graph until a result is produced or the graph ends.
func (e *Engine) run(ctx context.Context, rc *runContext) (*Output, error) {
	maxAttempts := e.settings.GetInt(ctx, config.KeyMaxAttempts)
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	e.limiter.setLimit(e.settings.GetInt(ctx, config.KeyMaxConcurrency))

	if rc.plan.Mode() == store.ChainModeFullChain {
		// LEGACY: chains without an explicit Start block keep the original linear
		// single-successor full-chain walk. Only chains built with the new
		// logical blocks get the parallel fan-out/join executor, so upgrading
		// never changes the behaviour of an existing saved chain.
		if rc.plan.HasStartBlock() {
			return e.runFullChain(ctx, rc, maxAttempts)
		}
		return e.runFullChainLegacy(ctx, rc, maxAttempts)
	}
	return e.runFirstSuccess(ctx, rc, maxAttempts, rc.plan.HasStartBlock())
}

// walkState carries mutable per-request walk counters shared by both modes.
// Full-chain mode executes sibling branches in parallel, so every accessor is
// mutex-guarded and the shared attempt budget is reserved with takeStep.
type walkState struct {
	mu          sync.Mutex
	maxAttempts int
	stepNo      int
	dead        map[string]bool
}

func newWalkState(maxAttempts int) *walkState {
	return &walkState{maxAttempts: maxAttempts, dead: map[string]bool{}}
}

// takeStep reserves one attempt from the shared budget, reporting false when
// the budget is exhausted.
func (ws *walkState) takeStep() bool {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.stepNo >= ws.maxAttempts {
		return false
	}
	ws.stepNo++
	return true
}

// addSteps consumes extra attempts (multi-call provider stages).
func (ws *walkState) addSteps(n int) {
	if n <= 0 {
		return
	}
	ws.mu.Lock()
	ws.stepNo += n
	ws.mu.Unlock()
}

func (ws *walkState) hasBudget() bool {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return ws.stepNo < ws.maxAttempts
}

func (ws *walkState) steps() int {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return ws.stepNo
}

func (ws *walkState) markDead(id string) {
	ws.mu.Lock()
	ws.dead[id] = true
	ws.mu.Unlock()
}

func (ws *walkState) isDead(id string) bool {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return ws.dead[id]
}

// nodeRun is the outcome of executing one block.
type nodeRun struct {
	rows      providers.Rows
	won       bool
	result    providers.Result
	attempted bool // false when the shared attempt budget was already exhausted
}

// nodeTarget is the resolved execution context of one graph node.
type nodeTarget struct {
	inst     *store.Provider
	proxy    *store.Proxy
	driver   string
	display  string
	provider providers.Provider
	known    bool
	answer   bool   // mode: answer — generated output, excluded from Row merge
	reason   string // non-empty: node must be skipped
}

// stageCollector is the engine's providers.StepReporter sink: multi-call
// drivers (pubmed) report every upstream HTTP call here.
type stageCollector struct {
	calls []providers.StageCall
}

func (c *stageCollector) Report(sc providers.StageCall) { c.calls = append(c.calls, sc) }

// honorRetryAfter pauses before moving to the next node when the upstream
// explicitly demanded a backoff (Stack Exchange `backoff`, Retry-After).
func honorRetryAfter(ctx context.Context, r providers.Result) {
	if r.RetryAfterMS <= 0 {
		return
	}
	sleepCtx(ctx, capDelay(time.Duration(r.RetryAfterMS)*time.Millisecond))
}

// resolveNode prepares one node for execution and decides whether it is
// skipped (dead/disabled/unconfigured provider or unknown driver).
func (e *Engine) resolveNode(ctx context.Context, rc *runContext, ws *walkState, node *store.ChainNode) nodeTarget {
	inst := rc.providers[node.ProviderID]
	driver := node.ProviderID
	if inst != nil {
		driver = strings.ToLower(inst.Code)
	}
	target := nodeTarget{inst: inst, driver: driver, known: true}
	if inst != nil && inst.ProxyID != "" {
		target.proxy = rc.proxies[inst.ProxyID]
	}
	target.display = e.providerDisplay(node.ProviderID, inst)
	provider, known := e.reg.Get(driver)
	target.provider = provider
	target.known = known
	target.answer = strings.EqualFold(strings.TrimSpace(node.Mode), store.NodeModeAnswer)
	target.reason = e.instanceDeadReason(ctx, driver, inst)
	if target.reason == "" && ws.isDead(node.ProviderID) {
		target.reason = deadMarked
	}
	if !known {
		target.reason = deadUnknownDriver + ":" + driver
	}
	return target
}

// skipNode records a skip step for a node and returns the node to walk to.
func (e *Engine) skipNode(ctx context.Context, rc *runContext, idx int, node *store.ChainNode, target nodeTarget, next func(*store.ChainNode) *store.ChainNode) *store.ChainNode {
	e.recordSkip(ctx, rc, idx, node, target.display, target.reason)
	e.log.Warn(rc.req.RID, target.display, "skip node "+node.Key+": "+deadReasonText(target.reason, target.driver))
	return next(node)
}

// execNode runs the attempt loop of one alive node: retries with a linear
// backoff inside the node, one persisted step + SSE event per attempt. idx is
// the block's stable trace index. It returns the rows of the winning attempt
// and whether the node produced a usable result in first-success semantics.
func (e *Engine) execNode(ctx context.Context, rc *runContext, ws *walkState, idx int, node *store.ChainNode, target nodeTarget) nodeRun {
	req := rc.req
	params := e.buildParams(ctx, target.driver, target.inst, node)
	creds := e.buildCredentials(target.inst)
	treatEmpty := e.treatEmptyAsFail(ctx, node)
	sparse := e.allowSparseSources(ctx, node)
	timeout := time.Duration(node.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	var result providers.Result
	attempted := false
	attempts := node.Retries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		if !ws.takeStep() {
			break
		}
		attempted = true
		startedAt := store.Now()
		t0 := time.Now()
		e.sink.Publish(req.ID, EventStepStarted, StepStarted{
			Idx: idx, Node: node.Key, Provider: target.display, Engine: result.Engine, Attempt: attempt, T0: startedAt,
		})

		query := providers.Query{
			Text:  req.Query,
			Count: req.Count,
			Extra: map[string]string{"attempt": strconv.Itoa(attempt)},
		}
		stages := &stageCollector{}
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		callCtx = providers.WithStepReporter(callCtx, stages)
		if pc := e.proxyConfig(target.proxy); pc != nil {
			callCtx = providers.WithProxy(callCtx, pc)
		}
		release, acquireErr := e.limiter.acquire(callCtx)
		if acquireErr != nil {
			cancel()
			result = providers.Result{Kind: providers.KindTimeout, Provider: target.display, Error: "engine: waiting for concurrency slot: " + acquireErr.Error()}
		} else {
			result = target.provider.Search(callCtx, query, creds, params)
			release()
			cancel()
		}
		result.Provider = target.display
		result.Error = e.log.Redact(result.Error)
		if target.answer && strings.TrimSpace(result.Answer) != "" {
			rc.answerMu.Lock()
			req.Answer = result.Answer
			rc.answerMu.Unlock()
		}
		// Answer-node sources only reach the Row result when sparse sources are
		// explicitly allowed; the step counters mirror what is actually merged.
		stepRows := result.Rows
		if target.answer && !sparse {
			stepRows = nil
		}
		if result.HasQuota && result.QuotaMax > 0 && result.QuotaRemaining*10 < result.QuotaMax {
			e.log.Warn(rc.req.RID, target.display, "provider quota low: remaining="+
				strconv.Itoa(result.QuotaRemaining)+"/"+strconv.Itoa(result.QuotaMax))
		}
		tookMS := int(time.Since(t0) / time.Millisecond)

		// A failed non-permanent attempt sleeps before the next one; the delay is
		// the node backoff, raised to the upstream's explicit demand if larger.
		willSleep := !result.OK && !result.Permanent && attempt+1 < attempts && ws.hasBudget()
		delay := time.Duration(0)
		if willSleep {
			delay = backoffDelay(node, attempt, true)
			if ra := capDelay(time.Duration(result.RetryAfterMS) * time.Millisecond); ra > delay {
				delay = ra
			}
		}

		status := "fail"
		switch {
		case result.OK && target.answer && strings.TrimSpace(result.Answer) != "":
			status = "ok"
		case result.OK && len(stepRows) > 0:
			status = "ok"
		case result.OK:
			status = "empty"
		}
		step := &store.RequestStep{
			ID:           uuid.NewString(),
			RequestID:    req.ID,
			Idx:          idx,
			NodeKey:      node.Key,
			Provider:     target.display,
			Engine:       result.Engine,
			AttemptNo:    attempt + 1,
			Status:       status,
			Kind:         result.Kind,
			HTTPStatus:   result.HTTPStatus,
			Error:        result.Error,
			Permanent:    result.Permanent,
			ResultsCount: len(stepRows),
			WaitBeforeMS: int(delay / time.Millisecond),
			TookMS:       tookMS,
			StartedAt:    startedAt,
			FinishedAt:   store.Now(),
		}
		if err := e.sink.SaveStep(ctx, step); err != nil {
			e.log.Error(req.RID, target.display, "failed to persist step: "+err.Error())
		}
		e.sink.Publish(req.ID, EventStepFinished, StepFinished{
			Idx: idx, Node: node.Key, Provider: target.display, Engine: result.Engine, Attempt: attempt,
			HTTP: result.HTTPStatus, Kind: result.Kind, Results: len(stepRows),
			TookMS: tookMS, Permanent: result.Permanent, Error: result.Error,
		})
		e.logStep(req.RID, target.display, node.Key, attempt, result, tookMS)

		// Multi-call drivers (pubmed esearch/esummary/efetch): one history row
		// per upstream call under the same node; the attempt budget is consumed
		// per HTTP call, so a two-stage node spends 2 steps of MAX_ATTEMPTS.
		for _, sc := range stages.calls {
			sub := &store.RequestStep{
				ID: uuid.NewString(), RequestID: req.ID, Idx: idx, NodeKey: node.Key,
				Provider: target.display, Engine: result.Engine, AttemptNo: attempt + 1, Stage: sc.Stage,
				Status: "ok", Kind: providers.KindOK, HTTPStatus: sc.HTTPStatus, TookMS: sc.TookMS,
				StartedAt: startedAt, FinishedAt: store.Now(),
			}
			if sc.Err != nil {
				sub.Status = "fail"
				sub.Kind = providers.KindHTTP
				if sc.HTTPStatus == 0 {
					sub.Kind = providers.ClassifyTransport(sc.Err)
				}
				sub.Error = e.log.Redact(sc.Err.Error())
			}
			if err := e.sink.SaveStep(ctx, sub); err != nil {
				e.log.Error(req.RID, target.display, "failed to persist stage step: "+err.Error())
			}
			e.sink.Publish(req.ID, EventStepFinished, StepFinished{
				Idx: idx, Node: node.Key, Provider: target.display, Engine: result.Engine, Stage: sc.Stage,
				Attempt: attempt, HTTP: sc.HTTPStatus, Kind: sub.Kind, TookMS: sc.TookMS, Error: sub.Error,
			})
		}
		if extra := len(stages.calls) - 1; extra > 0 {
			ws.addSteps(extra)
		}

		if result.OK {
			if target.answer {
				if strings.TrimSpace(result.Answer) != "" || len(stepRows) > 0 {
					return nodeRun{rows: stepRows, won: true, result: result, attempted: attempted}
				}
			} else if len(result.Rows) > 0 || !treatEmpty {
				return nodeRun{rows: result.Rows, won: true, result: result, attempted: attempted}
			}
		}
		if result.Permanent {
			ws.markDead(node.ProviderID)
			e.sink.Publish(req.ID, EventProviderDead, ProviderDead{Provider: target.display, Reason: result.Error})
			break
		}
		if willSleep {
			// Attempts of one block always share the provider, so the linear
			// backoff (retry_delay_ms × attempt №) applies between them.
			if !sleepCtx(ctx, delay) {
				break
			}
		}
	}
	return nodeRun{result: result, attempted: attempted}
}

// runFirstSuccess walks the graph in first_success mode: the first usable
// result stops the chain; ok/empty/fail edges steer the transitions. Logical
// start/join blocks are transparent in this mode (fallback order follows the
// first neutral edge).
//
// logicalNew is true when the chain uses an explicit Start block. Only then is
// the on_success=next/edge continuation honoured; legacy chains always stop on
// the first success exactly as before.
func (e *Engine) runFirstSuccess(ctx context.Context, rc *runContext, maxAttempts int, logicalNew bool) (*Output, error) {
	ws := newWalkState(maxAttempts)
	node := rc.plan.Start()
	var last providers.Result
	merged := newMerger()
	// idx is the execution step number (like the old ws.idx): it orders the
	// stored steps chronologically. The node slice is sorted by key, so the plan
	// index must not be used for the timeline.
	idx := 0

	for node != nil && ws.hasBudget() {
		if Kind(node) != store.NodeKindProvider {
			node = firstNeutral(rc.plan, node)
			continue
		}
		target := e.resolveNode(ctx, rc, ws, node)
		if target.reason != "" {
			node = e.skipNode(ctx, rc, idx, node, target, func(n *store.ChainNode) *store.ChainNode {
				return rc.plan.Next(n, "fail")
			})
			idx++
			continue
		}

		run := e.execNode(ctx, rc, ws, idx, node, target)
		last = run.result
		if run.won {
			// A successful block ends the chain unless on_success asks to keep
			// walking (edge/next). In that case its rows are merged and the walk
			// continues, so success/ok edges finally steer the chain. Legacy
			// chains ignore on_success and always stop (old behaviour).
			continueOnSuccess := logicalNew && defaultPolicy(node.OnSuccess, "stop") != "stop"
			if !continueOnSuccess {
				rows := run.rows
				used := target.display
				if merged.uniqueCount() > 0 {
					merged.add(target.display, run.rows)
					rows = merged.rowsUpTo(rc.req.Count)
					used = strings.Join(merged.providers(), ", ")
				}
				return e.finalize(ctx, rc, "ok", providerRows(rows, rc.req.Count), used, ws.steps())
			}
			if len(run.rows) > 0 {
				merged.add(target.display, run.rows)
			}
			idx++
			node = rc.plan.Next(node, "success")
			continue
		}
		honorRetryAfter(ctx, run.result)
		idx++
		outcome := outcomeOf(run.result, e.treatEmptyAsFail(ctx, node))
		if target.answer {
			outcome = outcomeOfAnswer(run.result, e.allowSparseSources(ctx, node))
		}
		node = rc.plan.Next(node, outcome)
	}

	// Collected rows from continued successes win over a trailing failure.
	if merged.uniqueCount() > 0 {
		return e.finalize(ctx, rc, "ok", merged.rowsUpTo(rc.req.Count), strings.Join(merged.providers(), ", "), ws.steps())
	}

	// Loop exhausted without a usable result.
	status := "fail"
	if last.Kind != "" {
		if last.Kind == providers.KindOK {
			status = "empty"
		} else if last.Error != "" {
			rc.req.Error = last.Error
		}
	}
	return e.finalize(ctx, rc, status, nil, "", ws.steps())
}

// firstNeutral returns the first neutral successor of a logical node.
func firstNeutral(p *Plan, node *store.ChainNode) *store.ChainNode {
	if next := p.NeutralSuccessors(node); len(next) > 0 {
		return next[0]
	}
	return nil
}

// runFullChainLegacy preserves the original full-chain behaviour for chains
// saved before logical blocks existed: a strictly linear walk that follows a
// single neutral "next" edge per block and merges everything it visits. It is
// what keeps an upgraded chain byte-for-byte behaviour compatible.
//
// LEGACY: do not extend this walker. New features belong in runFullChain.
func (e *Engine) runFullChainLegacy(ctx context.Context, rc *runContext, maxAttempts int) (*Output, error) {
	ws := newWalkState(maxAttempts)
	visited := map[string]bool{}
	merged := newMerger()
	lastStatus := ""
	lastErr := ""
	node := rc.plan.Start()
	idx := 0

	for node != nil && ws.hasBudget() {
		if visited[node.Key] {
			break // cycles are rejected by the validator; guard anyway
		}
		visited[node.Key] = true
		// Logical blocks are transparent in the legacy linear walk; a stale
		// kind=join on an old chain must not be mistaken for an unknown driver.
		if Kind(node) != store.NodeKindProvider {
			node = rc.plan.NextInFullChain(node)
			continue
		}
		target := e.resolveNode(ctx, rc, ws, node)
		if target.reason != "" {
			lastStatus = "skip"
			e.recordSkip(ctx, rc, idx, node, target.display, target.reason)
			e.log.Warn(rc.req.RID, target.display, "skip node "+node.Key+": "+deadReasonText(target.reason, target.driver))
			idx++
			node = rc.plan.NextInFullChain(node)
			continue
		}

		run := e.execNode(ctx, rc, ws, idx, node, target)
		honorRetryAfter(ctx, run.result)
		switch {
		case run.result.OK && len(run.rows) > 0:
			lastStatus = "ok"
			if merged.add(target.display, run.rows) {
				e.sink.Publish(rc.req.ID, EventMergeProgress, MergeProgress{
					Collected: merged.collectedTotal(),
					Unique:    merged.uniqueCount(),
				})
			}
		case run.result.OK && target.answer && strings.TrimSpace(run.result.Answer) != "":
			lastStatus = "ok"
		case run.result.OK:
			lastStatus = "empty"
		default:
			lastStatus = "fail"
			if run.result.Error != "" {
				lastErr = run.result.Error
			}
		}
		idx++
		node = rc.plan.NextInFullChain(node)
	}

	rows := merged.rowsUpTo(rc.req.Count)
	stats := &store.MergeStats{
		CollectedTotal:    merged.collectedTotal(),
		UniqueLinks:       len(rows),
		DuplicatesRemoved: merged.duplicatesRemoved(),
	}
	e.sink.Publish(rc.req.ID, EventMergeDone, MergeDone{
		CollectedTotal:    stats.CollectedTotal,
		UniqueLinks:       stats.UniqueLinks,
		DuplicatesRemoved: stats.DuplicatesRemoved,
	})
	e.log.Info(rc.req.RID, "", "full chain merged: collected="+strconv.Itoa(stats.CollectedTotal)+
		" unique="+strconv.Itoa(stats.UniqueLinks)+" duplicates="+strconv.Itoa(stats.DuplicatesRemoved))

	status := "fail"
	switch {
	case len(rows) > 0:
		status = "ok"
	case lastStatus == "ok":
		status = "ok"
	case lastStatus == "empty":
		status = "empty"
	default:
		if lastErr != "" {
			rc.req.Error = lastErr
		}
	}
	used := strings.Join(merged.providers(), ", ")
	rc.req.Merge = stats
	return e.finalize(ctx, rc, status, rows, used, ws.steps())
}

// runFullChain walks every block reachable from the start along neutral "next"
// edges. Sibling branches fan out from a start/provider block and run in
// parallel; a join block waits for every branch feeding it. All provider rows
// are merged into one result set, deduplicated by normalized URL.
func (e *Engine) runFullChain(ctx context.Context, rc *runContext, maxAttempts int) (*Output, error) {
	plan := rc.plan
	start := plan.Start()
	if start == nil {
		return e.finalize(ctx, rc, "fail", nil, "", 0)
	}

	ws := newWalkState(maxAttempts)
	merged := newMerger()

	// idxByKey is filled wave by wave with the execution step number, so stored
	// steps are ordered chronologically (the node slice itself is key-ordered).
	idxByKey := map[string]int{}
	nextIdx := 0

	// Neutral connectivity is the executable full-chain graph.
	succ := map[string][]string{}
	for _, key := range plan.order {
		succ[key] = nil
	}
	for _, edge := range plan.chain.Edges {
		switch edge.Condition {
		case "next", "any", "":
			succ[edge.FromKey] = append(succ[edge.FromKey], edge.ToKey)
		}
	}

	// Only blocks reachable from the start along neutral edges run.
	reachable := map[string]bool{}
	queue := []string{start.Key}
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		if reachable[key] {
			continue
		}
		reachable[key] = true
		queue = append(queue, succ[key]...)
	}

	// In-degree within the reachable subgraph: unreachable predecessors must
	// not stall a block forever.
	indeg := map[string]int{}
	for key := range reachable {
		for _, to := range succ[key] {
			if reachable[to] {
				indeg[to]++
			}
		}
	}
	// The entry point always runs first even if a (validator-rejected) edge
	// points back at it.
	indeg[start.Key] = 0

	type blockState struct {
		status string
		rows   providers.Rows
		result providers.Result
		answer string
	}
	// Preallocate one state per block so branch goroutines only write their own
	// entry (concurrent map reads are safe; the map itself is never mutated).
	states := map[string]*blockState{}
	for key := range reachable {
		states[key] = &blockState{}
	}

	// process executes one block. It is safe to call from a branch goroutine:
	// shared counters live in ws (mutex-guarded) and the answer is guarded by
	// rc.answerMu; everything else it touches is read-only.
	process := func(key string) {
		node := plan.Node(key)
		st := states[key]
		idx := idxByKey[key]
		if Kind(node) != store.NodeKindProvider {
			// Logical start/join blocks carry no provider call. A join is the
			// rendezvous point for its branches: it runs only once every branch
			// has finished, then the walk continues past it.
			st.status = "ok"
			return
		}
		target := e.resolveNode(ctx, rc, ws, node)
		if target.reason != "" {
			st.status = "skip"
			e.recordSkip(ctx, rc, idx, node, target.display, target.reason)
			e.log.Warn(rc.req.RID, target.display, "skip node "+node.Key+": "+deadReasonText(target.reason, target.driver))
			return
		}
		run := e.execNode(ctx, rc, ws, idx, node, target)
		if !run.attempted {
			// The shared attempt budget ran out before this branch got a slot;
			// that is a skip, not a provider failure.
			st.status = "skip"
			return
		}
		st.result = run.result
		honorRetryAfter(ctx, run.result)
		switch {
		case run.result.OK && target.answer && strings.TrimSpace(run.result.Answer) != "":
			st.status = "ok"
			st.answer = run.result.Answer
		case run.result.OK && len(run.rows) > 0:
			st.status = "ok"
			st.rows = run.rows
		case run.result.OK:
			st.status = "empty"
		default:
			st.status = "fail"
		}
	}

	done := map[string]bool{}
	for {
		if !ws.hasBudget() {
			break
		}
		ready := make([]string, 0, len(reachable))
		for _, key := range plan.order {
			if reachable[key] && !done[key] && indeg[key] == 0 {
				ready = append(ready, key)
			}
		}
		if len(ready) == 0 {
			break
		}
		// Number the blocks of this wave before launching them: provider blocks
		// consume a step number in plan order; logical blocks do not emit steps.
		for _, key := range ready {
			if Kind(plan.Node(key)) == store.NodeKindProvider {
				idxByKey[key] = nextIdx
				nextIdx++
			}
		}
		if len(ready) == 1 {
			process(ready[0])
		} else {
			// Fan out: sibling providers run concurrently (bounded by the engine
			// concurrency limiter inside execNode).
			var wg sync.WaitGroup
			for _, key := range ready {
				wg.Add(1)
				go func(k string) {
					defer wg.Done()
					process(k)
				}(key)
			}
			wg.Wait()
		}
		for _, key := range ready {
			done[key] = true
			for _, to := range succ[key] {
				if reachable[to] {
					indeg[to]--
				}
			}
		}
	}

	// Merge provider rows in stable plan order so the first occurrence wins
	// deterministically even when branches ran in parallel. The same pass picks
	// the generated answer deterministically (last answer block in plan order)
	// instead of letting parallel writes race.
	lastStatus := ""
	lastErr := ""
	for _, key := range plan.order {
		node := plan.Node(key)
		if !reachable[key] || Kind(node) != store.NodeKindProvider {
			continue
		}
		st := states[key]
		if st == nil {
			continue
		}
		switch st.status {
		case "ok", "empty", "fail", "skip":
			lastStatus = st.status
		}
		if st.status == "fail" && st.result.Error != "" {
			lastErr = st.result.Error
		}
		if st.answer != "" {
			rc.answerMu.Lock()
			rc.req.Answer = st.answer
			rc.answerMu.Unlock()
		}
		if st.status == "ok" && len(st.rows) > 0 {
			display := e.providerDisplay(node.ProviderID, rc.providers[node.ProviderID])
			if merged.add(display, st.rows) {
				e.sink.Publish(rc.req.ID, EventMergeProgress, MergeProgress{
					Collected: merged.collectedTotal(),
					Unique:    merged.uniqueCount(),
				})
			}
		}
	}

	rows := merged.rowsUpTo(rc.req.Count)
	stats := &store.MergeStats{
		CollectedTotal:    merged.collectedTotal(),
		UniqueLinks:       len(rows),
		DuplicatesRemoved: merged.duplicatesRemoved(),
	}
	e.sink.Publish(rc.req.ID, EventMergeDone, MergeDone{
		CollectedTotal:    stats.CollectedTotal,
		UniqueLinks:       stats.UniqueLinks,
		DuplicatesRemoved: stats.DuplicatesRemoved,
	})
	e.log.Info(rc.req.RID, "", "full chain merged: collected="+strconv.Itoa(stats.CollectedTotal)+
		" unique="+strconv.Itoa(stats.UniqueLinks)+" duplicates="+strconv.Itoa(stats.DuplicatesRemoved))

	status := "fail"
	switch {
	case len(rows) > 0:
		status = "ok"
	case lastStatus == "ok":
		status = "ok"
	case lastStatus == "empty":
		status = "empty"
	default:
		if lastErr != "" {
			rc.req.Error = lastErr
		}
	}
	used := strings.Join(merged.providers(), ", ")
	rc.req.Merge = stats
	return e.finalize(ctx, rc, status, rows, used, ws.steps())
}

func (e *Engine) recordSkip(ctx context.Context, rc *runContext, idx int, node *store.ChainNode, code, reason string) {
	now := store.Now()
	step := &store.RequestStep{
		ID: uuid.NewString(), RequestID: rc.req.ID, Idx: idx, NodeKey: node.Key, Provider: code,
		AttemptNo: 1, Status: "skip", Kind: "skip", Error: reason, StartedAt: now, FinishedAt: now,
	}
	if err := e.sink.SaveStep(ctx, step); err != nil {
		e.log.Error(rc.req.RID, code, "failed to persist skip step: "+err.Error())
	}
	e.sink.Publish(rc.req.ID, EventStepFinished, StepFinished{
		Idx: idx, Node: node.Key, Provider: code, Attempt: 0, Kind: "skip", Error: reason,
	})
}

func (e *Engine) finalize(ctx context.Context, rc *runContext, status string, rows providers.Rows, used string, steps int) (*Output, error) {
	req := rc.req
	// A generated answer is a valid outcome even when no Row was collected.
	if status == "empty" && strings.TrimSpace(req.Answer) != "" {
		status = "ok"
	}
	req.Status = status
	req.UsedProvider = used
	req.ResultsCount = len(rows)
	req.Results = toRequestResults(rows)
	req.TotalMS = int(time.Since(rc.startedAt) / time.Millisecond)
	req.StepsCount = steps
	if status == "fail" && req.Error == "" {
		if dbReq, err := e.store.GetRequest(ctx, req.ID); err == nil {
			req.Error = lastError(dbReq.Steps)
		}
	}
	if err := e.sink.UpdateRequest(ctx, req); err != nil {
		e.log.Error(req.RID, "", "failed to update request: "+err.Error())
	}
	done := RequestDone{
		ID: req.ID, RID: req.RID, Status: status, Used: used, Steps: steps,
		TotalMS: req.TotalMS, Results: len(rows), Error: req.Error, Answer: req.Answer,
	}
	if req.Merge != nil {
		done.CollectedTotal = req.Merge.CollectedTotal
		done.UniqueLinks = req.Merge.UniqueLinks
		done.DuplicatesRemoved = req.Merge.DuplicatesRemoved
	}
	e.sink.Publish(req.ID, EventRequestDone, done)
	e.sink.PublishGlobal(EventRequestDone, done)
	e.log.Info(req.RID, used, "request done status="+status+" steps="+strconv.Itoa(steps)+
		" results="+strconv.Itoa(len(rows))+" took="+strconv.Itoa(req.TotalMS)+"ms")
	// Refresh daily rollups lazily (cheap upsert of a single day).
	_ = e.store.RecomputeDailyStats(ctx, time.Now().UTC().Format("2006-01-02"))
	return &Output{Request: req, Rows: rows}, nil
}

func (e *Engine) logStep(rid, code, node string, attempt int, r providers.Result, tookMS int) {
	level := "info"
	if !r.OK {
		level = "warn"
	}
	msg := "attempt node=" + node + " attempt=" + strconv.Itoa(attempt+1) +
		" http=" + strconv.Itoa(r.HTTPStatus) + " kind=" + r.Kind +
		" results=" + strconv.Itoa(len(r.Rows)) + " took=" + strconv.Itoa(tookMS) + "ms"
	if r.Error != "" {
		msg += " error=" + r.Error
	}
	switch level {
	case "warn":
		e.log.Warn(rid, code, msg)
	default:
		e.log.Info(rid, code, msg)
	}
}

// treatEmptyAsFail resolves the node-level override or the global default.
func (e *Engine) treatEmptyAsFail(ctx context.Context, node *store.ChainNode) bool {
	if v, ok := node.Params["treat_empty_as_fail"]; ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return e.settings.GetBool(ctx, config.KeyTreatEmptyAsFail)
}

// allowSparseSources controls whether an answer node's sources (link+title,
// often without a snippet) are mixed into the Row result. The node param wins
// over the global setting.
func (e *Engine) allowSparseSources(ctx context.Context, node *store.ChainNode) bool {
	if v, ok := node.Params["allow_sparse_sources"]; ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return e.settings.GetBool(ctx, config.KeyAllowSparseSources)
}

func (e *Engine) buildParams(ctx context.Context, driver string, inst *store.Provider, node *store.ChainNode) providers.Params {
	params := providers.Params{}
	if inst != nil {
		for k, v := range inst.Params {
			params[k] = v
		}
		if inst.BaseURL != "" {
			params["base_url"] = inst.BaseURL
		}
	}

	// Global retry-code settings are defaults; a per-instance value wins.
	// Fatal-code tables are per-instance params of each provider driver.
	setDefault := func(key, value string) {
		if _, ok := params[key]; !ok {
			params[key] = value
		}
	}
	setDefault("retry_http_codes", joinInts(e.settings.GetIntCSV(ctx, config.KeyRetryHTTPCodes)))
	if driver == "apiserpent" {
		setDefault("ap_retry_codes", strings.Join(e.settings.GetCSV(ctx, config.KeyAPRetryCodes), ","))
	}

	// Node-level overrides win.
	for k, v := range node.Params {
		params[k] = v
	}
	return params
}

// proxyConfig converts a stored proxy into a request-scoped config, or returns
// nil when the proxy is missing, disabled or has no host. The password is
// registered with the log redactor so it never leaks through transport errors.
func (e *Engine) proxyConfig(p *store.Proxy) *providers.ProxyConfig {
	if p == nil || !p.Enabled || strings.TrimSpace(p.Host) == "" {
		return nil
	}
	e.log.AddSecret(p.Password)
	return &providers.ProxyConfig{
		Type:     p.Type,
		Host:     strings.TrimSpace(p.Host),
		Port:     strings.TrimSpace(p.Port),
		Username: p.Username,
		Password: p.Password,
	}
}

func (e *Engine) buildCredentials(inst *store.Provider) providers.Credentials {
	creds := providers.Credentials{}
	if inst != nil {
		for k, v := range inst.Credentials {
			creds[k] = v
		}
	}
	// Register every non-empty credential so it is masked in logs.
	secrets := make([]string, 0, len(creds))
	for _, v := range creds {
		if v != "" {
			secrets = append(secrets, v)
		}
	}
	e.log.AddSecret(secrets...)
	return creds
}

// nextRID produces the v11-style request id: zero-padded global counter plus a
// random hex suffix, unique across restarts.
func (e *Engine) nextRID() string {
	n := e.seq.Add(1) % 10000
	var buf [2]byte
	_, _ = rand.Read(buf[:])
	return fmt.Sprintf("%04d-%s", n, hex.EncodeToString(buf[:]))
}

func providerRequiresKey(code string) bool {
	switch code {
	case "apiserpent", "serpbase", "yandex", "yandex_gen", "google", "google_vertex", "google_cse",
		"brave", "serper", "serpapi", "searchapi", "dataforseo", "youcom", "jina", "firecrawl",
		"mojeek", "marginalia", "tavily", "exa", "linkup", "perplexity", "valyu",
		"parallel", "kagi", "vectara", "anthropic", "kimi_search", "kimi_search_pro", "ollama":
		return true
	default:
		return false
	}
}

func outcomeOf(r providers.Result, treatEmpty bool) string {
	if r.OK {
		if len(r.Rows) == 0 && treatEmpty {
			return "empty"
		}
		return "success"
	}
	return "fail"
}

// outcomeOfAnswer is the edge outcome of an answer node: success requires a
// non-empty generated answer, or sparse sources when they are allowed.
func outcomeOfAnswer(r providers.Result, sparse bool) string {
	if !r.OK {
		return "fail"
	}
	if strings.TrimSpace(r.Answer) != "" {
		return "success"
	}
	if sparse && len(r.Rows) > 0 {
		return "success"
	}
	return "empty"
}

func providerRows(rows providers.Rows, count int) providers.Rows {
	if count > 0 && len(rows) > count {
		return rows[:count]
	}
	return rows
}

// toRequestResults converts provider rows into the persisted history form.
func toRequestResults(rows providers.Rows) []store.RequestResult {
	if len(rows) == 0 {
		return nil
	}
	out := make([]store.RequestResult, len(rows))
	for i, row := range rows {
		sources := append([]string{}, row.Sources...)
		if len(sources) == 0 {
			sources = nil
		}
		out[i] = store.RequestResult{Link: row.Link, Title: row.Title, Snippet: row.Snippet, Sources: sources}
	}
	return out
}

func backoffDelay(node *store.ChainNode, attempt int, sameProvider bool) time.Duration {
	if !sameProvider {
		return 0
	}
	base := time.Duration(node.RetryDelayMS) * time.Millisecond
	switch strings.ToLower(node.DelayPolicy) {
	case "none":
		return 0
	case "fixed":
		return capDelay(base)
	default: // linear
		return capDelay(base * time.Duration(attempt+1))
	}
}

func capDelay(d time.Duration) time.Duration {
	const max = 30 * time.Second
	if d < 0 {
		return 0
	}
	if d > max {
		return max
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func lastError(steps []*store.RequestStep) string {
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Error != "" {
			return steps[i].Error
		}
	}
	return ""
}

func joinInts(v []int) string {
	if len(v) == 0 {
		return ""
	}
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}

// semLimiter is a resizable counting semaphore (0 = unlimited).
type semLimiter struct {
	mu    sync.Mutex
	limit int
	ch    chan struct{}
}

func newSemLimiter(limit int) *semLimiter {
	l := &semLimiter{}
	l.setLimitLocked(limit)
	return l
}

func (l *semLimiter) setLimit(limit int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if limit == l.limit {
		return
	}
	l.setLimitLocked(limit)
}

func (l *semLimiter) setLimitLocked(limit int) {
	l.limit = limit
	if limit > 0 {
		l.ch = make(chan struct{}, limit)
	} else {
		l.ch = nil
	}
}

func (l *semLimiter) acquire(ctx context.Context) (func(), error) {
	l.mu.Lock()
	ch := l.ch
	l.mu.Unlock()
	if ch == nil {
		return func() {}, nil
	}
	select {
	case ch <- struct{}{}:
		return func() { <-ch }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
