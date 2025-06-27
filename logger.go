package ntest

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// helperTracker keeps track of functions marked as helpers
type helperTracker struct {
	helpers map[string]bool
	mu      sync.RWMutex
}

func newHelperTracker() *helperTracker {
	return &helperTracker{
		helpers: make(map[string]bool),
	}
}

func (ht *helperTracker) markHelper() {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	// Get the caller's function name (the function that called Helper())
	pc, _, _, ok := runtime.Caller(2) // Skip markHelper, Helper method
	if ok {
		frames := runtime.CallersFrames([]uintptr{pc})
		frame, _ := frames.Next()
		if frame.Function != "" {
			ht.helpers[frame.Function] = true
		}
	}
}

func (ht *helperTracker) isHelper(funcName string) bool {
	ht.mu.RLock()
	defer ht.mu.RUnlock()
	return ht.helpers[funcName]
}

type loggerT[ET T] struct {
	T
	logger func(string)
}

// bufferedLoggerT extends loggerT with helper tracking for buffered logging
type bufferedLoggerT[ET T] struct {
	loggerT[ET]
	helperTracker *helperTracker
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
	return &loggerT[ET]{
		T:      t,
		logger: logger,
	}
}

func (t loggerT[ET]) Log(args ...interface{}) {
	t.T.Helper()
	line := fmt.Sprintln(args...)
	message := line[0 : len(line)-1]
	t.logger(message)
}

func (t loggerT[ET]) Logf(format string, args ...interface{}) {
	t.T.Helper()
	message := fmt.Sprintf(format, args...)
	t.logger(message)
}

// Run implements the runner interface
// Note: This passes the raw *testing.T to the function, losing logger wrapping.
// Use RunWithReWrap instead if you need to preserve logger wrapping in subtests.
func (t loggerT[ET]) Run(name string, f func(*testing.T)) bool {
	if runnable, ok := t.T.(interface {
		Run(string, func(*testing.T)) bool
	}); ok {
		return runnable.Run(name, f)
	}
	//nolint:staticcheck // QF1008: could remove embedded field "T" from selector
	t.T.Logf("Run not supported by %T", t.T)
	//nolint:staticcheck // QF1008: could remove embedded field "T" from selector
	t.T.Fail()
	return false
}

func (t loggerT[ET]) Parallel() {
	Parallel(t.T)
}

// ReWrap implements ReWrapper to recreate loggerT with fresh T
func (t loggerT[ET]) ReWrap(newT T) T {
	if reWrapper, ok := t.T.(ReWrapper); ok {
		rewrapped := reWrapper.ReWrap(newT)
		return ReplaceLogger(rewrapped, t.logger)
	}
	return ReplaceLogger(newT, t.logger)
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

type bufferedLogEntry struct {
	message string
	file    string
	line    int
}

// createBufferedLoggerWithHelperTracking creates a logger function that buffers log entries
// and outputs them during cleanup if the test fails, tracking helper functions
func createBufferedLoggerWithHelperTracking[ET T](t ET, helperTracker *helperTracker) func(string) {
	entries := make([]bufferedLogEntry, 0)
	var cleanupCalled bool
	var lock sync.Mutex

	// Register cleanup function to output buffered logs if test failed
	t.Cleanup(func() {
		lock.Lock()
		defer lock.Unlock()
		cleanupCalled = true
		if (t.Failed() || t.Skipped()) && len(entries) > 0 {
			var buffer strings.Builder
			var size int
			for _, entry := range entries {
				size += 9 + len(entry.file) + len(entry.message)
			}
			buffer.Grow(size)
			_, _ = buffer.Write([]byte("=== Buffered Log Output (test failed) ===\n"))
			for _, entry := range entries {
				_, _ = fmt.Fprintf(&buffer, "%s:%d %s\n", entry.file, entry.line, entry.message)
			}
			_, _ = buffer.Write([]byte("=== End Buffered Log Output ===\n"))
			t.Log(buffer.String())
		} else {
			t.Logf("dropping %d log entries (test passed)", len(entries))
		}
	})

	return func(message string) {
		// Get caller information, walking up the stack to find the first non-helper function
		var file string
		var line int

		// Get multiple frames at once and walk through them
		const maxFrames = 32
		pcs := make([]uintptr, maxFrames)
		n := runtime.Callers(2, pcs) // Skip this lambda + loggerT.Log/Logf
		frames := runtime.CallersFrames(pcs[:n])

		for {
			frame, more := frames.Next()

			// Skip internal logger functions and marked helpers
			if !helperTracker.isHelper(frame.Function) &&
				!strings.Contains(frame.Function, "loggerT[") {
				file = frame.File
				line = frame.Line
				// Get just the filename, not the full path
				if idx := strings.LastIndex(file, "/"); idx >= 0 {
					file = file[idx+1:]
				}
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

		lock.Lock()
		defer lock.Unlock()

		if cleanupCalled {
			t.Helper()
			t.Logf("[%s:%d] %s", file, line, message)
		} else {
			entries = append(entries, entry)
		}
	}
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
		loggerT: loggerT[ET]{
			T: t,
		},
		helperTracker: newHelperTracker(),
	}

	// Create the logger function that uses the helper tracker
	wrapped.logger = createBufferedLoggerWithHelperTracking(t, wrapped.helperTracker)

	return wrapped
}

// Helper method for bufferedLoggerT that tracks helpers
func (t bufferedLoggerT[ET]) Helper() {
	// Mark the caller as a helper in our tracker
	t.helperTracker.markHelper()
	// Also call the underlying T's Helper method
	t.T.Helper()
}
