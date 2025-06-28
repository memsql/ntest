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

// callParallel is a helper that attempts to call .Parallel() on the underlying T.
// It returns true if .Parallel() was successfully called, false otherwise.
func callParallel(t T) bool {
	t.Helper()
	// Walk down the wrapper chain to find something that supports Parallel
	current := t
	for {
		switch tt := current.(type) {
		case parallel:
			tt.Parallel()
			return true
		case ReWrapper:
			current = tt.Unwrap()
			continue
		}
		return false
	}
}

// Parallel calls .Parallel() on the underlying T if it supports .Parallel.
// If not, it logs a warning and continues without Parallel.
// If the input T is a ReWrapper then it will be unwrapped to find a T that supports Parallel.
func Parallel(t T) {
	t.Helper()
	if !callParallel(t) {
		t.Logf("Ignoring .Parallel() call on %T", t)
	}
}

// MustParallel calls .Parallel() on the underlying T if it supports .Parallel.
// If not, it fails the test.
// If the input T is a ReWrapper then it will be unwrapped to find a T that supports Parallel.
func MustParallel(t T) {
	t.Helper()
	if !callParallel(t) {
		t.Logf("Parallel() not supported by %T", t)
		t.Fail()
	}
}

// Run is a helper that runs a subtest and automatically handles ReWrap logic.
// This should be used instead of calling t.Run directly when using logger wrappers like
// ReplaceLogger, BufferedLogger, or ExtraDetailLogger that support the
// ReWrapper interface.
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
	reWrap := func(t T) T { return t }

	// Walk down the wrapper chain to find something that supports Run
	current := t
	for {
		switch tt := current.(type) {
		case runner:
			return tt.Run(name, func(subT *testing.T) {
				f(reWrap(subT))
			})
		case runnerB:
			return tt.Run(name, func(subT *testing.B) {
				f(reWrap(subT))
			})
		case ReWrapper:
			current = tt.Unwrap()
			oldWrap := reWrap
			reWrap = func(t T) T {
				return oldWrap(tt.ReWrap(t))
			}
			continue
		default:
			t.Logf("Run not supported by %T", t)
			t.Fail()
			return false
		}
	}
}

// ReWrapper allows types that wrap T to recreate themselves from fresh T
// This, combined with Run() and Parallel(), allows proper subtest handling in tests
// that wrap T.
type ReWrapper interface {
	T
	// ReWrap must return a T that is wrapped (with the current class) compared to it's input
	// This is re-applying the wrapping to get back to the type of the ReWrapper.
	// ReWrap only needs to care about it's own immediate wrapping. It does not need to
	// check if it's underlying type implements ReWrapper.
	ReWrap(T) T
	// Unwrap must return a T that is unwrapped compared to the ReWrapper.
	// This is providing access to the inner-T
	Unwrap() T
}
