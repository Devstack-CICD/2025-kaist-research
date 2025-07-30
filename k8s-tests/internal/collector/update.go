package collector

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"time"
	"fmt"
)

func timestamp() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// HandleAdd handles adding a new resource to the graph.
func HandleAdd(g *Graph, kind string, obj *unstructured.Unstructured) {
	fmt.Printf("[%s] [HandleAdd] kind=%s name=%s\n", timestamp(), kind, obj.GetName())
	g.AddResource(kind, obj)
}

// HandleUpdate handles updating a resource in the graph.
func HandleUpdate(g *Graph, kind string, obj *unstructured.Unstructured) {
	fmt.Printf("[%s] [HandleUpdate] kind=%s name=%s\n", timestamp(), kind, obj.GetName())
	g.UpdateResource(kind, nil, obj) // oldObj는 현재 사용하지 않음
}

// HandleDelete handles deleting a resource from the graph.
func HandleDelete(g *Graph, kind string, obj *unstructured.Unstructured) {
	fmt.Printf("[%s] [HandleDelete] kind=%s name=%s\n", timestamp(), kind, obj.GetName())
	g.DeleteResource(kind, obj)
}