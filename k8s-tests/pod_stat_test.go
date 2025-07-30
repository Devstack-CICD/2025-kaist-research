package main

import (
    "context"
    "testing"
    "os"
    "time"
    "fmt"

    corev1 "k8s.io/api/core/v1"

    res "sigs.k8s.io/e2e-framework/klient/k8s/resources"
    "sigs.k8s.io/e2e-framework/pkg/envconf"
    "sigs.k8s.io/e2e-framework/pkg/features"
	"go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
)

func TestAllPodsRunning(t *testing.T) {
    f := features.New("check all pod status").
        Assess("pods should be running or completed", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
            tracer := otel.Tracer("k8s-e2e-tests") 

            ctx, span := tracer.Start(ctx, "list kube-system pods")
            defer span.End()
			
			var pods corev1.PodList
            _ = cfg.Client().Resources("default").List(ctx, &pods)

            for _, pod := range pods.Items {
                if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodSucceeded {
                    t.Fatalf("Pod %s is in %s state", pod.Name, pod.Status.Phase)
                }
            }

            return ctx
        }).Feature()

    testenv.Test(t, f)
}

func TestListAllPodsFeature(t *testing.T) {
    feat := features.New("list all pods").
        Assess("print pod list", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
            tr := otel.Tracer("e2e")
            ctx, span := tr.Start(ctx, "ListAllPods")
            defer span.End()

            // ➊ RESTConfig 전달 → 두 값 반환 처리
            r, err := res.New(cfg.Client().RESTConfig())
            if err != nil {
                t.Fatalf("new resources client: %v", err)
            }

            var pods corev1.PodList
            // ➋ 네임스페이스 전체 조회 옵션
            if err := r.WithNamespace("").List(ctx, &pods); err != nil {
				t.Fatalf("list pods: %v", err)
			}

            ts := time.Now().Format("yyyymmdd_hhmmss") // YYYYMMDD_HHMMSS
            filename := fmt.Sprintf("pods_%s.txt", ts)

            f, err := os.Create(filename)
            if err != nil {
                t.Fatalf("file create: %v", err)
            }
            defer f.Close()

            for _, p := range pods.Items {
                line := fmt.Sprintf("[%s] %s (%s)\n", p.Namespace, p.Name, p.Status.Phase)
                if _, err := f.WriteString(line); err != nil {
                    t.Fatalf("write file: %v", err)
                }
            }

            span.SetAttributes(attribute.Int("pod.count", len(pods.Items)),
                               attribute.String("pod.list.file", filename))

            if len(pods.Items) == 0 {
                t.Fatalf("no pods found in cluster")
            }

            t.Logf("pod list saved to %s", filename) // optional
            return ctx
        })

    testenv.Test(t, feat.Feature())
}