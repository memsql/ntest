package ntest

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

type RunT[GT T] interface {
	T
	Run(string, func(GT)) bool
	Fail()
	Parallel()
}

// NewTestRunner creates a test runner that works with matrix testing from any RunT[T] type.
// This is useful for converting wrapper types to work with matrix testing functions.
func NewTestRunner[ET T](t RunT[ET]) RunT[T] {
	return tRunWrapper[ET]{inner: t}
}

// simpleRunT provides RunT functionality for plain T types
type simpleRunT[ET T] struct {
	runTHelper
	orig ET
}

func (s simpleRunT[ET]) Run(name string, f func(ET)) bool {
	if runT, ok := any(s.orig).(RunT[ET]); ok {
		return runT.Run(name, f)
	}
	s.T.Logf("Run not supported by %T", s.orig)
	s.T.FailNow()
	return false
}

// Runner provides RunT functionality without specific type constraints.
// This allows functions to return a type that can be used with matrix testing
// without exposing the concrete wrapper types.
type Runner interface {
	T
	Run(string, func(T)) bool
	Fail()
	Parallel()
}

// tRunWrapper wraps any RunT[WT] to implement RunT[T]
type tRunWrapper[WT T] struct {
	T
	inner RunT[WT]
}

func (w tRunWrapper[WT]) Run(name string, f func(T)) bool {
	return w.inner.Run(name, func(wt WT) {
		f(wt) // WT satisfies T, so this works
	})
}

func (w tRunWrapper[WT]) Fail()     { w.inner.Fail() }
func (w tRunWrapper[WT]) Parallel() { w.inner.Parallel() }

// AdjustSkipFrames forwards the skip frame adjustment to the inner wrapper if it supports it
// tRunWrapper adds 0 frames (it just delegates all T methods directly)
func (w tRunWrapper[WT]) AdjustSkipFrames(skip int) {
	if adjuster, ok := any(w.inner).(interface{ AdjustSkipFrames(int) }); ok {
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
	// Fallback - most T implementations have some way to fail
	r.T.FailNow()
}

func (r runTHelper) Parallel() {
	if parallel, ok := r.T.(interface{ Parallel() }); ok {
		parallel.Parallel()
		return
	}
	// If not supported, we just continue - parallel is optional
}
