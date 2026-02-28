package main

import (
	"flag"
	"fmt"
	"log"

	"mygocache"
	"mygocache/asynclog"
)

var db = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

func createGroup() *mygocache.Group {
	return mygocache.NewGroupWithOptions("scores", 2<<10, mygocache.GetterFunc(
		func(key string) ([]byte, error) {
			log.Println("[SlowDB] search key", key)
			if v, ok := db[key]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		}), 0, mygocache.StrategyLRUK, 2)
}

func startCacheServer(addr string, addrs []string, gee *mygocache.Group) {
	peers := mygocache.NewKitexPool(addr)
	peers.Set(addrs...)
	gee.RegisterPeers(peers)
	log.Println("mygocache Kitex is running at", addr)
	log.Fatal(mygocache.StartKitexServer(addr))
}

func main() {
	var port int
	flag.IntVar(&port, "port", 8001, "Geecache server port")
	flag.Parse()

	// 初始化异步日志
	asynclog.Init(16384)
	defer asynclog.Close()

	addrMap := map[int]string{
		8001: "http://localhost:8001",
		8002: "http://localhost:8002",
		8003: "http://localhost:8003",
	}

	var addrs []string
	for _, v := range addrMap {
		addrs = append(addrs, v)
	}

	gee := createGroup()
	startCacheServer(addrMap[port], addrs, gee)
}
