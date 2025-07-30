package main

import (
	// If testing with a cloud vendor managed cluster uncomment one of the below dependencies to properly get authorised.
	//_ "k8s.io/client-go/plugin/pkg/client/auth/azure" // auth for AKS clusters
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"   // auth for GKE clusters
	//_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"  // auth for OIDC
	"context"
	"log"
	"os"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	_ "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"sigs.k8s.io/e2e-framework/klient/conf"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	_ "sigs.k8s.io/e2e-framework/pkg/envfuncs"	
	_ "sigs.k8s.io/e2e-framework/support/kind"
)

var (
	testenv env.Environment
	tp      *sdktrace.TracerProvider
)


func initTracer(ctx context.Context) func() {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:44013"
	}

	exp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	if err != nil {
		log.Fatalf("OTLP exporter init: %v", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("k8s-e2e-tests"),
		))
	if err != nil {
		log.Fatalf("resource init: %v", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return func() {
		if err := tp.Shutdown(ctx); err != nil {
			log.Printf("tracer shutdown: %v", err)
		}
	}
}

func TestMain(m *testing.M) {
	ctx := context.Background()
	shutdown := initTracer(ctx)
	defer shutdown()

	// Tracing 적용
	tracer := otel.Tracer("e2e-test-suite")
	ctx, span := tracer.Start(ctx, "test-setup")
	defer span.End()

	// Test 환경 적용
	path := conf.ResolveKubeConfigFile()
	cfg := envconf.NewWithKubeConfig(path)
	testenv = env.NewWithConfig(cfg)

	// Test 실행
	os.Exit(testenv.Run(m))
}

