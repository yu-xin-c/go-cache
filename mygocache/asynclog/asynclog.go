package asynclog

import (
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// AsyncLogger 异步日志，将日志写入缓冲通道，由后台 goroutine 批量刷盘
// 避免 log.Println 的全局互斥锁成为热路径瓶颈
type AsyncLogger struct {
	ch     chan []byte
	done   chan struct{}
	logger *log.Logger
	wg     sync.WaitGroup

	// 关闭状态
	closed    int32    // 原子标志，防止 close 后继续写入 channel
	closeOnce sync.Once // 保证 Close 幂等

	// 统计信息
	droppedCount int64 // 丢弃的日志数量
}

var (
	defaultLogger *AsyncLogger
	once          sync.Once
)

// Init 初始化全局异步日志（缓冲区大小 bufSize）
func Init(bufSize int) {
	once.Do(func() {
		defaultLogger = New(bufSize, log.Default())
	})
}

// New 创建异步日志实例
func New(bufSize int, logger *log.Logger) *AsyncLogger {
	if bufSize <= 0 {
		bufSize = 8192
	}
	al := &AsyncLogger{
		ch:     make(chan []byte, bufSize),
		done:   make(chan struct{}),
		logger: logger,
	}
	al.wg.Add(1)
	go al.drain()
	return al
}

// drain 后台消费日志消息，批量写入
func (al *AsyncLogger) drain() {
	defer al.wg.Done()
	buf := make([][]byte, 0, 64)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case msg := <-al.ch:
			buf = append(buf, msg)
			// 尝试批量读取更多
			for len(buf) < 64 {
				select {
				case m := <-al.ch:
					buf = append(buf, m)
				default:
					goto flush
				}
			}
		flush:
			al.flushBatch(buf)
			buf = buf[:0]
		case <-ticker.C:
			// 定时刷盘（防止低流量时日志延迟过大）
			if len(buf) > 0 {
				al.flushBatch(buf)
				buf = buf[:0]
			}
		case <-al.done:
			// 收到关闭信号，刷出当前 buf
			if len(buf) > 0 {
				al.flushBatch(buf)
				buf = buf[:0]
			}
			// 非阻塞排空 channel 中残留的消息（不 close channel，避免写入方 panic）
			for {
				select {
				case msg := <-al.ch:
					buf = append(buf, msg)
					if len(buf) >= 64 {
						al.flushBatch(buf)
						buf = buf[:0]
					}
				default:
					// channel 已空
					if len(buf) > 0 {
						al.flushBatch(buf)
					}
					return
				}
			}
		}
	}
}

func (al *AsyncLogger) flushBatch(batch [][]byte) {
	for _, msg := range batch {
		os.Stderr.Write(msg)
	}
}

// Printf 异步写日志（非阻塞，缓冲区满则丢弃）
func (al *AsyncLogger) Printf(format string, v ...interface{}) {
	if atomic.LoadInt32(&al.closed) != 0 {
		return
	}
	msg := fmt.Appendf(nil, format, v...)
	msg = append(msg, '\n')
	select {
	case al.ch <- msg:
	default:
		// 缓冲区满，丢弃（高压情况下保护吞吐）
		atomic.AddInt64(&al.droppedCount, 1)
	}
}

// Println 异步写日志
func (al *AsyncLogger) Println(v ...interface{}) {
	if atomic.LoadInt32(&al.closed) != 0 {
		return
	}
	msg := fmt.Appendln(nil, v...)
	select {
	case al.ch <- msg:
	default:
		atomic.AddInt64(&al.droppedCount, 1)
	}
}

// DroppedCount 返回丢弃的日志数量
func (al *AsyncLogger) DroppedCount() int64 {
	return atomic.LoadInt64(&al.droppedCount)
}

// Close 关闭异步日志，刷完缓冲区（幂等，可多次调用）
func (al *AsyncLogger) Close() {
	al.closeOnce.Do(func() {
		atomic.StoreInt32(&al.closed, 1)
		close(al.done)
		al.wg.Wait()
	})
}

// --- 全局便捷函数 ---

// Printf 全局异步日志
func Printf(format string, v ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Printf(format, v...)
	} else {
		log.Printf(format, v...)
	}
}

// Println 全局异步日志
func Println(v ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Println(v...)
	} else {
		log.Println(v...)
	}
}

// Close 关闭全局异步日志
func Close() {
	if defaultLogger != nil {
		defaultLogger.Close()
	}
}

// DroppedCount 返回全局异步日志丢弃的日志数量
func DroppedCount() int64 {
	if defaultLogger != nil {
		return defaultLogger.DroppedCount()
	}
	return 0
}
