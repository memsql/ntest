package ntest

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

type LoggerT[ET T] struct {
	runTHelper    // Embeds T and provides Fail/Parallel
	orig       ET // Keep reference to original for Run method
	logger     func(string)
	skipFrames int // Additional skip frames for nested wrappers
}

// ReplaceLogger creates a logger wrapper that overrides the logging function.
// Returns RunT[LoggerT] for consistency with other logger wrappers.
func ReplaceLogger[ET T](t ET, logger func(string)) RunT[LoggerT[ET]] {
	wrapped := LoggerT[ET]{
		runTHelper: runTHelper{T: t}, // Set the embedded helper
		orig:       t,                // Keep reference to original
		logger:     logger,
		skipFrames: 0, // Initialize skip frames
	}
	return wrapped // LoggerT[ET] implements RunT[LoggerT[ET]]
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

// RunT methods
func (t LoggerT[ET]) Run(name string, f func(LoggerT[ET])) bool {
	if runT, ok := any(t.orig).(RunT[ET]); ok {
		return runT.Run(name, func(innerT ET) {
			inner := LoggerT[ET]{
				runTHelper: runTHelper{T: innerT},
				orig:       innerT,
				logger:     t.logger,
				skipFrames: t.skipFrames, // Preserve skip frames
			}
			f(inner)
		})
	}
	t.T.Logf("Run not supported by %T", t.orig)
	t.T.FailNow()
	return false
}

// ExtraDetailLogger creates a logger wrapper that adds both a
// prefix and a timestamp to each line that is logged.
// Returns RunT[LoggerT] which implements RunT for use with matrix testing.
func ExtraDetailLogger[ET T](t ET, prefix string) RunT[LoggerT[ET]] {
	logger := func(s string) {
		t.Log(prefix, time.Now().Format("15:04:05"), s)
	}

	wrapped := LoggerT[ET]{
		runTHelper: runTHelper{T: t}, // Set the embedded helper
		orig:       t,                // Keep reference to original
		logger:     logger,
		skipFrames: 0, // Initialize skip frames
	}

	// Adjust skip frames on the underlying T to account for our lambda function
	// ExtraDetailLogger adds 2 extra frames: this LoggerT.Log + the lambda function call
	if adjuster, ok := any(t).(interface{ AdjustSkipFrames(int) }); ok {
		adjuster.AdjustSkipFrames(2) // +2 for LoggerT.Log + lambda function call in ExtraDetailLogger
	}

	return wrapped // LoggerT[ET] implements RunT[LoggerT[ET]]
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

// AsRunT upgrades a T to RunT[LoggerT[T]] for use with matrix testing.
// Use this helper when you have a T (like from BufferedLogger) and need
// to use it with matrix testing functions that expect RunT[LoggerT[T]].
func AsRunT[ET T](t ET) RunT[LoggerT[ET]] {
	// If t is already a LoggerT, return it directly
	if loggerT, ok := any(t).(*LoggerT[ET]); ok {
		return loggerT
	}

	// Otherwise, wrap it in a minimal LoggerT that just passes through
	wrapped := &LoggerT[ET]{
		runTHelper: runTHelper{T: t},
		orig:       t,
		logger: func(message string) {
			t.Log(message)
		},
		skipFrames: 0,
	}
	return wrapped
}
