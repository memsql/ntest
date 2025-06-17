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

func TestBufferedLogger_PassingTest(t *testing.T) {
	t.Log("Testing BufferedLogger with passing test (should suppress all logs)")

	var captured []string
	captureT := ntest.ReplaceLogger(t, func(s string) {
		captured = append(captured, s)
	})

	t.Log("Creating BufferedLogger")
	buffered := ntest.BufferedLogger(captureT)

	t.Log("Adding logs that should be suppressed")
	buffered.Log("This should not appear")
	buffered.Logf("Neither should this: %d", 42)

	t.Logf("Captured %d entries (should be 0 for passing test)", len(captured))
	for i, entry := range captured {
		t.Logf("Unexpected entry %d: %s", i, entry)
	}

	// Since the test passes, no logs should be captured
	assert.Equal(t, 0, len(captured), "no logs should be captured for passing test")
	t.Log("BufferedLogger passing test completed successfully")
}

func TestBufferedLogger_FailingTest(t *testing.T) {
	t.Log("Testing BufferedLogger with failing test (should output buffered logs)")

	// Create a mock T that reports as failed
	mockT := newMockedT("TestBufferedLogger_FailingTest")
	t.Log("Created mockedT with name:", mockT.Name())

	t.Log("Creating BufferedLogger")
	buffered := ntest.BufferedLogger(mockT)

	t.Log("Adding logs that should be buffered and later output")
	buffered.Log("This should appear")
	buffered.Logf("This too: %d", 42)

	t.Log("Setting test as failed and triggering cleanup")
	mockT.setFailed()
	mockT.triggerCleanup()

	t.Logf("After cleanup, captured %d log entries", len(mockT.captured))
	for i, entry := range mockT.captured {
		t.Logf("Captured entry %d: %s", i, entry)
	}

	// Check that logs contain file:line information
	found := false
	for _, log := range mockT.captured {
		if strings.Contains(log, "logger_test.go:") && strings.Contains(log, "This should appear") {
			found = true
			t.Logf("Found expected log with file:line info: %s", log)
			break
		}
	}
	assert.True(t, found, "should contain filename and line number")
	t.Log("BufferedLogger failing test completed successfully")
}

// Test line number accuracy for BufferedLogger
func TestBufferedLogger_LineNumberAccuracy(t *testing.T) {
	t.Log("Testing BufferedLogger line number accuracy using runtime.Caller")

	mockT := newMockedT("TestBufferedLogger_LineNumberAccuracy")
	buffered := ntest.BufferedLogger(mockT)

	// Get the current line number for reference
	_, _, currentLine, _ := runtime.Caller(0)
	t.Logf("Current line number is %d", currentLine)

	buffered.Log("Test message 1") // This should capture this line
	logLine1 := currentLine + 3
	t.Logf("Expected line number for 'Test message 1': %d", logLine1)

	buffered.Logf("Test message %d", 2) // This should capture this line
	logLine2 := currentLine + 7
	t.Logf("Expected line number for 'Test message 2': %d", logLine2)

	// Set failed and trigger cleanup
	t.Log("Setting test as failed and triggering cleanup")
	mockT.setFailed()
	mockT.triggerCleanup()

	t.Logf("After cleanup, captured %d log entries", len(mockT.captured))
	for i, entry := range mockT.captured {
		t.Logf("Captured entry %d: %s", i, entry)
	}

	// Find the captured log entries with line numbers
	found1, found2 := false, false
	expectedLine1 := strconv.Itoa(logLine1)
	expectedLine2 := strconv.Itoa(logLine2)

	t.Logf("Looking for line numbers: %s and %s", expectedLine1, expectedLine2)

	for _, log := range mockT.captured {
		if strings.Contains(log, "Test message 1") && strings.Contains(log, "logger_test.go:"+expectedLine1) {
			found1 = true
			t.Logf("✓ Found Test message 1 with correct line number: %s", log)
		}
		if strings.Contains(log, "Test message 2") && strings.Contains(log, "logger_test.go:"+expectedLine2) {
			found2 = true
			t.Logf("✓ Found Test message 2 with correct line number: %s", log)
		}
	}

	assert.True(t, found1, "Should find Test message 1 with correct line number %s", expectedLine1)
	assert.True(t, found2, "Should find Test message 2 with correct line number %s", expectedLine2)
	t.Log("BufferedLogger line number accuracy test completed")
}

// Test line number accuracy for ExtraDetailLogger with BufferedLogger
func TestExtraDetailLogger_WithBufferedLogger_LineNumberAccuracy(t *testing.T) {
	t.Log("Testing ExtraDetailLogger with BufferedLogger for line number accuracy")

	mockT := newMockedT("TestExtraDetailLogger_LineNumberAccuracy")
	t.Log("Created mockedT")

	buffered := ntest.BufferedLogger(mockT)
	t.Log("Created BufferedLogger")

	extraDetail := ntest.ExtraDetailLogger(buffered, "PREFIX")
	t.Log("Created ExtraDetailLogger with prefix 'PREFIX'")

	// Get the current line number for reference
	_, _, currentLine, _ := runtime.Caller(0)
	t.Logf("Current line number is %d", currentLine)

	extraDetail.Log("Test message from extra detail") // This should capture this line
	logLine := currentLine + 3
	t.Logf("Expected line number for log message: %d", logLine)

	// Set failed and trigger cleanup
	t.Log("Setting test as failed and triggering cleanup")
	mockT.setFailed()
	mockT.triggerCleanup()

	t.Logf("After cleanup, captured %d log entries", len(mockT.captured))
	for i, entry := range mockT.captured {
		t.Logf("Captured entry %d: %s", i, entry)
	}

	// Find the captured log entry with correct line number
	found := false
	expectedLine := strconv.Itoa(logLine)
	t.Logf("Looking for line number: %s", expectedLine)

	for _, log := range mockT.captured {
		if strings.Contains(log, "Test message from extra detail") && strings.Contains(log, "logger_test.go:"+expectedLine) {
			found = true
			t.Logf("✓ Found log message with correct line number: %s", log)
			break
		}
	}

	assert.True(t, found, "Should find log message with correct line number %s", expectedLine)
	t.Log("ExtraDetailLogger line number accuracy test completed")
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

func (m *mockedT) Log(args ...interface{}) {
	line := fmt.Sprintln(args...)
	m.captured = append(m.captured, strings.TrimSpace(line))
}

func (m *mockedT) Logf(format string, args ...interface{}) {
	m.captured = append(m.captured, fmt.Sprintf(format, args...))
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
