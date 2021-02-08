package wpool

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
)

// Pool can execute tasks concurrently. It internally
// controls the number of goroutines, can reuse
// goroutines to reduce the overhead of creating
// and destorying them. Pool can also prevent excessive
// expansion of the number of goroutines to reduce
// the pressure on the go scheduler.
//
// Pool can receive and run tasks forever, or stop
// after executing a certain number of tasks. The
// external caller can also stop Pool by context.Context.
//
// But note that as long as one task returns error,
// the entire pool will stop.
type Pool struct {
	// cap is the max number of goroutines held in pool.
	cap int32

	// running is the number of running goroutines.
	running int32

	// free stores those idle workers, freeCond is
	// used to notify that a new woker is idle.
	free     *workerStack
	freeCond *sync.Cond

	// done is whether the Pool is stopped, err stores
	// error returned by task.
	done uint32
	err  error

	// lock is used for some sync operations.
	lock sync.Locker

	// ctx is used to monitor whether the Pool is stopped,
	// cancel is used to actively stop the Pool.
	ctx    context.Context
	cancel context.CancelFunc

	// workers will hold some resources(channel), and these
	// functions are used to release these resources after
	// the Pool stops.
	closes []func()

	// wg is used as a counter when the number of tasks
	// to be executed is passed in.
	wg *sync.WaitGroup

	// blockSubmit controls whether to block Submit function
	// when there is no idle worker.
	blockSubmit bool

	// when there is no free worker available, the submitted
	// task should be pushed to the overflow stack, and another
	// goroutine will pull tasks from the overflow stack and
	// send them to the idle worker.
	// NOTE: This is available when blockSubmit=false
	overflow     taskStack
	overflowCond *sync.Cond
	overflowLock sync.Locker
	overflowOnce sync.Once

	// mu is used for caller to perfom some sync operations.
	mu sync.Mutex
}

// New creates an empty default Pool, which is the same as
// NewWithCtx, except that it uses context.Backgroud to
// initialize "ctx".
func New(cap, total int) *Pool {
	return NewWithCtx(context.Background(), cap, total)
}

// NewWithCtx used the context.Context given by the caller
// to create and initialize the Pool. Caller can use ctx
// to stop Pool manually (like setting a timeout for Pool).
//
// cap is the max number of goroutines that Pool will create.
// cap must greater than 0.
//
// total is the max number of tasks executed by Pool. When
// the number of successfully executed tasks reaches total,
// Pool will be closed directly. If total is 0, the Pool can
// execute ulimited number of tasks.
func NewWithCtx(ctx context.Context, cap, total int) *Pool {
	if cap <= 0 {
		cap = -1
	}

	pctx, cancel := context.WithCancel(ctx)

	p := &Pool{
		cap:  int32(cap),
		lock: newLock(),

		ctx:    pctx,
		cancel: cancel,

		free: new(workerStack),
	}

	p.freeCond = sync.NewCond(p.lock)

	if total > 0 {
		// start a goroutine to monitor whether
		// the counter reaches total, and stop
		// the pool if it reaches.
		p.wg = new(sync.WaitGroup)
		p.wg.Add(total)
		go p.asyncWaitDone()
	}

	return p
}

// BlockSubmit sets whether to block Submit when there
// is no idle goroutine. The default is false.
func (p *Pool) BlockSubmit(bs bool) *Pool {
	p.blockSubmit = bs
	return p
}

// asyncWaitDone stops pool when wg is done.
func (p *Pool) asyncWaitDone() {
	p.wg.Wait()
	p.Stop(nil)
}

