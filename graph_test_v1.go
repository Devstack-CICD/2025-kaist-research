// graph_test.go
package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"regexp"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
    "sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

type edge struct{ From, To, Kind string }

var idSanitizer = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func safeID(name string) string {
    return idSanitizer.ReplaceAllString(name, "_")
}

func TestPrintCWD(t *testing.T) {
	dir, _ := os.Getwd()
	t.Logf("current working dir = %s", dir)
}

func getOwnedReplicaSets(ctx context.Context, r *resources.Resources, dp appsv1.Deployment) []appsv1.ReplicaSet {
	var rsList appsv1.ReplicaSetList
	_ = r.WithNamespace(dp.Namespace).List(ctx, &rsList)

	out := make([]appsv1.ReplicaSet, 0)
	for _, rs := range rsList.Items {
		for _, owner := range rs.OwnerReferences {
			if owner.Kind == "Deployment" && owner.Name == dp.Name {
				out = append(out, rs)
				break
			}
		}
	}
	return out
}

func getOwnedPods(ctx context.Context, r *resources.Resources, rs appsv1.ReplicaSet) []corev1.Pod {
	var podList corev1.PodList
	_ = r.WithNamespace(rs.Namespace).List(ctx, &podList)

	out := make([]corev1.Pod, 0)
	for _, p := range podList.Items {
		for _, owner := range p.OwnerReferences {
			if owner.Kind == "ReplicaSet" && owner.Name == rs.Name {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

func getPodsBySelector(ctx context.Context, r *resources.Resources, sel map[string]string, ns string) []corev1.Pod {
	var podList corev1.PodList
	_ = r.WithNamespace(ns).List(ctx, &podList)

	out := make([]corev1.Pod, 0)
	for _, p := range podList.Items {
		match := true
		for k, v := range sel {
			if p.Labels[k] != v {
				match = false
				break
			}
		}
		if match {
			out = append(out, p)
		}
	}
	return out
}



func TestDumpStaticGraph(t *testing.T) {
	ns := os.Getenv("NAMESPACE") // 빈 값이면 모든 네임스페이스
	feat := features.New("dump-static-deps").Assess("collect", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		r, _ := resources.New(cfg.Client().RESTConfig())
		g := make([]edge, 0)

		// 1) Deployments → ReplicaSets → Pods
		var dps appsv1.DeploymentList
		if ns == "" {
			_ = r.List(ctx, &dps)
		} else {
			_ = r.WithNamespace(ns).List(ctx, &dps)
		}
		for _, dp := range dps.Items {
			for _, rs := range getOwnedReplicaSets(ctx, r, dp) {
				g = append(g, edge{dp.Name, rs.Name, "owns"})
				for _, p := range getOwnedPods(ctx, r, rs) {
					g = append(g, edge{rs.Name, p.Name, "owns"})
				}
			}
		}

		// 2) Services → Pods
		var svcs corev1.ServiceList
		if ns == "" {
			_ = r.List(ctx, &svcs)
		} else {
			_ = r.WithNamespace(ns).List(ctx, &svcs)
		}
		for _, svc := range svcs.Items {
			pods := getPodsBySelector(ctx, r, svc.Spec.Selector, svc.Namespace)
			for _, p := range pods {
				g = append(g, edge{svc.Name, p.Name, "routes"})
			}
		}	

		// ── 파일로 저장 (Mermaid)
		dir := "artifacts"
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}

		path := filepath.Join(dir, "static.mmd")
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("create %s: %v", path, err)  // ← 문제 있으면 즉시 FAIL
			return ctx  
		}
		defer f.Close()

		f.WriteString("graph LR\n")
		for _, e := range g {
			fromID := safeID(e.From)
    		toID   := safeID(e.To)

    		// 라벨과 함께 쓰면 원본 이름이 UI에 그대로 표시됩니다
    		fmt.Fprintf(f, `%s["%s"] -->|%s| %s["%s"]`+"\n",
        		fromID, e.From, e.Kind, toID, e.To)
			//f.WriteString(e.From + " -->|" + e.Kind + "| " + e.To + "\n")
		}

		abs, _ := filepath.Abs(path)
		t.Logf("Mermaid graph saved to %s", abs)
		return ctx  
	})

	testenv.Test(t, feat.Feature())
}