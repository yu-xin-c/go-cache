package main

import (
	"log"
	"time"
)

func testPool() {
	// 创建一个缓存组
	gee := mygocache.NewGroupWithTTL("test", 2<<10, mygocache.GetterFunc(
		func(key string) ([]byte, error) {
			log.Println("[SlowDB] search key", key)
			return []byte("value"), nil
		}), 5)

	// 测试协程池
	log.Println("Testing goroutine pool...")
	for i := 0; i < 100; i++ {
		key := "key" + string(rune(i))
		go func(k string) {
			gee.Get(k)
		}(key)
	}

	// 测试对象池
	log.Println("Testing object pool...")
	for i := 0; i < 1000; i++ {
		key := "key" + string(rune(i))
		gee.Set(key, []byte("value"), 1)
	}

	// 等待一段时间，让协程池和对象池有时间处理
	time.Sleep(2 * time.Second)

	log.Println("Pool test completed!")
}

func main() {
	testPool()
}
