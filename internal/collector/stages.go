package collector

import (
	"context"
	"encoding/json"
	"net/http"
	"fmt"
	"log"

	corev1 "k8s.io/api/core/v1"
	discv1 "k8s.io/api/discovery/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)


// ───────────────────────── Workload (Deployment, Service) ──────────────────
func WorkloadStage(ctx context.Context, c *Client, g *Graph) error {
	before := len(g.Edges)
	for _, dp := range c.Deployments(ctx) {
		dpUID := g.AddNode(dp.Namespace, dp.Name, "Deployment")

		for _, rs := range c.ReplicaSetsForDeployment(ctx, dp) {
			rsUID := g.AddNode(rs.Namespace, rs.Name, "ReplicaSet")
			g.AddEdge(dpUID, rsUID, Owns)

			for _, pod := range c.PodsForReplicaSet(ctx, rs) {
				podUID := g.AddNode(pod.Namespace, pod.Name, "Pod")
				g.AddEdge(rsUID, podUID, Owns)
			}
		}
	}

	for _, svc := range c.Services(ctx) {
		svcUID := g.AddNode(svc.Namespace, svc.Name, "Service")
		for _, pod := range c.PodsForService(ctx, svc) {
			podUID := g.AddNode(pod.Namespace, pod.Name, "Pod")
			g.AddEdge(svcUID, podUID, Routes)
		}
	}
	defer func() {
		added := len(g.Edges) - before
		fmt.Printf("[WorkloadStage] targets added=%d\n", added)
	}()
	return nil
}


// ───────────────────────── Ingress / Gateway  ──────────────────────────────
func IngressStage(ctx context.Context, c *Client, g *Graph) error {
    before := len(g.Edges)

	var ings netv1.IngressList
	r := c.Resources().WithNamespace("")
	if err := r.List(ctx, &ings); err != nil {
		log.Printf("[IngressStage] list error: %v", err)
	} else {
		log.Printf("[IngressStage] found ingress count=%d", len(ings.Items))
	}

    // 2) 각 Ingress 처리
    for _, ing := range ings.Items {
        uid := g.AddNode(ing.Namespace, ing.Name, "Ingress")

        // 2-A) defaultBackend (v1)
        if db := ing.Spec.DefaultBackend; db != nil {
            if svc := db.Service; svc != nil {
                sid := g.AddNode(ing.Namespace, svc.Name, "Service")
                g.AddEdge(uid, sid, Routes)
            }
        }

        // 2-B) rules[].http.paths
        for _, rule := range ing.Spec.Rules {
            if rule.HTTP == nil {
                continue
            }
            for _, path := range rule.HTTP.Paths {
                // v1 방식
                if path.Backend.Service != nil {
                    svc := path.Backend.Service
                    sid := g.AddNode(ing.Namespace, svc.Name, "Service")
                    g.AddEdge(uid, sid, Routes)
                    continue
                }
            }
        }
    }

    // 3) 로깅
    added := len(g.Edges) - before
    fmt.Printf("[IngressStage] routes added=%d\n", added)
    return nil
}
// ───────────────────────── EndpointSlice → Pod  ────────────────────────────
func EndpointStage(ctx context.Context, c *Client, g *Graph) error {
	before := len(g.Edges)
    var esList discv1.EndpointSliceList
    _ = c.Resources().List(ctx, &esList)

    // ── Pod IP → Pod 캐시 ---------------------------------
    ipMap := make(map[string]corev1.Pod)
    for _, pod := range c.PodsBySelector(ctx, "", nil) {      // 모든 ns
        for _, ip := range pod.Status.PodIPs {
            ipMap[ip.IP] = pod
        }
		if pod.Status.PodIP != "" {                       
			ipMap[pod.Status.PodIP] = pod
		}
    }

    // ── Slice → Pod edges ---------------------------------
    for _, es := range esList.Items {
        esUID := g.AddNode(es.Namespace, es.Name, "EndpointSlice")
        for _, ep := range es.Endpoints {
            // 1) targetRef 있으면 그대로
            if ep.TargetRef != nil && ep.TargetRef.Kind == "Pod" {
                p := ep.TargetRef
                pUID := g.AddNode(es.Namespace, p.Name, "Pod")
                g.AddEdge(esUID, pUID, Targets)
                continue
            }
            // 2) 없으면 IP 역‑매핑
            for _, addr := range ep.Addresses {
                if pod, ok := ipMap[addr]; ok {
                    pUID := g.AddNode(pod.Namespace, pod.Name, "Pod")
                    g.AddEdge(esUID, pUID, Targets)
                }
            }
        }
    }
	defer func() {
		added := len(g.Edges) - before
		fmt.Printf("[EndpointStage] targets added=%d\n", added)
	}()

    return nil
}

func PVCStage(ctx context.Context, c *Client, g *Graph) error {
	// 기존 edge 개수 기록
	before := len(g.Edges)

	// 1) PVC 목록
	r := c.Resources().WithNamespace("")  
	var pvcList corev1.PersistentVolumeClaimList
	if err := r.List(ctx, &pvcList); err != nil {
		fmt.Printf("[PVCStage] list PVC error: %v\n", err)
		return nil
	}
	fmt.Printf("[PVCStage] found PVCs=%d\n", len(pvcList.Items))

	// 2) PV 목록
	var pvList corev1.PersistentVolumeList
	_ = r.List(ctx, &pvList)
	pvMap := make(map[string]*corev1.PersistentVolume, len(pvList.Items))
	for i := range pvList.Items {
		pv := &pvList.Items[i]
		pvMap[pv.Name] = pv
	}
	fmt.Printf("[PVCStage] found PVs=%d\n", len(pvList.Items))

	// 3) Edge 생성
	for _, pvc := range pvcList.Items {
		fmt.Printf("[PVCStage] pvc %s phase=%s volumeName=%q\n",
			pvc.Name, pvc.Status.Phase, pvc.Spec.VolumeName)

		if pvc.Status.Phase != corev1.ClaimBound {
			continue
		}

		pvcUID := g.AddNode(pvc.Namespace, pvc.Name, "PVC")
		if pv, exists := pvMap[pvc.Spec.VolumeName]; exists {
			pvUID := g.AddNode("", pv.Name, "PV")
			g.AddEdge(pvcUID, pvUID, Binds)

			sc := pv.Spec.StorageClassName
			if sc == "" {
				sc = "none"
			}
			scUID := g.AddNode("", sc, "StorageClass")
			g.AddEdge(pvUID, scUID, Uses)
		}
	}

	// 4) 현재 전체 Edges 중 새로 추가된 것만 수집 (간접 추정)
	counts := map[EdgeKind]int{}
	if len(g.Edges) > before {
		// map이므로 순회하면서 "index > before"인 것만 필터링 불가 → 전체 순회로 대체
		for _, e := range g.Edges {
			counts[e.Kind]++
		}
	}
	fmt.Printf("[PVCStage] binds=%d uses=%d\n",
		counts[Binds], counts[Uses])

	return nil
}

/*
// ───────────────────────── PVC → PV → StorageClass ─────────────────────────
func PVCStage(ctx context.Context, c *Client, g *Graph) error {
    before := len(g.Edges)

    // 1) 전역 스코프에서 PVC 목록
    r := c.Resources().WithNamespace("")  
    var pvcList corev1.PersistentVolumeClaimList
    if err := r.List(ctx, &pvcList); err != nil {
        fmt.Printf("[PVCStage] list PVC error: %v\n", err)
        return nil
    }
    fmt.Printf("[PVCStage] found PVCs=%d\n", len(pvcList.Items))

    // 2) 전역 스코프에서 PV 인덱스
    var pvList corev1.PersistentVolumeList
    _ = r.List(ctx, &pvList)
    pvMap := make(map[string]*corev1.PersistentVolume, len(pvList.Items))
    for i := range pvList.Items {
        pv := &pvList.Items[i]
        pvMap[pv.Name] = pv
    }
    fmt.Printf("[PVCStage] found PVs=%d\n", len(pvList.Items))

    // 3) 실제 Edge 생성
    for _, pvc := range pvcList.Items {
        fmt.Printf("[PVCStage] pvc %s phase=%s volumeName=%q\n",
            pvc.Name, pvc.Status.Phase, pvc.Spec.VolumeName)

        // 3-A) Pending PVC는 건너뛸 경우
        if pvc.Status.Phase != corev1.ClaimBound {
            continue
        }

        pvcUID := g.AddNode(pvc.Namespace, pvc.Name, "PVC")
        if pv, exists := pvMap[pvc.Spec.VolumeName]; exists {
            pvUID := g.AddNode("", pv.Name, "PV")
            g.AddEdge(pvcUID, pvUID, Binds)

            sc := pv.Spec.StorageClassName
            if sc == "" {
                sc = "none"
            }
            scUID := g.AddNode("", sc, "StorageClass")
            g.AddEdge(pvUID, scUID, Uses)
        }
    }

    // 4) Stage별 EdgeKind별 개수 로그
    delta := g.Edges[before:]
    counts := map[EdgeKind]int{}
    for _, e := range delta {
        counts[e.Kind]++
    }
    fmt.Printf("[PVCStage] binds=%d uses=%d\n",
        counts[Binds], counts[Uses])

    return nil
}
*/

// ───────────────────────── DaemonSet / StatefulSet ─────────────────────────
func DSSTSStage(ctx context.Context, c *Client, g *Graph) error {
	before := len(g.Edges)
	for _, ds := range c.DaemonSets(ctx) {
		dsUID := g.AddNode(ds.Namespace, ds.Name, "DaemonSet")
		for _, pod := range c.PodsBySelector(ctx, ds.Namespace, ds.Spec.Selector.MatchLabels) {
			pUID := g.AddNode(pod.Namespace, pod.Name, "Pod")
			g.AddEdge(dsUID, pUID, Owns)
		}
	}
	for _, st := range c.StatefulSets(ctx) {
		stUID := g.AddNode(st.Namespace, st.Name, "StatefulSet")
		for _, pod := range c.PodsBySelector(ctx, st.Namespace, st.Spec.Selector.MatchLabels) {
			pUID := g.AddNode(pod.Namespace, pod.Name, "Pod")
			g.AddEdge(stUID, pUID, Owns)
		}
	}
	defer func() {
		added := len(g.Edges) - before
		fmt.Printf("[DSSTSSStage] targets added=%d\n", added)
	}()
	return nil
}

func NetpolStage(ctx context.Context, c *Client, g *Graph) error {
	// 기존 edge 개수 기록 (현재는 참고용)
	//before := len(g.Edges)

	// 1) 전역(All-NS) NP·Pod 조회
	r := c.Resources().WithNamespace("")
	var nps netv1.NetworkPolicyList
	if err := r.List(ctx, &nps); err != nil {
		fmt.Printf("[NetpolStage] list NP error: %v\n", err)
		return nil
	}
	var podList corev1.PodList
	_ = r.List(ctx, &podList)
	pods := podList.Items
	fmt.Printf("[NetpolStage] found NetPol=%d Pods=%d\n", len(nps.Items), len(pods))

	for _, np := range nps.Items {
		// 2) 대상 Pod 집합
		selector := &np.Spec.PodSelector
		tgt := selectPods(pods, selector)
		if (len(selector.MatchLabels) == 0 && len(selector.MatchExpressions) == 0) || len(tgt) == 0 {
			tgt = pods
		}
		fmt.Printf("[NetpolStage] %s targetPods=%d\n", np.Name, len(tgt))

		// 3) Ingress 룰이 없으면 모든 Pod → 타겟 Pod 허용
		/*
		if len(np.Spec.Ingress) == 0 {
			for _, sp := range pods {
				sUID := g.AddNode(sp.Namespace, sp.Name, "Pod")
				for _, tp := range tgt {
					tUID := g.AddNode(tp.Namespace, tp.Name, "Pod")
					g.AddEdge(sUID, tUID, Allow)
				}
			}
			continue
		}
		*/
		
		// 4) 각 Ingress 룰 처리
		for _, rule := range np.Spec.Ingress {
			var srcPods []corev1.Pod
			if len(rule.From) == 0 {
				srcPods = pods
			} else {
				for _, peer := range rule.From {
					if peer.PodSelector == nil ||
						(len(peer.PodSelector.MatchLabels) == 0 && len(peer.PodSelector.MatchExpressions) == 0) {
						srcPods = pods
						break
					}
					matched := selectPods(pods, peer.PodSelector)
					srcPods = append(srcPods, matched...)
				}
			}

			// 5) 중복 제거
			uniq := make(map[string]corev1.Pod)
			for _, p := range srcPods {
				uniq[p.Name] = p
			}
			srcPods = srcPods[:0]
			for _, p := range uniq {
				srcPods = append(srcPods, p)
			}

			// 6) 엣지 추가
			for _, sp := range srcPods {
				sUID := g.AddNode(sp.Namespace, sp.Name, "Pod")
				for _, tp := range tgt {
					tUID := g.AddNode(tp.Namespace, tp.Name, "Pod")
					g.AddEdge(sUID, tUID, Allow)
				}
			}
		}
	}

	// 전체 Edge 중 Allow 타입만 카운트
	added := 0
	for _, e := range g.Edges {
		if e.Kind == Allow {
			added++
		}
	}
	fmt.Printf("[NetpolStage] allow edges added=%d (total Allow edges)\n", added)

	return nil
}

/*
func NetpolStage(ctx context.Context, c *Client, g *Graph) error {
    before := len(g.Edges)

    // 1) 전역(All-NS) NP·Pod 조회
    r := c.Resources().WithNamespace("")
    var nps netv1.NetworkPolicyList
    if err := r.List(ctx, &nps); err != nil {
        fmt.Printf("[NetpolStage] list NP error: %v\n", err)
        return nil
    }
    var podList corev1.PodList
    _ = r.List(ctx, &podList)
    pods := podList.Items
    fmt.Printf("[NetpolStage] found NetPol=%d Pods=%d\n", len(nps.Items), len(pods))

    for _, np := range nps.Items {
        // 2) 대상 Pod 집합 (PodSelector 매칭, 없으면 all)
        selector := &np.Spec.PodSelector
		tgt := selectPods(pods, selector)
        if (len(selector.MatchLabels) == 0 && len(selector.MatchExpressions) == 0) || len(tgt) == 0 {
        	tgt = pods
        }
        fmt.Printf("[NetpolStage] %s targetPods=%d\n", np.Name, len(tgt))

        // 3) Ingress 룰이 비어 있으면 “all→tgt” 엣지
        if len(np.Spec.Ingress) == 0 {
            for _, sp := range pods {
                sUID := g.AddNode(sp.Namespace, sp.Name, "Pod")
                for _, tp := range tgt {
                    tUID := g.AddNode(tp.Namespace, tp.Name, "Pod")
                    g.AddEdge(sUID, tUID, Allow)
                }
            }
            continue
        }

        // 4) 각 Ingress 룰 처리
        for _, rule := range np.Spec.Ingress {
            // from이 비어 있으면 all
            var srcPods []corev1.Pod
            if len(rule.From) == 0 {
                srcPods = pods
            } else {
                for _, peer := range rule.From {
                    // PodSelector nil 또는 빈 셀렉터 → all
                    if peer.PodSelector == nil ||
                        (len(peer.PodSelector.MatchLabels) == 0 &&
                         len(peer.PodSelector.MatchExpressions) == 0) {
                        srcPods = pods
                        break
                    }
                    matched := selectPods(pods, peer.PodSelector)
                    srcPods = append(srcPods, matched...)
                }
            }

            // 5) 중복 제거(Optional)
            uniq := make(map[string]corev1.Pod)
            for _, p := range srcPods { uniq[p.Name] = p }
            srcPods = srcPods[:0]
            for _, p := range uniq { srcPods = append(srcPods, p) }

            // 6) 엣지 추가
            for _, sp := range srcPods {
                sUID := g.AddNode(sp.Namespace, sp.Name, "Pod")
                for _, tp := range tgt {
                    tUID := g.AddNode(tp.Namespace, tp.Name, "Pod")
                    g.AddEdge(sUID, tUID, Allow)
                }
            }
        }
    }

    // 로깅
    added := 0
    for _, e := range g.Edges[before:] {
        if e.Kind == Allow {
            added++
        }
    }
    fmt.Printf("[NetpolStage] allow edges added=%d\n", added)
    return nil
}
*/

// selectPods: nil 또는 빈 Selector → all, else match labels/expressions
func selectPods(all []corev1.Pod, sel *metav1.LabelSelector) []corev1.Pod {
    if sel == nil ||
        (len(sel.MatchLabels) == 0 && len(sel.MatchExpressions) == 0) {
        return all
    }
    s, _ := metav1.LabelSelectorAsSelector(sel)
    out := make([]corev1.Pod, 0, len(all))
    for _, p := range all {
        if s.Matches(labels.Set(p.Labels)) {
            out = append(out, p)
        }
    }
    return out
}


func JobStage(ctx context.Context, c *Client, g *Graph) error {
	before := len(g.Edges)

	for _, job := range c.Jobs(ctx) {
		jobUID := g.AddNode(job.Namespace, job.Name, "Job")

		pods := c.PodsBySelector(ctx, job.Namespace, job.Spec.Selector.MatchLabels)
		for _, pod := range pods {
			podUID := g.AddNode(pod.Namespace, pod.Name, "Pod")
			g.AddEdge(jobUID, podUID, Owns)
		}
	}
	fmt.Printf("[JobStage] added=%d edges\n", len(g.Edges)-before)
	return nil
}

func ConfigSecretStage(ctx context.Context, c *Client, g *Graph) error {
	before := len(g.Edges)

	for _, pod := range c.PodsBySelector(ctx, "", nil) {
		podUID := g.AddNode(pod.Namespace, pod.Name, "Pod")

		for _, container := range pod.Spec.Containers {
			// EnvFrom: ConfigMapRef / SecretRef
			for _, envFrom := range container.EnvFrom {
				if envFrom.ConfigMapRef != nil {
					cmUID := g.AddNode(pod.Namespace, envFrom.ConfigMapRef.Name, "ConfigMap")
					g.AddEdge(podUID, cmUID, Reads)
				}
				if envFrom.SecretRef != nil {
					secUID := g.AddNode(pod.Namespace, envFrom.SecretRef.Name, "Secret")
					g.AddEdge(podUID, secUID, Reads)
				}
			}
		}

		// VolumeMount: ConfigMap / Secret
		for _, vol := range pod.Spec.Volumes {
			switch {
			case vol.ConfigMap != nil:
				cmUID := g.AddNode(pod.Namespace, vol.ConfigMap.Name, "ConfigMap")
				g.AddEdge(podUID, cmUID, Mounts)
			case vol.Secret != nil:
				secUID := g.AddNode(pod.Namespace, vol.Secret.SecretName, "Secret")
				g.AddEdge(podUID, secUID, Mounts)
			}
		}
	}

	fmt.Printf("[ConfigSecretStage] added=%d edges\n", len(g.Edges)-before)
	return nil
}

func ServiceAccountStage(ctx context.Context, c *Client, g *Graph) error {
	before := len(g.Edges)

	for _, pod := range c.PodsBySelector(ctx, "", nil) {
		podUID := g.AddNode(pod.Namespace, pod.Name, "Pod")

		sa := pod.Spec.ServiceAccountName
		if sa == "" {
			sa = "default"
		}
		saUID := g.AddNode(pod.Namespace, sa, "ServiceAccount")
		g.AddEdge(podUID, saUID, Uses)
	}

	fmt.Printf("[ServiceAccountStage] added=%d edges\n", len(g.Edges)-before)
	return nil
}

// ───────────────────────── Jaeger Deep‑Dependencies ────────────────────────
func JaegerStage(api string) Stage {
	return func(ctx context.Context, c *Client, g *Graph) error {
		resp, err := http.Get(api + "/api/dependencies?lookback=3600")
		if err != nil { return nil } // Jaeger 미구축 시 무시
		defer resp.Body.Close()
		var deps []struct{ Parent, Child string }
		_ = json.NewDecoder(resp.Body).Decode(&deps)
		for _, d := range deps {
			from := g.AddNode("", d.Parent, "Service")
			to   := g.AddNode("", d.Child,  "Service")
			g.AddEdge(from, to, Calls)
		}
		return nil
	}
}