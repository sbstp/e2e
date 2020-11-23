package e2e

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
)

// TestFunc represents a single test. The context sent to the function represents the test's timeout, if any.
// If the test uses shared resources, and multiple workers are configured in the Run function, make sure that
// those shared resources are thread-safe or protected by a Mutex, since multiple tests using those resources
// could be running at once in different goroutines.
type TestFunc func(ctx context.Context, log *Logger)

type testResult struct {
	failure interface{}
	log     *Logger
}

type testEntry struct {
	f      TestFunc
	name   string
	result *testResult
}

var registry = make([]*testEntry, 0, 100)
var registryMu = new(sync.Mutex)
var printMu = new(sync.Mutex)

// Register registers a test to be run by the test suite.
func Register(name string, f TestFunc) {
	registryMu.Lock()
	defer registryMu.Unlock()

	registry = append(registry, &testEntry{
		f:      f,
		name:   name,
		result: nil,
	})
}

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
	// Number of workers.
	workers int
	// Job queue, a channel with test entries in it to distribute work to workers.
	jobQueue chan *testEntry
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
	t.workersWG.Add(1)

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

func (t *testRun) dispatch() {
	term := make(chan os.Signal)
	signal.Notify(term, os.Kill, os.Interrupt)

	for _, entry := range registry {
		select {
		case t.jobQueue <- entry:
		case <-term:
			// If the program is terminated, we return which will initiate a shutdown of running tests,
			// and cancel the contexts of tests currently running.
			fmt.Println("\nShutting down...")
			return
		}
	}
}

func (t *testRun) shutdown() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Cancel context of any running test
	for _, cancelFunc := range t.cancelFuncs {
		cancelFunc()
	}

	close(t.jobQueue)
}

func (t *testRun) wait() {
	t.workersWG.Wait()
}

// Run runs all the tests registered with the Register function using the given amount of workers.
func Run(workers int) {
	if workers < 1 {
		panic("e2e: Run needs at least one worker")
	}

	t := &testRun{
		mu:          new(sync.Mutex),
		workers:     workers,
		jobQueue:    make(chan *testEntry, 0),
		workersWG:   new(sync.WaitGroup),
		cancelFuncs: make(map[int]context.CancelFunc),
	}

	// Spawn workers
	for i := 0; i < workers; i++ {
		runnerID := i + 1
		runnerName := fmt.Sprintf("W%d", runnerID)

		go t.startRunner(runnerID, runnerName)
	}

	// Dispatch jobs and wait for workers to finish.
	t.dispatch()
	t.shutdown()
	t.wait()

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
