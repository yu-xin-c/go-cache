package main

import (
	"log"
	"testing"
	"time"

	"mygocache"
)

var benchmarkDB = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

func createBenchmarkGroup() *mygocache.Group {
	return mygocache.NewGroupWithTTL("scores", 2<<10, mygocache.GetterFunc(
		func(key string) ([]byte, error) {
			log.Println("[SlowDB] search key", key)
			if v, ok := benchmarkDB[key]; ok {
				return []byte(v), nil
			}
			return nil, nil
		}), 5)
}

// BenchmarkCacheGet 测试缓存读取性能
func BenchmarkCacheGet(b *testing.B) {
	gee := createBenchmarkGroup()

	// 预热缓存
	for k := range benchmarkDB {
		gee.Get(k)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for k := range benchmarkDB {
			gee.Get(k)
		}
	}
}

// BenchmarkCacheSet 测试缓存写入性能
func BenchmarkCacheSet(b *testing.B) {
	gee := createBenchmarkGroup()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := "key" + string(rune(i))
		value := []byte("value" + string(rune(i)))
		gee.Set(key, value, 10)
	}
}

// BenchmarkConcurrentGet 测试并发读取性能
func BenchmarkConcurrentGet(b *testing.B) {
	gee := createBenchmarkGroup()

	// 预热缓存
	for k := range benchmarkDB {
		gee.Get(k)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			for k := range benchmarkDB {
				gee.Get(k)
			}
		}
	})
}

// BenchmarkTTLExpiration 测试 TTL 过期性能
func BenchmarkTTLExpiration(b *testing.B) {
	gee := createBenchmarkGroup()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := "key" + string(rune(i))
		value := []byte("value" + string(rune(i)))
		gee.Set(key, value, 1)
	}

	// 等待过期
	time.Sleep(2 * time.Second)

	// 检查过期
	for i := 0; i < b.N; i++ {
		key := "key" + string(rune(i))
		gee.Get(key)
	}
}
