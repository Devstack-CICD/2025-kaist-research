package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
	"os/signal"
	"syscall"
	"encoding/json"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	corev1 "k8s.io/api/core/v1"
    //appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//networkingv1 "k8s.io/api/networking/v1"
	//discoveryv1 "k8s.io/api/discovery/v1"
    "k8s.io/apimachinery/pkg/api/meta"

	"sigs.k8s.io/e2e-framework/pkg/envconf"

	"github.com/kaist2025/k8s-e2e-tests/internal/collector"
	"github.com/kaist2025/k8s-e2e-tests/internal/exporter"
	"github.com/kaist2025/k8s-e2e-tests/internal/k8sclient"
)

func main() {
	var kubeconfig string
	var resyncPeriod time.Duration
	var debounce time.Duration
	var outputDir string

	flag.StringVar(&kubeconfig, "kubeconfig", "", "Absolute path to the kubeconfig file")
	flag.DurationVar(&resyncPeriod, "resync", time.Hour, "Shared informer resync period")
	flag.DurationVar(&debounce, "debounce", 5*time.Second, "Debounce interval for saving graph")
	flag.StringVar(&outputDir, "output", "artifacts", "Directory to write graph outputs")
	flag.Parse()

	configPath := kubeconfig
	if configPath == "" {
		configPath = clientcmd.RecommendedHomeFile
	}
	restCfg, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		log.Fatalf("error loading kubeconfig: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		log.Fatalf("error creating kubernetes client: %v", err)
	}

	envCfg := envconf.NewWithKubeConfig(configPath)
	k8sCli := k8sclient.NewFromEnv(envCfg)
	coll := collector.NewCollector(k8sCli, []collector.Stage{
		collector.WorkloadStage,
		collector.IngressStage,
		collector.EndpointStage,
		collector.DSSTSStage,
		collector.PVCStage,
		collector.NetpolStage,
		collector.JobStage,
		collector.ConfigSecretStage,
		collector.ServiceAccountStage,
	})

	factory := informers.NewSharedInformerFactory(clientset, resyncPeriod)

	triggerCh := make(chan struct{}, 1)
	debounced := time.AfterFunc(debounce, func() {})
	debounced.Stop()
	trigger := func() {
		select {
		case triggerCh <- struct{}{}:
		default:
		}
	}

	resources := []struct {
		informer cache.SharedIndexInformer
		kind     string
	}{
		{factory.Core().V1().Pods().Informer(), "Pod"},
		{factory.Apps().V1().Deployments().Informer(), "Deployment"},
		{factory.Core().V1().Services().Informer(), "Service"},
		{factory.Networking().V1().Ingresses().Informer(), "Ingress"},
		{factory.Networking().V1().NetworkPolicies().Informer(), "NetworkPolicy"},
		{factory.Core().V1().PersistentVolumeClaims().Informer(), "PVC"},
		{factory.Core().V1().PersistentVolumes().Informer(), "PV"},
		{factory.Discovery().V1().EndpointSlices().Informer(), "EndpointSlice"},
		{factory.Apps().V1().DaemonSets().Informer(), "DaemonSet"},
		{factory.Apps().V1().StatefulSets().Informer(), "StatefulSet"},
		{factory.Core().V1().Events().Informer(), "Event"}, 
		{factory.Batch().V1().Jobs().Informer(), "Job"},
		{factory.Core().V1().ConfigMaps().Informer(), "ConfigMap"},
		{factory.Core().V1().Secrets().Informer(), "Secret"},
		{factory.Core().V1().ServiceAccounts().Informer(), "ServiceAccount"},
	}
	for _, r := range resources {
		r.informer.AddEventHandler(makeHandler(r.kind, coll, trigger))
	}

	stopCh := make(chan struct{})
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalCh
		log.Println("termination signal received, shutting down...")
		close(stopCh)
	}()

	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)
