# TTL 和过期淘汰策略

TTL (Time-To-Live) 是 GeeCache 中的一个重要功能，用于为缓存项设置过期时间。本文档详细介绍 TTL 的实现原理和过期淘汰策略。

## 设计理念

TTL 和过期淘汰策略的设计主要解决以下问题：

1. **缓存数据时效性**：确保缓存中的数据不会过期，保持数据的新鲜度
2. **内存使用效率**：及时删除过期的缓存项，释放内存空间
3. **性能优化**：避免过期检查对系统性能的影响

## 实现原理

### 核心数据结构

#### 1. 缓存条目结构

在 `geecache/lru/lru.go` 文件中，`entry` 结构体添加了 `expiresAt` 字段：

```go
// entry 表示缓存中的一个条目
type entry struct {
	key       string // 键
	value     Value  // 值
	expiresAt int64  // 过期时间戳，0 表示永不过期
}
```

#### 2. 最小堆结构

为了实现主动过期策略，使用最小堆管理过期项：

```go
// Cache 是一个带过期时间支持的 LRU 缓存
type Cache struct {
	// ... 其他字段
	// 优先级队列（最小堆），用于过期管理
	heap    []*pool.HeapItem // 最小堆数组
	heapMap map[string]int   // 键到堆索引的映射
	// ... 其他字段
}
```

### 关键方法

1. **Add**：向缓存中添加值，支持 TTL
2. **Get**：从缓存中获取值，检查过期
3. **checkExpiration**：检查并删除过期的项
4. **expirationLoop**：后台协程定期检查过期项

### 过期淘汰策略

GeeCache 实现了两种过期淘汰策略：

#### 1. 惰性过期

在 `Get` 方法中检查缓存项是否过期，过期则删除：

```go
// Get 从缓存中获取值
func (c *Cache) Get(key string) (value Value, ok bool) {
	if ele, hit := c.cache[key]; hit {
		// 检查是否过期
		kv := ele.Value.(*entry)
		if kv.expiresAt > 0 && kv.expiresAt < time.Now().Unix() {
			// 已过期，删除
			c.removeEntry(ele)
			return nil, false
		}
		// 未过期，更新访问时间并返回
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		return kv.value, true
	}
	return nil, false
}
```

#### 2. 主动过期

使用最小堆管理过期项，后台协程定期检查并删除：

```go
// expirationLoop 处理基于堆的主动过期
func (c *Cache) expirationLoop() {
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
func (c *Cache) checkExpiration() {
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
		if ele, ok := c.cache[key]; ok {
			c.removeEntry(ele)
		}

		c.heapMu.Lock()
	}
	c.heapMu.Unlock()
}
```

### 最小堆操作

为了实现主动过期策略，需要对最小堆进行以下操作：

1. **添加元素**：`addToHeap` 方法，将键添加到过期堆中
2. **删除元素**：`removeFromHeap` 方法，从过期堆中删除键
3. **堆化**：`heapifyUp` 和 `heapifyDown` 方法，维护堆属性

## 集成到 GeeCache

### 1. 缓存组中的应用

在 `geecache/geecache.go` 文件中，`Group` 结构体添加了 `defaultTTL` 字段：

```go
// Group 是 GeeCache 的核心数据结构
type Group struct {
	// ... 其他字段
	defaultTTL int64 // 默认 TTL（秒）
}
```

### 2. 支持 TTL 的方法

添加了以下支持 TTL 的方法：

1. **NewGroupWithTTL**：创建带默认 TTL 的缓存组
2. **GetWithTTL**：获取缓存，支持 TTL
3. **Set**：设置缓存，支持 TTL

```go
// NewGroupWithTTL 创建一个带默认 TTL 的缓存组
func NewGroupWithTTL(name string, cacheBytes int64, getter Getter, defaultTTL int64) *Group {
	// ... 实现代码
}

// GetWithTTL 从缓存中获取值，支持 TTL
func (g *Group) GetWithTTL(key string, ttl int64) (ByteView, error) {
	// ... 实现代码
}

// Set 向缓存中设置值，支持 TTL
func (g *Group) Set(key string, value []byte, ttl int64) {
	// ... 实现代码
}
```

## 优化效果

### 1. 数据时效性

- **确保数据新鲜度**：过期的缓存项会被及时删除，避免使用过期数据
- **灵活的过期时间设置**：支持为不同的缓存项设置不同的过期时间

### 2. 内存使用效率

- **及时释放内存**：过期的缓存项会被及时删除，释放内存空间
- **优化内存使用**：最小堆结构高效管理过期项，减少内存开销

### 3. 性能优化

- **惰性过期**：只在访问时检查过期，避免对所有缓存项的遍历
- **主动过期**：后台协程定期检查过期项，避免过期项累积
- **最小堆**：使用最小堆管理过期项，快速获取即将过期的项

## 配置建议

TTL 和过期淘汰策略的配置需要根据系统的负载情况和数据特性进行调整：

- **默认 TTL**：根据数据的更新频率设置合理的默认 TTL
- **过期检查间隔**：默认每 100ms 检查一次，可根据系统负载调整
- **最小堆大小**：根据缓存项数量调整，确保堆操作的性能

## 测试验证

通过 `test_ttl.go` 文件测试 TTL 和过期淘汰策略的功能：

```go
// 测试基本 TTL 功能
func testBasicTTL() {
	// ... 测试代码
}

// 测试默认 TTL 功能
func testDefaultTTL() {
	// ... 测试代码
}

// 测试永不过期功能
func testNeverExpire() {
	// ... 测试代码
}

// 测试主动过期功能
func testActiveExpiration() {
	// ... 测试代码
}
```

测试结果表明，TTL 和过期淘汰策略能够有效地管理缓存项的过期，确保缓存数据的新鲜度和内存的有效使用。

## 总结

TTL 和过期淘汰策略是 GeeCache 中的重要功能，通过惰性过期和主动过期相结合的方式，确保了缓存数据的时效性和内存的有效使用。最小堆的使用使得主动过期策略更加高效，后台协程定期检查过期项，避免了过期检查对系统性能的影响。这些设计使得 GeeCache 在处理缓存数据时更加灵活和高效。
