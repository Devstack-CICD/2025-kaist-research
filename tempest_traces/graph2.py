import os
import json
import re
import matplotlib.pyplot as plt
import networkx as nx

def normalize_relation(name: str) -> str:
    name = re.sub(r'0x[0-9a-fA-F]+', 'ADDR', name)
    name = re.sub(r'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}', '<UUID>', name)
    name = re.sub(r'[a-f0-9]{32}', '<ID>', name)
    name = re.sub(r'/v3/[^/]+/', '/v3/<PROJECT>/', name)
    return name

def extract_relation(span):
    for tag in span.get("tags", []):
        if tag["key"] == "name":
            return normalize_relation(tag["value"])
    return normalize_relation(span.get("operationName", "unknown"))

def build_span_graph_from_folder(folder_path: str):
    G = nx.DiGraph()
    files = [f for f in os.listdir(folder_path) if f.endswith(".json")]
    print(f"[INFO] Found {len(files)} JSON trace files in {folder_path}")

    for filename in files:
        path = os.path.join(folder_path, filename)
        with open(path) as f:
            try:
                data = json.load(f)
            except Exception as e:
                print(f"[WARN] Could not load {filename}: {e}")
                continue

            for trace in data.get("data", []):
                span_map = {s["spanID"]: s for s in trace["spans"]}
                proc_map = {pid: p["serviceName"] for pid, p in trace.get("processes", {}).items()}
                span_id_to_node_id = {}

                for span in trace["spans"]:
                    sid = span["spanID"]
                    proc = proc_map.get(span["processID"], "unknown")
                    rel = extract_relation(span)
                    node_id = f"{proc}::{rel}"  # <-- 공통 노드 기준
                    span_id_to_node_id[sid] = node_id

                    if not G.has_node(node_id):
                        label = f"{proc}\n{rel}"
                        G.add_node(node_id, label=label)

                for span in trace["spans"]:
                    sid = span["spanID"]
                    for ref in span.get("references", []):
                        if ref["refType"] == "CHILD_OF" and ref["spanID"] in span_id_to_node_id:
                            parent_node = span_id_to_node_id[ref["spanID"]]
                            child_node = span_id_to_node_id[sid]
                            if parent_node != child_node and not G.has_edge(parent_node, child_node):
                                G.add_edge(parent_node, child_node)

    return G

def visualize_graph(G, category="trace", max_nodes=500):
    print(f"[INFO] Visualizing category: {category}")
    if len(G.nodes) > max_nodes:
        top_nodes = sorted(G.degree, key=lambda x: x[1], reverse=True)[:max_nodes]
        G = G.subgraph([n for n, _ in top_nodes]).copy()
        print(f"[INFO] Graph trimmed to top {max_nodes} nodes.")

    pos = nx.spring_layout(G, k=0.5, iterations=80)
    labels = nx.get_node_attributes(G, "label")

    plt.figure(figsize=(24, 18))
    nx.draw_networkx_nodes(G, pos, node_size=400, node_color="lightblue")
    nx.draw_networkx_edges(G, pos, arrows=True, arrowstyle="-|>", arrowsize=10, edge_color="gray")
    nx.draw_networkx_labels(G, pos, labels, font_size=6)

    plt.title(f"OpenStack Span Graph – {category}", fontsize=16)
    plt.axis("off")
    plt.tight_layout()
    output_file = f"{category}_graph_2.png"
    plt.savefig(output_file, dpi=300)
    plt.show()

    print(f"[SUMMARY] Nodes: {len(G.nodes)}, Edges: {len(G.edges)}")
    print(f"[OUTPUT] Graph image saved to {output_file}")

# 예시: compute 카테고리 trace 디렉토리
category = "compute"
folder_path = f"./service/{category}"

# 그래프 생성 및 시각화
G = build_span_graph_from_folder(folder_path)
visualize_graph(G, category)