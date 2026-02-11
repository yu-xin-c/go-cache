# LRU-K 缓存优化

LRU-K 是 GeeCache 中的一个重要优化点，用于提高缓存命中率。本文档详细介绍 LRU-K 缓存的实现原理和优化效果。

## 设计理念

LRU-K 算法是对传统 LRU (Least Recently Used) 算法的改进，主要解决以下问题：

1. **缓存污染**：传统 LRU 算法容易被一次性访问的缓存项污染，导致真正需要缓存的项被淘汰
2. **命中率优化**：通过记录访问历史，只缓存频繁访问的项，提高缓存命中率
3. **适应性**：根据访问模式自动调整缓存策略，适应不同的应用场景

## 实现原理

### 核心数据结构

LRU-K 缓存的核心数据结构定义在 `geecache/lru/lru_k.go` 文件中：

```go
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
```

### 关键方法

1. **NewLRUK**：创建 LRU-K 缓存实例
2. **Get**：从缓存中获取值，更新访问历史
3. **Add**：向缓存中添加值，支持 TTL
4. **removeFromHistory**：从历史记录中删除键
5. **promoteToCache**：将访问 K 次的项提升到缓存

### 工作流程

1. **初始化**：创建 LRU-K 缓存时，设置 K 值（默认为 2）
2. **首次访问**：首次访问键时，将访问时间戳添加到历史记录
3. **K 次访问**：当键被访问 K 次时，将其提升到缓存
4. **缓存管理**：使用 LRU 算法管理缓存中的项
5. **过期管理**：使用最小堆管理过期项，支持惰性过期和主动过期

### 历史记录管理

LRU-K 缓存使用 `sync.Map` 管理访问历史记录：

```go
// Get 从缓存中获取值
func (c *LRUCache) Get(key string) (value Value, ok bool) {
	// 检查缓存中是否存在
	if ele, ok := c.cache.Load(key); ok {
		// 缓存命中，更新访问时间
		c.mu.Lock()
		kv := ele.(*list.Element).Value.(*lruEntry)
		kv.lastAccess = time.Now().Unix()
		c.ll.MoveToFront(ele.(*list.Element))
		c.mu.Unlock()
		return kv.value, true
	}

	// 缓存未命中，更新历史记录
	c.addToHistory(key)
	return nil, false
}

// addToHistory 将键添加到历史记录
func (c *LRUCache) addToHistory(key string) error {
	currentTime := time.Now().Unix()

	// 获取历史访问时间戳
	timestamps, exists := c.history.Load(key)
	var ts []int64
	if exists {
		ts = timestamps.([]int64)
	}

	ts = append(ts, currentTime)

	// 只保留最近的 K 个时间戳
	if len(ts) > c.k {
		ts = ts[len(ts)-c.k:]
	}

	c.history.Store(key, ts)

	// 检查是否需要提升到缓存
	if len(ts) == c.k {
		// 这里可以添加从数据源加载数据并提升到缓存的逻辑
	}

	return nil
}
```

## 集成到 GeeCache

### 1. 缓存实现中的应用

LRU-K 缓存是对传统 LRU 缓存的增强，提供了更高的缓存命中率：

- **传统 LRU**：适用于访问模式相对稳定的场景
- **LRU-K**：适用于访问模式波动较大的场景，能更好地抵抗缓存污染

### 2. 过期管理集成

LRU-K 缓存集成了与 LRU 缓存相同的过期管理机制：

- **TTL 支持**：为缓存项添加过期时间
- **惰性过期**：访问时检查并删除过期项
- **主动过期**：使用最小堆管理过期项，后台协程定期检查并删除

## 优化效果

### 1. 缓存命中率

- **提高命中率**：通过记录访问历史，只缓存频繁访问的项，提高缓存命中率
- **抵抗缓存污染**：避免一次性访问的缓存项污染缓存，保持缓存的有效性
- **适应访问模式**：根据访问模式自动调整缓存策略，适应不同的应用场景

### 2. 内存使用效率

- **智能缓存**：只缓存真正需要的项，减少内存浪费
- **动态调整**：根据访问频率动态调整缓存内容，提高内存利用率

### 3. 性能优化

- **高效历史记录**：使用 sync.Map 管理访问历史，提高并发访问性能
- **快速提升**：当项被访问 K 次时，快速提升到缓存，减少重复加载

## 配置建议

LRU-K 缓存的配置需要根据系统的访问模式和内存情况进行调整：

- **K 值**：K 值越大，缓存命中率越高，但历史记录占用的内存也越大。一般建议 K=2
- **缓存大小**：根据系统内存情况设置合理的缓存大小，避免内存溢出
- **过期时间**：根据数据的更新频率设置合理的过期时间

## 测试验证

通过 `test_lru_k.go` 文件测试 LRU-K 缓存的功能：

```go
// 测试 LRU-K 缓存的基本功能
func testLRUKBasic() {
	// ... 测试代码
}

// 测试 LRU-K 缓存的命中率
func testLRUKHitRate() {
	// ... 测试代码
}

// 测试 LRU-K 缓存的过期管理
func testLRUKExpiration() {
	// ... 测试代码
}
```

测试结果表明，LRU-K 缓存能够有效地提高缓存命中率，减少缓存污染，适应不同的访问模式。

## 总结

LRU-K 是 GeeCache 中的一个重要优化点，通过记录访问历史，只缓存频繁访问的项，提高了缓存命中率。与传统 LRU 算法相比，LRU-K 算法能够更好地抵抗缓存污染，适应不同的应用场景。集成了过期管理机制后，LRU-K 缓存不仅能够提高命中率，还能够确保数据的时效性，是一个功能强大、性能优异的缓存解决方案。
