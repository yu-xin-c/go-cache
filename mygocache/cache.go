package mygocache

import (
	"hash/fnv"
	"mygocache/lru"
	"sync/atomic"
)

// CacheStrategy 定义缓存策略类型
type CacheStrategy int

const (
	// StrategyLRU 使用标准 LRU 缓存策略
	StrategyLRU CacheStrategy = iota
	// StrategyLRUK 使用 LRU-K 缓存策略
	StrategyLRUK
)

// 默认分片数，必须是 2 的幂
const defaultShardCount = 32

// cacheShard 是缓存的一个分片，拥有独立的 LRU 实例
type cacheShard struct {
	lru        *lru.Cache
	lruK       *lru.LRUCache
	cacheBytes int64
	strategy   CacheStrategy
	k          int
}

// cache 是分片缓存，将 key 哈希到不同的 shard 以降低锁竞争
type cache struct {
	shards    []cacheShard
	shardMask uint32 // shardCount - 1，用于位运算取模

	// 全局统计计数器（独立于分片，避免 recordMiss/recordHit 只操作单一分片的问题）
	hitCount  int64
	missCount int64
}

// fnvHash 计算 key 的 FNV-1a 哈希值
func fnvHash(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	return h.Sum32()
}

// getShard 根据 key 返回对应的分片
func (c *cache) getShard(key string) *cacheShard {
	return &c.shards[fnvHash(key)&c.shardMask]
}

// NewCache 创建一个新的分片缓存实例
func NewCache(cacheBytes int64, strategy CacheStrategy, k int) *cache {
	shardCount := defaultShardCount
	// 每个 shard 分配 cacheBytes/shardCount 的容量
	perShard := cacheBytes / int64(shardCount)
	if perShard < 1 {
		perShard = 1
	}

	c := &cache{
		shards:    make([]cacheShard, shardCount),
		shardMask: uint32(shardCount - 1),
	}

	for i := 0; i < shardCount; i++ {
		s := &c.shards[i]
		s.cacheBytes = perShard
		s.strategy = strategy
		s.k = k
		switch strategy {
		case StrategyLRUK:
			s.lruK = lru.NewLRUK(perShard, k, nil)
		default:
			s.lru = lru.New(perShard, nil)
		}
	}

	return c
}

// 默认缓存创建函数（保持向后兼容）
func defaultCache(cacheBytes int64) *cache {
	return NewCache(cacheBytes, StrategyLRU, 2)
}

func (c *cache) add(key string, value ByteView, ttl int64) {
	s := c.getShard(key)

	switch s.strategy {
	case StrategyLRUK:
		s.lruK.Add(key, value, ttl)
	default:
		s.lru.Add(key, value, ttl)
	}
}

func (c *cache) get(key string) (value ByteView, ok bool) {
	s := c.getShard(key)

	switch s.strategy {
	case StrategyLRUK:
		if s.lruK == nil {
			return
		}
		if v, ok := s.lruK.Get(key); ok {
			return v.(ByteView), ok
		}
	default:
		if s.lru == nil {
			return
		}
		if v, ok := s.lru.Get(key); ok {
			return v.(ByteView), ok
		}
	}

	return
}

func (c *cache) delete(key string) {
	s := c.getShard(key)

	switch s.strategy {
	case StrategyLRUK:
		if s.lruK != nil {
			s.lruK.Remove(key)
		}
	default:
		if s.lru != nil {
			s.lru.Remove(key)
		}
	}
}

func (c *cache) clear() {
	for i := range c.shards {
		s := &c.shards[i]
		switch s.strategy {
		case StrategyLRUK:
			if s.lruK != nil {
				s.lruK.Clear()
			}
		default:
			if s.lru != nil {
				s.lru.Clear()
			}
		}
	}
}

func (c *cache) stats() Stats {
	var totalItems int

	// 遍历分片获取 item 数量
	for i := range c.shards {
		s := &c.shards[i]
		switch s.strategy {
		case StrategyLRUK:
			if s.lruK != nil {
				totalItems += s.lruK.Len()
			}
		default:
			if s.lru != nil {
				totalItems += s.lru.Len()
			}
		}
	}

	// 使用全局计数器获取 hit/miss 统计
	hits := atomic.LoadInt64(&c.hitCount)
	misses := atomic.LoadInt64(&c.missCount)

	return Stats{
		ItemCount:  totalItems,
		HitCount:   int(hits),
		MissCount:  int(misses),
		TotalCount: int(hits + misses),
	}
}

func (c *cache) recordMiss() {
	atomic.AddInt64(&c.missCount, 1)
}

func (c *cache) recordHit() {
	atomic.AddInt64(&c.hitCount, 1)
}
