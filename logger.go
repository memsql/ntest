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

type loggerT[ET T] struct {
	replaceLoggerT[ET]     // Embed replaceLoggerT for basic functionality
	skipFrames         int // Additional skip frames for nested wrappers
}

// replaceLoggerT directly implements T interface to avoid double loggerT wrapping
type replaceLoggerT[ET T] struct {
	T
	logger func(string)
}

// ReplaceLogger creates a logger wrapper that overrides the logging function. When layered
// on top of BufferedLogger, it assumes that only one extra stack frame is added. If that's
// not the case, cast and adjust:
//
//	if asf, ok := t.(interface{ AdjustSkipFrames(int) }); ok {
//		asf.AdjustSkipFrames(2)
//	}
//
// This adjustment should be done before using the the returned T
func ReplaceLogger[ET T](t ET, logger func(string)) T {
	// If the underlying logger supports AdjustSkipFrames, adjust it to account for
	// the extra call frames: replaceLoggerT.Log -> custom logger function -> underlying logger call
	if adjuster, ok := any(t).(interface{ AdjustSkipFrames(int) }); ok {
		adjuster.AdjustSkipFrames(2) // +2 for replaceLoggerT.Log and the custom logger function
	}

	wrapped := &replaceLoggerT[ET]{
		T:      t,
		logger: logger,
	}
	return any(wrapped).(T)
}

func (t replaceLoggerT[ET]) Log(args ...interface{}) {
	line := fmt.Sprintln(args...)
	message := line[0 : len(line)-1]
	t.logger(message)
}

func (t replaceLoggerT[ET]) Logf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	t.logger(message)
}

// Run implements the new RunT interface
func (t replaceLoggerT[ET]) Run(name string, f func(*testing.T)) bool {
	if runnable, ok := any(t.T).(interface {
		Run(string, func(*testing.T)) bool
	}); ok {
		return runnable.Run(name, f)
	}
	t.T.Logf("Run not supported by %T", t.T)
	//nolint:staticcheck // QF1008: could remove embedded field "T" from selector
	t.T.FailNow()
	return false
}

// ReWrap implements ReWrapper to recreate replaceLoggerT with fresh *testing.T
func (t replaceLoggerT[ET]) ReWrap(newT *testing.T) T {
	if reWrapper, ok := any(t.T).(ReWrapper); ok {
		rewrapped := reWrapper.ReWrap(newT)
		return ReplaceLogger(rewrapped, t.logger)
	}
	return ReplaceLogger(newT, t.logger)
}

// AdjustSkipFrames forwards to the underlying logger if it supports it
func (t *replaceLoggerT[ET]) AdjustSkipFrames(skip int) {
	if adjuster, ok := any(t.T).(interface{ AdjustSkipFrames(int) }); ok {
		adjuster.AdjustSkipFrames(skip)
	}
}

// ReWrap implements ReWrapper to recreate loggerT with fresh *testing.T
func (t loggerT[ET]) ReWrap(newT *testing.T) T {
	// Delegate to the embedded replaceLoggerT's ReWrap to handle chaining properly
	reWrappedBase := t.replaceLoggerT.ReWrap(newT)

	// Since loggerT is essentially a BufferedLogger wrapping a ReplaceLogger,
	// we need to recreate this structure. The embedded replaceLoggerT already
	// handled the ReWrap chaining, so we just need to wrap it in buffering again.
	return BufferedLogger(reWrappedBase)
}

// AdjustSkipFrames adjusts skip frames on this loggerT instance
func (t *loggerT[ET]) AdjustSkipFrames(skip int) {
	t.skipFrames += skip
	// Also forward to the underlying T if it supports AdjustSkipFrames
	if adjuster, ok := any(t.T).(interface{ AdjustSkipFrames(int) }); ok {
		adjuster.AdjustSkipFrames(skip)
	}
}

// ExtraDetailLogger creates a logger wrapper that adds both a
// prefix and a timestamp to each line that is logged. A space after
// the prefix is also added.
func ExtraDetailLogger[ET T](t ET, prefix string) T {
	return ReplaceLogger(t, func(s string) {
		prefixedMessage := fmt.Sprintf("%s %s %s", prefix, time.Now().Format("15:04:05"), s)
		t.Log(prefixedMessage)
	})
}

type bufferedLogEntry struct {
	message string
	file    string
	line    int
}

// createBufferedLoggerWithDynamicSkip creates a logger function that buffers log entries
// and outputs them during cleanup if the test fails, using a dynamic skip frames function
func createBufferedLoggerWithDynamicSkip[ET T](t ET, skipFramesFunc func() int) func(string) {
	entries := make([]bufferedLogEntry, 0)
	var cleanupCalled bool
	var lock sync.Mutex

	// Register cleanup function to output buffered logs if test failed
	t.Cleanup(func() {
		lock.Lock()
		defer lock.Unlock()
		cleanupCalled = true
		if t.Failed() && len(entries) > 0 {
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
		// Get caller information
		// Stack: runtime.Caller <- this lambda <- loggerT.Log/Logf <- user code
		// We need to skip: this function (1) + loggerT.Log/Logf (1) + any additional frames (skipFramesFunc())
		skipFrames := skipFramesFunc()
		_, file, line, ok := runtime.Caller(2 + skipFrames)
		if !ok {
			file = "unknown"
			line = 0
		} else {
			// Get just the filename, not the full path
			if idx := strings.LastIndex(file, "/"); idx >= 0 {
				file = file[idx+1:]
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
// Use AsRunT to upgrade the T into a RunT if using it with matrix tests.
func BufferedLogger[ET T](t ET) T {
	if os.Getenv("NTEST_BUFFERING") == "false" {
		// When buffering is disabled, return the original T directly to avoid any intermediate calls
		return t
	}

	wrapped := &loggerT[ET]{
		replaceLoggerT: replaceLoggerT[ET]{
			T: t, // Direct embedding of T interface
		},
		skipFrames: 0, // Initialize skip frames, will be adjusted by AdjustSkipFrames
	}

	// Create the logger function that uses the current skipFrames from wrapped
	wrapped.logger = createBufferedLoggerWithDynamicSkip(t, func() int { return wrapped.skipFrames })

	return wrapped // Return by reference so AdjustSkipFrames works
}

// AsRunT upgrades a T to RunT for use with matrix testing.
// Use this helper when you have a T and need to use it with matrix testing functions.
func AsRunT[ET T](t ET) RunT {
	// If t already implements RunT, return it directly
	if runT, ok := any(t).(RunT); ok {
		return runT
	}

	// Otherwise, wrap it using NewTestRunner
	return NewTestRunner(t)
}

// ReWrapper allows types to recreate themselves with a fresh *testing.T
// This enables proper sub-test handling in matrix testing while preserving wrapper behavior
type ReWrapper interface {
	ReWrap(*testing.T) T
}
