package main

import (
	"context"
	"os/exec"
	"testing"
	"time"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/e2e-framework/klient"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/pkg/types" 
)

func TestCrashLoopBackOffWithRollback(t *testing.T) {
	feat := features.New("nova-api-osapi CrashLoopBackOff with rollback")

	feat = feat.WithSetup("Inject CRASH_ME env",  func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Log("[SETUP] Injecting CRASH_ME env to nova-api-osapi")
		cmd := exec.Command("kubectl", "-n", "openstack", "set", "env", "deployment/nova-api-osapi", "CRASH_ME=true")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Failed to inject env: %v\n%s", err, string(out))
		}
		time.Sleep(10 * time.Second)
		return ctx
	})

	feat = feat.WithStep("Check CrashLoopBackOff", types.LevelAssess, func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Log("[CHECK] Looking for CrashLoopBackOff pod")

		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		}
		restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			t.Fatalf("Failed to load kubeconfig: %v", err)
		}

		client, err := klient.New(restCfg)
		if err != nil {
			t.Fatalf("Failed to create klient: %v", err)
		}

		pods := &corev1.PodList{}
		if err := client.Resources().List(ctx, pods); err != nil {
			t.Fatalf("Failed to list pods: %v", err)
		}

		for _, pod := range pods.Items {
			if pod.Namespace != "openstack" {
				continue
			}
			if val, ok := pod.Labels["app.kubernetes.io/component"]; ok && val == "os-api" {
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
						t.Logf("✅ Detected CrashLoopBackOff in pod: %s", pod.Name)
						return ctx
					}
				}
			}
		}

		t.Fatalf("❌ No pod in CrashLoopBackOff state found")
		return ctx
	})

	feat = feat.WithTeardown("Rollback CRASH_ME", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Log("[TEARDOWN] Rolling back env CRASH_ME")
		cmd := exec.Command("kubectl", "-n", "openstack", "set", "env", "deployment/nova-api-osapi", "CRASH_ME-")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Failed to rollback: %v\n%s", err, string(out))
		}
		time.Sleep(10 * time.Second)
		return ctx
	})

	testenv.Test(t, feat.Feature())
}