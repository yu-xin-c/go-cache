package lru

import (
	"container/list"
	"geecache/pool"
	"sync"
	"time"
)

// LRUCache 是一个带过期时间支持的 LRU-K 缓存。
type LRUCache struct {
	maxBytes int64      // 缓存的最大字节数
	nbytes   int64      // 当前缓存的字节数
	ll       *list.List // 双向链表，用于实现 LRU
	cache    sync.Map   // 键到链表元素的映射 (string -> *list.Element)

	// LRU-K 相关
	k       int      // K 值，表示需要访问 K 次才进入缓存
	history sync.Map // 键到访问时间戳列表的映射 (string -> []int64)

	// 优先级队列（最小堆），用于过期管理
	heap    []*pool.HeapItem // 最小堆数组
	heapMap map[string]int   // 键到堆索引的映射

	// 当条目被删除时执行的回调函数
	OnEvicted func(key string, value Value)

	// 过期协程的停止信号
	stopChan chan struct{}

	// 堆操作的互斥锁
	heapMu sync.Mutex

	// 缓存操作的互斥锁（主要用于nbytes和链表操作）
	mu sync.Mutex
}

// entry 表示缓存中的一个条目
type lruEntry struct {
	key        string // 键
	value      Value  // 值
	expiresAt  int64  // 过期时间戳，0 表示永不过期
	lastAccess int64  // 最后访问时间戳
}

// NewLRUK 创建一个新的 LRU-K 缓存实例
// maxBytes 是缓存的最大字节数
// k 是 LRU-K 的 K 值
// onEvicted 是当条目被删除时执行的回调函数
func NewLRUK(maxBytes int64, k int, onEvicted func(string, Value)) *LRUCache {
	if k <= 0 {
		k = 2 // 默认 K=2
	}

	c := &LRUCache{
		maxBytes:  maxBytes,
		ll:        list.New(),
		cache:     sync.Map{},
		k:         k,
		history:   sync.Map{},
		heap:      make([]*pool.HeapItem, 0),
		heapMap:   make(map[string]int),
		OnEvicted: onEvicted,
		stopChan:  make(chan struct{}),
	}

	// 启动过期检查协程
	go c.expirationLoop()
	return c
}

// Close 停止过期检查协程
func (c *LRUCache) Close() {
	close(c.stopChan)
}

// Add 向缓存中添加一个值，带有可选的过期时间
// key 是缓存的键
// value 是缓存的值
// ttl 是生存时间（秒），0 表示永不过期
func (c *LRUCache) Add(key string, value Value, ttl int64) {
	var expiresAt int64
	if ttl > 0 {
		expiresAt = time.Now().Unix() + ttl
	}

	// 检查是否已存在
	if ele, ok := c.cache.Load(key); ok {
		c.mu.Lock()
		defer c.mu.Unlock()

		// 更新现有条目
		listEle := ele.(*list.Element)
		c.ll.MoveToFront(listEle)
		kv := listEle.Value.(*lruEntry)
		c.nbytes += int64(value.Len()) - int64(kv.value.Len())
		kv.value = value
		kv.lastAccess = time.Now().Unix()

		// 更新过期时间
		oldExpiresAt := kv.expiresAt
		kv.expiresAt = expiresAt

		// 如果过期时间发生变化，更新堆
		if oldExpiresAt > 0 || expiresAt > 0 {
			c.heapMu.Lock()
			if oldExpiresAt > 0 {
				// 从堆中移除
				c.removeFromHeap(key)
			}
			if expiresAt > 0 {
				// 添加到堆中
				c.addToHeap(key, expiresAt)
			}
			c.heapMu.Unlock()
		}
	} else {
		// 检查访问历史
		timestamps, exists := c.history.Load(key)
		var ts []int64
		if exists {
			ts = timestamps.([]int64)
		}

		// 添加当前访问时间戳
		currentTime := time.Now().Unix()
		ts = append(ts, currentTime)

		// 只保留最近的 K 个时间戳
		if len(ts) > c.k {
			ts = ts[len(ts)-c.k:]
		}

		c.history.Store(key, ts)

		// 检查是否达到 K 次访问
		shouldCache := !exists || len(ts) >= c.k

		if shouldCache {
			c.mu.Lock()
			// 再次检查，避免并发问题
			if _, ok := c.cache.Load(key); ok {
				c.mu.Unlock()
				return
			}

			// 添加新条目到缓存
			ele := c.ll.PushFront(&lruEntry{
				key:        key,
				value:      value,
				expiresAt:  expiresAt,
				lastAccess: currentTime,
			})
			c.cache.Store(key, ele)
			c.nbytes += int64(len(key)) + int64(value.Len())

			// 如果有过期时间，添加到堆中
			if expiresAt > 0 {
				c.heapMu.Lock()
				c.addToHeap(key, expiresAt)
				c.heapMu.Unlock()
			}

			// 清理超出容量的项
			for c.maxBytes != 0 && c.maxBytes < c.nbytes {
				c.RemoveOldest()
			}
			c.mu.Unlock()
		}
	}
}

