package ntest

import "testing"

// T is subset of what *testing.T provides.
//
// It is missing:
//
//	.Run - not present in ginkgo.GinkgoT()
//	.Parallel - not present in *testing.B
type T interface {
	Cleanup(func())
	Setenv(key, value string)
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Fail()
	FailNow()
	Failed() bool
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Helper()
	Log(args ...interface{})
	Logf(format string, args ...interface{})
	Name() string
	Skip(args ...interface{})
	Skipf(format string, args ...interface{})
	Skipped() bool
}

var (
	_ runnerB  = (*testing.B)(nil)
	_ runner   = (*testing.T)(nil)
	_ parallel = (*testing.T)(nil)
)

type runner interface {
	T
	Run(string, func(*testing.T)) bool
}

type runnerB interface {
	T
	Run(string, func(*testing.B)) bool
}

type parallel interface {
	T
	Parallel()
}

// Parallel calls .Parallel() on the underlying T if it supports .Parallel.
// If not, it logs a warning and continues without Parallel.
func Parallel(t T) {
	t.Helper()
	p, ok := t.(parallel)
	if ok {
		p.Parallel()
	} else {
		t.Logf("Ignoring .Parallel() call on %T", t)
	}
}

// MustParallel calls .Parallel() on the underlying T if it supports .Parallel.
// If not, it fails the test
func MustParallel(t T) {
	if p, ok := t.(parallel); ok {
		p.Parallel()
	} else {
		t.Logf("Parallel() not supported by %T", t)
		t.Fail()
	}
}

// Run is a helper that runs a subtest and automatically handles ReWrap logic.
// This should be used instead of calling t.Run directly when using logger wrappers like
// ReplaceLogger, BufferedLogger, or ExtraDetailLogger.
//
// Key benefits:
// - Works with both *testing.T and *testing.B (they have different Run signatures)
// - Automatically rewraps logger wrappers for subtests via ReWrap interface
// - Can be used with any T, whether wrapped or not
//
// Example:
//
//	logger := ntest.BufferedLogger(t)
//	ntest.Run(logger, "subtest", func(subT ntest.T) {
//	    // subT is automatically a properly wrapped BufferedLogger
//	    subT.Log("This will be buffered correctly")
//	})
func Run(t T, name string, f func(T)) bool {
	inner := func(subT T) {
		var reWrapped T
		if reWrapper, ok := t.(ReWrapper); ok {
			reWrapped = reWrapper.ReWrap(subT)
		} else {
			reWrapped = subT
		}
		f(reWrapped)
	}
	switch tt := t.(type) {
	case runner:
		return tt.Run(name, func(subT *testing.T) {
			inner(subT)
		})
	case runnerB:
		return tt.Run(name, func(subT *testing.B) {
			inner(subT)
		})
	default:
		t.Logf("Run not supported by %T", t)
		t.Fail()
		return false
	}
}

// ReWrapper allows types that wrap T to recreate themselves from fresh T
// This, combined with RunWithReWrap, allows proper subtest handling in tests
// that wrap T.
type ReWrapper interface {
	ReWrap(T) T
}
