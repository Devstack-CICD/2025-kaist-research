package main

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "testing"

    "github.com/kaist2025/k8s-e2e-tests/internal/collector"
    "github.com/kaist2025/k8s-e2e-tests/internal/k8sclient"

    "sigs.k8s.io/e2e-framework/klient/conf"     
	"sigs.k8s.io/e2e-framework/pkg/envconf"    
)

func TestStaticGraph(t *testing.T) {
    ctx := context.Background()

	// 1) kubeconfig → envconf.Config
	kubePath := conf.ResolveKubeConfigFile()
	cfg      := envconf.NewWithKubeConfig(kubePath)

	// 2) k8sclient 생성
	cli := k8sclient.NewFromEnv(cfg)   
	if ns := os.Getenv("NAMESPACE"); ns != "" {
		cli = cli.Namespace(ns)
	}

	// 3) 수집기 실행

    col := &collector.Collector{
        Client: cli,
        Stages: []collector.Stage{
            collector.WorkloadStage,
            collector.IngressStage,
            collector.EndpointStage,
			collector.DSSTSStage,
            collector.PVCStage,
            collector.NetpolStage,
            //collector.JaegerStage("http://jaeger.logging-tracing.svc:16686"),
        },
    }

    g, err := col.Run(ctx)
    if err != nil { t.Fatalf("collect: %v", err) }

    // ── Mermaid Graph 저장 
    dir, _ := filepath.Abs("artifacts")
    os.MkdirAll(dir, 0o755)
    f, _ := os.Create(filepath.Join(dir, "static_v3.mmd"))
    defer f.Close()
    fmt.Fprintln(f, "graph LR")
    for _, e := range g.Edges {
        fmt.Fprintf(f, `%s["%s"] -->|%s| %s["%s"]`+"\n",
                e.From, g.Nodes[e.From].Label,  
                e.Kind, e.To, g.Nodes[e.To].Label)
    }
}