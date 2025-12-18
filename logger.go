package ntest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const failBeforeTimeout = 10 * time.Second

type loggerT[ET T] struct {
	T
	logger func(string)
}

// replaceLogger creates a wrapped loggerT that overrides the logging function.
func replaceLogger[ET T](t ET, logger func(string)) *loggerT[ET] {
	return &loggerT[ET]{
		T:      t,
		logger: logger,
	}
}

// ReplaceLogger creates a wrapped T that overrides the logging function.
// For accurate line number reporting in log output, call t.Helper() at the
// beginning of your logger function to mark it as a helper function.
//
// Example:
//
//	logger := ntest.ReplaceLogger(t, func(s string) {
//	    t.Helper() // Mark this function as a helper for accurate line numbers
//	    t.Log("PREFIX: " + s)
//	})
func ReplaceLogger[ET T](t ET, logger func(string)) T {
	return replaceLogger(t, logger)
}

func (t loggerT[ET]) Log(args ...interface{}) {
	//nolint:staticcheck // QF1008: could remove embedded field "T" from selector
	t.T.Helper()
	line := fmt.Sprintln(args...)
	message := line[0 : len(line)-1]
	t.logger(message)
}

func (t loggerT[ET]) Logf(format string, args ...interface{}) {
	//nolint:staticcheck // QF1008: could remove embedded field "T" from selector
	t.T.Helper()
	message := fmt.Sprintf(format, args...)
	t.logger(message)
}

// ReWrap implements ReWrapper to recreate loggerT with fresh T
func (t loggerT[ET]) ReWrap(newT T) T {
	return ReplaceLogger(newT, t.logger)
}

// Unwrap implements ReWrapper to return the wrapped T
func (t loggerT[ET]) Unwrap() T {
	return t.T
}

// ExtraDetailLogger creates a logger wrapper that adds both a
// prefix and a timestamp to each line that is logged. A space after
// the prefix is also added.
func ExtraDetailLogger[ET T](t ET, prefix string) T {
	return ReplaceLogger(t, func(s string) {
		t.Helper() // Mark this wrapper function as a helper
		t.Logf("%s %s %s", prefix, time.Now().Format("15:04:05"), s)
	})
}

// bufferedLoggerT wraps T and adds helper tracking for buffered logging
type bufferedLoggerT[ET T] struct {
	T
	helpers       map[string]struct{}
	seen          map[uintptr]struct{}
	mu            sync.RWMutex
	entries       []bufferedLogEntry
	cleanupCalled bool
	entryLock     sync.Mutex
}

type bufferedLogEntry struct {
	message string
	file    string
	line    int
}

// BufferedLogger creates a logger wrapper that buffers all log output and only
// outputs it during test cleanup if the test failed. Each log entry
// includes the filename and line number where the log was called.
// The purpose of this is for situations where go tests are defaulting
// to -v but output should be suppressed anyway.
//
// If the environment variable NTEST_BUFFERING is set to "false", buffering
// will be turned off and the original T will be returned directly.
//
// One advantage of using BufferedLogger over using "go test" (without -v) is
// that you can see the skipped tests with BufferedLogger whereas non-v go test
// hides the skips.
func BufferedLogger[ET T](t ET) T {
	if os.Getenv("NTEST_BUFFERING") == "false" {
		// When buffering is disabled, return the original T directly to avoid any intermediate calls
		return t
	}

	wrapped := &bufferedLoggerT[ET]{
		T:       t,
		helpers: make(map[string]struct{}),
		seen:    make(map[uintptr]struct{}),
		entries: make([]bufferedLogEntry, 0),
	}

	// Register cleanup function to output buffered logs if test failed
	flush := func() {
		wrapped.entryLock.Lock()
		defer wrapped.entryLock.Unlock()
		wrapped.cleanupCalled = true
		if (t.Failed() || t.Skipped()) && len(wrapped.entries) > 0 {
			var buffer strings.Builder
			var size int
			for _, entry := range wrapped.entries {
				size += 9 + len(entry.file) + len(entry.message)
			}
			buffer.Grow(size)
			_, _ = buffer.Write([]byte("=== Buffered Log Output (test failed) ===\n"))
			for _, entry := range wrapped.entries {
				_, _ = fmt.Fprintf(&buffer, "%s:%d %s\n", entry.file, entry.line, entry.message)
			}
			_, _ = buffer.Write([]byte("=== End Buffered Log Output ===\n"))
			t.Log(buffer.String())
			wrapped.entries = make([]bufferedLogEntry, 0)
		} else {
			t.Logf("dropping %d log entries (test passed)", len(wrapped.entries))
		}
	}
	t.Cleanup(flush)
	if deadline, ok := callDeadline(t); ok {
		timer := time.NewTimer(time.Until(deadline) - failBeforeTimeout)
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		go func() {
			select {
			case <-timer.C:
				t.Error("test is about to time out, flushing logs")
				flush()
			case <-done:
				timer.Stop()
			}
		}()
	}
	return wrapped
}

