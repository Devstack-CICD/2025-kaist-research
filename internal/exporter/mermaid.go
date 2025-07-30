package exporter

import (
    "fmt"
    "io"
    "github.com/kaist2025/k8s-e2e-tests/internal/collector"
)

func WriteMermaid(g *collector.Graph, w io.Writer) error {
    fmt.Fprintln(w, "graph LR")
    for _, n := range g.Nodes {
        fmt.Fprintf(w, `%s["%s"]:::ns_%s\n`, n.UID, n.Label, n.NS)
    }
    for _, e := range g.Edges {
        fmt.Fprintf(w, "%s -->|%s| %s\n", e.From, e.Kind, e.To)
    }
    return nil
}