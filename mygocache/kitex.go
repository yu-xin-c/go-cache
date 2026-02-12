package mygocache

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"

	"mygocache/consistenthash"
	"mygocache/kitex_gen/geecache"
	"mygocache/kitex_gen/geecache/groupcache"

	"github.com/cloudwego/kitex/server"
)

const (
	defaultReplicas = 50
)

// KitexPool 用于管理 Kitex 节点池
type KitexPool struct {
	self         string
	mu           sync.Mutex
	peers        *consistenthash.Map
	kitexClients map[string]groupcache.Client
}

// NewKitexPool 初始化 Kitex 节点池
func NewKitexPool(self string) *KitexPool {
	return &KitexPool{
		self:         self,
		kitexClients: make(map[string]groupcache.Client),
	}
}

// Set 更新节点列表
func (p *KitexPool) Set(peers ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.peers = consistenthash.New(defaultReplicas, nil)
	p.peers.Add(peers...)
	p.kitexClients = make(map[string]groupcache.Client, len(peers))
	for _, peer := range peers {
		if peer != p.self {
			cli, err := groupcache.NewClient(peer)
			if err != nil {
				log.Printf("创建 Kitex 客户端失败: %v", err)
				continue
			}
			p.kitexClients[peer] = cli
		}
	}
}

// PickPeer 根据 key 选择节点
func (p *KitexPool) PickPeer(key string) (PeerGetter, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if peer := p.peers.Get(key); peer != "" && peer != p.self {
		log.Printf("Pick peer %s", peer)
		if client, ok := p.kitexClients[peer]; ok {
			return &kitexGetter{client: client}, true
		}
	}
	return nil, false
}

var _ PeerPicker = (*KitexPool)(nil)

type kitexGetter struct {
	client groupcache.Client
}

func (g *kitexGetter) Get(group string, key string) ([]byte, error) {
	kiteReq := &geecache.Request{
		Group: group,
		Key:   key,
	}
	resp, err := g.client.Get(context.Background(), kiteReq)
	if err != nil {
		return nil, err
	}
	return resp.Value, nil
}

var _ PeerGetter = (*kitexGetter)(nil)

// KitexServer 实现 GroupCache 服务
type KitexServer struct {
}

// NewKitexServer 创建 Kitex 服务端
func NewKitexServer() *KitexServer {
	return &KitexServer{}
}

// Get 实现 GroupCache 的 Get 方法
func (s *KitexServer) Get(ctx context.Context, req *geecache.Request) (resp *geecache.Response, err error) {
	group := GetGroup(req.Group)
	if group == nil {
		return nil, fmt.Errorf("group not found: %s", req.Group)
	}

	view, err := group.Get(req.Key)
	if err != nil {
		return nil, err
	}

	return &geecache.Response{Value: view.ByteSlice()}, nil
}

// Set 实现 GroupCache 的 Set 方法
func (s *KitexServer) Set(ctx context.Context, req *geecache.SetRequest) (resp *geecache.SetResponse, err error) {
	group := GetGroup(req.Group)
	if group == nil {
		return nil, fmt.Errorf("group not found: %s", req.Group)
	}

	err = group.Set(req.Key, req.Value, req.Ttl)
	if err != nil {
		return &geecache.SetResponse{Success: false}, err
	}

	return &geecache.SetResponse{Success: true}, nil
}

// Delete 实现 GroupCache 的 Delete 方法
func (s *KitexServer) Delete(ctx context.Context, req *geecache.DeleteRequest) (resp *geecache.DeleteResponse, err error) {
	group := GetGroup(req.Group)
	if group == nil {
		return nil, fmt.Errorf("group not found: %s", req.Group)
	}

	err = group.Delete(req.Key)
	if err != nil {
		return &geecache.DeleteResponse{Success: false}, err
	}

	return &geecache.DeleteResponse{Success: true}, nil
}

// Clear 实现 GroupCache 的 Clear 方法
func (s *KitexServer) Clear(ctx context.Context, req *geecache.ClearRequest) (resp *geecache.ClearResponse, err error) {
	group := GetGroup(req.Group)
	if group == nil {
		return nil, fmt.Errorf("group not found: %s", req.Group)
	}

	err = group.Clear()
	if err != nil {
		return &geecache.ClearResponse{Success: false}, err
	}

	return &geecache.ClearResponse{Success: true}, nil
}

// Stats 实现 GroupCache 的 Stats 方法
func (s *KitexServer) Stats(ctx context.Context, req *geecache.StatsRequest) (resp *geecache.StatsResponse, err error) {
	group := GetGroup(req.Group)
	if group == nil {
		return nil, fmt.Errorf("group not found: %s", req.Group)
	}

	stats := group.Stats()
	return &geecache.StatsResponse{
		ItemCount:  int64(stats.ItemCount),
		HitCount:   int64(stats.HitCount),
		MissCount:  int64(stats.MissCount),
		TotalCount: int64(stats.TotalCount),
	}, nil
}

// GetMulti 实现 GroupCache 的 GetMulti 方法
func (s *KitexServer) GetMulti(ctx context.Context, req *geecache.GetMultiRequest) (resp *geecache.GetMultiResponse, err error) {
	group := GetGroup(req.Group)
	if group == nil {
		return nil, fmt.Errorf("group not found: %s", req.Group)
	}

	values, err := group.GetMulti(req.Keys)
	if err != nil {
		return &geecache.GetMultiResponse{Values: make(map[string][]byte)}, err
	}

	return &geecache.GetMultiResponse{Values: values}, nil
}

// SetMulti 实现 GroupCache 的 SetMulti 方法
func (s *KitexServer) SetMulti(ctx context.Context, req *geecache.SetMultiRequest) (resp *geecache.SetMultiResponse, err error) {
	group := GetGroup(req.Group)
	if group == nil {
		return nil, fmt.Errorf("group not found: %s", req.Group)
	}

	err = group.SetMulti(req.Values, req.Ttl)
	if err != nil {
		return &geecache.SetMultiResponse{Success: false}, err
	}

	return &geecache.SetMultiResponse{Success: true}, nil
}

// StartKitexServer 启动 Kitex 服务
func StartKitexServer(addr string) error {
	// 从地址中解析端口
	portStr := strings.TrimPrefix(addr, "http://")
	portStr = strings.Split(portStr, ":")[1]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid addr: %v", err)
	}

	s := server.NewServer(
		server.WithServiceAddr(&net.TCPAddr{Port: port}),
	)
	service := NewKitexServer()
	if err := groupcache.RegisterService(s, service); err != nil {
		return err
	}
	log.Printf("Kitex server is running at %s", addr)
	return s.Run()
}
