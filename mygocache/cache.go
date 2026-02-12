package mygocache

import (
	"mygocache/lru"
	"sync"
)

// CacheStrategy 定义缓存策略类型
type CacheStrategy int

const (
	// StrategyLRU 使用标准 LRU 缓存策略
	StrategyLRU CacheStrategy = iota
	// StrategyLRUK 使用 LRU-K 缓存策略
	StrategyLRUK
)

type cache struct {
	mu         sync.Mutex
	lru        *lru.Cache
	lruK       *lru.LRUCache
	cacheBytes int64
	strategy   CacheStrategy
	k          int // LRU-K 的 K 值
}

// NewCache 创建一个新的缓存实例
// cacheBytes 是缓存的最大字节数
// strategy 是缓存策略
// k 是 LRU-K 的 K 值（仅当 strategy 为 StrategyLRUK 时有效）
func NewCache(cacheBytes int64, strategy CacheStrategy, k int) *cache {
	return &cache{
		cacheBytes: cacheBytes,
		strategy:   strategy,
		k:          k,
	}
}

// 默认缓存创建函数（保持向后兼容）
func defaultCache(cacheBytes int64) *cache {
	return NewCache(cacheBytes, StrategyLRU, 2)
}

func (c *cache) add(key string, value ByteView, ttl int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.strategy {
	case StrategyLRUK:
		if c.lruK == nil {
			c.lruK = lru.NewLRUK(c.cacheBytes, c.k, nil)
		}
		c.lruK.Add(key, value, ttl)
	default: // StrategyLRU
		if c.lru == nil {
			c.lru = lru.New(c.cacheBytes, nil)
		}
		c.lru.Add(key, value, ttl)
	}
}

func (c *cache) get(key string) (value ByteView, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.strategy {
	case StrategyLRUK:
		if c.lruK == nil {
			return
		}
		if v, ok := c.lruK.Get(key); ok {
			return v.(ByteView), ok
		}
	default: // StrategyLRU
		if c.lru == nil {
			return
		}
		if v, ok := c.lru.Get(key); ok {
			return v.(ByteView), ok
		}
	}

	return
}

func (c *cache) delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.strategy {
	case StrategyLRUK:
		if c.lruK != nil {
			c.lruK.Remove(key)
		}
	default: // StrategyLRU
		if c.lru != nil {
			c.lru.Remove(key)
		}
	}
}

func (c *cache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.strategy {
	case StrategyLRUK:
		if c.lruK != nil {
			c.lruK.Clear()
		}
	default: // StrategyLRU
		if c.lru != nil {
			c.lru.Clear()
		}
	}
}

func (c *cache) stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()

	var itemCount, hitCount, missCount int

	switch c.strategy {
	case StrategyLRUK:
		if c.lruK != nil {
			itemCount = c.lruK.Len()
			// TODO: 增加 LRU-K 的命中与未命中统计
		}
	default: // StrategyLRU
		if c.lru != nil {
			itemCount = c.lru.Len()
			// TODO: 增加 LRU 的命中与未命中统计
		}
	}

	return Stats{
		ItemCount:  itemCount,
		HitCount:   hitCount,
		MissCount:  missCount,
		TotalCount: hitCount + missCount,
	}
}
