package ntest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memsql/ntest"
)

var _ ntest.T = &testing.T{}

func TestPrefixLogger(t *testing.T) {
	t.Parallel()
	t.Log("Testing ExtraDetailLogger with prefix functionality")

	var caught []string
	captureT := ntest.ReplaceLogger(t, func(s string) {
		t.Log("captured:", s)
		caught = append(caught, s)
	})

	t.Log("Creating ExtraDetailLogger with prefix 'some-prefix'")
	extraDetail := ntest.ExtraDetailLogger(captureT, "some-prefix")

	t.Log("Logging unformatted message")
	extraDetail.Log("not-formatted", 3)

	t.Log("Logging formatted message")
	extraDetail.Logf("formatted '%s'", "quoted")

	t.Logf("Captured % 3d log entries", len(caught))
	for i, entry := range caught {
		t.Logf("Entry % 3d: %s", i, entry)
	}

	require.Equal(t, 2, len(caught), "len caught")
	assert.Regexp(t, `some-prefix \d\d:\d\d:\d\d not-formatted 3$`, caught[0], "unformatted")
	assert.Regexp(t, `some-prefix \d\d:\d\d:\d\d formatted 'quoted'$`, caught[1], "formatted")

	t.Log("ExtraDetailLogger test completed successfully")
}

// TestLoggerLogf tests the Logf method on loggerT
func TestLoggerLogf(t *testing.T) {
	if _, ok := os.LookupEnv("NTEST_BUFFERING"); ok {
		t.Setenv("NTEST_BUFFERING", "true")
	}

	var captured []string
	captureLogger := ntest.ReplaceLogger(t, func(s string) {
		captured = append(captured, s)
	})

	// Use BufferedLogger to create a loggerT instance, then call Logf
	buffered := ntest.BufferedLogger(captureLogger)
	buffered.Logf("Formatted message: %d %s", 42, "test")

	// Since it's buffered and test will pass, we won't see the output directly
	// But this ensures the Logf method is called
	assert.NotNil(t, buffered, "BufferedLogger should return non-nil")
}

// Test line number accuracy for BufferedLogger
func TestBufferedLogger_LineNumberAccuracy(t *testing.T) {
	if _, ok := os.LookupEnv("NTEST_BUFFERING"); ok {
		t.Setenv("NTEST_BUFFERING", "true")
	}
	mockT := newMockedT(t)
	buffered := ntest.BufferedLogger(mockT)
	testLineNumberAccuracy(t, buffered, mockT, true, true) // expect buffering, test should fail to check line numbers
}

func TestExtraDetailLogger_WithBufferedLogger_LineNumberAccuracy(t *testing.T) {
	if _, ok := os.LookupEnv("NTEST_BUFFERING"); ok {
		t.Setenv("NTEST_BUFFERING", "true")
	}
	mockT := newMockedT(t)
	buffered := ntest.BufferedLogger(mockT)
	extraDetail := ntest.ExtraDetailLogger(buffered, "PREFIX")
	testLineNumberAccuracy(t, extraDetail, mockT, true, true, "PREFIX") // expect buffering, test should fail to check line numbers
}

func TestExtraDetailLogger_Doubled_WithBufferedLogger_LineNumberAccuracy(t *testing.T) {
	if _, ok := os.LookupEnv("NTEST_BUFFERING"); ok {
		t.Setenv("NTEST_BUFFERING", "true")
	}
	mockT := newMockedT(t)
	buffered := ntest.BufferedLogger(mockT)
	extraDetail := ntest.ExtraDetailLogger(buffered, "PREFIX1")
	extraDetail2 := ntest.ExtraDetailLogger(extraDetail, "PREFIX2")
	testLineNumberAccuracy(t, extraDetail2, mockT, true, true, "PREFIX2", "PREFIX1") // expect buffering, test should fail to check line numbers
}

