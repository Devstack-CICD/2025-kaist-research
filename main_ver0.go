package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"time"
	"os"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/e2e-framework/pkg/envconf"

	"github.com/kaist2025/k8s-e2e-tests/internal/collector"
	"github.com/kaist2025/k8s-e2e-tests/internal/k8sclient"
)

func main() {
	var kubeconfig string
	var resyncPeriod time.Duration
	var debounce time.Duration
	var outputDir string
	
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Absolute path to the kiubeconfig file")
	flag.DurationVar(&resyncPeriod, "resync", time.Hour, "Shared informer resync period")
	flag.DurationVar(&debounce, "debounce", 5*time.Second, "Debounce interval for collector triggers")
	flag.StringVar(&outputDir, "output", "artifacts", "Directory to write graph outputs")
	flag.Parse()

	// 1. Build config and clients
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

	// 2. Wrap in e2e-framework client
	envCfg := envconf.NewWithKubeConfig(configPath)
	k8sCli := k8sclient.NewFromEnv(envCfg)
	coll := &collector.Collector{
		Client: k8sCli,
		Stages: []collector.Stage{
			collector.WorkloadStage,
			collector.IngressStage,
			collector.EndpointStage,
			collector.DSSTSStage,
			collector.PVCStage,
			collector.NetpolStage,
			collector.JaegerStage("http://35.216.127.215:30168"),
		},
	}

	// 3. Prepare informer factory
	factory := informers.NewSharedInformerFactory(clientset, resyncPeriod)

	// 4. Watch resources
	watchers := []cache.SharedIndexInformer{
		factory.Core().V1().Pods().Informer(),
		factory.Apps().V1().Deployments().Informer(),
		factory.Core().V1().Services().Informer(),
		factory.Networking().V1().Ingresses().Informer(),
		factory.Networking().V1().NetworkPolicies().Informer(),
		factory.Core().V1().PersistentVolumeClaims().Informer(),
		factory.Core().V1().PersistentVolumes().Informer(),
		factory.Discovery().V1().EndpointSlices().Informer(),
		factory.Apps().V1().DaemonSets().Informer(),
		factory.Apps().V1().StatefulSets().Informer(),
	}

	// 5. Debounced trigger
	triggerCh := make(chan struct{}, 1)
	debounced := time.AfterFunc(debounce, func() {})
	debounced.Stop()
	trigger := func() {
		select {
		case triggerCh <- struct{}{}:
		default:
		}
	}

	// 6. Event handlers
	handler := cache.ResourceEventHandlerFuncs{
		AddFunc:	func(obj interface{}) { 
			log.Println("Add")
			trigger() 
		},
		UpdateFunc:	func(_, _ interface{}) { 
			trigger() 
		},
		DeleteFunc: func(obj interface{}) { 
			log.Println("Delete")
			trigger() 
		},
	}
	for _, inf := range watchers {
		inf.AddEventHandler(handler)
	}

	// 7. Start Informers
	stopCh := make(chan struct{})
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	// 8. Initial collector run
	ctx := context.Background()
	log.Println("▶ initial graph collection")
	runOnce(ctx, coll, outputDir)

	// 9. Run loop
	for {
		<- triggerCh
		//reset debounce timer
		debounced.Reset(debounce)
		select {
		case <-debounced.C:
			//actual run
			runOnce(ctx, coll, outputDir)
		case <-stopCh:
			return
		}
	}
}

func runOnce(ctx context.Context, coll *collector.Collector, dir string) {
	log.Printf(">> Running collector")
	g, err := coll.Run(ctx)
	if err != nil {
		log.Printf("collector error: %v", err)
		return
	}
	//save mermaid
	if err := saveMermaid(g, dir); err != nil {
		log.Printf("saveMermaid error: %v", err)
	}
	//save csv
	if err := saveCSV(g, dir); err != nil {
		log.Printf("saveCSV error: %v", err)
	}
	log.Printf("<< Collector completed")
}


// Save 

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
	// 1) nodes.csv
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
	if err := w1.Write([]string{"UID", "Label", "Type", "NS"}); err != nil {
		return err
	}
	for _, n := range g.Nodes {
		if err := w1.Write([]string{n.UID, n.Label, n.Type, n.NS}); err != nil {
			return err
		}
	}
	w1.Flush()

	// 2) edges.csv
	fn2 := filepath.Join(dir, "edges.csv")
	f2, err := os.Create(fn2)
	if err != nil {
		return err
	}
	defer f2.Close()
	w2 := csv.NewWriter(f2)
	if err := w2.Write([]string{"FromUID", "ToUID", "Kind"}); err != nil {
		return err
	}
	for _, e := range g.Edges {
		if err := w2.Write([]string{e.From, e.To, string(e.Kind)}); err != nil {
			return err
		}
	}
	w2.Flush()
	return nil
}