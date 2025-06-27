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
	if reWrapper, ok := t.T.(ReWrapper); ok {
		rewrapped := reWrapper.ReWrap(newT)
		return ReplaceLogger(rewrapped, t.logger)
	}
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

// helperTracker keeps track of functions marked as helpers
type helperTracker struct {
	helpers map[string]struct{}
	seen    map[uintptr]struct{}
	mu      sync.RWMutex
}

func newHelperTracker() *helperTracker {
	return &helperTracker{
		helpers: make(map[string]struct{}),
		seen:    make(map[uintptr]struct{}),
	}
}

func (ht *helperTracker) markHelper() {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	// Get the caller's function name (the function that called Helper())
	pc, _, _, ok := runtime.Caller(2) // Skip markHelper, Helper method
	if ok {
		if _, ok := ht.seen[pc]; ok {
			return
		}
		ht.seen[pc] = struct{}{}
		frames := runtime.CallersFrames([]uintptr{pc})
		frame, _ := frames.Next()
		if frame.Function != "" {
			ht.helpers[frame.Function] = struct{}{}
		}
	}
}

func (ht *helperTracker) isHelper(funcName string) bool {
	ht.mu.RLock()
	defer ht.mu.RUnlock()
	_, ok := ht.helpers[funcName]
	return ok
}

// bufferedLoggerT extends loggerT with helper tracking for buffered logging
type bufferedLoggerT[ET T] struct {
	loggerT[ET]
	helperTracker *helperTracker
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

	helperTracker := newHelperTracker()
	loggerFunc := createBufferedLoggerWithHelperTracking(t, helperTracker)

	wrapped := &bufferedLoggerT[ET]{
		loggerT:       *replaceLogger(t, loggerFunc),
		helperTracker: helperTracker,
	}

	return wrapped
}

// Helper method for bufferedLoggerT that tracks helpers
func (t bufferedLoggerT[ET]) Helper() {
	// Mark the caller as a helper in our tracker
	t.helperTracker.markHelper()
	// Also call the underlying T's Helper method
	t.T.Helper()
}
