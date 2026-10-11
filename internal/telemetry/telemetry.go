// Package telemetry wires OpenTelemetry tracing. It is a no-op unless an OTLP
// endpoint is configured through the standard OTEL_EXPORTER_OTLP_ENDPOINT
// (or OTEL_EXPORTER_OTLP_TRACES_ENDPOINT) environment variable.
package telemetry

import (
	"context"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Setup installs a global trace provider when an OTLP endpoint is configured.
// It returns a shutdown function and whether tracing is enabled. When tracing is
// disabled the shutdown function is a safe no-op.
func Setup(ctx context.Context, serviceName, serviceVersion string) (func(context.Context) error, bool, error) {
	noop := func(context.Context) error { return nil }
	if !enabled() {
		return noop, false, nil
	}
	if serviceName == "" {
		serviceName = "serpentseek"
	}
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return noop, false, err
	}
	res := resource.NewWithAttributes("",
		attribute.String("service.name", serviceName),
		attribute.String("service.version", serviceVersion),
	)
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp), sdktrace.WithResource(res))
	otel.SetTracerProvider(tp)
	return tp.Shutdown, true, nil
}

func enabled() bool {
	return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != "" ||
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")) != ""
}
