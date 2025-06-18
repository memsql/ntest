package ntest

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

type logWrappedT[ET T] struct {
	runTHelper    // Embeds T and provides Fail/Parallel
	orig       ET // Keep reference to original for Run method
	logger     func(string)
}

// ReplaceLogger creates a T that is wrapped so that the logger is
// overridden with the provided function.
func ReplaceLogger[ET T](t ET, logger func(string)) T {
	self := logWrappedT[ET]{
		runTHelper: runTHelper{T: t}, // Set the embedded helper
		orig:       t,                // Keep reference to original
		logger:     logger,
	}
	return self
}

func (t logWrappedT[ET]) Log(args ...interface{}) {
	line := fmt.Sprintln(args...)
	t.logger(line[0 : len(line)-1])
}

func (t logWrappedT[ET]) Logf(format string, args ...interface{}) {
	t.logger(fmt.Sprintf(format, args...))
}

// AdjustSkipFrames adjusts skip frames on the underlying T if it supports it
// logWrappedT adds 2 frames (Log/Logf method + the lambda function call)
func (t logWrappedT[ET]) AdjustSkipFrames(skip int) {
	if adjuster, ok := any(t.orig).(interface{ AdjustSkipFrames(int) }); ok {
		adjuster.AdjustSkipFrames(skip + 2) // +2 for logWrappedT.Log/Logf + lambda function
	}
}

// RunT methods
func (t logWrappedT[ET]) Run(name string, f func(logWrappedT[ET])) bool {
	if runT, ok := any(t.orig).(RunT[ET]); ok {
		return runT.Run(name, func(innerT ET) {
			inner := logWrappedT[ET]{
				runTHelper: runTHelper{T: innerT},
				orig:       innerT,
				logger:     t.logger,
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
// Returns logWrappedT which implements RunT for use with matrix testing.
func ExtraDetailLogger[ET T](t ET, prefix string) logWrappedT[ET] {
	logger := func(s string) {
		t.Log(prefix, time.Now().Format("15:04:05"), s)
	}

	wrapped := logWrappedT[ET]{
		runTHelper: runTHelper{T: t}, // Set the embedded helper
		orig:       t,                // Keep reference to original
		logger:     logger,
	}

	// Adjust skip frames on the underlying T to account for our wrapper
	if adjuster, ok := any(t).(interface{ AdjustSkipFrames(int) }); ok {
		adjuster.AdjustSkipFrames(0) // This will add 2 frames via logWrappedT.AdjustSkipFrames
	}

	return wrapped
}

type bufferedLogEntry struct {
	message  string
	file     string
	line     int
	function string
}

// BufferedLogWrappedT is the type returned by BufferedLogger
type BufferedLogWrappedT[ET T] struct {
	runTHelper    // Embeds T and provides Fail/Parallel
	orig       ET // Keep reference to original for Run method
	entries    []bufferedLogEntry
	skipFrames int  // Number of additional stack frames to skip
	disabled   bool // Whether buffering is disabled
}

// AdjustSkipFrames changes the number of stack frames to skip when capturing caller info
// BufferedLogWrappedT adds 2 frames (Log/Logf → addLogEntry)
func (t *BufferedLogWrappedT[ET]) AdjustSkipFrames(skip int) {
	t.skipFrames = skip + 2 // +2 for BufferedLogWrappedT.Log/Logf → addLogEntry
}

// RunT methods
func (t *BufferedLogWrappedT[ET]) Run(name string, f func(*BufferedLogWrappedT[ET])) bool {
	if runT, ok := any(t.orig).(RunT[ET]); ok {
		return runT.Run(name, func(innerT ET) {
			inner := &BufferedLogWrappedT[ET]{
				runTHelper: runTHelper{T: innerT},
				orig:       innerT,
				entries:    make([]bufferedLogEntry, 0),
				skipFrames: t.skipFrames,
				disabled:   t.disabled,
			}
			f(inner)
		})
	}
	t.T.Logf("Run not supported by %T", t.orig)
	t.T.FailNow()
	return false
}

// BufferedLogger creates a T that buffers all log output and only
// outputs it during test cleanup if the test failed. Each log entry
// includes the filename and line number where the log was called.
// The purpose of this is for situations where go tests are defaulting
// to -v but output should be supressed anyway.
//
// If the environment variable NTEST_BUFFERING is set to "false", buffering
// will be turned off.
// BufferedLoggerT returns a BufferedLogger that implements RunT for matrix testing
func BufferedLoggerT[ET T](t ET) *BufferedLogWrappedT[ET] {
	if os.Getenv("NTEST_BUFFERING") == "false" {
		// When buffering is disabled, we still need to return the wrapper type
		// but we'll make it pass through to the underlying T directly
		return &BufferedLogWrappedT[ET]{
			runTHelper: runTHelper{T: t},
			orig:       t,
			entries:    make([]bufferedLogEntry, 0),
			skipFrames: 0,
			disabled:   true, // Mark as disabled
		}
	}

	wrapped := &BufferedLogWrappedT[ET]{
		runTHelper: runTHelper{T: t},
		orig:       t,
		entries:    make([]bufferedLogEntry, 0),
		skipFrames: 0,
		disabled:   false,
	}

	// Register cleanup function to output buffered logs if test failed
	t.Cleanup(func() {
		if t.Failed() && len(wrapped.entries) > 0 {
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

// BufferedLogger creates a T that buffers all log output and supports RunT functionality.
func BufferedLogger[ET T](t ET) RunT[T] {
	if os.Getenv("NTEST_BUFFERING") == "false" {
		// When buffering is disabled, return the original T wrapped only for matrix testing
		// This avoids any extra stack frames from the buffering wrapper
		if runT, ok := any(t).(RunT[ET]); ok {
			return NewTestRunner(runT)
		}
		// If t is not already a RunT, we need to create a minimal wrapper
		// This should rarely happen in practice since most T's passed here implement RunT
		simple := simpleRunT[ET]{
			runTHelper: runTHelper{T: t},
			orig:       t,
		}
		return NewTestRunner[ET](simple)
	}

	buffered := BufferedLoggerT(t)
	return tRunWrapper[*BufferedLogWrappedT[ET]]{
		T:     buffered, // Use buffered, not original t
		inner: buffered,
	}
}

func (t *BufferedLogWrappedT[ET]) addLogEntry(message string) {
	// Get caller information (skip 2 frames + additional skipFrames: this function and the Log/Logf wrapper + any additional)
	pc, file, line, ok := runtime.Caller(2 + t.skipFrames)
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

	t.entries = append(t.entries, entry)
}

func (t *BufferedLogWrappedT[ET]) Log(args ...interface{}) {
	line := fmt.Sprintln(args...)
	message := line[0 : len(line)-1] // Remove trailing newline
	t.addLogEntry(message)
}

func (t *BufferedLogWrappedT[ET]) Logf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	t.addLogEntry(message)
}