/*
	ctx := context.Background()
	log.Println("▶ initial graph collection")
	if _, err := coll.Run(ctx); err != nil {
		log.Fatalf("initial run error: %v", err)
	}
	saveMermaid(coll.Graph, outputDir)
	saveCSV(coll.Graph, outputDir)

	for {
		select {
		case <-triggerCh:
			debounced.Reset(debounce)
		case <-debounced.C:
			log.Println("⏱ writing updated graph")
			saveMermaid(coll.Graph, outputDir)
			saveCSV(coll.Graph, outputDir)
		case <-stopCh:
			log.Println("⏱ writing updated graph")
			saveMermaid(coll.Graph, outputDir)
			saveCSV(coll.Graph, outputDir)
			return
		}
	}
		*/

	ctx := context.Background()
		
	neo4jUri := "bolt://localhost:7687"
	neo4jUser := "neo4j"
	neo4jPass := "devSTACK1!"

	driver := exporter.ConnectNeo4j(neo4jUri, neo4jUser, neo4jPass)
	defer driver.Close(ctx)
	
	log.Println("▶ initial graph collection")
	if _, err := coll.Run(ctx); err != nil {
		log.Fatalf("initial run error: %v", err)
	}
	if err := exporter.ExportToNeo4j(ctx, coll.Graph, driver); err != nil {
		log.Fatalf("neo4j export failed: %v", err)
	}

	for {
		select {
		case <-triggerCh:
			debounced.Reset(debounce)
		case <-debounced.C:
			log.Println("⏱ writing updated graph")
			if err := exporter.ExportToNeo4j(ctx, coll.Graph, driver); err != nil {
				log.Printf("neo4j export failed: %v", err)
			}
		case <-stopCh:
			log.Println("⏱ writing final graph")
			if err := exporter.ExportToNeo4j(ctx, coll.Graph, driver); err != nil {
				log.Printf("neo4j export failed: %v", err)
			}
			return
		}
	}
}

func logEvent(kind, event string, obj interface{}) {
    metaObj, ok := obj.(metav1.Object)
    if !ok {
        accessor, err := meta.Accessor(obj)
        if err != nil {
            log.Printf("cannot extract metadata from %s object", kind)
            return
        }
        metaObj = accessor
    }

    name := metaObj.GetName()
    ns := metaObj.GetNamespace()
    ts := time.Now().Format(time.RFC3339)
    log.Printf("- Event %s on [%s] %s/%s @ %s", event, kind, ns, name, ts)
	/*
    switch kind {
    case "Pod":
        if pod, ok := obj.(*corev1.Pod); ok {
            for _, cs := range pod.Status.ContainerStatuses {
                log.Printf("-- Container %s: State=%#v, Restart=%d", cs.Name, cs.State, cs.RestartCount)
            }
            for _, cond := range pod.Status.Conditions {
                log.Printf("-- Condition %s: %s - %s", cond.Type, cond.Status, cond.Message)
            }
        }

    case "Deployment":
        if deploy, ok := obj.(*appsv1.Deployment); ok {
            log.Printf("-- Replicas: desired=%d, available=%d, ready=%d",
                *deploy.Spec.Replicas, deploy.Status.AvailableReplicas, deploy.Status.ReadyReplicas)
        }

    case "ReplicaSet":
        if rs, ok := obj.(*appsv1.ReplicaSet); ok {
            log.Printf("-- Replicas: desired=%d, ready=%d, available=%d",
                *rs.Spec.Replicas, rs.Status.ReadyReplicas, rs.Status.AvailableReplicas)
        }

    case "StatefulSet":
        if sts, ok := obj.(*appsv1.StatefulSet); ok {
            log.Printf("-- Replicas: desired=%d, ready=%d, current=%d",
                *sts.Spec.Replicas, sts.Status.ReadyReplicas, sts.Status.CurrentReplicas)
        }

    case "DaemonSet":
        if ds, ok := obj.(*appsv1.DaemonSet); ok {
            log.Printf("-- Pods: desired=%d, current=%d, ready=%d",
                ds.Status.DesiredNumberScheduled, ds.Status.CurrentNumberScheduled, ds.Status.NumberReady)
        }

    case "Service":
        if svc, ok := obj.(*corev1.Service); ok {
            log.Printf("-- Service Type=%s, ClusterIP=%s, Selector=%v",
                svc.Spec.Type, svc.Spec.ClusterIP, svc.Spec.Selector)
        }

    case "PVC":
        if pvc, ok := obj.(*corev1.PersistentVolumeClaim); ok {
            log.Printf("-- PVC Phase=%s, Volume=%s, Requested=%v",
                pvc.Status.Phase, pvc.Spec.VolumeName, pvc.Spec.Resources.Requests.Storage().String())
        }

    case "PV":
        if pv, ok := obj.(*corev1.PersistentVolume); ok {
            log.Printf("-- PV Phase=%s, Capacity=%v, ReclaimPolicy=%s",
                pv.Status.Phase, pv.Spec.Capacity.Storage().String(), pv.Spec.PersistentVolumeReclaimPolicy)
        }

    case "Ingress":
        if ing, ok := obj.(*networkingv1.Ingress); ok {
            for _, rule := range ing.Spec.Rules {
                log.Printf("-- Ingress Host=%s", rule.Host)
                for _, path := range rule.HTTP.Paths {
                    log.Printf("   ↳ Path=%s → Service=%s", path.Path, path.Backend.Service.Name)
                }
            }
        }

	case "EndpointSlice":
		if es, ok := obj.(*discoveryv1.EndpointSlice); ok {
			log.Printf("-- EndpointSlice AddressType=%s, Ports=%v", es.AddressType, es.Ports)
			for _, ep := range es.Endpoints {
				log.Printf("   ↳ Endpoint: addresses=%v, ready=%v", ep.Addresses, *ep.Conditions.Ready)
			}
		}
	}
	*/
}
/*
func handleEvent(obj interface{}) {
	event, ok := obj.(*corev1.Event)
	if !ok {
		return
	}

	data := map[string]interface{}{
		"timestamp":   event.LastTimestamp.Time.Format(time.RFC3339),
		"type":        event.Type,
		"reason":      event.Reason,
		"message":     event.Message,
		"kind":        event.InvolvedObject.Kind,
		"name":        event.InvolvedObject.Name,
		"namespace":   event.InvolvedObject.Namespace,
		"component":   event.Source.Component,
	}

	file, err := os.OpenFile("artifacts/events.json", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		defer file.Close()
		json.NewEncoder(file).Encode(data)
	}

	log.Printf("Event [%s] %s: %s/%s - %s",
		data["type"], data["reason"],
		data["kind"], data["name"],
		data["message"],
	)
}
*/