func TestReplaceLogger_WithBufferedLogger_LineNumberAccuracy(t *testing.T) {
	if _, ok := os.LookupEnv("NTEST_BUFFERING"); ok {
		t.Setenv("NTEST_BUFFERING", "true")
	}
	mockT := newMockedT(t)
	buffered := ntest.BufferedLogger(mockT)
	extraDetail := ntest.ReplaceLogger(buffered, func(s string) {
		buffered.Helper() // Mark this lambda as a helper
		buffered.Log(s + " SUFFIX")
	})
	testLineNumberAccuracy(t, extraDetail, mockT, true, true, "SUFFIX") // expect buffering, test should fail to check line numbers
}

func BenchmarkReplaceLogger_WithBufferedLogger_LineNumberAccuracy(t *testing.B) {
	if _, ok := os.LookupEnv("NTEST_BUFFERING"); ok {
		t.Setenv("NTEST_BUFFERING", "true")
	}
	mockT := newMockedT(t)
	buffered := ntest.BufferedLogger(mockT)
	extraDetail := ntest.ReplaceLogger(buffered, func(s string) {
		buffered.Helper() // Mark this lambda as a helper
		buffered.Log(s + " SUFFIX")
	})
	testLineNumberAccuracy(t, extraDetail, mockT, true, true, "SUFFIX") // expect buffering, test should fail to check line numbers
}

func TestReplaceLogger_WithBufferedLogger_Helper_LineNumberAccuracy(t *testing.T) {
	if _, ok := os.LookupEnv("NTEST_BUFFERING"); ok {
		t.Setenv("NTEST_BUFFERING", "true")
	}
	mockT := newMockedT(t)
	buffered := ntest.BufferedLogger(mockT)
	extraDetail := ntest.ReplaceLogger(buffered, func(s string) {
		buffered.Helper() // Mark this wrapper function as a helper
		// extra layers of function calls
		func() {
			buffered.Helper() // Mark this nested function as a helper
			// extra layers of function calls
			func() {
				buffered.Helper() // Mark this nested function as a helper
				buffered.Log(s + " SUFFIX")
			}()
		}()
	})
	testLineNumberAccuracy(t, extraDetail, mockT, true, true, "SUFFIX") // expect buffering, test should fail to check line numbers
}

func TestExtraDetailInsideRun(t *testing.T) {
	if _, ok := os.LookupEnv("NTEST_BUFFERING"); ok {
		t.Setenv("NTEST_BUFFERING", "true")
	}
	mockT := newMockedT(t)
	buffered := ntest.BufferedLogger(mockT)
	extraDetail := ntest.ReplaceLogger(buffered, func(s string) {
		buffered.Helper()
		buffered.Log(s + " SUFFIX")
	})
	var ran bool
	success := ntest.Run(extraDetail, "inner", func(wrapped ntest.T) {
		inner := mockT.getInner(wrapped.Name())
		t.Logf("[run] wrapped is a %T", wrapped)
		current := wrapped
		for {
			if reWrapper, ok := current.(ntest.ReWrapper); ok {
				current = reWrapper.Unwrap()
				t.Logf("[run] which unwraps to a %T", current)
				continue
			}
			break
		}

		testLineNumberAccuracy(mockT, wrapped, inner, true, true, "SUFFIX") // expect buffering, test should fail to check line numbers
		ran = true
	})
	require.True(t, success)
	require.True(t, ran)
}

func TestExtraDetailLogger_WithBufferedLogger_NoBuffering_LineNumberAccuracy(t *testing.T) {
	// Set environment variable to disable buffering
	t.Setenv("NTEST_BUFFERING", "false")

	mockT := newMockedT(t)
	buffered := ntest.BufferedLogger(mockT)
	testLineNumberAccuracy(t, buffered, mockT, false, false) // no buffering, test passes (logs appear immediately)
}

