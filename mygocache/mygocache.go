package mygocache

import (
	"errors"
	"fmt"
	"mygocache/asynclog"
	"mygocache/pool"
	"mygocache/singleflight"
	"sync"
)

// ErrKeyNotFound 表示 key 不存在（负缓存命中时返回）
var ErrKeyNotFound = errors.New("key not found")

// DefaultNegativeCacheTTL 默认负缓存的 TTL（秒），防止不存在的 key 反复穿透
const DefaultNegativeCacheTTL int64 = 10

// Group 表示缓存命名空间与其数据加载逻辑
type Group struct {
	name      string
	getter    Getter
	mainCache *cache
	peers     PeerPicker
	// 使用 singleflight.Group 确保每个 key 只被加载一次
	loader *singleflight.Group
	// 默认 TTL（秒），0 表示永不过期
	defaultTTL int64
	// 负缓存 TTL（秒），防止不存在的 key 反复穿透
	negativeCacheTTL int64
	// 并发操作使用的协程池
	goroutinePool *pool.GoroutinePool
}

// Getter 用于加载某个 key 的数据
type Getter interface {
	Get(key string) ([]byte, error)
}

// GetterFunc 使用函数实现 Getter
type GetterFunc func(key string) ([]byte, error)

// Get 实现 Getter 接口
func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}

var (
	mu     sync.RWMutex
	groups = make(map[string]*Group)
)

// NewGroup 创建 Group 实例
func NewGroup(name string, cacheBytes int64, getter Getter) *Group {
	return NewGroupWithOptions(name, cacheBytes, getter, 0, StrategyLRU, 2)
}

// NewGroupWithTTL 创建带默认 TTL 的 Group 实例
func NewGroupWithTTL(name string, cacheBytes int64, getter Getter, defaultTTL int64) *Group {
	return NewGroupWithOptions(name, cacheBytes, getter, defaultTTL, StrategyLRU, 2)
}

// NewGroupWithOptions 创建带自定义参数的 Group 实例
func NewGroupWithOptions(name string, cacheBytes int64, getter Getter, defaultTTL int64, strategy CacheStrategy, k int) *Group {
	if getter == nil {
		panic("nil Getter")
	}
	mu.Lock()
	defer mu.Unlock()
	g := &Group{
		name:             name,
		getter:           getter,
		mainCache:        NewCache(cacheBytes, strategy, k),
		loader:           &singleflight.Group{},
		defaultTTL:       defaultTTL,
		negativeCacheTTL: DefaultNegativeCacheTTL,
		goroutinePool:    pool.NewGoroutinePool(10, 500, 1000), // 动态伸缩：[10, 500] worker，队列容量 1000
	}
	groups[name] = g
	return g
}

// SetNegativeCacheTTL 设置负缓存 TTL
func (g *Group) SetNegativeCacheTTL(ttl int64) {
	g.negativeCacheTTL = ttl
}

// GetGroup 返回指定名称的 Group，不存在则返回 nil
func GetGroup(name string) *Group {
	mu.RLock()
	g := groups[name]
	mu.RUnlock()
	return g
}

// Get 获取 key 对应的缓存值
func (g *Group) Get(key string) (ByteView, error) {
	return g.GetWithTTL(key, g.defaultTTL)
}

// GetWithTTL 获取 key 对应的缓存值，并指定 TTL
func (g *Group) GetWithTTL(key string, ttl int64) (ByteView, error) {
	if key == "" {
		return ByteView{}, fmt.Errorf("key is required")
	}

	if v, ok := g.mainCache.get(key); ok {
		if v.Len() == 0 {
			// 负缓存命中：key 不存在，短时间内不再穿透
			asynclog.Println("[GeeCache] negative cache hit")
			return ByteView{}, ErrKeyNotFound
		}
		asynclog.Println("[GeeCache] hit")
		return v, nil
	}

	// 缓存未命中，通过 singleflight 加载
	return g.loadWithTTL(key, ttl)
}