func handleEvent(obj interface{}) {
	event, ok := obj.(*corev1.Event)
	if !ok {
		return
	}

	// 안전하게 시간 처리 (LastTimestamp가 비어 있을 수도 있음)
	ts := event.EventTime.Time
	if ts.IsZero() && event.LastTimestamp.Time.Unix() > 0 {
		ts = event.LastTimestamp.Time
	}
	if ts.IsZero() {
		ts = time.Now()
	}

	data := map[string]interface{}{
		"timestamp":       ts.Format(time.RFC3339),
		"type":            event.Type,
		"reason":          event.Reason,
		"message":         event.Message,
		"kind":            event.InvolvedObject.Kind,
		"name":            event.InvolvedObject.Name,
		"namespace":       event.InvolvedObject.Namespace,
		"component":       event.Source.Component,
		"host":            event.Source.Host,
		"firstTimestamp":  event.FirstTimestamp.Time.Format(time.RFC3339),
		"lastTimestamp":   event.LastTimestamp.Time.Format(time.RFC3339),
		"count":           event.Count,
		"involvedObject":  event.InvolvedObject,  // 전체 참조 가능
		"reportingController": event.ReportingController,
		"reportingInstance":   event.ReportingInstance,
		"action":          event.Action,
		"related":         event.Related, // 예: PVC가 Pod와 관련되었을 때
	}

	// JSONL 형태로 기록
	file, err := os.OpenFile("artifacts/events.json", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		defer file.Close()
		enc := json.NewEncoder(file)
		enc.SetIndent("", "  ") 
		enc.Encode(data)
	}

	log.Printf("Event [%s] %s: %s/%s - %s",
		data["type"], data["reason"],
		data["kind"], data["name"],
		data["message"],
	)
}

func makeHandler(kind string, coll *collector.Collector, trigger func()) cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if kind == "Event" {
				handleEvent(obj)
			} else {
				logEvent(kind, "add", obj)
				coll.ApplyEvent(kind, "add", obj)
				trigger()
			}
		},
		UpdateFunc: func(_, newObj interface{}) {
			if kind == "Event" {
				handleEvent(newObj)
			} else {
				logEvent(kind, "update", newObj)
				coll.ApplyEvent(kind, "update", newObj)
				trigger()
			}
		},
		DeleteFunc: func(obj interface{}) {
			if kind == "Event" {
				return
			}
			logEvent(kind, "delete", obj)
			coll.ApplyEvent(kind, "delete", obj)
			trigger()
		},
	}
}

func saveMermaid(g *collector.Graph, dir string) error {
	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "staticgraph.mmd"))
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintln(f, "graph LR")
	for _, e := range g.Edges {
		fmt.Fprintf(f, "%s[\"%s\"] -->|%s| %s[\"%s\"]\n",
			e.From, g.Nodes[e.From].Label, e.Kind, e.To, g.Nodes[e.To].Label)
	}
	return nil
}

func saveCSV(g *collector.Graph, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	fn1 := filepath.Join(dir, "nodes.csv")
	f1, err := os.Create(fn1)
	if err != nil {
		return err
	}
	defer f1.Close()
	w1 := csv.NewWriter(f1)
	w1.Write([]string{"UID", "Label", "Type", "NS"})
	for _, n := range g.Nodes {
		w1.Write([]string{n.UID, n.Label, n.Type, n.NS})
	}
	w1.Flush()
	fn2 := filepath.Join(dir, "edges.csv")
	f2, err := os.Create(fn2)
	if err != nil {
		return err
	}
	defer f2.Close()
	w2 := csv.NewWriter(f2)
	w2.Write([]string{"FromUID", "ToUID", "Kind"})
	for _, e := range g.Edges {
		w2.Write([]string{e.From, e.To, string(e.Kind)})
	}
	w2.Flush()
	return nil
}
