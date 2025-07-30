package collector

import (
    "context"  
    "log"

    "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "github.com/kaist2025/k8s-e2e-tests/internal/k8sclient"
)

type Client = k8sclient.Client

type Stage func(ctx context.Context, c *Client, g *Graph) error

type Collector struct {
    Client *Client
    Stages []Stage
    Graph  *Graph
}

func NewCollector(client *Client, stages []Stage) *Collector {
    return &Collector{
        Client: client,
        Stages: stages,
        Graph: &Graph{
            Nodes:   make(map[string]Node),
            Edges:   make(map[string]Edge),
            EdgeMap: make(map[string]map[string]struct{}),
        },
    }
}

func (co *Collector) Run(ctx context.Context) (*Graph, error) {
    // Node 구조체를 담을 맵으로 초기화
    g := &Graph{
        Nodes:   make(map[string]Node),
        Edges:   make(map[string]Edge),
        EdgeMap: make(map[string]map[string]struct{}),
    }

    for _, st := range co.Stages {
        if err := st(ctx, co.Client, g); err != nil {
            return nil, err
        }
    }
    co.Graph = g
    return g, nil
}

func (co *Collector) ApplyEvent(kind string, event string, obj interface{}) {
	u, err := toUnstructured(obj)
	if err != nil {
		log.Printf("invalid object type in event '%s' for kind '%s': %T\n", event, kind, obj)
		return
	}

	switch event {
	case "add":
		HandleAdd(co.Graph, kind, u)
	case "update":
		HandleUpdate(co.Graph, kind, u)
	case "delete":
		HandleDelete(co.Graph, kind, u)
	}
}

func toUnstructured(obj interface{}) (*unstructured.Unstructured, error) {
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: m}, nil
}