// asyncHandleOverflow pulls task from the overflow stack,
// and wait for idle worker to execute it.
func (p *Pool) asyncHandleOverflow() {
reentry:
	p.overflowLock.Lock()
reentryOverflow:
	if p.Done() {
		p.overflowLock.Unlock()
		return
	}
	act := p.overflow.pop()
	if act == nil {
		p.overflowCond.Wait()
		goto reentryOverflow
	}

	p.overflowLock.Unlock()

	p.lock.Lock()
reentryFree:
	if p.Done() {
		p.lock.Unlock()
		return
	}
	w := p.free.pop()
	if w == nil {
		p.freeCond.Wait()
		goto reentryFree
	}

	p.lock.Unlock()

	safedo(func() { w.invoke <- act })
	goto reentry
}

// startHandleOverflow starts a goroutine that processes
// the overflow tasks. Each pool should call this
// function only once.
func (p *Pool) startHandleOverflow() {
	p.overflowLock = newLock()
	p.overflowCond = sync.NewCond(p.overflowLock)
	go p.asyncHandleOverflow()
}

// Submit submits a task to the pool.
//
// If set BlockSubmit to true, and there is no free
// goroutine, the function will block until there is a free
// goroutine. If BlockSubmit is false(default), it will not
// block even if there is no idle goroutine, and the task
// will be automatically submited asynchronously later.
//
// action represents the function executed by the submitted
// task. Note that if the error it returns is not nil,
// the entire pool will stop and the error will be returned
// through Wait.
//
// The order of calling Submit and the order in which
// the tasks are actually executed may be inconsistent.
// Do not perform operations based on the calling
// sequence of Submit.
//
// If call Submit on a stopped pool, the passed action
// will not be executed. This will not be notified, the
// caller can judge this by Done.
//
// If total is greater than 0 when the pool is created,
// the number of calls to Submit should be equal to total.
// If it is less than total, then calling Wait may cause
// permanent blocking; if it is greater than total, then
// part of the tasks may not be executed.
func (p *Pool) Submit(act Action) {
	if p.wg != nil {
		p.submit(func() error {
			defer p.wg.Done()
			return act()
		})
		return
	}
	p.submit(act)
}

// submit is the implementation of submitTask operation.
func (p *Pool) submit(act Action) {
	if p.Done() {
		return
	}
	w := p.nextWorker()
	if w == nil {
		// there is no idle worker, put the task in the
		// overflow stack and wait for the overflow
		// goroutine to process it.
		p.overflowOnce.Do(p.startHandleOverflow)
		p.overflowLock.Lock()
		p.overflow.push(act)
		p.overflowCond.Signal()
		p.overflowLock.Unlock()
		return
	}

	w.invoke <- act
}

// SyncRun is used to perform some synchronization
// operations in the task (such as inserting values
// into slice or map).
// The function passed to SyncRun will be executed
// synchronously under any circumstances.
func (p *Pool) SyncRun(act Action) error {
	p.mu.Lock()
	err := act()
	p.mu.Unlock()
	return err
}

// Wait blocks until the Pool is stopped. Stop can be
// triggered by a variety of conditions:
//  1. The number of completed task(s) reaches the total
//     value.(Not applicable to total less than or equal 0)
//  2. A task returns an error.
//  3. Stop function is called.
//  4. The passed in ctx is done.
// Wait will return the error returned in the pool execution
// task. If multiple tasks return errors, the Pool will return
// a random one of them.
func (p *Pool) Wait() error {
	<-p.ctx.Done()
	for _, closeFunc := range p.closes {
		closeFunc()
	}
	if p.overflowCond != nil {
		p.overflowCond.Signal()
		p.freeCond.Signal()
	}
	return p.err
}

// increase running
func (p *Pool) incrRunning() {
	atomic.AddInt32(&p.running, 1)
}

// decreasing running
func (p *Pool) decrRunning() {
	atomic.AddInt32(&p.running, -1)
}

// Running returns the number of goroutines currently in
// execution. It changes dynamically, but does not exceed
// the cap value passed in when the pool is created.
func (p *Pool) Running() int {
	return int(atomic.LoadInt32(&p.running))
}

// Done returns whether the current pool has stopped.
func (p *Pool) Done() bool {
	return atomic.LoadUint32(&p.done) == 1
}

