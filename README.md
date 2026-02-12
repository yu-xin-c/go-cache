# MyGoCache

MyGoCache 是一个基于 Go 语言实现的分布式缓存系统，参考了 Memcached 和 Redis 的设计理念，结合了字节跳动 Kitex 的内存管理策略所设计，融合Go语言协程，轻量的美学，具有以下特点：

## 核心功能

- **分布式缓存**：支持多节点分布式部署，使用一致性哈希进行负载均衡
- **TTL 支持**：为缓存项添加过期时间，支持惰性过期和主动过期策略
- **过期淘汰策略**：实现了惰性过期结合最小堆主动过期的策略
- **LRU 缓存**：使用 LRU (Least Recently Used) 算法进行内存淘汰
- **LRU-K 缓存**：支持 LRU-K 算法，提高缓存命中率
- **高性能**：使用协程池和对象池优化并发处理和内存管理
- **并发安全**：使用 sync.Map 优化并发访问性能
- **Kitex 通信**：支持使用 Kitex 框架进行节点间通信，提高性能和可靠性，使用字节同款thrift通信

## 项目结构

```
├── mygocache/           # 核心缓存实现
│   ├── lru/            # LRU 缓存实现（包含过期管理）
│   ├── pool/           # 协程池和对象池实现
│   ├── kitex_gen/      # Kitex 代码生成目录
│   ├── cache.go        # 缓存封装
│   ├── mygocache.go     # 核心缓存组实现
│   ├── kitex.go        # Kitex 服务实现
│   ├── peers.go        # 节点管理
│   └── singleflight/   # 并发请求合并
├── main.go             # 示例程序
├── test_ttl.go         # TTL 测试
├── test_pool.go        # 协程池和对象池测试
└── README.md           # 项目文档
```

## 文档

- 在本仓库的docs文件夹下，包括了各个优化点的详细文档

## 优化点

### 1. 协程池（Goroutine Pool）
- 实现了非阻塞协程池，支持限制协程数量
- 优化并发处理能力，避免创建过多协程导致的资源消耗
- 参考了字节跳动 kitex 和 hertz 中的协程池设计

### 2. 对象池（Object Pool）
- 实现了基于 sync.Pool 的对象池，用于缓存条目的内存管理
- 减少内存分配和 GC 压力，提高内存使用效率
- 包含通用对象池、条目对象池和堆项对象池

### 3. TTL 和过期淘汰策略
- 支持为缓存项设置过期时间（TTL）
- 实现了惰性过期：访问时检查并删除过期项
- 实现了主动过期：使用最小堆管理过期项，后台协程定期检查并删除

### 4. LRU-K 缓存
- 实现了 LRU-K 算法，提高缓存命中率
- 支持设置 K 值，默认为 2
- 维护访问历史记录，只有访问 K 次的项才进入缓存

### 5. sync.Map 优化
- 使用 sync.Map 替代 map+Mutex，优化并发访问性能
- 减少锁竞争，提高并发读取效率
- 适用于读多写少的场景

### 6. 分布式支持
- 使用一致性哈希进行节点间负载均衡
- 支持 Kitex 协议进行节点间通信

### 7. Kitex 通信优化
- 支持使用 Kitex 框架进行节点间通信，提高性能和可靠性
- 基于 Netpoll 网络库，实现非阻塞 I/O，提高并发性能
- 使用长连接复用，减少连接建立开销
- 支持 Thrift 二进制序列化，提高序列化效率
- 内存占用低，采用对象池等优化策略

## 如何使用

### 基本使用

```go
import "mygocache"

// 创建缓存组，设置默认 TTL 为 5 秒
gee := mygocache.NewGroupWithTTL("test", 2<<10, mygocache.GetterFunc(
    func(key string) ([]byte, error) {
        // 缓存未命中时的回调函数
        return []byte("value"), nil
    }), 5)

// 获取缓存
value, err := gee.Get("key")

// 设置缓存，TTL 为 10 秒
gee.Set("key", []byte("value"), 10)
```

### 分布式部署

1. 启动多个缓存服务器实例
2. 在每个实例中注册其他节点
3. 使用一致性哈希进行负载均衡

### 使用 Kitex 通信

```bash
go run main.go -port=8001
```

## 如何运行测试

### 运行基本测试

```bash
go test -v
```

### 运行示例

```bash
go run main.go -port=8001
```

## 技术栈

- **Go 语言**：使用 Go 1.18+ 特性
- **Kitex**：字节跳动开源的高性能 RPC 框架，用于节点间通信
- **Thrift**：用于 Kitex 服务的接口定义和序列化
- **sync 包**：用于并发控制（Mutex、RWMutex、Pool）
- **container/list**：用于实现 LRU 缓存
- **container/heap**：用于实现过期管理的最小堆

## 优化方向

1. **进一步优化协程池**：根据系统负载动态调整协程数量
2. **优化一致性哈希**：支持虚拟节点数量的动态调整
3. **添加监控指标**：收集缓存命中率、过期率等指标
4. **支持更多序列化格式**：支持 JSON、MsgPack 等
5. **添加持久化支持**：支持将缓存数据持久化到磁盘

## 参考

- [7 Days Golang](https://github.com/geektutu/7days-golang)
- [Memcached](https://memcached.org/)
- [Redis](https://redis.io/)
- [Kitex](https://github.com/cloudwego/kitex)
- [Hertz](https://github.com/cloudwego/hertz)
