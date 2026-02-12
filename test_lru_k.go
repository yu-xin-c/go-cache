package main

import (
	"fmt"
	"log"
	"time"

	"mygocache"
)

// 模拟数据源
func getData(key string) ([]byte, error) {
	log.Printf("Fetching data for key: %s", key)
	return []byte(fmt.Sprintf("value for %s", key)), nil
}

func mainLRUK() {
	// 创建使用 LRU-K 策略的缓存组
	cacheGroup := mygocache.NewGroupWithOptions(
		"test-group",
		1024*1024, // 1MB 缓存
		mygocache.GetterFunc(getData),
		60, // 默认TTL 60秒
		mygocache.StrategyLRUK,
		2, // K=2
	)

	log.Println("Testing LRU-K Cache Strategy")
	log.Println("================================")

	// 第一次访问 key1，应该缓存未命中并记录访问历史
	log.Println("\n1. First access to key1:")
	val, err := cacheGroup.Get("key1")
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		log.Printf("Got value: %s", val.String())
	}

	// 第二次访问 key1，达到 K=2，应该缓存命中
	log.Println("\n2. Second access to key1:")
	val, err = cacheGroup.Get("key1")
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		log.Printf("Got value: %s", val.String())
	}

	// 第一次访问 key2，应该缓存未命中并记录访问历史
	log.Println("\n3. First access to key2:")
	val, err = cacheGroup.Get("key2")
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		log.Printf("Got value: %s", val.String())
	}

	// 第三次访问 key1，应该缓存命中
	log.Println("\n4. Third access to key1:")
	val, err = cacheGroup.Get("key1")
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		log.Printf("Got value: %s", val.String())
	}

	// 第二次访问 key2，达到 K=2，应该缓存命中
	log.Println("\n5. Second access to key2:")
	val, err = cacheGroup.Get("key2")
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		log.Printf("Got value: %s", val.String())
	}

	// 测试 TTL
	log.Println("\n6. Testing TTL:")
	cacheGroup.Set("key3", []byte("value3"), 1) // 1秒TTL
	val, err = cacheGroup.Get("key3")
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		log.Printf("Got value: %s", val.String())
	}

	// 等待 TTL 过期
	log.Println("\n7. Waiting for TTL expiration...")
	time.Sleep(2 * time.Second)

	// 访问已过期的 key3，应该缓存未命中
	log.Println("\n8. Accessing expired key3:")
	val, err = cacheGroup.Get("key3")
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		log.Printf("Got value: %s", val.String())
	}

	log.Println("\nLRU-K Cache Test Completed")
	log.Println("=========================")
}
