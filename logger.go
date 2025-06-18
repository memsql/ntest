package ntest

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

type LoggerT[ET T] struct {
	runTHelper    // Embeds T and provides Fail/Parallel
	orig       ET // Keep reference to original for Run method
	logger     func(string)
	skipFrames int // Additional skip frames for nested wrappers
}

// ReplaceLogger creates a logger wrapper that overrides the logging function.
func ReplaceLogger[ET T](t ET, logger func(string)) T {
	wrapped := LoggerT[ET]{
		runTHelper: runTHelper{T: t}, // Set the embedded helper
		orig:       t,                // Keep reference to original
		logger:     logger,
		skipFrames: 0, // Initialize skip frames
	}
	return any(wrapped).(T)
}

func (t LoggerT[ET]) Log(args ...interface{}) {
	line := fmt.Sprintln(args...)
	t.logger(line[0 : len(line)-1])
}

func (t LoggerT[ET]) Logf(format string, args ...interface{}) {
	t.logger(fmt.Sprintf(format, args...))
}

// AdjustSkipFrames adjusts skip frames on this LoggerT instance
func (t *LoggerT[ET]) AdjustSkipFrames(skip int) {
	t.skipFrames += skip
	// Also forward to the underlying T if it supports AdjustSkipFrames
	if adjuster, ok := any(t.orig).(interface{ AdjustSkipFrames(int) }); ok {
		adjuster.AdjustSkipFrames(skip)
	}
}

// Run implements the new RunT interface that expects func(*testing.T)
func (t LoggerT[ET]) Run(name string, f func(*testing.T)) bool {
	// Delegate to the underlying orig's Run method
	if runnable, ok := any(t.orig).(interface {
		Run(string, func(*testing.T)) bool
	}); ok {
		return runnable.Run(name, f)
	}
	t.T.Logf("Run not supported by %T", t.orig)
	t.T.FailNow()
	return false
}

// ReWrap implements ReWrapper to recreate LoggerT with fresh *testing.T
func (t LoggerT[ET]) ReWrap(newT *testing.T) T {
	// Simple approach: let the underlying layer handle ReWrap if it supports it
	if reWrapper, ok := any(t.orig).(ReWrapper); ok {
		reWrapper.ReWrap(newT)
	}

	// Create new LoggerT with the same logger function but fresh underlying
	wrapped := &LoggerT[*testing.T]{
		runTHelper: runTHelper{T: newT},
		orig:       newT,
		logger:     t.logger, // Reuse the same logger function
		skipFrames: t.skipFrames,
	}

	return any(wrapped).(T)
}

// ExtraDetailLoggerT directly implements T interface to avoid double LoggerT wrapping
type ExtraDetailLoggerT[ET T] struct {
	runTHelper
	orig   ET
	prefix string
}

// ExtraDetailLogger creates a logger wrapper that adds both a
// prefix and a timestamp to each line that is logged.
func ExtraDetailLogger[ET T](t ET, prefix string) T {
	// If the underlying logger supports AdjustSkipFrames, adjust it to account for
	// the extra call frame that ExtraDetailLoggerT.Log will add
	if adjuster, ok := any(t).(interface{ AdjustSkipFrames(int) }); ok {
		adjuster.AdjustSkipFrames(1)
	}

	wrapped := &ExtraDetailLoggerT[ET]{
		runTHelper: runTHelper{T: t},
		orig:       t,
		prefix:     prefix,
	}
	return any(wrapped).(T)
}

func (t ExtraDetailLoggerT[ET]) Log(args ...interface{}) {
	line := fmt.Sprintln(args...)
	message := line[0 : len(line)-1]
	prefixedMessage := fmt.Sprintf("%s %s %s", t.prefix, time.Now().Format("15:04:05"), message)
	t.orig.Log(prefixedMessage)
}

func (t ExtraDetailLoggerT[ET]) Logf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	prefixedMessage := fmt.Sprintf("%s %s %s", t.prefix, time.Now().Format("15:04:05"), message)
	t.orig.Log(prefixedMessage)
}

