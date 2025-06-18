package ntest_test

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memsql/ntest"
)

var _ ntest.T = &testing.T{}

func TestPrefixLogger(t *testing.T) {
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

	t.Logf("Captured %d log entries", len(caught))
	for i, entry := range caught {
		t.Logf("Entry %d: %s", i, entry)
	}

	require.Equal(t, 2, len(caught), "len caught")
	assert.Regexp(t, `some-prefix \d\d:\d\d:\d\d not-formatted 3$`, caught[0], "unformatted")
	assert.Regexp(t, `some-prefix \d\d:\d\d:\d\d formatted 'quoted'$`, caught[1], "formatted")

	t.Log("ExtraDetailLogger test completed successfully")
}

// Test line number accuracy for BufferedLogger
func TestBufferedLogger_LineNumberAccuracy(t *testing.T) {
	mockT := newMockedT("TestBufferedLogger_LineNumberAccuracy")
	buffered := ntest.BufferedLogger(mockT)
	testLineNumberAccuracy(t, buffered, mockT, true, true, "") // expect buffering, test should fail to check line numbers
}

func TestExtraDetailLogger_WithBufferedLogger_LineNumberAccuracy(t *testing.T) {
	mockT := newMockedT("TestExtraDetailLogger_LineNumberAccuracy")
	buffered := ntest.BufferedLogger(mockT)
	extraDetail := ntest.ExtraDetailLogger(buffered, "PREFIX")
	testLineNumberAccuracy(t, extraDetail, mockT, true, true, "PREFIX") // expect buffering, test should fail to check line numbers
}

func TestReplaceLogger_WithBufferedLogger_LineNumberAccuracy(t *testing.T) {
	mockT := newMockedT("TestExtraDetailLogger_LineNumberAccuracy")
	buffered := ntest.BufferedLogger(mockT)
	extraDetail := ntest.ReplaceLogger(buffered, func(s string) {
		buffered.Log(s + " SUFFIX")
	})
	testLineNumberAccuracy(t, extraDetail, mockT, true, true, "SUFFIX") // expect buffering, test should fail to check line numbers
}

func TestExtraDetailLogger_WithBufferedLogger_NoBuffering_LineNumberAccuracy(t *testing.T) {
	// Set environment variable to disable buffering
	t.Setenv("NTEST_BUFFERING", "false")

	mockT := newMockedT("TestExtraDetailLogger_NoBuffering")
	buffered := ntest.BufferedLogger(mockT)
	testLineNumberAccuracy(t, buffered, mockT, false, false, "") // no buffering, test passes (logs appear immediately)
}

// Generic line number accuracy test that works with different logger configurations
func testLineNumberAccuracy(t *testing.T, logger ntest.T, mockT *mockedT, expectBuffering bool, shouldFail bool, mustFind string) {
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
		t.Logf("Captured entry %d: %s", i, entry)
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
		for _, log := range strings.Split(entry, "\n") {
			t.Logf("examining: %s", log)
			if mustFind != "" {
				if !strings.Contains(log, mustFind) {
					continue
				}
			}
			if strings.Contains(log, "Test message for line accuracy") && strings.Contains(log, "logger_test.go:"+expectedLine) && strings.Contains(log, "Test message for line accuracy") && !strings.Contains(log, "logger.go:") {
				found = true
				t.Logf("✓ Found log message with correct line number: %s", log)
				break
			}
		}
	}

	assert.True(t, found, "Should find log message with correct line number %s", expectedLine)
	t.Log("Line number accuracy test completed")
}

// Mock T implementation for testing with log capture capabilities
type mockedT struct {
	ntest.T
	failed   bool
	cleanups []func()
	captured []string
	skipped  bool
	name     string
	envs     map[string]string
}

func newMockedT(name string) *mockedT {
	return &mockedT{
		name:     name,
		envs:     make(map[string]string),
		captured: make([]string, 0),
		cleanups: make([]func(), 0),
	}
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
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		file = file[idx+1:]
	}
	m.captured = append(m.captured, fmt.Sprintf("%s:%d %s", file, line, s))
}

func (m *mockedT) Log(args ...interface{}) {
	line := fmt.Sprintln(args...)
	m.log(line)
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

func (m *mockedT) setFailed() {
	m.failed = true
}
