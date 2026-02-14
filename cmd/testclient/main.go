package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mygocache/kitex_gen/geecache"
	"mygocache/kitex_gen/geecache/groupcache"

	"github.com/cloudwego/kitex/client"
)

const (
	apiGateway = "http://localhost:9999"
	group      = "scores"
)

var (
	cacheNodes = []string{
		"http://localhost:8001",
		"http://localhost:8002",
		"http://localhost:8003",
	}
)

type TestClient struct {
	httpClient *http.Client
	rpcClients map[string]groupcache.Client
	scanner    *bufio.Scanner
	useHTTP    bool
}

type StatsResponse struct {
	ItemCount  int64 `json:"item_count"`
	HitCount   int64 `json:"hit_count"`
	MissCount  int64 `json:"miss_count"`
	TotalCount int64 `json:"total_count"`
}

func NewTestClient() *TestClient {
	// 创建 HTTP 客户端
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	// 创建 RPC 客户端（用于节点状态检查）
	rpcClients := make(map[string]groupcache.Client)
	for _, addr := range cacheNodes {
		// 从 URL 中提取主机地址（去掉 http:// 前缀）
		hostAddr := strings.TrimPrefix(addr, "http://")

		// 创建 Kitex 客户端，使用正确的服务名和主机地址
		client, err := groupcache.NewClient(
			"GroupCache",
			client.WithHostPorts(hostAddr),
		)
		if err != nil {
			fmt.Printf("警告: 无法连接到 %s: %v\n", addr, err)
			continue
		}
		rpcClients[addr] = client
	}

	return &TestClient{
		httpClient: httpClient,
		rpcClients: rpcClients,
		scanner:    bufio.NewScanner(os.Stdin),
		useHTTP:    true, // 默认使用 HTTP 模式
	}
}

