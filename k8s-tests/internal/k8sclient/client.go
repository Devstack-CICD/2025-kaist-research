package k8sclient

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

// Client 래퍼 ---------------------------------------------------

type Client struct {
	res *resources.Resources
	ns  string 
}

func New(res *resources.Resources) *Client                { return &Client{res: res} }
func (c *Client) Resources() *resources.Resources { return c.res }
func (c *Client) Namespace(ns string) *Client             { return &Client{res: c.res.WithNamespace(ns), ns: ns} }
func NewFromEnv(cfg *envconf.Config) *Client              { r, _ := resources.New(cfg.Client().RESTConfig()); return New(r) }

// 기본 리스트 ---------------------------------------------------

// 모든 Deployment
func (c *Client) Deployments(ctx context.Context) []appsv1.Deployment {
	var list appsv1.DeploymentList
	_ = c.res.List(ctx, &list)
	return list.Items
}

// 모든 Service
func (c *Client) Services(ctx context.Context) []corev1.Service {
	var list corev1.ServiceList
	_ = c.res.List(ctx, &list)
	return list.Items
}

// 라벨 셀렉터 기반 Pod
func (c *Client) PodsBySelector(ctx context.Context, ns string, sel map[string]string) []corev1.Pod {
	var pods corev1.PodList
	r := c.res
	if ns != "" { r = r.WithNamespace(ns) }

	selector := labels.SelectorFromSet(sel)
	_ = r.List(ctx, &pods, resources.WithLabelSelector(selector.String()))
	return pods.Items
}

// PVC 목록
func (c *Client) PVCs(ctx context.Context) []corev1.PersistentVolumeClaim {
    var list corev1.PersistentVolumeClaimList
    _ = c.Resources().List(ctx, &list)
    return list.Items
}

// DaemonSet, StatefulSet
func (c *Client) DaemonSets(ctx context.Context) []appsv1.DaemonSet {
    var list appsv1.DaemonSetList
    _ = c.Resources().List(ctx, &list)
    return list.Items
}
func (c *Client) StatefulSets(ctx context.Context) []appsv1.StatefulSet {
    var list appsv1.StatefulSetList
    _ = c.Resources().List(ctx, &list)
    return list.Items
}

//Jobs
func (c *Client) Jobs(ctx context.Context) []batchv1.Job {
	var list batchv1.JobList
	_ = c.res.List(ctx, &list)
	return list.Items
}

// ConfigMaps
func (c *Client) ConfigMaps(ctx context.Context) []corev1.ConfigMap {
	var list corev1.ConfigMapList
	_ = c.res.List(ctx, &list)
	return list.Items
}

// Secrets
func (c *Client) Secrets(ctx context.Context) []corev1.Secret {
	var list corev1.SecretList
	_ = c.res.List(ctx, &list)
	return list.Items
}

// ServiceAccounts
func (c *Client) ServiceAccounts(ctx context.Context) []corev1.ServiceAccount {
	var list corev1.ServiceAccountList
	_ = c.res.List(ctx, &list)
	return list.Items
}

// 관계형 헬퍼 ---------------------------------------------------

// Deployment → ReplicaSets
func (c *Client) ReplicaSetsForDeployment(ctx context.Context, dp appsv1.Deployment) []appsv1.ReplicaSet {
	var rsList appsv1.ReplicaSetList
	_ = c.res.WithNamespace(dp.Namespace).List(ctx, &rsList)

	out := make([]appsv1.ReplicaSet, 0)
	for _, rs := range rsList.Items {
		for _, o := range rs.OwnerReferences {
			if o.Kind == "Deployment" && o.Name == dp.Name {
				out = append(out, rs)
			}
		}
	}
	return out
}

// ReplicaSet → Pods
func (c *Client) PodsForReplicaSet(ctx context.Context, rs appsv1.ReplicaSet) []corev1.Pod {
	return c.PodsBySelector(ctx, rs.Namespace, rs.Spec.Selector.MatchLabels)
}

// Service → Pods
func (c *Client) PodsForService(ctx context.Context, svc corev1.Service) []corev1.Pod {
	return c.PodsBySelector(ctx, svc.Namespace, svc.Spec.Selector)
}

// PV name → PV* 인덱스
func (c *Client) PVIndex(ctx context.Context) map[string]*corev1.PersistentVolume {
    var list corev1.PersistentVolumeList
    _ = c.Resources().List(ctx, &list)
    out := make(map[string]*corev1.PersistentVolume)
    for i := range list.Items {
        out[list.Items[i].Name] = &list.Items[i]
    }
    return out
}

