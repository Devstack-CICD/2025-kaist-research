from neo4j import GraphDatabase

class StatefulNeo4jTool:
    def __init__(self, uri, user, password):
        self.driver = GraphDatabase.driver(uri, auth=(user, password))
        self.context = {
            "visited_nodes": set(),
            "visited_edges": set(),
            "paths": [],
        }

    def run_query(self, cypher_query):
        with self.driver.session() as session:
            result = session.run(cypher_query)
            nodes_in_result = set()
            edges_in_result = set()
            path_trace = []

            for record in result:
                data = record.data()
                # 예: {'src': 'pod-123', 'dst': 'svc-abc'}
                if 'src' in data and 'dst' in data:
                    src = data['src']
                    dst = data['dst']
                    self.context["visited_nodes"].update([src, dst])
                    self.context["visited_edges"].add((src, dst))
                    path_trace.append((src, dst))

            self.context["paths"].append(path_trace)
            return "\n".join(str(r.data()) for r in result)

    def get_subgraph_cypher(self):
        nodes = self.context["visited_nodes"]
        edges = self.context["visited_edges"]

        node_match = "\n".join([f"MATCH (n {{uid: '{uid}'}})" for uid in nodes])
        edge_match = "\n".join([
            f"MATCH (a {{uid: '{src}'}})-[r]->(b {{uid: '{dst}'}})"
            for src, dst in edges
        ])

        return f"{node_match}\n{edge_match}\nRETURN a, b, r"