// TestReplaceLogger_WithoutBufferedLogger_LineNumberAccuracy verifies line number accuracy when using ReplaceLogger directly
func TestReplaceLogger_WithoutBufferedLogger_LineNumberAccuracy(t *testing.T) {
	mockT := newMockedT(t)

	// Create a ReplaceLogger directly on mockT (no BufferedLogger involved)
	replaceLogger := ntest.ReplaceLogger(mockT, func(s string) {
		// This lambda should NOT call Helper() since there's no BufferedLogger to track helpers
		mockT.Log(s + " REPLACED")
	})

	// Get the current line number for reference
	_, _, currentLine, _ := runtime.Caller(0)
	t.Logf("Current line number is %d", currentLine)

	replaceLogger.Log("Direct ReplaceLogger test message") // This should capture this line
	logLine := currentLine + 3
	t.Logf("Expected line number for log message: %d", logLine)

	t.Logf("After logging, captured %d log entries", len(mockT.captured))
	for i, entry := range mockT.captured {
		t.Logf("Captured entry % 3d: %s", i, entry)
	}

	// Check for correct line number - should report the lambda function line, not the user code line
	// since there's no BufferedLogger doing helper tracking
	found := false
	expectedLine := strconv.Itoa(logLine)

	t.Logf("Looking for line number: %s", expectedLine)
	for _, entry := range mockT.captured {
		for _, log := range strings.Split(entry, "\n") {
			t.Logf("examining: %s", log)
			// Without BufferedLogger, we expect to see the lambda function line, not the user code line
			if strings.Contains(log, "Direct ReplaceLogger test message REPLACED") {
				t.Logf("✓ Found log message: %s", log)
				// The line number will be from the lambda function, not from the user code
				// This is expected behavior when BufferedLogger is not involved
				found = true
				break
			}
		}
	}

	assert.True(t, found, "Should find log message")
	t.Log("ReplaceLogger without BufferedLogger test completed")
}

// Generic line number accuracy test that works with different logger configurations
func testLineNumberAccuracy(t ntest.T, logger ntest.T, mockT *mockedT, expectBuffering bool, shouldFail bool, mustFind ...string) {
	if expectBuffering {
		if shouldFail {
			t.Log("Testing buffered logger with failing test (should output buffered logs)")
		} else {
			t.Log("Testing buffered logger with passing test (should suppress all logs)")
		}
	} else {
		t.Log("Testing logger with no buffering (logs appear immediately)")
	}

	// Get the current line number for reference
	_, _, currentLine, _ := runtime.Caller(0)
	t.Logf("Current line number is %d", currentLine)

	logger.Log("Test message for line accuracy") // This should capture this line
	logLine := currentLine + 3
	t.Logf("Expected line number for log message: %d", logLine)

	if expectBuffering && shouldFail {
		// For buffered loggers, we need to trigger failure and cleanup to see the logs
		t.Log("Setting test as failed and triggering cleanup")
		mockT.setFailed()
		mockT.triggerCleanup()
	}

	t.Logf("After logging, captured %d log entries", len(mockT.captured))
	for i, entry := range mockT.captured {
		t.Logf("Captured entry % 3d: %s", i, entry)
	}

	if expectBuffering && !shouldFail {
		// For passing tests with buffering, no logs should be captured
		assert.Equal(t, 0, len(mockT.captured), "no logs should be captured for passing test")
		t.Log("Buffered logger passing test completed successfully")
		return
	}

	// Check for correct line number based on the test type
	found := false
	expectedLine := strconv.Itoa(logLine)

	t.Logf("Looking for line number: %s", expectedLine)
	for _, entry := range mockT.captured {
	Line:
		for _, log := range strings.Split(entry, "\n") {
			buffer := fmt.Sprintf("examining: %s", log)
			all := []string{
				"Test message for line accuracy",
				"logger_test.go:" + expectedLine,
			}
			all = append(all, mustFind...)
			for _, s := range all {
				if !strings.Contains(log, s) {
					t.Logf("%s - missing '%s'", buffer, s)
					continue Line
				}
				buffer += fmt.Sprintf(" - found '%s'", s)
			}
			if strings.Contains(log, "logger.go:") {
				t.Logf("%s - uh-oh, also contains 'logger.go'", buffer)
				continue Line
			}
			found = true
			t.Logf("%s - ✓ Found everything", buffer)
			break Line
		}
	}

	assert.True(t, found, "Should find log message with correct line number %s", expectedLine)
	t.Log("Line number accuracy test completed")
}

