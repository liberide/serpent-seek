package engine

import (
	"context"

	"github.com/liberide/serpent-seek/internal/logging"
	"github.com/liberide/serpent-seek/internal/sse"
	"github.com/liberide/serpent-seek/internal/store"
)

// SSE event names.
const (
	EventStepStarted   = "step_started"
	EventStepFinished  = "step_finished"
	EventProviderDead  = "provider_dead"
	EventRequestDone   = "request_done"
	EventRequestNew    = "request_created"
	EventMergeProgress = "merge_progress"
	EventMergeDone     = "merge_done"
)

// StepStarted is the payload for step_started.
type StepStarted struct {
	Idx      int    `json:"idx"`
	Node     string `json:"node"`
	Provider string `json:"provider"`
	Engine   string `json:"engine,omitempty"`
	Attempt  int    `json:"attempt"`
	T0       string `json:"t0"`
}

// StepFinished is the payload for step_finished. Stage is set for the
// individual upstream calls of a multi-call node (pubmed esearch/esummary).
// When non-empty, HTTP/Kind/TookMS describe that single sub-call rather than
// the logical node attempt.
type StepFinished struct {
	Idx       int    `json:"idx"`
	Node      string `json:"node"`
	Provider  string `json:"provider"`
	Engine    string `json:"engine,omitempty"`
	Stage     string `json:"stage,omitempty"`
	Attempt   int    `json:"attempt"`
	HTTP      int    `json:"http"`
	Kind      string `json:"kind"`
	Results   int    `json:"results"`
	TookMS    int    `json:"took_ms"`
	Permanent bool   `json:"permanent"`
	Error     string `json:"error,omitempty"`
}

// ProviderDead is the payload for provider_dead.
type ProviderDead struct {
	Provider string `json:"provider"`
	Reason   string `json:"reason"`
}

// MergeProgress is the payload of merge_progress (full-chain live counter).
type MergeProgress struct {
	Collected int `json:"collected"`
	Unique    int `json:"unique"`
}

// MergeDone is the payload of the terminal merge_done event (full-chain mode).
type MergeDone struct {
	CollectedTotal    int `json:"collected_total"`
	UniqueLinks       int `json:"unique_links"`
	DuplicatesRemoved int `json:"duplicates_removed"`
}

// RequestDone is the payload for request_done.
type RequestDone struct {
	ID                string `json:"id"`
	RID               string `json:"rid"`
	Status            string `json:"status"`
	Used              string `json:"used"`
	Steps             int    `json:"steps"`
	TotalMS           int    `json:"total_ms"`
	Results           int    `json:"results"`
	Answer            string `json:"answer,omitempty"`
	Error             string `json:"error,omitempty"`
	CollectedTotal    int    `json:"collected_total,omitempty"`
	UniqueLinks       int    `json:"unique_links,omitempty"`
	DuplicatesRemoved int    `json:"duplicates_removed,omitempty"`
}

// Sink persists request state and publishes live events. It is an interface so
// the engine can be unit tested without a database or SSE hub.
type Sink interface {
	CreateRequest(ctx context.Context, r *store.Request) error
	SaveStep(ctx context.Context, s *store.RequestStep) error
	UpdateRequest(ctx context.Context, r *store.Request) error
	Publish(requestID, event string, data any)
	PublishGlobal(event string, data any)
}

// StoreSink is the production Sink backed by the store and the SSE hub.
type StoreSink struct {
	store store.Storage
	hub   *sse.Hub
	log   *logging.Logger
}

// NewStoreSink builds a production Sink.
func NewStoreSink(st store.Storage, hub *sse.Hub, log *logging.Logger) *StoreSink {
	return &StoreSink{store: st, hub: hub, log: log}
}

// CreateRequest persists the running request.
func (s *StoreSink) CreateRequest(ctx context.Context, r *store.Request) error {
	return s.store.CreateRequest(ctx, r)
}

// SaveStep persists one step attempt.
func (s *StoreSink) SaveStep(ctx context.Context, step *store.RequestStep) error {
	return s.store.AddStep(ctx, step)
}

// UpdateRequest persists the final request state.
func (s *StoreSink) UpdateRequest(ctx context.Context, r *store.Request) error {
	return s.store.UpdateRequest(ctx, r)
}

// Publish emits a per-request event.
func (s *StoreSink) Publish(requestID, event string, data any) {
	s.hub.Publish(requestID, event, data)
}

// PublishGlobal emits a dashboard-level event.
func (s *StoreSink) PublishGlobal(event string, data any) {
	s.hub.Publish("", event, data)
}
