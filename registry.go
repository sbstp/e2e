package e2e

import (
	"context"
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

var (
	registry   = make([]*testEntry, 0, 100)
	registryMu = new(sync.Mutex)
)

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
