package main

import (
	"sync"
	"testing"
	"time"
)

// 测试配置
const (
	BenchmarkSize    = 10000 // 测试数据规模
	BenchmarkWorkers = 10    // 并发工作协程数
	ReadRatio        = 0.8   // 读操作比例
	WriteRatio       = 0.1   // 写操作比例
	DeleteRatio      = 0.1   // 删除操作比例
)

// 普通map+锁实现
type NormalMap struct {
	data map[string]string
	mu   sync.RWMutex
}

func NewNormalMap() *NormalMap {
	return &NormalMap{
		data: make(map[string]string),
	}
}

func (m *NormalMap) Get(key string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.data[key]
	return val, ok
}

func (m *NormalMap) Set(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
}

func (m *NormalMap) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
}

// sync.Map实现
type SyncMap struct {
	data sync.Map
}

func NewSyncMap() *SyncMap {
	return &SyncMap{}
}

func (m *SyncMap) Get(key string) (string, bool) {
	val, ok := m.data.Load(key)
	if !ok {
		return "", false
	}
	return val.(string), true
}

func (m *SyncMap) Set(key, value string) {
	m.data.Store(key, value)
}

func (m *SyncMap) Delete(key string) {
	m.data.Delete(key)
}

// 基准测试：读多写少场景
func BenchmarkMapReadHeavy(b *testing.B) {
	// 初始化数据
	normalMap := NewNormalMap()
	syncMap := NewSyncMap()

	// 预填充数据
	for i := 0; i < BenchmarkSize; i++ {
		key := "key" + string(rune(i))
		value := "value" + string(rune(i))
		normalMap.Set(key, value)
		syncMap.Set(key, value)
	}

	b.Run("NormalMap", func(b *testing.B) {
		benchmarkMap(b, normalMap)
	})

	b.Run("SyncMap", func(b *testing.B) {
		benchmarkMap(b, syncMap)
	})
}

// 基准测试：读写均衡场景
func BenchmarkMapBalanced(b *testing.B) {
	// 初始化数据
	normalMap := NewNormalMap()
	syncMap := NewSyncMap()

	// 预填充数据
	for i := 0; i < BenchmarkSize; i++ {
		key := "key" + string(rune(i))
		value := "value" + string(rune(i))
		normalMap.Set(key, value)
		syncMap.Set(key, value)
	}

	b.Run("NormalMap", func(b *testing.B) {
		benchmarkMapBalanced(b, normalMap)
	})

	b.Run("SyncMap", func(b *testing.B) {
		benchmarkMapBalanced(b, syncMap)
	})
}

// 通用Map接口
type MapInterface interface {
	Get(key string) (string, bool)
	Set(key, value string)
	Delete(key string)
}

// 读多写少场景测试
func benchmarkMap(b *testing.B, m MapInterface) {
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := "key" + string(rune(i%BenchmarkSize))
			value := "value" + string(rune(i%BenchmarkSize))

			r := int(time.Now().UnixNano() % 100)
			if r < int(ReadRatio*100) {
				// 读操作
				m.Get(key)
			} else if r < int((ReadRatio+WriteRatio)*100) {
				// 写操作
				m.Set(key, value)
			} else {
				// 删除操作
				m.Delete(key)
			}
			i++
		}
	})
}

// 读写均衡场景测试
func benchmarkMapBalanced(b *testing.B, m MapInterface) {
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := "key" + string(rune(i%BenchmarkSize))
			value := "value" + string(rune(i%BenchmarkSize))

			r := int(time.Now().UnixNano() % 2)
			if r == 0 {
				// 读操作
				m.Get(key)
			} else {
				// 写操作
				m.Set(key, value)
			}
			i++
		}
	})
}

// 基准测试：缓存场景模拟
func BenchmarkCacheScenario(b *testing.B) {
	// 初始化数据
	normalMap := NewNormalMap()
	syncMap := NewSyncMap()

	// 预填充数据
	for i := 0; i < BenchmarkSize; i++ {
		key := "cache_key" + string(rune(i))
		value := "cache_value" + string(rune(i))
		normalMap.Set(key, value)
		syncMap.Set(key, value)
	}

	b.Run("NormalMap", func(b *testing.B) {
		benchmarkCacheScenario(b, normalMap)
	})

	b.Run("SyncMap", func(b *testing.B) {
		benchmarkCacheScenario(b, syncMap)
	})
}

// 缓存场景模拟测试
func benchmarkCacheScenario(b *testing.B, m MapInterface) {
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// 90% 的概率访问已存在的键（缓存命中）
			// 10% 的概率访问新键（缓存未命中）
			var key string
			if int(time.Now().UnixNano()%10) < 9 {
				// 缓存命中
				key = "cache_key" + string(rune(i%BenchmarkSize))
			} else {
				// 缓存未命中
				key = "cache_key_new" + string(rune(i))
			}

			// 先尝试读取
			_, ok := m.Get(key)
			if !ok {
				// 缓存未命中，写入数据
				m.Set(key, "cache_value"+string(rune(i)))
			}
			i++
		}
	})
}

func runMapBenchmarks() {
	// 运行基准测试
	t := &testing.B{}

	println("=== 读多写少场景 ===")
	BenchmarkMapReadHeavy(t)

	println("\n=== 读写均衡场景 ===")
	BenchmarkMapBalanced(t)

	println("\n=== 缓存场景模拟 ===")
	BenchmarkCacheScenario(t)

	println("\n基准测试完成，请使用 'go test -bench=.' 命令运行完整测试")
}
