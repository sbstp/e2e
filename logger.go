package e2e

import (
	"fmt"
)

// Logger buffers messages sent to it during a test, allowing easy debugging of tests.
type Logger struct {
	buf []string
}

// Printf append a formatted message to the log.
func (l *Logger) Printf(format string, args ...interface{}) {
	l.buf = append(l.buf, fmt.Sprintf(format, args...))
}

// Println append a message to the log.
func (l *Logger) Println(msg string) {
	l.buf = append(l.buf, msg)
}
