#!/bin/bash
trap "rm cache-server api-gateway;kill 0" EXIT

# 编译缓存服务器和 API 网关
go build -o cache-server .
go build -o api-gateway ./cmd/api-gateway

# 启动 3 个缓存节点
./cache-server -port=8001 > node1.log 2>&1 &
./cache-server -port=8002 > node2.log 2>&1 &
./cache-server -port=8003 > node3.log 2>&1 &

# 启动 API 网关
./api-gateway -port=9999 > api-gateway.log 2>&1 &

# 等待服务器启动
sleep 2

echo ">>> Cache nodes running on ports 8001, 8002, 8003"
echo ">>> API Gateway running on port 9999"
echo ">>> Start testing..."

# 测试 GET 请求
curl "http://localhost:9999/api?key=Tom" &
curl "http://localhost:9999/api?key=Tom" &
curl "http://localhost:9999/api?key=Tom" &

wait

echo ""
echo ">>> Test completed"