// nextWorker returns an idle worker, if not, returns nil.
func (p *Pool) nextWorker() *worker {
	p.lock.Lock()
	defer p.lock.Unlock()
	w := p.free.pop()
	if w != nil {
		return w
	}

	if p.cap < 0 || atomic.LoadInt32(&p.running) < p.cap {
		return p.spawnWorker()
	}

	if !p.blockSubmit {
		return nil
	}

reentry:
	p.freeCond.Wait()
	if p.Running() == 0 {
		return p.spawnWorker()
	}

	w = p.free.pop()
	if w == nil {
		goto reentry
	}

	return w
}

// spwanWorker creates and runs a new worker, it should
// be executed in synchronization semantics.
func (p *Pool) spawnWorker() *worker {
	w := &worker{
		p:      p,
		invoke: make(chan Action, 1),
	}
	go w.run()
	p.closes = append(p.closes, func() {
		close(w.invoke)
	})
	return w
}

// resetWorker is called after the worker is executed, it
// sets the worker's status to idle(put it into the free
// stack). The return value indicates whether the status
// setting is successful, if it is false, the worker should
// be terminated.
func (p *Pool) resetWorker(w *worker) bool {
	if atomic.LoadInt32(&p.running) > p.cap || p.Done() {
		return false
	}

	p.lock.Lock()
	defer p.lock.Unlock()
	if p.Done() {
		return false
	}

	p.free.push(w)
	p.freeCond.Broadcast()
	return true
}

// Stop forcibly terminates the entire pool, which will
// cause all goroutines and tasks to stop, and stop the
// block of Wait. The passes in err will be returned by
// Wait.
func (p *Pool) Stop(err error) {
	if err != nil {
		p.err = err
	}
	atomic.StoreUint32(&p.done, 1)
	p.cancel()
}

// worker is a reusable unit for executing tasks.
type worker struct {
	p *Pool

	// invoke uses to receive tasks.
	invoke chan Action
}

// run receives and executes tasks.
func (w *worker) run() {
	w.p.incrRunning()
	defer w.p.decrRunning()
	for act := range w.invoke {
		err := act()
		if err != nil {
			w.p.Stop(err)
			return
		}

		if ok := w.p.resetWorker(w); !ok {
			return
		}
	}
}

type workerStack struct {
	slice []*worker
}

func (ws *workerStack) push(w *worker) {
	ws.slice = append(ws.slice, w)
}

func (ws *workerStack) pop() *worker {
	l := len(ws.slice)
	if l == 0 {
		return nil
	}

	w := ws.slice[l-1]
	ws.slice[l-1] = nil
	ws.slice = ws.slice[:l-1]

	return w
}

type taskStack struct {
	slice []Action
}

func (ws *taskStack) push(task Action) {
	ws.slice = append(ws.slice, task)
}

func (ws *taskStack) pop() Action {
	l := len(ws.slice)
	if l == 0 {
		return nil
	}

	a := ws.slice[l-1]
	ws.slice[l-1] = nil
	ws.slice = ws.slice[:l-1]

	return a
}

// Action is the task function to be executed passed in
// by the caller. If its return value is not nil, it will
// stop the entire pool.
type Action func() error

func mustn(n int) {
	if n <= 0 {
		panic("n must > 0")
	}
}

// ignores any panic
func safedo(f func()) {
	defer func() {
		if r := recover(); r != nil {
			return
		}
	}()
	f()
}

// spinLock is a lightweight and fast sync.Locker implementation.
type spinLock uint32

func (l *spinLock) Lock() {
	for !atomic.CompareAndSwapUint32((*uint32)(l), 0, 1) {
		runtime.Gosched()
	}
}

func (l *spinLock) Unlock() {
	atomic.StoreUint32((*uint32)(l), 0)
}

// newLock creates a new spinLock.
func newLock() sync.Locker {
	return new(spinLock)
}
