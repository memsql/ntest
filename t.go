package ntest

import "testing"

// T is subset of what testing.T provides and is also a subset of
// of what ginkgo.GinkgoT() provides.  This interface is probably
// richer than strictly required so more could be removed from it
// (or more added).
type T interface {
	Cleanup(func())
	Setenv(key, value string)
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
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

// RunT brings in more of the *testing.T API including Run and Parallel which are needed
// for matrix tests.
// *testing.T satisfies the RunT interface.
type RunT interface {
	T
	Run(string, func(*testing.T)) bool
	Parallel()
}

// NewTestRunner creates a test runner that works with matrix testing by
// upgrading a T to a RunT.
// This is useful for converting T types to work with matrix testing functions.
// If the concrete type underlying T doesn't implement Run then the
// test will fail if Run is called. If the underlying type doesn't implement
// Parallel, then the test won't be parallel.
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
	s.T.Logf("Run not supported by %T", s.orig)
	s.T.FailNow()
	return false
}

func (s simpleRunT) Parallel() {
	if parallel, ok := s.orig.(interface{ Parallel() }); ok {
		parallel.Parallel()
	}
}

// tRunWrapper wraps any RunT to implement RunT
type tRunWrapper struct {
	T
	inner RunT
}

func (w tRunWrapper) Run(name string, f func(*testing.T)) bool {
	return w.inner.Run(name, f)
}

func (w tRunWrapper) Parallel() { w.inner.Parallel() }

// AdjustSkipFrames forwards the skip frame adjustment to the inner wrapper if it supports it
// tRunWrapper adds 0 frames (it just delegates all T methods directly)
func (w tRunWrapper) AdjustSkipFrames(skip int) {
	if adjuster, ok := w.inner.(interface{ AdjustSkipFrames(int) }); ok {
		adjuster.AdjustSkipFrames(skip) // +0 since tRunWrapper doesn't add any frames
	}
}

// runTHelper is a tiny wrapper around T that provides Fail and Parallel methods
// by casting to appropriate interfaces. This can be embedded since it's not parameterized.
type runTHelper struct {
	T
}

func (r runTHelper) Fail() {
	if failer, ok := r.T.(interface{ Fail() }); ok {
		failer.Fail()
		return
	}
	// Fallback
	r.T.FailNow()
}

func (r runTHelper) Parallel() {
	if parallel, ok := r.T.(interface{ Parallel() }); ok {
		parallel.Parallel()
		return
	}
	// If not supported, we just continue - parallel is optional
}

// RunWithReWrap is a helper that runs a subtest and automatically handles ReWrap logic.
// This should be used instead of calling t.Run in tests that use
// ReplaceLogger, BufferedLogger, or ExtraDetailLogger.
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
