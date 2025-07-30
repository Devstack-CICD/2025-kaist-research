package main

import (
	"context"
	_ "os"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.17.0"
)

func TestSimpleSpan(t *testing.T) {
	ctx := context.Background()

	// Exporter
	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint("35.216.127.215:30418"),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("k8s-e2e-tests"), 
		)),
	)
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(ctx)

	// Generate span
	tracer := otel.Tracer("k8s-e2e-tests")
	ctx, span := tracer.Start(ctx, "simple-test-span")
	span.End()
}