// Get 查找并返回缓存中键对应的值（惰性过期）
// key 是要查找的键
// 返回值和是否找到的标志
func (c *LRUCache) Get(key string) (value Value, ok bool) {
	// 检查缓存
	if ele, ok := c.cache.Load(key); ok {
		c.mu.Lock()
		defer c.mu.Unlock()

		listEle := ele.(*list.Element)
		kv := listEle.Value.(*lruEntry)
		// 检查是否过期（惰性过期）
		if kv.expiresAt > 0 && kv.expiresAt < time.Now().Unix() {
			// 过期，删除该项
			c.removeEntry(listEle)
			return nil, false
		}

		// 未过期，移到队首并更新访问时间
		c.ll.MoveToFront(listEle)
		kv.lastAccess = time.Now().Unix()
		return kv.value, true
	}

	// 缓存未命中，记录访问历史
	timestamps, exists := c.history.Load(key)
	var ts []int64
	if exists {
		ts = timestamps.([]int64)
	}

	currentTime := time.Now().Unix()
	ts = append(ts, currentTime)

	// 只保留最近的 K 个时间戳
	if len(ts) > c.k {
		ts = ts[len(ts)-c.k:]
	}

	c.history.Store(key, ts)

	return nil, false
}

// RemoveOldest 删除最旧的条目
func (c *LRUCache) RemoveOldest() {
	c.mu.Lock()
	defer c.mu.Unlock()

	ele := c.ll.Back()
	if ele != nil {
		c.removeEntry(ele)
	}
}

// removeEntry 从缓存和堆中删除一个条目
// ele 是要删除的链表元素
func (c *LRUCache) removeEntry(ele *list.Element) {
	kv := ele.Value.(*lruEntry)
	key := kv.key

	// 从链表中删除
	c.ll.Remove(ele)
	c.cache.Delete(key)
	c.nbytes -= int64(len(key)) + int64(kv.value.Len())

	// 如果有过期时间，从堆中删除
	if kv.expiresAt > 0 {
		c.heapMu.Lock()
		c.removeFromHeap(key)
		c.heapMu.Unlock()
	}

	// 从历史记录中删除
	c.history.Delete(key)

	// 调用删除回调
	if c.OnEvicted != nil {
		c.OnEvicted(key, kv.value)
	}
}

// addToHeap 将键添加到过期堆中
// key 是要添加的键
// expiresAt 是过期时间戳
func (c *LRUCache) addToHeap(key string, expiresAt int64) {
	item := &pool.HeapItem{Key: key, ExpiresAt: expiresAt}
	c.heap = append(c.heap, item)
	index := len(c.heap) - 1
	c.heapMap[key] = index
	c.heapifyUp(index)
}

// removeFromHeap 从过期堆中删除键
// key 是要删除的键
func (c *LRUCache) removeFromHeap(key string) {
	if index, ok := c.heapMap[key]; ok {
		lastIndex := len(c.heap) - 1
		// 与最后一个元素交换
		c.heap[index] = c.heap[lastIndex]
		c.heapMap[c.heap[index].Key] = index
		// 删除最后一个元素
		c.heap = c.heap[:lastIndex]
		delete(c.heapMap, key)

		// 堆化
		if index < len(c.heap) {
			c.heapifyDown(index)
			c.heapifyUp(index)
		}
	}
}

// heapifyUp 将元素向上移动以维护堆属性
// index 是要移动的元素的索引
func (c *LRUCache) heapifyUp(index int) {
	for index > 0 {
		parent := (index - 1) / 2
		if c.heap[index].ExpiresAt >= c.heap[parent].ExpiresAt {
			break
		}
		// 与父元素交换
		c.heap[index], c.heap[parent] = c.heap[parent], c.heap[index]
		c.heapMap[c.heap[index].Key] = index
		c.heapMap[c.heap[parent].Key] = parent
		index = parent
	}
}

// heapifyDown 将元素向下移动以维护堆属性
// index 是要移动的元素的索引
func (c *LRUCache) heapifyDown(index int) {
	for {
		left := 2*index + 1
		right := 2*index + 2
		smallest := index

		if left < len(c.heap) && c.heap[left].ExpiresAt < c.heap[smallest].ExpiresAt {
			smallest = left
		}
		if right < len(c.heap) && c.heap[right].ExpiresAt < c.heap[smallest].ExpiresAt {
			smallest = right
		}

		if smallest == index {
			break
		}

		// 与最小的子元素交换
		c.heap[index], c.heap[smallest] = c.heap[smallest], c.heap[index]
		c.heapMap[c.heap[index].Key] = index
		c.heapMap[c.heap[smallest].Key] = smallest
		index = smallest
	}
}

// expirationLoop 处理基于堆的主动过期
func (c *LRUCache) expirationLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.checkExpiration()
		case <-c.stopChan:
			return
		}
	}
}

// checkExpiration 检查并删除过期的项
func (c *LRUCache) checkExpiration() {
	now := time.Now().Unix()

	c.heapMu.Lock()
	// 处理堆中的过期项
	for len(c.heap) > 0 && c.heap[0].ExpiresAt < now {
		item := c.heap[0]
		key := item.Key

		// 从堆中移除
		c.removeFromHeap(key)
		c.heapMu.Unlock()

		// 如果缓存中还存在，删除它
		c.mu.Lock()
		if ele, ok := c.cache.Load(key); ok {
			c.removeEntry(ele.(*list.Element))
		}
		c.mu.Unlock()

		c.heapMu.Lock()
	}
	c.heapMu.Unlock()
}

// Len 返回缓存中的条目数
func (c *LRUCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}
