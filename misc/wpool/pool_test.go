package wpool

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func runWp(cap, total, take int) {
	wp := New(cap, total)
	for idx := 0; idx < total; idx++ {
		wp.Submit(func() error {
			time.Sleep(time.Duration(take) * time.Millisecond)
			return nil
		})
	}
	wp.Wait()
}

func runGo(total, take int) {
	var wg sync.WaitGroup
	wg.Add(total)
	for idx := 0; idx < total; idx++ {
		go func() {
			time.Sleep(time.Duration(take) * time.Millisecond)
			wg.Done()
		}()
	}
	wg.Wait()
}

func TestWorkerStack(t *testing.T) {
	ws := new(workerStack)
	ws.push(new(worker))
	ws.push(new(worker))

	fmt.Println(ws.pop())
	fmt.Println(ws.pop())
	fmt.Println(ws.pop())
}

func TestPoolBase(t *testing.T) {
	runWp(500, 1000, 100)
	runWp(50, 1000, 10)
	runWp(-1, 1000, 20)
}

const (
	times   = 1000000
	take    = 10
	poolCap = 200000
)

func demo() {
	time.Sleep(time.Duration(take) * time.Millisecond)
}

func BenchmarkGoroutines(b *testing.B) {
	b.ReportAllocs()
	var wg sync.WaitGroup
	for i := 0; i < b.N; i++ {
		wg.Add(times)
		for j := 0; j < times; j++ {
			go func() {
				demo()
				wg.Done()
			}()
		}
		wg.Wait()
	}
}

func BenchmarkWorkerPool(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		wp := New(poolCap, times)
		for j := 0; j < times; j++ {
			wp.Submit(func() error {
				demo()
				return nil
			})
		}
		wp.Wait()
	}
}
