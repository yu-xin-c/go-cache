package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"mygocache/consistenthash"
	"mygocache/kitex_gen/geecache"
	"mygocache/kitex_gen/geecache/groupcache"

	"github.com/cloudwego/kitex/client"
)

// APIServer HTTP API 网关
type APIServer struct {
	addr      string
	clients   map[string]groupcache.Client
	hashRing  *consistenthash.Map
	nodeAddrs []string
}

// NewAPIServer 创建 API 服务器
func NewAPIServer(addr string, cacheNodes []string) (*APIServer, error) {
	clients := make(map[string]groupcache.Client)

	// 为每个缓存节点创建 Kitex 客户端
	for _, node := range cacheNodes {
		// 从 URL 中提取主机地址（去掉 http:// 前缀）
		hostAddr := strings.TrimPrefix(node, "http://")

		// 创建 Kitex 客户端，使用 node 作为服务名，并指定目标地址
		cli, err := groupcache.NewClient(node, client.WithHostPorts(hostAddr))
		if err != nil {
			return nil, fmt.Errorf("failed to create client for %s: %v", node, err)
		}
		clients[node] = cli
	}

	// 初始化一致性哈希环，使用 50 个虚拟节点
	hashRing := consistenthash.New(50, nil)
	hashRing.Add(cacheNodes...)

	return &APIServer{
		addr:      addr,
		clients:   clients,
		hashRing:  hashRing,
		nodeAddrs: cacheNodes,
	}, nil
}

// ServeHTTP 处理 HTTP 请求
func (s *APIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 解析请求路径
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) < 1 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	// 根据不同的路径处理请求
	switch parts[0] {
	case "api":
		s.handleGet(w, r)
	case "set":
		s.handleSet(w, r)
	case "delete":
		s.handleDelete(w, r)
	case "stats":
		s.handleStats(w, r)
	default:
		http.Error(w, "unknown endpoint", http.StatusNotFound)
	}
}

// handleGet 处理 GET 请求
func (s *APIServer) handleGet(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("group")
	if group == "" {
		group = "scores" // 默认组
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}

	// 使用一致性哈希选择节点
	nodeAddr := s.hashRing.Get(key)
	if nodeAddr == "" {
		http.Error(w, "no available cache nodes", http.StatusServiceUnavailable)
		return
	}

	client := s.clients[nodeAddr]
	if client == nil {
		http.Error(w, "selected node not available", http.StatusServiceUnavailable)
		return
	}

	// 调用 Kitex RPC
	req := &geecache.Request{
		Group: group,
		Key:   key,
	}

	resp, err := client.Get(context.Background(), req)
	if err != nil {
		log.Printf("[API] failed to get %s: %v", key, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[API] hit %s => %s", key, string(resp.Value))
	w.Header().Set("Content-Type", "text/plain")
	w.Write(resp.Value)
}

// handleSet 处理 SET 请求
func (s *APIServer) handleSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	group := r.URL.Query().Get("group")
	if group == "" {
		group = "scores"
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}

	// 读取请求体作为 value
	value, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 使用一致性哈希选择节点（与 GET 保持一致）
	nodeAddr := s.hashRing.Get(key)
	if nodeAddr == "" {
		http.Error(w, "no available cache nodes", http.StatusServiceUnavailable)
		return
	}

	client := s.clients[nodeAddr]
	if client == nil {
		http.Error(w, "selected node not available", http.StatusServiceUnavailable)
		return
	}

	// 调用 Kitex RPC
	req := &geecache.SetRequest{
		Group: group,
		Key:   key,
		Value: value,
		Ttl:   0, // 默认不过期
	}

	resp, err := client.Set(context.Background(), req)
	if err != nil || !resp.Success {
		log.Printf("[API] failed to set %s: %v", key, err)
		http.Error(w, "set failed", http.StatusInternalServerError)
		return
	}

	log.Printf("[API] set %s => %s", key, string(value))
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

// handleDelete 处理 DELETE 请求
func (s *APIServer) handleDelete(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("group")
	if group == "" {
		group = "scores"
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}

	// 使用一致性哈希选择节点（与 GET/SET 保持一致）
	nodeAddr := s.hashRing.Get(key)
	if nodeAddr == "" {
		http.Error(w, "no available cache nodes", http.StatusServiceUnavailable)
		return
	}

	client := s.clients[nodeAddr]
	if client == nil {
		http.Error(w, "selected node not available", http.StatusServiceUnavailable)
		return
	}

	req := &geecache.DeleteRequest{
		Group: group,
		Key:   key,
	}

	resp, err := client.Delete(context.Background(), req)
	if err != nil || !resp.Success {
		log.Printf("[API] failed to delete %s: %v", key, err)
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}

	log.Printf("[API] deleted %s", key)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

// handleStats 处理 STATS 请求
func (s *APIServer) handleStats(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("group")
	if group == "" {
		group = "scores"
	}

	var client groupcache.Client
	for _, c := range s.clients {
		client = c
		break
	}

	if client == nil {
		http.Error(w, "no available cache nodes", http.StatusServiceUnavailable)
		return
	}

	req := &geecache.StatsRequest{
		Group: group,
	}

	resp, err := client.Stats(context.Background(), req)
	if err != nil {
		log.Printf("[API] failed to get stats: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"item_count":%d,"hit_count":%d,"miss_count":%d,"total_count":%d}`,
		resp.ItemCount, resp.HitCount, resp.MissCount, resp.TotalCount)
}

// Start 启动 HTTP 服务器
func (s *APIServer) Start() error {
	log.Printf("API Gateway is running at %s", s.addr)
	return http.ListenAndServe(s.addr, s)
}

func main() {
	var port int
	flag.IntVar(&port, "port", 9999, "API server port")
	flag.Parse()

	// 缓存节点地址
	cacheNodes := []string{
		"http://localhost:8001",
		"http://localhost:8002",
		"http://localhost:8003",
	}

	addr := fmt.Sprintf("localhost:%d", port)
	server, err := NewAPIServer(addr, cacheNodes)
	if err != nil {
		log.Fatalf("Failed to create API server: %v", err)
	}

	log.Fatal(server.Start())
}
