To build and run the k8s-e2e-collector, run this command:

```
cd k8s-tests
go build -o bin/k8s-e2e-collector ./cmd/graph-collector
cd bin
chmod +x k8s-e2e-collector
./k8s-e2e-collector
```

To use Neo4j

```
docker run -d \
  --name test-neo4j \
  -p 7474:7474 -p 7687:7687 \
  -e NEO4J_AUTH=neo4j/devSTACK1! \
  -e NEO4J_PLUGINS='["apoc"]' \
  -e NEO4J_dbms_security_procedures_unrestricted=apoc.* \
  -e NEO4J_dbms_security_procedures_allowlist=apoc.* \
  -e NEO4J_apoc_export_file_enabled=true \
  -e NEO4J_apoc_import_file_enabled=true \
  -e NEO4J_apoc_import_file_use__neo4j__config=true \
  neo4j:5.20
```
```  
docker start test-neo4j
```

The traces from the tempest tests are collected in "combined" and "service" folder.

The graph of resources is constructed using kg.py
The graph of relations is constructed using relation_graph.py