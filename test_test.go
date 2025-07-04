package ntest_test

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/muir/nject/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memsql/ntest"
)

var timeout = func() time.Duration {
	// Use longer timeout on macOS due to CI resource contention
	if runtime.GOOS == "darwin" {
		return time.Minute
	}
	return time.Second
}()

func TestRunTest(t *testing.T) {
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
	testParallelMatrixLogger(ntest.ExtraDetailLogger(t, "TPMED-"))
}

func TestParallelMatrixBuffered(t *testing.T) {
	testParallelMatrix(ntest.BufferedLogger(t))
}

func TestParallelMatrixExtraBuffered(t *testing.T) {
	testParallelMatrix(ntest.ExtraDetailLogger(ntest.BufferedLogger(t), "TPMEB-"))
}

func testParallelMatrix(t ntest.T) {
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
	ntest.Run(t, "validate", func(t ntest.T) {
		ntest.MustParallel(t)

		t.Logf("Waiting for testA completion...")
		select {
		case <-doneA:
			t.Logf("testA completed successfully")
		case <-time.After(timeout):
			mu.Lock()
			t.Logf("Current completed tests: %+v", testsRun)
			mu.Unlock()
			require.False(t, true, "timeout waiting for testA after %v", timeout)
		}

		t.Logf("Waiting for testB completion...")
		select {
		case <-doneB:
			t.Logf("testB completed successfully")
		case <-time.After(timeout):
			mu.Lock()
			t.Logf("Current completed tests: %+v", testsRun)
			mu.Unlock()
			require.False(t, true, "timeout waiting for testB after %v", timeout)
		}

		mu.Lock()
		finalTests := make(map[string]struct{})
		for k, v := range testsRun {
			finalTests[k] = v
		}
		mu.Unlock()

		assert.Equal(t, map[string]struct{}{
			name + "/testA": {},
			name + "/testB": {},
		}, finalTests)
	})
}

func testParallelMatrixLogger(t ntest.T) {
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
	ntest.Run(t, "validate", func(subT ntest.T) {
		ntest.MustParallel(subT)
		t.Log("waiting for doneA")
		select {
		case <-doneA:
		case <-time.After(timeout):
			require.False(subT, true, "loggerA timeout")
		}
		t.Log("waiting for doneB")
		select {
		case <-doneB:
		case <-time.After(timeout):
			require.False(subT, true, "loggerB timeout")
		}
		t.Log("all done")
	})
}

func TestMatrix(t *testing.T) {
	testMatrix(t)
}

func BenchmarkMatrix(t *testing.B) {
	testMatrix(t)
}

func testMatrix[ET ntest.T](t ET) {
	ntest.Parallel(t)
	testsRun := make(map[string]struct{})
	ntest.RunMatrix(t,
		func() int { return 7 },
		map[string]nject.Provider{
			"testA": nject.Provide("testA", func(t ntest.T) string { return t.Name() }),
			"testB": nject.Sequence("testB",
				func(t ntest.T, _ int) string { return t.Name() },
			),
		},
		func(t ET, s string) {
			t.Logf("final func for %s", t.Name())
			t.Logf("s = %s", s)
			testsRun[s] = struct{}{}
		},
	)
	assert.Equal(t, map[string]struct{}{
		t.Name() + "/testA": {},
		t.Name() + "/testB": {},
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
	mk := newMockedT(t)
	ntest.RunMatrix(mk,
		func() int { return 7 },
		func(t *testing.T, i int) {
			assert.Equal(t, 7, i)
		},
	)
	assert.True(t, mk.Failed())
}

// TestRunWrapper tests the RunWithReWrap functionality directly
func TestRunWrapper(t *testing.T) {
	t.Parallel()

	// Capture log output to verify layering is preserved
	var capturedLogs []string
	captureLogger := ntest.ReplaceLogger(t, func(s string) {
		capturedLogs = append(capturedLogs, s)
	})

	// Test with a logger that implements ReWrapper - wrap the capture logger with ExtraDetailLogger
	logger := ntest.ExtraDetailLogger(captureLogger, "RWRW-")

	var subTestRan bool
	success := ntest.Run(logger, "rewrap-test", func(reWrapped ntest.T) {
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
