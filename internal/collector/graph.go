package collector

import (
	"fmt"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type EdgeKind string

const (
	Owns    EdgeKind = "owns"
	Routes  EdgeKind = "routes"
	Binds   EdgeKind = "binds"
	Calls   EdgeKind = "calls"
	Reads   EdgeKind = "reads"
	Mounts	EdgeKind = "mounts"
	Uses    EdgeKind = "uses"
	Allow   EdgeKind = "allow"
	Targets EdgeKind = "targets"
)

type Node struct {
	UID   string
	Label string // 원본 resource 이름
	Type  string // Deployment, Pod, Service, ...
	NS    string // namespace
}

type Edge struct {
	From, To string   // UID
	Kind    EdgeKind // relation
}

type Graph struct {
	Nodes   map[string]Node       // UID -> Node
	Edges   map[string]Edge       // edgeID -> Edge
	EdgeMap map[string]map[string]struct{} // UID -> set of edgeIDs
}

func edgeID(from, to string, kind EdgeKind) string {
	return fmt.Sprintf("%s->%s:%s", from, to, kind)
}

func (g *Graph) AddNode(ns, name, typ string) string {
	uid := safeID(ns, name)
	if _, exists := g.Nodes[uid]; !exists {
		g.Nodes[uid] = Node{UID: uid, Label: name, Type: typ, NS: ns}
	}
	return uid
}

func (g *Graph) AddEdge(fromUID, toUID string, kind EdgeKind) {
	if g.Edges == nil {
		g.Edges = make(map[string]Edge)
	}
	if g.EdgeMap == nil {
		g.EdgeMap = make(map[string]map[string]struct{})
	}
	id := edgeID(fromUID, toUID, kind)
	g.Edges[id] = Edge{From: fromUID, To: toUID, Kind: kind}

	for _, uid := range []string{fromUID, toUID} {
		if g.EdgeMap[uid] == nil {
			g.EdgeMap[uid] = make(map[string]struct{})
		}
		g.EdgeMap[uid][id] = struct{}{}
	}
}

func (g *Graph) AddResource(kind string, obj *unstructured.Unstructured) {
	_ = g.AddNode(obj.GetNamespace(), obj.GetName(), kind)
}

func (g *Graph) UpdateResource(kind string, oldObj, newObj *unstructured.Unstructured) {
	g.AddResource(kind, newObj)
}

func (g *Graph) DeleteResource(kind string, obj *unstructured.Unstructured) {
	uid := safeID(obj.GetNamespace(), obj.GetName())
	delete(g.Nodes, uid)

	if edgeSet, ok := g.EdgeMap[uid]; ok {
		for eid := range edgeSet {
			delete(g.Edges, eid)
		}
		delete(g.EdgeMap, uid)
	}
}
