from neo4j import GraphDatabase
from tabulate import tabulate

# Neo4j 연결 정보
uri = "bolt://localhost:7687" 
user = "neo4j"
password = "devSTACK1!"  

# 드라이버 설정
driver = GraphDatabase.driver(uri, auth=(user, password))

# 세션 열고 쿼리 실행
with driver.session() as session:
    query = """
    MATCH (n)
    WHERE n.type IS NOT NULL
    RETURN n.type AS type, count(*) AS count
    ORDER BY count DESC
    """
    result = session.run(query)
    
    # 결과 출력
    rows = []
    for record in result:
        rows.append([record["type"], record["count"]])

    print(tabulate(rows, headers=["Type", "Count"], tablefmt="fancy_grid"))

# 드라이버 종료
driver.close()