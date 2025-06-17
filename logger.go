package ntest

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

type logWrappedT struct {
	T
	logger func(string)
}

// ReplaceLogger creates a T that is wrapped so that the logger is
// overridden with the provided function.
func ReplaceLogger(t T, logger func(string)) T {
	return logWrappedT{
		T:      t,
		logger: logger,
	}
}

func (t logWrappedT) Log(args ...interface{}) {
	line := fmt.Sprintln(args...)
	t.logger(line[0 : len(line)-1])
}

func (t logWrappedT) Logf(format string, args ...interface{}) {
	t.logger(fmt.Sprintf(format, args...))
}

// AdjustSkipFrames adjusts skip frames on the underlying T if it supports it
func (t logWrappedT) AdjustSkipFrames(skip int) {
	if adjuster, ok := t.T.(interface{ AdjustSkipFrames(int) }); ok {
		adjuster.AdjustSkipFrames(skip)
	}
}

// ExtraDetailLogger creates a T that wraps the logger to add both a
// prefix and a timestamp to each line that is logged.
func ExtraDetailLogger(t T, prefix string) T {
	// First, adjust skip frames on the underlying T if it supports it
	// We need to skip 2 additional frames: the lambda function and the ReplaceLogger wrapper
	if adjuster, ok := t.(interface{ AdjustSkipFrames(int) }); ok {
		adjuster.AdjustSkipFrames(2)
	}

	wrapped := ReplaceLogger(t, func(s string) {
		t.Log(prefix, time.Now().Format("15:04:05"), s)
	})

	return wrapped
}

type bufferedLogEntry struct {
	message  string
	file     string
	line     int
	function string
}

type bufferedLogWrappedT struct {
	T
	entries    []bufferedLogEntry
	skipFrames int // Number of additional stack frames to skip
}

// AdjustSkipFrames changes the number of stack frames to skip when capturing caller info
func (t *bufferedLogWrappedT) AdjustSkipFrames(skip int) {
	t.skipFrames = skip
}

// BufferedLogger creates a T that buffers all log output and only
// outputs it during test cleanup if the test failed. Each log entry
// includes the filename and line number where the log was called.
// The purpose of this is for situations where go tests are defaulting
// to -v but output should be supressed anyway.
//
// If the environment variable NTEST_BUFFERING is set to "false", buffering
// will be turned off.
func BufferedLogger(t T) T {
	if os.Getenv("NTEST_BUFFERING") == "false" {
		return t
	}
	wrapped := &bufferedLogWrappedT{
		T:          t,
		entries:    make([]bufferedLogEntry, 0),
		skipFrames: 0,
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

func (t *bufferedLogWrappedT) addLogEntry(message string) {
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

func (t *bufferedLogWrappedT) Log(args ...interface{}) {
	line := fmt.Sprintln(args...)
	message := line[0 : len(line)-1] // Remove trailing newline
	t.addLogEntry(message)
}

func (t *bufferedLogWrappedT) Logf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	t.addLogEntry(message)
}