// Set 设置 key 对应的缓存值，并指定 TTL
func (g *Group) Set(key string, value []byte, ttl int64) error {
	byteView := ByteView{b: cloneBytes(value)}
	g.mainCache.add(key, byteView, ttl)
	return nil
}

// Delete 删除缓存中的 key
func (g *Group) Delete(key string) error {
	g.mainCache.delete(key)
	return nil
}

// Clear 清空缓存
func (g *Group) Clear() error {
	g.mainCache.clear()
	return nil
}

// Stats 表示缓存统计信息
type Stats struct {
	ItemCount  int
	HitCount   int
	MissCount  int
	TotalCount int
}

// Stats 返回缓存统计信息
func (g *Group) Stats() Stats {
	return g.mainCache.stats()
}

// GetMulti 批量获取缓存
func (g *Group) GetMulti(keys []string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	for _, key := range keys {
		if v, ok := g.mainCache.get(key); ok {
			result[key] = v.ByteSlice()
		} else {
			g.mainCache.recordMiss()
		}
	}
	return result, nil
}

// SetMulti 批量设置缓存
func (g *Group) SetMulti(values map[string][]byte, ttl int64) error {
	for key, value := range values {
		byteView := ByteView{b: cloneBytes(value)}
		g.mainCache.add(key, byteView, ttl)
	}
	return nil
}

// RegisterPeers 注册用于选择远端节点的 PeerPicker
func (g *Group) RegisterPeers(peers PeerPicker) {
	if g.peers != nil {
		panic("RegisterPeerPicker called more than once")
	}
	g.peers = peers
}

func (g *Group) loadWithTTL(key string, ttl int64) (value ByteView, err error) {
	// 每个 key 只会被加载一次，无论并发调用有多少
	viewi, err, shared := g.loader.Do(key, func() (interface{}, error) {
		if g.peers != nil {
			if peer, ok := g.peers.PickPeer(key); ok {
				// 通过协程池限流后端 RPC 调用
				type result struct {
					value ByteView
					err   error
				}
				resultCh := make(chan result, 1)
				g.goroutinePool.Submit(func() {
					peerValue, peerErr := g.getFromPeer(peer, key)
					resultCh <- result{peerValue, peerErr}
				})
				res := <-resultCh

				if res.err == nil {
					g.mainCache.add(key, res.value, ttl)
					return res.value, nil
				}
				asynclog.Println("[GeeCache] Failed to get from peer", res.err)
			}
		}

		return g.getLocallyWithTTL(key, ttl)
	})

	if err == nil {
		if shared {
			g.mainCache.recordHit()
			asynclog.Println("[GeeCache] singleflight hit (shared)")
		} else {
			g.mainCache.recordMiss()
			asynclog.Println("[GeeCache] singleflight miss (first)")
		}
		return viewi.(ByteView), nil
	}
	return
}

func (g *Group) populateCache(key string, value ByteView, ttl int64) {
	g.mainCache.add(key, value, ttl)
}

func (g *Group) getLocallyWithTTL(key string, ttl int64) (ByteView, error) {
	bytes, err := g.getter.Get(key)
	if err != nil {
		// 负缓存：缓存空值，短 TTL 防穿透
		g.populateCache(key, ByteView{}, g.negativeCacheTTL)
		asynclog.Printf("[GeeCache] negative cache set for key=%s ttl=%ds", key, g.negativeCacheTTL)
		return ByteView{}, err
	}
	value := ByteView{b: cloneBytes(bytes)}
	g.populateCache(key, value, ttl)
	return value, nil
}

func (g *Group) getFromPeer(peer PeerGetter, key string) (ByteView, error) {
	bytes, err := peer.Get(g.name, key)
	if err != nil {
		return ByteView{}, err
	}
	return ByteView{b: bytes}, nil
}
