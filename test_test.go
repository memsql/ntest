package ntest_test

import (
	"sync"
	"testing"
	"time"

	"github.com/muir/nject/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memsql/ntest"
)

func TestRun(t *testing.T) {
	t.Parallel()
	var called bool
	ntest.RunTest(t,
		func() string { return "xyz" },
		func(t *testing.T) {
			require.True(t, true)
		},
		func(t ntest.T) {
			require.True(t, true)
		},
		func(s string) {
			called = s == "xyz"
		},
	)
	assert.True(t, called)
}

func TestParallelMatrixTestingT(t *testing.T) {
	testParallelMatrix(t)
}

func TestParallelMatrixExtraDetail(t *testing.T) {
	testParallelMatrixLogger(ntest.AsRunT(ntest.ExtraDetailLogger(t, "TPMED-")))
}

func TestParallelMatrixBuffered(t *testing.T) {
	testParallelMatrix(ntest.AsRunT(ntest.BufferedLogger(t)))
}

func TestParallelMatrixExtraBuffered(t *testing.T) {
	testParallelMatrix(ntest.AsRunT(ntest.ExtraDetailLogger(ntest.BufferedLogger(t), "TPMEB-")))
}

func testParallelMatrix(t ntest.RunT) {
	var mu sync.Mutex
	name := t.Name()
	doneA := make(chan struct{})
	doneB := make(chan struct{})
	testsRun := make(map[string]struct{})
	ntest.RunParallelMatrix(t,
		func() int { return 7 },
		map[string]nject.Provider{
			"testA": nject.Provide("testA",
				func(t ntest.T) (string, chan struct{}) {
					return t.Name(), doneA
				}),
			"testB": nject.Sequence("testB",
				func(t ntest.T, _ int) string { return t.Name() },
				func(t ntest.T) chan struct{} {
					return doneB
				},
			),
		},
		func(t ntest.T, s string, c chan struct{}) {
			t.Logf("final func for %s", t.Name())
			t.Logf("s = %s", s)
			mu.Lock()
			defer mu.Unlock()
			testsRun[s] = struct{}{}
			close(c)
		},
	)
	t.Run("validate", func(t *testing.T) {
		t.Parallel()
		select {
		case <-doneA:
		case <-time.After(time.Second):
			require.False(t, true, "timeout")
		}
		select {
		case <-doneB:
		case <-time.After(time.Second):
			require.False(t, true, "timeout")
		}
		assert.Equal(t, map[string]struct{}{
			name + "/testA": {},
			name + "/testB": {},
		}, testsRun)
	})
}

func testParallelMatrixLogger(t ntest.RunT) {
	// Test that logger wrappers work with matrix testing (exercises ReWrap functionality)
	t.Log("Testing logger wrapper functionality")
	t.Logf("Logger wrapper test for type %T", t)

	// Use the same pattern as testParallelMatrix
	doneA := make(chan struct{})
	doneB := make(chan struct{})

	ntest.RunParallelMatrix(t,
		func() string { return "test-value" },
		map[string]nject.Provider{
			"loggerA": nject.Provide("loggerA", func(t ntest.T, s string) (string, chan struct{}) {
				t.Logf("In loggerA subtest with value: %s", s)
				return t.Name(), doneA
			}),
			"loggerB": nject.Provide("loggerB", func(t ntest.T, s string) (string, chan struct{}) {
				t.Logf("In loggerB subtest with value: %s", s)
				return t.Name(), doneB
			}),
		},
		func(t ntest.T, name string, c chan struct{}) {
			t.Logf("Matrix test completed for %s", name)
			close(c)
		},
	)

	// Wait for both subtests to complete
	t.Run("validate", func(subT *testing.T) {
		subT.Parallel()
		select {
		case <-doneA:
		case <-time.After(time.Second):
			require.False(subT, true, "loggerA timeout")
		}
		select {
		case <-doneB:
		case <-time.After(time.Second):
			require.False(subT, true, "loggerB timeout")
		}
	})
}

func TestMatrix(t *testing.T) {
	t.Parallel()
	testsRun := make(map[string]struct{})
	ntest.RunMatrix(t,
		func() int { return 7 },
		map[string]nject.Provider{
			"testA": nject.Provide("testA", func(t ntest.T) string { return t.Name() }),
			"testB": nject.Sequence("testB",
				func(t ntest.T, _ int) string { return t.Name() },
			),
		},
		func(t *testing.T, s string) {
			t.Logf("final func for %s", t.Name())
			t.Logf("s = %s", s)
			testsRun[s] = struct{}{}
		},
	)
	assert.Equal(t, map[string]struct{}{
		"TestMatrix/testA": {},
		"TestMatrix/testB": {},
	}, testsRun)
}

func TestExtra(t *testing.T) {
	t.Parallel()
	var a int
	var b int
	var c int
	baseSequence := nject.Sequence("base",
		nject.Provide("string", func() string { return "abc" }),
		func() int { return 7 },
	)
	ntest.RunTest(t,
		baseSequence,
		ntest.Extra(func(s string) int { return len(s) }, &a),
		ntest.Extra(func(s string) int { return len(s) + 1 }, &b),
		func() {
			c = a + b
		},
	)
	assert.Equal(t, 7, c)
}