// Run implements the new RunT interface
func (t ExtraDetailLoggerT[ET]) Run(name string, f func(*testing.T)) bool {
	if runnable, ok := any(t.orig).(interface {
		Run(string, func(*testing.T)) bool
	}); ok {
		return runnable.Run(name, f)
	}
	t.T.Logf("Run not supported by %T", t.orig)
	t.T.FailNow()
	return false
}

// ReWrap implements ReWrapper to recreate ExtraDetailLoggerT with fresh *testing.T
func (t ExtraDetailLoggerT[ET]) ReWrap(newT *testing.T) T {
	if reWrapper, ok := any(t.orig).(ReWrapper); ok {
		rewrapped := reWrapper.ReWrap(newT)
		return ExtraDetailLogger(rewrapped, t.prefix)
	}
	return ExtraDetailLogger(newT, t.prefix)
}

// AdjustSkipFrames forwards to the underlying logger if it supports it
func (t *ExtraDetailLoggerT[ET]) AdjustSkipFrames(skip int) {
	if adjuster, ok := any(t.orig).(interface{ AdjustSkipFrames(int) }); ok {
		adjuster.AdjustSkipFrames(skip)
	}
}

type bufferedLogEntry struct {
	message  string
	file     string
	line     int
	function string
}

// createBufferedLoggerWithDynamicSkip creates a logger function that buffers log entries
// and outputs them during cleanup if the test fails, using a dynamic skip frames function
func createBufferedLoggerWithDynamicSkip[ET T](t ET, skipFramesFunc func() int) func(string) {
	if os.Getenv("NTEST_BUFFERING") == "false" {
		// When buffering is disabled, we still need to provide accurate line numbers
		// but log immediately instead of buffering
		return func(message string) {
			// Get caller information with proper skip frames
			skipFrames := skipFramesFunc()
			_, file, line, ok := runtime.Caller(2 + skipFrames)
			if ok {
				// Get just the filename, not the full path
				if idx := strings.LastIndex(file, "/"); idx >= 0 {
					file = file[idx+1:]
				}
				// Log with file:line prefix to preserve line number information
				t.Logf("%s:%d %s", file, line, message)
			} else {
				// Fallback if we can't get caller info
				t.Log(message)
			}
		}
	}

	entries := make([]bufferedLogEntry, 0)

	// Register cleanup function to output buffered logs if test failed
	t.Cleanup(func() {
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
		// Stack: runtime.Caller <- this lambda <- LoggerT.Log/Logf <- user code
		// We need to skip: this function (1) + LoggerT.Log/Logf (1) + any additional frames (skipFramesFunc())
		skipFrames := skipFramesFunc()
		pc, file, line, ok := runtime.Caller(2 + skipFrames)
		if !ok {
			file = "unknown"
			line = 0
		} else {
			// Get just the filename, not the full path
			if idx := strings.LastIndex(file, "/"); idx >= 0 {
				file = file[idx+1:]
			}
		}

		var function string
		if pc != 0 {
			if fn := runtime.FuncForPC(pc); fn != nil {
				function = fn.Name()
				// Strip package path from function name
				if idx := strings.LastIndex(function, "."); idx >= 0 {
					function = function[idx+1:]
				}
			}
		}

		entry := bufferedLogEntry{
			message:  message,
			file:     file,
			line:     line,
			function: function,
		}

		entries = append(entries, entry)
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
// Returns T for direct use, or use AsRunT helper to convert to RunT[LoggerT[T]] for matrix testing.
func BufferedLogger[ET T](t ET) T {
	if os.Getenv("NTEST_BUFFERING") == "false" {
		// When buffering is disabled, return the original T directly to avoid any intermediate calls
		return t
	}

	wrapped := &LoggerT[ET]{
		runTHelper: runTHelper{T: t}, // Set the embedded helper
		orig:       t,                // Keep reference to original
		skipFrames: 0,                // Initialize skip frames, will be adjusted by AdjustSkipFrames
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
