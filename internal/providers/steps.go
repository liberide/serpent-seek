package providers

import "context"

// StageCall describes one upstream HTTP call of a multi-call provider
// (e.g. PubMed esearch → esummary → efetch). The engine persists every stage
// as its own request_steps row under the same node, keeping the history honest
// while the node stays ONE logical unit of the chain.
type StageCall struct {
	// Stage names the call: "esearch", "esummary", "efetch", ...
	Stage string
	// HTTPStatus is the upstream status code (0 on transport errors).
	HTTPStatus int
	// TookMS is the wall time of the single upstream call.
	TookMS int
	// Err is the stage error, nil on success.
	Err error
}

// StepReporter is optionally injected into the request context by the engine so
// multi-call providers can report each upstream call as a separate step.
// Implementations must be safe for use from a single goroutine (Search is
// synchronous); the engine collects them after Search returns.
type StepReporter interface {
	Report(StageCall)
}

type ctxKeyStepReporter struct{}

// WithStepReporter returns a context carrying the reporter.
func WithStepReporter(ctx context.Context, r StepReporter) context.Context {
	return context.WithValue(ctx, ctxKeyStepReporter{}, r)
}

// ReporterFrom extracts the reporter or returns nil.
func ReporterFrom(ctx context.Context) StepReporter {
	r, _ := ctx.Value(ctxKeyStepReporter{}).(StepReporter)
	return r
}

// ReportStage is a nil-safe helper for drivers.
func ReportStage(ctx context.Context, stage string, httpStatus, tookMS int, err error) {
	if r := ReporterFrom(ctx); r != nil {
		r.Report(StageCall{Stage: stage, HTTPStatus: httpStatus, TookMS: tookMS, Err: err})
	}
}
