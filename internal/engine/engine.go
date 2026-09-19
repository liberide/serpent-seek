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
	r := provider.Search(testCtx, providers.Query{Text: "test", Count: 3, Extra: map[string]string{}}, creds, params)
	r.Provider = e.providerDisplay(id, inst)
	r.Error = e.log.Redact(r.Error)
	return TestResult{Result: r, TookMS: int(time.Since(start) / time.Millisecond)}
}

// runContext carries per-request execution state.
type runContext struct {
	req        *store.Request
	plan       *Plan
	providers  map[string]*store.Provider
	startedAt  time.Time
	stepsTaken int
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

	req := &store.Request{
		ID:            uuid.NewString(),
		RID:           e.nextRID(),
		Query:         query,
		Count:         count,
		Status:        "running",
		ChainID:       chain.ID,
		ChainSnapshot: plan.Snapshot(),
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

	rc := &runContext{req: req, plan: plan, providers: byID, startedAt: time.Now()}
	e.emitDeadProviders(ctx, rc)
	return rc, nil
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
		return e.runFullChain(ctx, rc, maxAttempts)
	}
	return e.runFirstSuccess(ctx, rc, maxAttempts)
}

// walkState carries mutable per-request walk counters shared by both modes.
type walkState struct {
	idx    int
	stepNo int
	dead   map[string]bool
	last   providers.Result
}

// nodeTarget is the resolved execution context of one graph node.
type nodeTarget struct {
	inst     *store.Provider
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
	target.display = e.providerDisplay(node.ProviderID, inst)
	provider, known := e.reg.Get(driver)
	target.provider = provider
	target.known = known
	target.answer = strings.EqualFold(strings.TrimSpace(node.Mode), store.NodeModeAnswer)
	target.reason = e.instanceDeadReason(ctx, driver, inst)
	if target.reason == "" && ws.dead[node.ProviderID] {
		target.reason = deadMarked
	}
	if !known {
		target.reason = deadUnknownDriver + ":" + driver
	}
	return target
}

// skipNode records a skip step for a node and returns the node to walk to.
func (e *Engine) skipNode(ctx context.Context, rc *runContext, ws *walkState, node *store.ChainNode, target nodeTarget, next func(*store.ChainNode) *store.ChainNode) *store.ChainNode {
	e.recordSkip(ctx, rc, ws.idx, node, target.display, target.reason)
	e.log.Warn(rc.req.RID, target.display, "skip node "+node.Key+": "+deadReasonText(target.reason, target.driver))
	ws.idx++
	return next(node)
}

// execNode runs the attempt loop of one alive node: retries with a linear
// backoff inside the node, one persisted step + SSE event per attempt. It
// returns the (count-capped) rows of the winning attempt and whether the node
// produced a usable result in first-success semantics.
func (e *Engine) execNode(ctx context.Context, rc *runContext, ws *walkState, node *store.ChainNode, target nodeTarget, maxAttempts int) (providers.Rows, bool) {
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
	attempts := node.Retries + 1
	for attempt := 0; attempt < attempts && ws.stepNo < maxAttempts; attempt++ {
		ws.stepNo++
		startedAt := store.Now()
		t0 := time.Now()
		e.sink.Publish(req.ID, EventStepStarted, StepStarted{
			Idx: ws.idx, Node: node.Key, Provider: target.display, Engine: result.Engine, Attempt: attempt, T0: startedAt,
		})

		query := providers.Query{
			Text:  req.Query,
			Count: req.Count,
			Extra: map[string]string{"attempt": strconv.Itoa(attempt)},
		}
		stages := &stageCollector{}
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		callCtx = providers.WithStepReporter(callCtx, stages)
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
			req.Answer = result.Answer
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
		ws.last = result

		// A failed non-permanent attempt sleeps before the next one; the delay is
		// the node backoff, raised to the upstream's explicit demand if larger.
		willSleep := !result.OK && !result.Permanent && attempt+1 < attempts && ws.stepNo < maxAttempts
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
			Idx:          ws.idx,
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
			Idx: ws.idx, Node: node.Key, Provider: target.display, Engine: result.Engine, Attempt: attempt,
			HTTP: result.HTTPStatus, Kind: result.Kind, Results: len(stepRows),
			TookMS: tookMS, Permanent: result.Permanent, Error: result.Error,
		})
		e.logStep(req.RID, target.display, node.Key, attempt, result, tookMS)

		// Multi-call drivers (pubmed esearch/esummary/efetch): one history row
		// per upstream call under the same node; the attempt budget is consumed
		// per HTTP call, so a two-stage node spends 2 steps of MAX_ATTEMPTS.
		for _, sc := range stages.calls {
			sub := &store.RequestStep{
				ID: uuid.NewString(), RequestID: req.ID, Idx: ws.idx, NodeKey: node.Key,
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
				Idx: ws.idx, Node: node.Key, Provider: target.display, Engine: result.Engine, Stage: sc.Stage,
				Attempt: attempt, HTTP: sc.HTTPStatus, Kind: sub.Kind, TookMS: sc.TookMS, Error: sub.Error,
			})
		}
		if extra := len(stages.calls) - 1; extra > 0 {
			ws.stepNo += extra
		}

		if result.OK {
			if target.answer {
				if strings.TrimSpace(result.Answer) != "" || len(stepRows) > 0 {
					return stepRows, true
				}
			} else if len(result.Rows) > 0 || !treatEmpty {
				return result.Rows, true
			}
		}
		if result.Permanent {
			ws.dead[node.ProviderID] = true
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
	return nil, false
}

