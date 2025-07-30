package exporter

import (
	"context"
	"fmt"
	"log"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/kaist2025/k8s-e2e-tests/internal/collector"
)

func ConnectNeo4j(uri, user, password string) neo4j.DriverWithContext {
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, password, ""))
	if err != nil {
		log.Fatalf("[Neo4j] connection failed: %v", err)
	}
	return driver
}

func ExportToNeo4j(ctx context.Context, g *collector.Graph, driver neo4j.DriverWithContext) error {
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	for _, node := range g.Nodes {
		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			query := `
MERGE (n:Resource {uid: $uid})
SET n.name = $name, n.type = $type, n.namespace = $ns
`
			params := map[string]any{
				"uid":  node.UID,
				"name": node.Label,
				"type": node.Type,
				"ns":   node.NS,
			}
			return tx.Run(ctx, query, params)
		})
		if err != nil {
			log.Printf("[Neo4j] node error: %v", err)
		}
	}

	for _, edge := range g.Edges {
		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			query := fmt.Sprintf(`
MATCH (a:Resource {uid: $from}), (b:Resource {uid: $to})
MERGE (a)-[:%s]->(b)
`, edge.Kind)
			params := map[string]any{
				"from": edge.From,
				"to":   edge.To,
			}
			return tx.Run(ctx, query, params)
		})
		if err != nil {
			log.Printf("[Neo4j] edge error: %v", err)
		}
	}

	log.Println("[Neo4j] export completed.")
	return nil
}