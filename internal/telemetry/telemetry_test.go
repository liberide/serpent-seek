package telemetry

import (
	"context"
	"testing"
)

func TestSetupDisabledWithoutEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	shutdown, on, err := Setup(context.Background(), "serpentseek", "test")
	if err != nil || on {
		t.Fatalf("tracing should be disabled: on=%v err=%v", on, err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown: %v", err)
	}
}

func TestSetupEnabledWithEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")
	shutdown, on, err := Setup(context.Background(), "serpentseek", "test")
	if err != nil || !on {
		t.Fatalf("tracing should be enabled: on=%v err=%v", on, err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}
