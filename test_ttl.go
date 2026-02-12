package main

import (
	"fmt"
	"log"
	"time"

	"mygocache"
)

var ttlDB = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

func createTTLGroup() *mygocache.Group {
	return mygocache.NewGroupWithTTL("scores", 2<<10, mygocache.GetterFunc(
		func(key string) ([]byte, error) {
			log.Println("[SlowDB] search key", key)
			if v, ok := ttlDB[key]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		}), 5) // 默认 TTL 为 5 秒
}

func testTTL() {
	gee := createTTLGroup()

	// 测试 1: 基本 TTL 功能
	log.Println("=== Test 1: Basic TTL functionality ===")
	gee.Set("test1", []byte("value1"), 2) // 2秒过期
	value, err := gee.Get("test1")
	if err == nil {
		log.Println("test1 found:", string(value.ByteSlice()))
	} else {
		log.Println("test1 not found:", err)
	}

	// 等待 3 秒，让键过期
	log.Println("Waiting for 3 seconds...")
	time.Sleep(3 * time.Second)

	value, err = gee.Get("test1")
	if err == nil {
		log.Println("test1 found (should be expired):", string(value.ByteSlice()))
	} else {
		log.Println("test1 not found (expired as expected):", err)
	}

	// 测试 2: 默认 TTL
	log.Println("\n=== Test 2: Default TTL ===")
	// 这里会调用 getter，因为键不存在
	value, err = gee.Get("Tom")
	if err == nil {
		log.Println("Tom found:", string(value.ByteSlice()))
	} else {
		log.Println("Tom not found:", err)
	}

	// 立即再次获取，应该从缓存中命中
	value, err = gee.Get("Tom")
	if err == nil {
		log.Println("Tom found (from cache):", string(value.ByteSlice()))
	} else {
		log.Println("Tom not found:", err)
	}

	// 测试 3: 永不过期
	log.Println("\n=== Test 3: Never expire ===")
	gee.Set("test3", []byte("value3"), 0) // 永不过期
	value, err = gee.Get("test3")
	if err == nil {
		log.Println("test3 found:", string(value.ByteSlice()))
	} else {
		log.Println("test3 not found:", err)
	}

	// 等待 2 秒
	log.Println("Waiting for 2 seconds...")
	time.Sleep(2 * time.Second)

	value, err = gee.Get("test3")
	if err == nil {
		log.Println("test3 found (should not expire):", string(value.ByteSlice()))
	} else {
		log.Println("test3 not found (unexpected):", err)
	}

	// 测试 4: 主动过期（最小堆）
	log.Println("\n=== Test 4: Active expiration (min-heap) ===")
	gee.Set("test4", []byte("value4"), 1) // 1秒过期
	log.Println("Added test4 with 1 second TTL")

	// 等待 2 秒，让主动过期协程删除它
	log.Println("Waiting for 2 seconds...")
	time.Sleep(2 * time.Second)

	// 检查是否被主动删除
	value, err = gee.Get("test4")
	if err == nil {
		log.Println("test4 found (should be expired):", string(value.ByteSlice()))
	} else {
		log.Println("test4 not found (actively expired as expected):", err)
	}

	log.Println("\n=== All tests completed ===")
}

func mainTTL() {
	// 运行 TTL 测试
	testTTL()
}
