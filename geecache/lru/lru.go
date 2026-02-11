package lru

import (
	"container/list"
	"time"
)

// Cache is a LRU cache. It is not safe for concurrent access.
type Cache struct {
	maxBytes int64
	nbytes   int64
	ll       *list.List
	cache    map[string]*list.Element
	// optional and executed when an entry is purged.
	OnEvicted func(key string, value Value)
}

type entry struct {
	key       string
	value     Value
	expiresAt int64 // 过期时间戳，0 表示永不过期
}

// Value use Len to count how many bytes it takes
type Value interface {
	Len() int
}

// New is the Constructor of Cache
func New(maxBytes int64, onEvicted func(string, Value)) *Cache {
	return &Cache{
		maxBytes:  maxBytes,
		ll:        list.New(),
		cache:     make(map[string]*list.Element),
		OnEvicted: onEvicted,
	}
}

// Add adds a value to the cache with optional expiration time.
// ttl is the time to live in seconds, 0 means never expire.
func (c *Cache) Add(key string, value Value, ttl int64) {
	var expiresAt int64
	if ttl > 0 {
		expiresAt = time.Now().Unix() + ttl
	}

	if ele, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		c.nbytes += int64(value.Len()) - int64(kv.value.Len())
		kv.value = value
		kv.expiresAt = expiresAt
	} else {
		ele := c.ll.PushFront(&entry{key, value, expiresAt})
		c.cache[key] = ele
		c.nbytes += int64(len(key)) + int64(value.Len())
	}
	// 清理过期项
	c.removeExpired()
	// 清理超出容量的项
	for c.maxBytes != 0 && c.maxBytes < c.nbytes {
		c.RemoveOldest()
	}
}

// Get look ups a key's value
func (c *Cache) Get(key string) (value Value, ok bool) {
	if ele, ok := c.cache[key]; ok {
		kv := ele.Value.(*entry)
		// 检查是否过期
		if kv.expiresAt > 0 && kv.expiresAt < time.Now().Unix() {
			// 过期，删除该项
			c.ll.Remove(ele)
			delete(c.cache, key)
			c.nbytes -= int64(len(kv.key)) + int64(kv.value.Len())
			if c.OnEvicted != nil {
				c.OnEvicted(kv.key, kv.value)
			}
			return nil, false
		}
		// 未过期，移到队首
		c.ll.MoveToFront(ele)
		return kv.value, true
	}
	return
}

// RemoveOldest removes the oldest item
func (c *Cache) RemoveOldest() {
	ele := c.ll.Back()
	if ele != nil {
		c.ll.Remove(ele)
		kv := ele.Value.(*entry)
		delete(c.cache, kv.key)
		c.nbytes -= int64(len(kv.key)) + int64(kv.value.Len())
		if c.OnEvicted != nil {
			c.OnEvicted(kv.key, kv.value)
		}
	}
}

// removeExpired removes all expired entries
func (c *Cache) removeExpired() {
	now := time.Now().Unix()
	for k, ele := range c.cache {
		kv := ele.Value.(*entry)
		if kv.expiresAt > 0 && kv.expiresAt < now {
			c.ll.Remove(ele)
			delete(c.cache, k)
			c.nbytes -= int64(len(kv.key)) + int64(kv.value.Len())
			if c.OnEvicted != nil {
				c.OnEvicted(kv.key, kv.value)
			}
		}
	}
}

// Len the number of cache entries
func (c *Cache) Len() int {
	// 先清理过期项
	c.removeExpired()
	return c.ll.Len()
}
