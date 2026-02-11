package pool

import (
	"sync"
)

// ObjectPool 对象池，参考 kitex 和 hertz 的实现
type ObjectPool struct {
	pool sync.Pool
}

// NewObjectPool 创建一个新的对象池
// newFunc: 创建新对象的函数
func NewObjectPool(newFunc func() interface{}) *ObjectPool {
	return &ObjectPool{
		pool: sync.Pool{
			New: newFunc,
		},
	}
}

// Get 从对象池获取一个对象
func (p *ObjectPool) Get() interface{} {
	return p.pool.Get()
}

// Put 将对象放回对象池
func (p *ObjectPool) Put(obj interface{}) {
	if obj != nil {
		p.pool.Put(obj)
	}
}

// EntryPool 专门用于缓存条目的对象池
type EntryPool struct {
	pool sync.Pool
}

// NewEntryPool 创建一个新的条目对象池
func NewEntryPool() *EntryPool {
	return &EntryPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &Entry{}
			},
		},
	}
}

// Entry 表示缓存中的一个条目
type Entry struct {
	Key       string
	Value     interface{}
	ExpiresAt int64
}

// Get 从对象池获取一个条目
func (p *EntryPool) Get() *Entry {
	return p.pool.Get().(*Entry)
}

// Put 将条目放回对象池
func (p *EntryPool) Put(entry *Entry) {
	if entry != nil {
		// 重置条目，避免引用外部资源
		entry.Key = ""
		entry.Value = nil
		entry.ExpiresAt = 0
		p.pool.Put(entry)
	}
}

// HeapItemPool 专门用于堆项的对象池
type HeapItemPool struct {
	pool sync.Pool
}

// NewHeapItemPool 创建一个新的堆项对象池
func NewHeapItemPool() *HeapItemPool {
	return &HeapItemPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &HeapItem{}
			},
		},
	}
}

// HeapItem 表示堆中的一个项
type HeapItem struct {
	Key       string
	ExpiresAt int64
}

// Get 从对象池获取一个堆项
func (p *HeapItemPool) Get() *HeapItem {
	return p.pool.Get().(*HeapItem)
}

// Put 将堆项放回对象池
func (p *HeapItemPool) Put(item *HeapItem) {
	if item != nil {
		// 重置堆项
		item.Key = ""
		item.ExpiresAt = 0
		p.pool.Put(item)
	}
}
