package mygocache

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"mygocache/consistenthash"
	pb "mygocache/geecachepb"
	"mygocache/kitex_gen/geecache"
	"mygocache/kitex_gen/geecache/groupcache"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/server"
)

const (
	defaultReplicas = 50
)

// KitexPool implements PeerPicker for a pool of Kitex peers.
type KitexPool struct {
	self         string
	mu           sync.Mutex
	peers        *consistenthash.Map
	kitexClients map[string]groupcache.Client
}

// NewKitexPool initializes a Kitex pool of peers.
func NewKitexPool(self string) *KitexPool {
	return &KitexPool{
		self:         self,
		kitexClients: make(map[string]groupcache.Client),
	}
}

// Set updates the pool's list of peers.
func (p *KitexPool) Set(peers ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.peers = consistenthash.New(defaultReplicas, nil)
	p.peers.Add(peers...)
	p.kitexClients = make(map[string]groupcache.Client, len(peers))
	for _, peer := range peers {
		if peer != p.self {
			cli := groupcache.NewClient(peer, client.WithTransportInitializer(func() client.Option {
				return client.WithDialTimeout(5 * time.Second)
			}))
			p.kitexClients[peer] = cli
		}
	}
}

// PickPeer picks a peer according to key
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

func (g *kitexGetter) Get(in *pb.Request, out *pb.Response) error {
	kiteReq := &geecache.Request{
		Group: in.GetGroup(),
		Key:   in.GetKey(),
	}
	resp, err := g.client.Get(context.Background(), kiteReq)
	if err != nil {
		return err
	}
	out.Value = resp.Value
	return nil
}

var _ PeerGetter = (*kitexGetter)(nil)

// KitexServer implements the GroupCache service
type KitexServer struct {
}

// NewKitexServer creates a new Kitex server
func NewKitexServer() *KitexServer {
	return &KitexServer{}
}

// Get implements the GroupCache Get method
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

// Set implements the GroupCache Set method
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

// Delete implements the GroupCache Delete method
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

// Clear implements the GroupCache Clear method
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

// Stats implements the GroupCache Stats method
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

// GetMulti implements the GroupCache GetMulti method
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

// SetMulti implements the GroupCache SetMulti method
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

// StartKitexServer starts a Kitex server
func StartKitexServer(addr string) error {
	// Parse port from addr
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
	if err := groupcache.RegisterService(service, s); err != nil {
		return err
	}
	log.Printf("Kitex server is running at %s", addr)
	return s.Run()
}
