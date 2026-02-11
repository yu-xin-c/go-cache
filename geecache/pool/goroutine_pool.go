package pool

import (
	"sync"
)

// GoroutinePool 协程池，参考 kitex 和 hertz 的实现
type GoroutinePool struct {
	tasks  chan func()
	wg     sync.WaitGroup
	mu     sync.Mutex
	closed bool
}

// NewGoroutinePool 创建一个新的协程池
// size: 协程池大小
// queueSize: 任务队列大小
func NewGoroutinePool(size int, queueSize int) *GoroutinePool {
	if size <= 0 {
		size = 1
	}
	if queueSize <= 0 {
		queueSize = 1000
	}

	pool := &GoroutinePool{
		tasks: make(chan func(), queueSize),
	}

	// 启动工作协程
	for i := 0; i < size; i++ {
		pool.wg.Add(1)
		go pool.worker()
	}

	return pool
}

// worker 工作协程
func (p *GoroutinePool) worker() {
	defer p.wg.Done()

	for task := range p.tasks {
		if task == nil {
			return
		}
		task()
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

	// 非阻塞提交任务
	select {
	case p.tasks <- task:
		return nil
	default:
		// 任务队列已满，直接执行
		go task()
		return nil
	}
}

// Close 关闭协程池
func (p *GoroutinePool) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()

	// 关闭任务通道
	close(p.tasks)
	// 等待所有协程结束
	p.wg.Wait()
}

// Size 返回协程池大小
func (p *GoroutinePool) Size() int {
	// 这里可以返回实际的协程数量
	return cap(p.tasks)
}