// logMessage handles the actual logging logic for buffered logging
func (bl *bufferedLoggerT[ET]) logMessage(message string) {
	// Get caller information, walking up the stack to find the first non-helper function
	var file string
	var line int

	// Get multiple frames at once and walk through them
	const maxFrames = 32
	pcs := make([]uintptr, maxFrames)
	n := runtime.Callers(3, pcs) // Skip logMessage + Log/Logf + caller
	frames := runtime.CallersFrames(pcs[:n])

	for {
		frame, more := frames.Next()

		// Skip internal logger functions and marked helpers
		if !bl.isHelper(frame.Function) {
			file = frame.File
			line = frame.Line
			// Get just the filename, not the full path
			file = filepath.Base(file)
			break
		}

		if !more {
			file = "unknown"
			line = 0
			break
		}
	}

	entry := bufferedLogEntry{
		message: message,
		file:    file,
		line:    line,
	}

	bl.entryLock.Lock()
	defer bl.entryLock.Unlock()

	if bl.cleanupCalled {
		bl.T.Helper()
		bl.T.Logf("[%s:%d] %s", file, line, message)
	} else {
		bl.entries = append(bl.entries, entry)
	}
}

// Log method for bufferedLoggerT that uses the buffered logger function
func (bl *bufferedLoggerT[ET]) Log(args ...interface{}) {
	bl.Helper() // Call our own Helper method to track this as a helper
	line := fmt.Sprintln(args...)
	message := line[0 : len(line)-1]
	bl.logMessage(message)
}

// Logf method for bufferedLoggerT that uses the buffered logger function
func (bl *bufferedLoggerT[ET]) Logf(format string, args ...interface{}) {
	bl.Helper() // Call our own Helper method to track this as a helper
	message := fmt.Sprintf(format, args...)
	bl.logMessage(message)
}

// Helper method for bufferedLoggerT that tracks helpers
func (bl *bufferedLoggerT[ET]) Helper() {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	// Walk up the stack, skipping over any functions named "Helper"
	// to find the actual caller that should be marked as a helper
	const maxFrames = 32
	pcs := make([]uintptr, maxFrames)
	n := runtime.Callers(2, pcs) // Skip Helper and its caller
	frames := runtime.CallersFrames(pcs[:n])

	for {
		frame, more := frames.Next()

		// Skip over functions named "Helper" to find the real caller
		funcName := frame.Function
		if funcName != "" && !strings.HasSuffix(funcName, ".Helper") {
			// This is the actual caller we want to mark as a helper
			if _, ok := bl.seen[frame.PC]; !ok {
				bl.seen[frame.PC] = struct{}{}
				bl.helpers[funcName] = struct{}{}
			}
			break
		}

		if !more {
			break
		}
	}

	// Propagate Helper call to wrapped T
	bl.T.Helper()
}

// ReWrap implements ReWrapper to recreate bufferedLoggerT with fresh T
func (bl *bufferedLoggerT[ET]) ReWrap(newT T) T {
	return BufferedLogger(newT)
}

// Unwrap implements ReWrapper to return the wrapped T
func (bl *bufferedLoggerT[ET]) Unwrap() T {
	return bl.T
}

// Helper tracking methods for bufferedLoggerT
func (bl *bufferedLoggerT[ET]) isHelper(funcName string) bool {
	bl.mu.RLock()
	defer bl.mu.RUnlock()
	_, ok := bl.helpers[funcName]
	return ok
}
