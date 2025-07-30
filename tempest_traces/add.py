import json
from neo4j import GraphDatabase

# 파일 경로
PODS_FILE = "pods.json"
DEPLOYMENTS_FILE = "deployments.json"

# Neo4j 설정
NEO4J_URI = "bolt://localhost:7687"
NEO4J_USER = "neo4j"
NEO4J_PASSWORD = "devSTACK1!"  

# Trace 상의 serviceName과 pod 연결 기준 
SERVICE_TO_POD_KEYWORDS = {
    "nova-osapi_compute": "nova-api",
    "neutron-server": "neutron-server",
    "cinder-api": "cinder-api",
    "glance-api": "glance-api",
    "keystone-public": "keystone",
}

# 1. Deployment name 추출
with open(DEPLOYMENTS_FILE) as f:
    deploy_data = json.load(f)
    deployments = {
        item["metadata"]["name"]: item
        for item in deploy_data["items"]
    }

# 2. Pod 정보 추출
with open(PODS_FILE) as f:
    pod_data = json.load(f)

triplets = []

for item in pod_data["items"]:
    pod_name = item["metadata"]["name"]
    labels = item["metadata"].get("labels", {})
    node_name = item["spec"].get("nodeName", "unknown")
    owner = item["metadata"].get("ownerReferences", [{}])[0]
    deployment_name = owner.get("name", "<unknown>")

    # 서비스명 추론
    matched_services = [
        svc for svc, keyword in SERVICE_TO_POD_KEYWORDS.items()
        if keyword in pod_name
    ]
    if not matched_services:
        continue  # 알 수 없는 Pod은 스킵

    service = matched_services[0]

    triplets.append((service, pod_name, deployment_name, node_name))

# 3. Neo4j 반영
driver = GraphDatabase.driver(NEO4J_URI, auth=(NEO4J_USER, NEO4J_PASSWORD))

with driver.session() as session:
    for svc, pod, dep, node in triplets:
        session.run("""
        MERGE (s:Service {name: $svc})
        MERGE (p:Pod {name: $pod})
        MERGE (d:Deployment {name: $dep})
        MERGE (n:Node {name: $node})
        MERGE (s)-[:DEPLOYED_AS]->(p)
        MERGE (p)-[:PART_OF]->(d)
        MERGE (p)-[:SCHEDULED_ON]->(n)
        """, svc=svc, pod=pod, dep=dep, node=node)

print(f"총 {len(triplets)}개 연결관계가 Neo4j에 저장되었습니다.")