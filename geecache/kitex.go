package geecache

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"geecache/consistenthash"
	pb "geecache/geecachepb"
	"geecache/kitex_gen/geecache"
	"geecache/kitex_gen/geecache/groupcache"

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