// runFirstSuccess walks the graph in first_success mode: the first usable
// result stops the chain; ok/empty/fail edges steer the transitions.
func (e *Engine) runFirstSuccess(ctx context.Context, rc *runContext, maxAttempts int) (*Output, error) {
	ws := &walkState{dead: map[string]bool{}}
	node := rc.plan.Start()

	for node != nil && ws.stepNo < maxAttempts {
		target := e.resolveNode(ctx, rc, ws, node)
		if target.reason != "" {
			node = e.skipNode(ctx, rc, ws, node, target, func(n *store.ChainNode) *store.ChainNode {
				return rc.plan.Next(n, "fail")
			})
			continue
		}

		rows, won := e.execNode(ctx, rc, ws, node, target, maxAttempts)
		if won {
			return e.finalize(ctx, rc, "ok", providerRows(rows, rc.req.Count), target.display, ws.stepNo)
		}
		honorRetryAfter(ctx, ws.last)
		ws.idx++
		outcome := outcomeOf(ws.last, e.treatEmptyAsFail(ctx, node))
		if target.answer {
			outcome = outcomeOfAnswer(ws.last, e.allowSparseSources(ctx, node))
		}
		node = rc.plan.Next(node, outcome)
	}

	// Loop exhausted without a usable result.
	status := "fail"
	if ws.last.Kind != "" {
		if ws.last.Kind == providers.KindOK {
			status = "empty"
		} else if ws.last.Error != "" {
			rc.req.Error = ws.last.Error
		}
	}
	return e.finalize(ctx, rc, status, nil, "", ws.stepNo)
}

// runFullChain walks every block reachable from the start along "next" edges,
// regardless of successes/failures, and merges all collected links (deduped).
func (e *Engine) runFullChain(ctx context.Context, rc *runContext, maxAttempts int) (*Output, error) {
	ws := &walkState{dead: map[string]bool{}}
	visited := map[string]bool{}
	merged := newMerger()
	lastStatus := ""
	node := rc.plan.Start()

	for node != nil && ws.stepNo < maxAttempts {
		if visited[node.Key] {
			break // cycles are rejected by the validator; guard anyway
		}
		visited[node.Key] = true
		target := e.resolveNode(ctx, rc, ws, node)
		if target.reason != "" {
			lastStatus = "skip"
			node = e.skipNode(ctx, rc, ws, node, target, rc.plan.NextInFullChain)
			continue
		}

		rows, _ := e.execNode(ctx, rc, ws, node, target, maxAttempts)
		honorRetryAfter(ctx, ws.last)
		switch {
		case ws.last.OK && len(rows) > 0:
			lastStatus = "ok"
			if merged.add(target.display, rows) {
				e.sink.Publish(rc.req.ID, EventMergeProgress, MergeProgress{
					Collected: merged.collectedTotal(),
					Unique:    merged.uniqueCount(),
				})
			}
		case ws.last.OK && target.answer && strings.TrimSpace(ws.last.Answer) != "":
			lastStatus = "ok"
		case ws.last.OK:
			lastStatus = "empty"
		default:
			lastStatus = "fail"
		}
		ws.idx++
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
		if ws.last.Error != "" {
			rc.req.Error = ws.last.Error
		}
	}
	used := strings.Join(merged.providers(), ", ")
	rc.req.Merge = stats
	return e.finalize(ctx, rc, status, rows, used, ws.stepNo)
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
		"mojeek", "marginalia", "tavily", "exa", "linkup", "perplexity_search", "valyu",
		"parallel", "kagi", "vectara", "anthropic":
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
