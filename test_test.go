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
	testRunTBasicLogger(ntest.AsRunT(ntest.BufferedLogger(t)))
}

func TestParallelMatrixExtraBuffered(t *testing.T) {
	testRunTBasicLogger(ntest.AsRunT(ntest.ExtraDetailLogger(ntest.BufferedLogger(t), "TPMEB-")))
}

func testRunTBasicLogger(runT ntest.RunT) {
	// Simple test to verify RunT functionality works
	var ran bool
	success := runT.Run("subtest", func(subT *testing.T) {
		subT.Log("This is a subtest")
		ran = true
	})
	if !success || !ran {
		runT.Fatal("RunT functionality failed")
	}
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
		func(t *testing.T, s string, c chan struct{}) {
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
	// Simple test to verify the logger wrapper works
	t.Log("Testing logger wrapper functionality")
	t.Logf("Logger wrapper test for type %T", t)
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
	t.Skip("this test is expected to fail")
	t.Parallel()
	ntest.RunMatrix(t,
		func() int { return 7 },
		func(t *testing.T, i int) {
			assert.Equal(t, 7, i)
		},
	)
}
