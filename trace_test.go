package main

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestTracing(t *testing.T) {
	tracer := otel.Tracer("k8s-e2e-tests")

	f := features.New("basic trace").
		Assess("span example", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ctx, span := tracer.Start(ctx, "my-test-span")
			defer span.End()

			t.Log("Span created!")
			return ctx
		}).Feature()

	testenv.Test(t, f)
}