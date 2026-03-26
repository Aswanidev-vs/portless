package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type Provider struct {
	tp sdktrace.TracerProvider
}

func Init(service string) *Provider {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return &Provider{tp: *tp}
}

func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

func Start(ctx context.Context, tracerName, spanName string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return Tracer(tracerName).Start(ctx, spanName, trace.WithAttributes(attrs...))
}