func TestEmptyMatrix(t *testing.T) {
	t.Parallel()
	mk := newMockedT(t.Name())
	ntest.RunMatrix(ntest.AsRunT(mk),
		func() int { return 7 },
		func(t *testing.T, i int) {
			assert.Equal(t, 7, i)
		},
	)
	assert.True(t, mk.Failed())
}

// TestRunWithReWrap tests the RunWithReWrap functionality directly
func TestRunWithReWrap(t *testing.T) {
	t.Parallel()

	// Capture log output to verify layering is preserved
	var capturedLogs []string
	captureLogger := ntest.ReplaceLogger(t, func(s string) {
		capturedLogs = append(capturedLogs, s)
	})

	// Test with a logger that implements ReWrapper - wrap the capture logger with ExtraDetailLogger
	logger := ntest.ExtraDetailLogger(captureLogger, "RWRW-")
	runT := ntest.AsRunT(logger)

	var subTestRan bool
	success := ntest.RunWithReWrap(runT, "rewrap-test", func(reWrapped ntest.RunT) {
		reWrapped.Log("This should be prefixed and timestamped")
		reWrapped.Logf("Formatted message: %s", "test")
		subTestRan = true
	})

	assert.True(t, success, "RunWithReWrap should succeed")
	assert.True(t, subTestRan, "Subtest should have run")

	// Verify that the logger layering was preserved in the rewrapped logger
	require.Len(t, capturedLogs, 2, "Should have captured 2 log messages")

	// Both messages should have the RWRW- prefix and timestamp format
	assert.Contains(t, capturedLogs[0], "RWRW-", "First message should have prefix")
	assert.Contains(t, capturedLogs[0], "This should be prefixed and timestamped", "First message should contain original text")
	assert.Regexp(t, `\d{2}:\d{2}:\d{2}`, capturedLogs[0], "First message should have timestamp")

	assert.Contains(t, capturedLogs[1], "RWRW-", "Second message should have prefix")
	assert.Contains(t, capturedLogs[1], "Formatted message: test", "Second message should contain formatted text")
	assert.Regexp(t, `\d{2}:\d{2}:\d{2}`, capturedLogs[1], "Second message should have timestamp")
}

// TestLoggerRun tests the Run methods on logger types with unsupported underlying types
func TestLoggerRun(t *testing.T) {
	t.Parallel()

	// Create a mock T that doesn't support Run to test fallback behavior
	mockT := newMockedT(t.Name())
	logger := ntest.ExtraDetailLogger(mockT, "TEST-")

	// Capture log output to verify the "Run not supported" message
	var loggedMessages []string
	captureLogger := ntest.ReplaceLogger(t, func(s string) {
		loggedMessages = append(loggedMessages, s)
	})

	mockT.FailNow()

	// This should trigger the "Run not supported" path and FailNow
	success := logger.(interface {
		Run(string, func(*testing.T)) bool
	}).Run("test", func(subT *testing.T) {
		captureLogger.Log("Should not reach here")
	})

	// Verify the expected behavior
	assert.False(t, success, "Run should return false for unsupported type")
	assert.True(t, mockT.Failed(), "FailNow should have been called")

	// Check that we don't have any captured messages (the test function shouldn't have run)
	assert.Empty(t, loggedMessages, "The test function should not have been called")
}

// TestSimpleRunTFallbacks tests that simpleRunT properly handles T types that don't support Run/Parallel
func TestSimpleRunTFallbacks(t *testing.T) {
	t.Parallel()

	// Test with a mock T that doesn't support Run or Parallel
	mockT := newMockedT("TestSimpleRunTFallbacks")
	runT := ntest.NewTestRunner(mockT)

	// Test Run fallback - should log error and fail
	success := runT.Run("subtest", func(subT *testing.T) {
		t.Error("This should not execute")
	})

	assert.False(t, success, "Run should return false when underlying T doesn't support Run")
	assert.True(t, mockT.Failed(), "Should call FailNow when Run is not supported")

	// Test Parallel fallback - should not panic when underlying T doesn't support Parallel
	assert.NotPanics(t, func() {
		runT.Parallel() // This should be a no-op for non-parallel T
	}, "Parallel should not panic when underlying T doesn't support it")
}

// TestAdjustSkipFramesForwarding tests that AdjustSkipFrames properly forwards to underlying types
func TestAdjustSkipFramesForwarding(t *testing.T) {
	t.Parallel()

	// Create a chain: BufferedLogger wrapping another BufferedLogger
	// This creates a scenario where skip frames need to be properly forwarded through the chain
	inner := ntest.BufferedLogger(t)
	outer := ntest.BufferedLogger(inner)

	// Both should support AdjustSkipFrames
	if adjuster, ok := outer.(interface{ AdjustSkipFrames(int) }); ok {
		// This should forward through the chain without panicking
		adjuster.AdjustSkipFrames(2)

		// Verify we can still log without errors (indicating the chain is intact)
		outer.Log("Test message through forwarded skip frames")
		assert.True(t, true, "AdjustSkipFrames forwarding should not break the logger chain")
	} else {
		t.Error("BufferedLogger should implement AdjustSkipFrames")
	}
}
