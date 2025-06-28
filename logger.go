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
	t.Cleanup(func() {
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
		} else {
			t.Logf("dropping %d log entries (test passed)", len(wrapped.entries))
		}
	})

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
	// Mark the caller of Helper as a helper
	bl.markHelper(0)
	MarkHelpers(bl.T)
}

// MarkHelpers examines the Unwrap layers for t, calling FlexHelper(2) on all
// FlexHelper instances it finds. Call MarkHelper from inside your custom
// Helper implementation. Unfortunately, *testing.T doesn't implement FlexHelper.
func MarkHelpers(t T) {
	// Walk the full wrapper chain and call FlexHelper on each level with the same frame number
	current := t
	skipFrames := 2 // Same skip frames for all levels

	for {
		if flexHelper, ok := current.(FlexHelper); ok {
			flexHelper.FlexHelper(skipFrames)
		}
		if reWrapper, ok := current.(ReWrapper); ok {
			current = reWrapper.Unwrap()
			continue
		}
		return
	}
}

// FlexHelper allows types that wrap T to properly propagate helper marking
// through wrapper chains with correct stack frame skipping. In particular,
// BufferedLogger uses FlexHelper to propagate .Helper() calls to underlying
// (Unwrap) loggers using MarkHelpers.
type FlexHelper interface {
	FlexHelper(skipFrames int)
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
func (bl *bufferedLoggerT[ET]) markHelper(skipFrames int) {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	// Get the caller's function name (the function that called Helper())
	pc, _, _, ok := runtime.Caller(2 + skipFrames) // Skip markHelper, Helper/FlexHelper method, plus additional frames
	if ok {
		if _, ok := bl.seen[pc]; ok {
			return
		}
		bl.seen[pc] = struct{}{}
		frames := runtime.CallersFrames([]uintptr{pc})
		frame, _ := frames.Next()
		if frame.Function != "" {
			bl.helpers[frame.Function] = struct{}{}
		}
	}
}

func (bl *bufferedLoggerT[ET]) isHelper(funcName string) bool {
	bl.mu.RLock()
	defer bl.mu.RUnlock()
	_, ok := bl.helpers[funcName]
	return ok
}