// HTTP 方法
func (tc *TestClient) httpGet(key string) (string, error) {
	url := fmt.Sprintf("%s/api?key=%s", apiGateway, key)
	resp, err := tc.httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func (tc *TestClient) httpSet(key, value string) error {
	url := fmt.Sprintf("%s/set?key=%s", apiGateway, key)
	resp, err := tc.httpClient.Post(url, "application/octet-stream", bytes.NewBufferString(value))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (tc *TestClient) httpDelete(key string) error {
	url := fmt.Sprintf("%s/delete?key=%s", apiGateway, key)
	resp, err := tc.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (tc *TestClient) httpStats() (*StatsResponse, error) {
	url := fmt.Sprintf("%s/stats", apiGateway)
	resp, err := tc.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var stats StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (tc *TestClient) printMenu() {
	fmt.Println("\n==========================================")
	fmt.Println("       分布式缓存测试客户端")
	fmt.Println("==========================================")
	fmt.Println("1. 获取缓存值 (GET)")
	fmt.Println("2. 设置缓存值 (SET)")
	fmt.Println("3. 删除缓存值 (DELETE)")
	fmt.Println("4. 查看统计信息 (STATS)")
	fmt.Println("5. 批量测试")
	fmt.Println("6. 性能压测")
	fmt.Println("7. 查看节点状态")
	fmt.Println("8. 切换连接模式")
	fmt.Println("0. 退出")
	fmt.Println("==========================================")
	fmt.Print("请选择操作: ")
}

func (tc *TestClient) readInput() string {
	tc.scanner.Scan()
	return strings.TrimSpace(tc.scanner.Text())
}

func (tc *TestClient) handleGet() {
	fmt.Print("请输入 key: ")
	key := tc.readInput()

	value, err := tc.httpGet(key)
	if err != nil {
		fmt.Printf("❌ 获取失败: %v\n", err)
		return
	}

	fmt.Printf("✅ 获取成功: %s = %s\n", key, value)
}

func (tc *TestClient) handleSet() {
	fmt.Print("请输入 key: ")
	key := tc.readInput()

	fmt.Print("请输入 value: ")
	value := tc.readInput()

	err := tc.httpSet(key, value)
	if err != nil {
		fmt.Printf("❌ 设置失败: %v\n", err)
		return
	}

	fmt.Printf("✅ 设置成功: %s = %s\n", key, value)
}

func (tc *TestClient) handleDelete() {
	fmt.Print("请输入 key: ")
	key := tc.readInput()

	err := tc.httpDelete(key)
	if err != nil {
		fmt.Printf("❌ 删除失败: %v\n", err)
		return
	}

	fmt.Printf("✅ 删除成功: %s\n", key)
}

func (tc *TestClient) handleStats() {
	stats, err := tc.httpStats()
	if err != nil {
		fmt.Printf("❌ 获取统计失败: %v\n", err)
		return
	}

	fmt.Println("\n📊 缓存统计信息:")
	fmt.Printf("   缓存条目数: %d\n", stats.ItemCount)
	fmt.Printf("   命中次数: %d\n", stats.HitCount)
	fmt.Printf("   未命中次数: %d\n", stats.MissCount)
	fmt.Printf("   总请求数: %d\n", stats.TotalCount)
	if stats.TotalCount > 0 {
		hitRate := float64(stats.HitCount) / float64(stats.TotalCount) * 100
		fmt.Printf("   命中率: %.2f%%\n", hitRate)
	}
}

func (tc *TestClient) handleBatchTest() {
	fmt.Println("开始批量测试...")

	// 1. 批量写入
	fmt.Println("\n[1/4] 批量写入测试数据...")
	successCount := 0
	for i := 1; i <= 10; i++ {
		key := fmt.Sprintf("BatchTest%d", i)
		value := fmt.Sprintf("value_%d", i)
		err := tc.httpSet(key, value)
		if err != nil {
			fmt.Printf("   ✗ 写入失败 %s: %v\n", key, err)
		} else {
			successCount++
		}
	}
	fmt.Printf("写入完成: %d/10 成功\n", successCount)

	// 2. 批量读取
	fmt.Println("\n[2/4] 批量读取测试...")
	successCount = 0
	for i := 1; i <= 10; i++ {
		key := fmt.Sprintf("BatchTest%d", i)
		expectedValue := fmt.Sprintf("value_%d", i)
		value, err := tc.httpGet(key)
		if err != nil {
			fmt.Printf("   ✗ 读取失败 %s: 期望=%s, 实际=%s, 错误=%v\n", key, expectedValue, value, err)
		} else if value != expectedValue {
			fmt.Printf("   ✗ 值不匹配 %s: 期望=%s, 实际=%s\n", key, expectedValue, value)
		} else {
			successCount++
		}
	}
	fmt.Printf("读取完成: %d/10 成功\n", successCount)

	// 3. 批量删除（删除前5个）
	fmt.Println("\n[3/4] 批量删除测试（删除前5个）...")
	successCount = 0
	for i := 1; i <= 5; i++ {
		key := fmt.Sprintf("BatchTest%d", i)
		err := tc.httpDelete(key)
		if err != nil {
			fmt.Printf("   ✗ 删除失败 %s: %v\n", key, err)
		} else {
			successCount++
		}
	}
	fmt.Printf("删除完成: %d/5 成功\n", successCount)

	// 4. 验证删除结果
	fmt.Println("\n[4/4] 验证删除结果...")
	for i := 1; i <= 10; i++ {
		key := fmt.Sprintf("BatchTest%d", i)
		value, err := tc.httpGet(key)
		if i <= 5 {
			// 前5个应该被删除
			if err != nil {
				fmt.Printf("   ✓ %s 已删除（符合预期）\n", key)
			} else {
				fmt.Printf("   ✗ %s 未删除（不符合预期）: %s\n", key, value)
			}
		} else {
			// 后5个应该还存在
			if err == nil {
				fmt.Printf("   ✓ %s 存在（符合预期）: %s\n", key, value)
			} else {
				fmt.Printf("   ✗ %s 不符合预期: %v\n", key, err)
			}
		}
	}

	fmt.Println("\n✅ 批量测试完成")
}

func (tc *TestClient) handleBenchmark() {
	fmt.Println("性能压测")
	fmt.Println("1. 单 key 并发读取")
	fmt.Println("2. 多 key 随机读取")
	fmt.Println("3. 读写混合测试")
	fmt.Print("请选择测试类型: ")

	choice := tc.readInput()

	fmt.Print("请输入并发数 (默认 100): ")
	concurrencyStr := tc.readInput()
	concurrency := 100
	if concurrencyStr != "" {
		if c, err := strconv.Atoi(concurrencyStr); err == nil {
			concurrency = c
		}
	}

	fmt.Print("请输入总请求数 (默认 1000): ")
	totalStr := tc.readInput()
	total := 1000
	if totalStr != "" {
		if t, err := strconv.Atoi(totalStr); err == nil {
			total = t
		}
	}

	switch choice {
	case "1":
		tc.benchmarkSingleKey(concurrency, total)
	case "2":
		tc.benchmarkMultiKey(concurrency, total)
	case "3":
		tc.benchmarkMixed(concurrency, total)
	default:
		fmt.Println("无效选择")
	}
}

func (tc *TestClient) benchmarkSingleKey(concurrency, total int) {
	key := "benchmark_key"
	value := "benchmark_value"

	// 先设置一个值
	tc.httpSet(key, value)

	fmt.Printf("\n开始单 key 并发测试: 并发=%d, 总请求=%d\n", concurrency, total)

	var wg sync.WaitGroup
	successCount := int64(0)
	failCount := int64(0)
	var mu sync.Mutex
	latencies := make([]time.Duration, 0, total)

	start := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < total/concurrency; j++ {
				reqStart := time.Now()
				_, err := tc.httpGet(key)
				latency := time.Since(reqStart)

				mu.Lock()
				latencies = append(latencies, latency)
				if err == nil {
					successCount++
				} else {
					failCount++
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	tc.printBenchmarkResult(total, successCount, failCount, elapsed, latencies)
}

func (tc *TestClient) benchmarkMultiKey(concurrency, total int) {
	// 先写入一些测试数据
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("bench_key_%d", i)
		value := fmt.Sprintf("bench_value_%d", i)
		tc.httpSet(key, value)
	}

	fmt.Printf("\n开始多 key 随机测试: 并发=%d, 总请求=%d\n", concurrency, total)

	var wg sync.WaitGroup
	successCount := int64(0)
	failCount := int64(0)
	var mu sync.Mutex
	latencies := make([]time.Duration, 0, total)

	start := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano()))
			for j := 0; j < total/concurrency; j++ {
				key := fmt.Sprintf("bench_key_%d", r.Intn(100))
				reqStart := time.Now()
				_, err := tc.httpGet(key)
				latency := time.Since(reqStart)

				mu.Lock()
				latencies = append(latencies, latency)
				if err == nil {
					successCount++
				} else {
					failCount++
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	tc.printBenchmarkResult(total, successCount, failCount, elapsed, latencies)
}

func (tc *TestClient) benchmarkMixed(concurrency, total int) {
	fmt.Printf("\n开始读写混合测试 (70%% 读 + 30%% 写): 并发=%d, 总请求=%d\n", concurrency, total)

	var wg sync.WaitGroup
	successCount := int64(0)
	failCount := int64(0)
	var mu sync.Mutex
	latencies := make([]time.Duration, 0, total)

	start := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)))
			for j := 0; j < total/concurrency; j++ {
				key := fmt.Sprintf("mixed_key_%d", r.Intn(100))
				reqStart := time.Now()

				var err error
				if r.Float64() < 0.7 {
					// 70% 读操作
					_, err = tc.httpGet(key)
				} else {
					// 30% 写操作
					value := fmt.Sprintf("mixed_value_%d", r.Intn(1000))
					err = tc.httpSet(key, value)
				}

				latency := time.Since(reqStart)

				mu.Lock()
				latencies = append(latencies, latency)
				if err == nil {
					successCount++
				} else {
					failCount++
				}
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	tc.printBenchmarkResult(total, successCount, failCount, elapsed, latencies)
}

func (tc *TestClient) printBenchmarkResult(total int, success, fail int64, elapsed time.Duration, latencies []time.Duration) {
	fmt.Println("\n✅ 压测完成")
	fmt.Printf("   总请求数: %d\n", total)
	fmt.Printf("   成功: %d\n", success)
	fmt.Printf("   失败: %d\n", fail)
	fmt.Printf("   成功率: %.2f%%\n", float64(success)/float64(total)*100)
	fmt.Printf("   总耗时: %v\n", elapsed)
	fmt.Printf("   QPS: %.2f\n", float64(total)/elapsed.Seconds())

	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool {
			return latencies[i] < latencies[j]
		})

		sum := time.Duration(0)
		for _, l := range latencies {
			sum += l
		}
		avg := sum / time.Duration(len(latencies))

		p50 := latencies[len(latencies)*50/100]
		p95 := latencies[len(latencies)*95/100]
		p99 := latencies[len(latencies)*99/100]

		fmt.Printf("   平均延迟: %v\n", avg)
		fmt.Printf("   P50 延迟: %v\n", p50)
		fmt.Printf("   P95 延迟: %v\n", p95)
		fmt.Printf("   P99 延迟: %v\n", p99)
	}
}

func (tc *TestClient) handleNodeStatus() {
	fmt.Println("检查节点状态...")
	for _, addr := range cacheNodes {
		client, ok := tc.rpcClients[addr]
		if !ok {
			fmt.Printf("❌ %s: 未连接\n", addr)
			continue
		}

		// 尝试获取统计信息
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		req := &geecache.StatsRequest{Group: group}
		stats, err := client.Stats(ctx, req)
		cancel()

		if err != nil {
			fmt.Printf("❌ %s: 离线 (%v)\n", addr, err)
		} else {
			fmt.Printf("✅ %s: 在线\n", addr)
			fmt.Printf("   缓存条目: %d, 命中: %d, 未命中: %d\n",
				stats.ItemCount, stats.HitCount, stats.MissCount)
		}
	}
}

func (tc *TestClient) handleToggleMode() {
	tc.useHTTP = !tc.useHTTP
	if tc.useHTTP {
		fmt.Println("✅ 已切换到 HTTP 模式（通过 API 网关）")
	} else {
		fmt.Println("✅ 已切换到 RPC 模式（直连节点）")
	}
}

func (tc *TestClient) Run() {
	fmt.Println("欢迎使用分布式缓存测试客户端！")
	fmt.Printf("API 网关: %s\n", apiGateway)
	fmt.Printf("缓存节点: %v\n", cacheNodes)
	fmt.Printf("当前模式: HTTP (通过 API 网关)\n")

	for {
		tc.printMenu()
		choice := tc.readInput()

		switch choice {
		case "1":
			tc.handleGet()
		case "2":
			tc.handleSet()
		case "3":
			tc.handleDelete()
		case "4":
			tc.handleStats()
		case "5":
			tc.handleBatchTest()
		case "6":
			tc.handleBenchmark()
		case "7":
			tc.handleNodeStatus()
		case "8":
			tc.handleToggleMode()
		case "0":
			fmt.Println("再见！")
			return
		default:
			fmt.Println("无效选择，请重试")
		}
	}
}

func main() {
	client := NewTestClient()
	client.Run()
}
