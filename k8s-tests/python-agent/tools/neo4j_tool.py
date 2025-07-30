from langchain.tools import Tool
from neo4j import GraphDatabase

class Neo4jQueryTool:
    def __init__(self, uri, user, password):
        self.driver = GraphDatabase.driver(uri, auth=(user, password))

    def query(self, cypher_query):
        with self.driver.session() as session:
            result = session.run(cypher_query)
            return "\n".join(str(record.data()) for record in result)

def make_neo4j_tool(uri, user, password):
    db = Neo4jQueryTool(uri, user, password)
    return Tool.from_function(
        name="neo4j_query_tool",
        func=db.query,
        description="Run Cypher queries on the Kubernetes/OpenStack resource graph in Neo4j.",
    )