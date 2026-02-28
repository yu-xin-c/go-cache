package pool

import (
	"mygocache/asynclog"
	"sync"
	"sync/atomic"
	"time"
)

// GoroutinePool 自适应协程池
// 根据队列深度和 worker 繁忙率动态调整协程数量：
//   - 队列利用率 > 70% 或 worker 利用率 > 80% 且有排队 → 扩容
//   - worker 利用率 < 30% → 缩容
//   - 始终保持在 [minWorkers, maxWorkers] 范围内
type GoroutinePool struct {
	tasks chan func()
	quit  chan struct{} // 向 worker 发送退出信号（缩容用）
	done  chan struct{} // 关闭 monitor

	wg     sync.WaitGroup
	mu     sync.Mutex
	closed bool

	// 动态伸缩参数
	workers    int
	minWorkers int
	maxWorkers int

	// 运行时指标（原子操作，无锁）
	activeWorkers atomic.Int64 // 当前正在执行任务的 worker 数
	totalTasks    atomic.Int64 // 累计完成的任务数
}

// NewGoroutinePool 创建自适应协程池
//   - minWorkers: 最小协程数
//   - maxWorkers: 最大协程数
//   - queueSize: 任务队列容量
func NewGoroutinePool(minWorkers, maxWorkers, queueSize int) *GoroutinePool {
	if minWorkers <= 0 {
		minWorkers = 1
	}
	if maxWorkers < minWorkers {
		maxWorkers = minWorkers
	}
	if queueSize <= 0 {
		queueSize = 1000
	}

	p := &GoroutinePool{
		tasks:      make(chan func(), queueSize),
		quit:       make(chan struct{}, maxWorkers),
		done:       make(chan struct{}),
		workers:    minWorkers,
		minWorkers: minWorkers,
		maxWorkers: maxWorkers,
	}

	// 启动初始 worker
	for i := 0; i < minWorkers; i++ {
		p.wg.Add(1)
		go p.worker()
	}

	// 启动监控协程，定期调整池大小
	go p.monitor()

	return p
}

// worker 工作协程，通过 select 监听任务和退出信号
func (p *GoroutinePool) worker() {
	defer p.wg.Done()
	for {
		select {
		case task, ok := <-p.tasks:
			if !ok || task == nil {
				return
			}
			p.activeWorkers.Add(1)
			task()
			p.activeWorkers.Add(-1)
			p.totalTasks.Add(1)
		case <-p.quit:
			// 收到缩容信号，退出
			return
		}
	}
}

// Submit 提交任务到协程池
func (p *GoroutinePool) Submit(task func()) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	// 非阻塞提交
	select {
	case p.tasks <- task:
		return nil
	default:
		// 队列已满，直接起临时协程执行（保证不丢任务）
		go task()
		return nil
	}
}

// monitor 定期检查负载并调整 worker 数量
func (p *GoroutinePool) monitor() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.adjust()
		case <-p.done:
			return
		}
	}
}

// adjust 根据当前负载调整 worker 数量
func (p *GoroutinePool) adjust() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}

	pending := len(p.tasks)
	queueCap := cap(p.tasks)
	active := int(p.activeWorkers.Load())
	workers := p.workers
	oldWorkers := workers

	queueUtil := float64(pending) / float64(queueCap)
	workerUtil := float64(0)
	if workers > 0 {
		workerUtil = float64(active) / float64(workers)
	}

	switch {
	case queueUtil > 0.7 || (workerUtil > 0.8 && pending > 0):
		// 高负载：扩容（每次增加 25% 或至少 1 个）
		add := workers/4 + 1
		newWorkers := workers + add
		if newWorkers > p.maxWorkers {
			newWorkers = p.maxWorkers
		}
		toAdd := newWorkers - workers
		for i := 0; i < toAdd; i++ {
			p.wg.Add(1)
			go p.worker()
		}
		p.workers = newWorkers

	case workerUtil < 0.3 && workers > p.minWorkers:
		// 低负载：缩容（保留至少 active+1 个 worker，每次最多减 25%）
		target := active + 1
		if target < p.minWorkers {
			target = p.minWorkers
		}
		toRemove := workers - target
		maxRemove := workers/4 + 1
		if toRemove > maxRemove {
			toRemove = maxRemove
		}
		for i := 0; i < toRemove; i++ {
			select {
			case p.quit <- struct{}{}:
				p.workers--
			default:
				// quit 通道满，跳过
			}
		}
	}

	if p.workers != oldWorkers {
		asynclog.Printf("[GoroutinePool] workers adjusted: %d -> %d (active=%d, pending=%d, queueUtil=%.1f%%, workerUtil=%.1f%%)",
			oldWorkers, p.workers, active, pending, queueUtil*100, workerUtil*100)
	}
}

// Close 关闭协程池，等待所有 worker 退出
func (p *GoroutinePool) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()

	close(p.done)  // 停止 monitor
	close(p.tasks) // 通知所有 worker 退出
	p.wg.Wait()
}

// Size 返回当前 worker 数量
func (p *GoroutinePool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.workers
}

// ActiveWorkers 返回当前正在执行任务的 worker 数
func (p *GoroutinePool) ActiveWorkers() int {
	return int(p.activeWorkers.Load())
}

// TotalTasks 返回累计完成的任务数
func (p *GoroutinePool) TotalTasks() int64 {
	return p.totalTasks.Load()
}
