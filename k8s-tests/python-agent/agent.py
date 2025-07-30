from langchain_ollama import OllamaLLM
from langchain.agents import initialize_agent, AgentType
from langchain.tools import Tool
from tools.neo4j import StatefulNeo4jTool  # 수정: 새 도구 클래스 import

# LLM
llm = OllamaLLM(model="tinyllama")

# Neo4j Tool Instance
db = StatefulNeo4jTool("bolt://localhost:7687", "neo4j", "devSTACK1!")

# LangChain Tool
neo4j_tool = Tool.from_function(
    name="neo4j_query_tool",
    func=db.run_query,
    description="Run Cypher queries on the Kubernetes/OpenStack resource graph in Neo4j and accumulate exploration history.",
)

# Agent Initialization
agent = initialize_agent(
    tools=[neo4j_tool],
    llm=llm,
    agent=AgentType.ZERO_SHOT_REACT_DESCRIPTION,
    verbose=True,
    agent_kwargs={
        "system_message": """You are an agent that answers questions about a Kubernetes/OpenStack graph using the 'neo4j_query_tool'.
Respond using the ReAct format strictly:
Thought: your reasoning
Action: neo4j_query_tool
Action Input: <a cypher query>
(Do not explain, just respond with Action)
"""
    },
    handle_parsing_errors=True
)

# query example
query = """You are an agent that answers questions about a Kubernetes/OpenStack graph. 
You should answer questions that asks to find the root cause of incidents, when OpenStack deployed on Kubenetes cluster.
Find the candidates of root cause resource according to this warning event: 
2025/07/17 10:11:21 Event [Warning] FailedGetResourceMetric: HorizontalPodAutoscaler/octavia-api - failed to get memory usage: unable to get metrics for resource memory: unable to fetch metrics from resource metrics API: the server could not find the requested resource (get pods.metrics.k8s.io)
"""
response = agent.run(query)

print("\nLLM Response:\n", response)

# Optional: 탐색 이력 출력
print("\n탐색된 노드들:", db.context["visited_nodes"])
print("탐색된 엣지들:", db.context["visited_edges"])