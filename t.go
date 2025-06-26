package ntest

import "testing"

// T is subset of what testing.T provides and is also a subset of
// of what ginkgo.GinkgoT() provides. Compared to *testing.T,
// main thing it is missing is Run()
type T interface {
	Cleanup(func())
	Setenv(key, value string)
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Fail()
	Parallel()
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

// RunT adds Run() to T.
// *testing.T satisfies the RunT interface.
type RunT interface {
	T
	Run(string, func(*testing.T)) bool
}

// NewTestRunner creates a test runner that works with matrix testing by
// upgrading a T to a RunT. It checks to see if the T is actually a RunT,
// like *testing.T.
// This is useful for converting T types to work with matrix testing functions.
// If the concrete type underlying T doesn't implement Run then the
// test will fail if Run is called.
func NewTestRunner(t T) RunT {
	if runT, ok := t.(RunT); ok {
		return runT
	}
	return simpleRunT{T: t, orig: t}
}

// simpleRunT provides RunT functionality for plain T types
type simpleRunT struct {
	T
	orig T
}

func (s simpleRunT) Run(name string, f func(*testing.T)) bool {
	if runT, ok := s.orig.(RunT); ok {
		return runT.Run(name, f)
	}
	//nolint:staticcheck // QF1008: could remove embedded field "T" from selector
	s.T.Logf("Run not supported by %T", s.orig)
	//nolint:staticcheck // QF1008: could remove embedded field "T" from selector
	s.T.Fail()
	return false
}

// RunWithReWrap is a helper that runs a subtest and automatically handles ReWrap logic.
// This should be used instead of calling t.Run in tests that use
// ReplaceLogger, BufferedLogger, or ExtraDetailLogger. If running a test with a
// wrapped logger that supports ReWrap, use RunWithReWrap instead of .Run().
func RunWithReWrap(t RunT, name string, f func(RunT)) bool {
	return t.Run(name, func(subT *testing.T) {
		var reWrapped RunT
		if reWrapper, ok := t.(ReWrapper); ok {
			reWrapped = NewTestRunner(reWrapper.ReWrap(subT))
		} else {
			reWrapped = NewTestRunner(subT)
		}
		f(reWrapped)
	})
}

// ReWrapper allows types that wrap T to recreate themselves from fresh T
// This, combined with RunWithReWrap, allows proper subtest handling in tests
// that wrap T.
type ReWrapper interface {
	ReWrap(T) T
}
