from neo4j import GraphDatabase

driver = GraphDatabase.driver("bolt://localhost:7687", auth=("neo4j", "devSTACK1!"))
with driver.session() as session:
    result = session.run("RETURN 'Neo4j 연결 성공!' AS msg")
    print(result.single()["msg"])