import os
import json
import re
from neo4j import GraphDatabase

def normalize_relation_name(name: str) -> str:
    name = re.sub(r'0x[0-9a-fA-F]+', 'ADDR', name)
    name = re.sub(r'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}', '<UUID>', name)
    name = re.sub(r'[a-f0-9]{32}', '<ID>', name)
    name = re.sub(r'/v3/[^/]+/', '/v3/<PROJECT>/', name)
    return name

def extract_relation_name(span):
    for tag in span.get("tags", []):
        if tag["key"] == "name":
            return tag["value"]
    return span.get("operationName", "unknown")

input_dir = "./combined"
triplets = set()

for root, _, files in os.walk(input_dir):
    for file in files:
        if not file.endswith(".jsonl"):
            continue
        file_path = os.path.join(root, file)
        with open(file_path, "r") as f:
            for line in f:
                try:
                    trace = json.loads(line)
                except Exception:
                    continue

                span_map = {span["spanID"]: span for span in trace.get("spans", [])}
                proc_map = {pid: p["serviceName"] for pid, p in trace.get("processes", {}).items()}

                for child_span in trace.get("spans", []):
                    child_proc = proc_map.get(child_span.get("processID"))
                    rel = normalize_relation_name(extract_relation_name(child_span))

                    for ref in child_span.get("references", []):
                        if ref.get("refType") != "CHILD_OF":
                            continue
                        parent_span = span_map.get(ref["spanID"])
                        if not parent_span:
                            continue
                        parent_proc = proc_map.get(parent_span.get("processID"))
                        if parent_proc and child_proc:
                            triplets.add((parent_proc, rel, child_proc))

# 출력 결과 확인
print(f"총 {len(triplets)}개 관계 추출됨")
for t in sorted(triplets):
    print(t)
    
NEO4J_URI = "bolt://localhost:7687"
NEO4J_USER = "neo4j"
NEO4J_PASSWORD = "test1234"

driver = GraphDatabase.driver(NEO4J_URI, auth=(NEO4J_USER, NEO4J_PASSWORD))

def save_triplets_to_neo4j(triplets):
    with driver.session() as session:
        for source, relation, target in triplets:
            session.run("""
                MERGE (a:Service {name: $src})
                MERGE (b:Service {name: $dst})
                MERGE (a)-[r:CALLS {operation: $op}]->(b)
            """, src=source, dst=target, op=relation)

# triplets 변수는 이미 위에서 추출된 상태라고 가정
save_triplets_to_neo4j(triplets)
print("Neo4j 저장 완료")