package ntest

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

type logWrappedT[ET T] struct {
	eitherT[logWrappedT[ET], ET]
	logger func(string)
}

// ReplaceLogger creates a T that is wrapped so that the logger is
// overridden with the provided function.
func ReplaceLogger[ET T](t ET, logger func(string)) T {
	self := logWrappedT[ET]{
		logger: logger,
	}
	self.eitherT = makeEitherTSimple[ET](t, self)
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
func (t logWrappedT[ET]) AdjustSkipFrames(skip int) {
	if adjuster, ok := any(t.eitherT).(interface{ AdjustSkipFrames(int) }); ok {
		adjuster.AdjustSkipFrames(skip)
	}
}

// ExtraDetailLogger creates a logger wrapper that adds both a
// prefix and a timestamp to each line that is logged.
// Returns logWrappedT which implements RunT for use with matrix testing.
func ExtraDetailLogger[ET T](t ET, prefix string) logWrappedT[ET] {
	// First, adjust skip frames on the underlying T if it supports it
	// We need to skip 2 additional frames: the lambda function and the ReplaceLogger wrapper
	skipFrames := 2

	// Check if the underlying T is a tRunWrapper by checking if it has the specific method signature
	// This is a bit of a hack, but it works to detect tRunWrapper without generics issues
	tValue := any(t)
	if wrapper, ok := tValue.(interface {
		Run(string, func(T)) bool
	}); ok {
		// This looks like a tRunWrapper, so we need one more skip frame for tRunWrapper.Log
		_ = wrapper // avoid unused variable
		skipFrames = 3
	}

	if adjuster, ok := any(t).(interface{ AdjustSkipFrames(int) }); ok {
		adjuster.AdjustSkipFrames(skipFrames)
	}

	logger := func(s string) {
		t.Log(prefix, time.Now().Format("15:04:05"), s)
	}

	// Create a constructor function that captures the logger
	makeLogWrapped := func(either eitherT[logWrappedT[ET], ET]) logWrappedT[ET] {
		return logWrappedT[ET]{
			eitherT: either,
			logger:  logger,
		}
	}

	wrapped := logWrappedT[ET]{
		eitherT: makeEitherT[logWrappedT[ET], ET](t, makeLogWrapped),
		logger:  logger,
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
	eitherT[*BufferedLogWrappedT[ET], ET]
	entries    []bufferedLogEntry
	skipFrames int // Number of additional stack frames to skip
}

// AdjustSkipFrames changes the number of stack frames to skip when capturing caller info
func (t *BufferedLogWrappedT[ET]) AdjustSkipFrames(skip int) {
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
// BufferedLoggerT returns a BufferedLogger that implements RunT for matrix testing
func BufferedLoggerT[ET T](t ET) *BufferedLogWrappedT[ET] {
	if os.Getenv("NTEST_BUFFERING") == "false" {
		// When buffering is disabled, we still need to return the wrapper type
		// but we'll make it pass through to the underlying T with proper stack frame handling
		wrapped := &BufferedLogWrappedT[ET]{
			entries:    make([]bufferedLogEntry, 0),
			skipFrames: 2, // Account for the wrapper stack frames
		}
		wrapped.eitherT = makeEitherTSimple[ET](t, wrapped)
		return wrapped
	}

	// Create a constructor function for the buffered wrapper
	makeBufferedWrapped := func(either eitherT[*BufferedLogWrappedT[ET], ET]) *BufferedLogWrappedT[ET] {
		return &BufferedLogWrappedT[ET]{
			eitherT:    either,
			entries:    make([]bufferedLogEntry, 0),
			skipFrames: 0,
		}
	}

	wrapped := &BufferedLogWrappedT[ET]{
		eitherT:    makeEitherT[*BufferedLogWrappedT[ET], ET](t, makeBufferedWrapped),
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

// BufferedLogger creates a T that buffers all log output and supports RunT functionality.
func BufferedLogger[ET T](t ET) RunT[T] {
	buffered := BufferedLoggerT(t)
	// When wrapping with tRunWrapper, we need to skip one additional frame
	// because tRunWrapper.Log() adds another level to the call stack
	buffered.AdjustSkipFrames(1)
	return tRunWrapper[*BufferedLogWrappedT[ET]]{inner: buffered}
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