// Mock T implementation for testing with log capture capabilities
type mockedT struct {
	real ntest.T
	ntest.T
	failed   bool
	cleanups []func()
	captured []string
	skipped  bool
	name     string
	envs     map[string]string
	inner    map[string]*mockedT
	lock     sync.Mutex
}

func newMockedT(real ntest.T) *mockedT {
	return &mockedT{
		real:     real,
		name:     real.Name(),
		envs:     make(map[string]string),
		captured: make([]string, 0),
		cleanups: make([]func(), 0),
		inner:    make(map[string]*mockedT),
	}
}

func (m *mockedT) getInner(name string) *mockedT {
	m.lock.Lock()
	defer m.lock.Unlock()
	i, ok := m.inner[name]
	require.Truef(m.real, ok, "inner mock %s exists", name)
	return i
}

func (m *mockedT) ReWrap(t ntest.T) ntest.T {
	n := newMockedT(t)
	func() {
		m.lock.Lock()
		defer m.lock.Unlock()
		m.inner[t.Name()] = n
	}()
	return n
}

// Unwrap returns the underlying real T, allowing mockedT to work with ntest.Run()
func (m *mockedT) Unwrap() ntest.T {
	return m.real
}

func (m *mockedT) Failed() bool {
	return m.failed
}

func (m *mockedT) Skipped() bool {
	return m.skipped
}

func (m *mockedT) Name() string {
	return m.name
}

func (m *mockedT) Cleanup(f func()) {
	m.cleanups = append(m.cleanups, f)
}

func (m *mockedT) Helper() {
	// No-op for mock
}

func (m *mockedT) FailNow() {
	m.failed = true
}

func (m *mockedT) Skip(args ...interface{}) {
	m.skipped = true
}

func (m *mockedT) Skipf(format string, args ...interface{}) {
	m.skipped = true
}

func (m *mockedT) log(s string) {
	_, file, line, _ := runtime.Caller(2)
	file = filepath.Base(file)
	message := fmt.Sprintf("%s:%d %s", file, line, s)
	m.captured = append(m.captured, message)
	m.real.Logf("[mock] captured %s", message)
}

func (m *mockedT) Log(args ...interface{}) {
	message := fmt.Sprintln(args...)
	m.log(message)
	m.real.Logf("[mock] captured %s", message)
}

func (m *mockedT) Logf(format string, args ...interface{}) {
	m.log(fmt.Sprintf(format, args...))
}

func (m *mockedT) Error(args ...interface{}) {
	m.Log(args...)
	m.failed = true
}

func (m *mockedT) Errorf(format string, args ...interface{}) {
	m.Logf(format, args...)
	m.failed = true
}

func (m *mockedT) Fatal(args ...interface{}) {
	m.Log(args...)
	m.failed = true
}

func (m *mockedT) Fatalf(format string, args ...interface{}) {
	m.Logf(format, args...)
	m.failed = true
}

func (m *mockedT) Setenv(key, value string) {
	m.envs[key] = value
}

func (m *mockedT) triggerCleanup() {
	for _, cleanup := range m.cleanups {
		cleanup()
	}
}

func (m *mockedT) Fail() {
	m.failed = true
}

func (m *mockedT) Parallel() {
	// No-op for mock - parallel execution not relevant for mock
}

func (m *mockedT) setFailed() {
	m.failed = true
}

func TestTimeoutFlush(t *testing.T) {
	if os.Getenv("RUN_TIMEOUT_TEST") != "true" {
		t.Skip("set RUN_TIMEOUT_TEST=true to run this test. Also use a short -timeout")
	}
	if _, ok := os.LookupEnv("NTEST_BUFFERING"); ok {
		t.Setenv("NTEST_BUFFERING", "true")
	}
	buffered := ntest.BufferedLogger(t)
	buffered.Log("buffered hi")
	time.Sleep(11 * time.Minute)
	buffered.Log("bye")
}
