package e2e

import (
	"context"
	"fmt"
	"os"
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

func runTest(job *testEntry) (result *testResult) {
	result = &testResult{
		failure: nil,
		log:     &Logger{},
	}

	defer func() {
		if r := recover(); r != nil {
			result.failure = r
		}
	}()

	job.f(context.TODO(), result.log)
	return
}

func startRunner(runnerID int, runnerName string, jobs chan *testEntry, wg *sync.WaitGroup) {
	for job := range jobs {
		withPrintLock(func() {
			fmt.Printf("%s: starting test '%s'\n", runnerName, job.name)
		})

		job.result = runTest(job)

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
	wg.Done()
}

// Run runs all the tests registered with the Register function using the given amount of workers.
func Run(workers int) {
	if workers < 1 {
		panic("e2e: Run needs at least one worker")
	}

	jobs := make(chan *testEntry, 0)
	wg := new(sync.WaitGroup)
	wg.Add(workers)

	// Spawn workers
	for i := 0; i < workers; i++ {
		runnerID := i + 1
		runnerName := fmt.Sprintf("W%d", runnerID)

		go startRunner(runnerID, runnerName, jobs, wg)
	}

	// Dispatch jobs and wait for workers to finish.
	for _, entry := range registry {
		jobs <- entry
	}
	close(jobs)
	wg.Wait()

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
