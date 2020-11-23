package e2e

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
)

var (
	printMu = new(sync.Mutex)
)

// withPrintLock takes a lock on stdout in order to avoid printing information out of order.
func withPrintLock(f func()) {
	printMu.Lock()
	defer printMu.Unlock()
	f()
}

// testRun contains all the information needed to run the test suite, including information to synchronize
// workers and cancel currently running tests.
type testRun struct {
	// Mutex to protect internal data structures.
	mu *sync.Mutex
	// Job queue, a channel with test entries in it to distribute work to workers.
	jobQueue chan *testEntry
	//
	jobQueueOnce *sync.Once
	// Wait group to wait for all workers to be finished.
	workersWG *sync.WaitGroup
	// Store cancel functions of running tests, allowing to terminate executing tests.
	cancelFuncs map[int]context.CancelFunc
}

func (t *testRun) setCancelFunc(runnerID int, cancel context.CancelFunc) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cancelFuncs[runnerID] = cancel
}

func (t *testRun) deleteCancelFunc(runnerID int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.cancelFuncs, runnerID)
}

func (t *testRun) runTest(runnerID int, job *testEntry) (result *testResult) {
	result = &testResult{
		failure: nil,
		log:     &Logger{},
	}

	defer func() {
		if r := recover(); r != nil {
			result.failure = r
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())

	// Store the cancel function in the testRun to that the context can be cancelled if the process is terminated.
	t.setCancelFunc(runnerID, cancel)

	// Run the test
	job.f(ctx, result.log)

	// Remove the cancel function from the testRun.
	t.deleteCancelFunc(runnerID)

	return
}

func (t *testRun) startRunner(runnerID int, runnerName string) {
	for job := range t.jobQueue {
		withPrintLock(func() {
			fmt.Printf("%s: starting test '%s'\n", runnerName, job.name)
		})

		job.result = t.runTest(runnerID, job)

		withPrintLock(func() {
			if job.result.failure != nil {
				fmt.Printf("%s: test '%s' failed: '%v'\n", runnerName, job.name, job.result.failure)
				for _, entry := range job.result.log.buf {
					fmt.Printf("%s: log: %s\n", runnerName, entry)
				}
			} else {
				fmt.Printf("%s: test '%s' success!\n", runnerName, job.name)
			}
		})
	}

	t.workersWG.Done()
}

func (t *testRun) close() {
	t.jobQueueOnce.Do(func() {
		close(t.jobQueue)
	})
}

func (t *testRun) shutdown() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Cancel context of any running test
	for _, cancelFunc := range t.cancelFuncs {
		cancelFunc()
	}
}

func (t *testRun) dispatch() {
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Kill, os.Interrupt)

	// close the term channel in order to not leak the goroutine which waits on it.
	defer close(term)

	stopLoop := make(chan int, 1)

	go func() {
		signal := <-term

		// The signal can be nil if the channel was closed without receiving a signal. If the signal is not nil
		// we trigger the termination of the tests and the dispatch loop.
		if signal != nil {
			fmt.Println("\nShutting down...")

			// If we receive a termination signal, we tell the dispatch loop to stop dispatching and tell all the test
			// contexts to cancel.
			stopLoop <- 1
			t.close()
			t.shutdown()
		}
	}()

loop:
	for _, entry := range registry {
		// Send jobs to workers. If a termination signal is received on the stopLoop channel, we stop the dispatch
		// loop in order to avoid sending on a closed channel.
		select {
		case t.jobQueue <- entry:
		case <-stopLoop:
			break loop
		}
	}

	// In the event that the dispatch loop stopped without being canceled, we need to close the job queue to signal
	// workers to shut down.
	t.close()

	// Wait for workers to shutdown gracefully.
	t.workersWG.Wait()
}

// Run runs all the tests registered with the Register function using the given amount of workers.
func Run(workers int) {
	if workers < 1 {
		panic("e2e: Run needs at least one worker")
	}

	t := &testRun{
		mu:           new(sync.Mutex),
		jobQueue:     make(chan *testEntry, 0),
		jobQueueOnce: new(sync.Once),
		workersWG:    new(sync.WaitGroup),
		cancelFuncs:  make(map[int]context.CancelFunc),
	}

	t.workersWG.Add(workers)

	// Spawn workers
	for i := 0; i < workers; i++ {
		runnerID := i + 1
		runnerName := fmt.Sprintf("W%d", runnerID)

		go t.startRunner(runnerID, runnerName)
	}

	// Dispatch jobs and wait for workers to finish.
	t.dispatch()

	// Tally the results
	total := len(registry)
	success := 0
	failure := 0
	for _, entry := range registry {
		if entry.result != nil {
			if entry.result.failure != nil {
				failure++
			} else {
				success++
			}
		}
	}

	fmt.Println()
	fmt.Printf("Success: %d\n", success)
	fmt.Printf("Failure: %d\n", failure)
	fmt.Printf("Total: %d\n", total)

	if failure > 0 {
		os.Exit(1)
	}
}
