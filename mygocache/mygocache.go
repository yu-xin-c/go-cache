package mygocache

import (
	"fmt"
	"log"
	"mygocache/pool"
	"mygocache/singleflight"
	"sync"
)

// Group 表示缓存命名空间与其数据加载逻辑
type Group struct {
	name      string
	getter    Getter
	mainCache cache
	peers     PeerPicker
	// 使用 singleflight.Group 确保每个 key 只被加载一次
	loader *singleflight.Group
	// 默认 TTL（秒），0 表示永不过期
	defaultTTL int64
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
	// 初始化协程池，大小为 CPU 核心数的 2 倍
	goroutinePoolSize := 10 // 默认大小
	g := &Group{
		name:          name,
		getter:        getter,
		mainCache:     *NewCache(cacheBytes, strategy, k),
		loader:        &singleflight.Group{},
		defaultTTL:    defaultTTL,
		goroutinePool: pool.NewGoroutinePool(goroutinePoolSize, 1000),
	}
	groups[name] = g
	return g
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
		log.Println("[GeeCache] hit")
		return v, nil
	}

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
	viewi, err := g.loader.Do(key, func() (interface{}, error) {
		if g.peers != nil {
			if peer, ok := g.peers.PickPeer(key); ok {
				// 使用协程池从远程节点获取数据
				var peerValue ByteView
				var peerErr error

				// 创建一个通道来接收结果
				resultCh := make(chan struct {
					value ByteView
					err   error
				}, 1)

				// 提交任务到协程池
				err := g.goroutinePool.Submit(func() {
					v, e := g.getFromPeer(peer, key)
					resultCh <- struct {
						value ByteView
						err   error
					}{v, e}
				})

				if err == nil {
					// 等待结果
					result := <-resultCh
					peerValue, peerErr = result.value, result.err

					if peerErr == nil {
						g.mainCache.add(key, peerValue, ttl)
						return peerValue, nil
					}
					log.Println("[GeeCache] Failed to get from peer", peerErr)
				}
			}
		}

		return g.getLocallyWithTTL(key, ttl)
	})

	if err == nil {
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
