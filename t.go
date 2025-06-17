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

// Runner provides RunT functionality without specific type constraints.
// This allows functions to return a type that can be used with matrix testing
// without exposing the concrete wrapper types.
type Runner interface {
	T
	Run(string, func(T)) bool
	Fail()
	Parallel()
}

// eitherT provides RunT functionality for wrapper types (internal use only)
// WT is the wrapper type (like logWrappedT[ET])
// ET is the underlying T type
// This allows embedding types to automatically implement RunT[WT]
type eitherT[WT any, ET T] struct {
	t        ET
	wrapFunc func(eitherT[WT, ET]) WT
}

func makeEitherT[WT any, ET T](t ET, wrapFunc func(eitherT[WT, ET]) WT) eitherT[WT, ET] {
	return eitherT[WT, ET]{
		t:        t,
		wrapFunc: wrapFunc,
	}
}

func makeEitherTSimple[ET T, WT any](t ET, wrapper WT) eitherT[WT, ET] {
	return eitherT[WT, ET]{
		t: t,
		wrapFunc: func(eitherT[WT, ET]) WT {
			return wrapper
		},
	}
}

func (t eitherT[WT, ET]) Run(name string, f func(WT)) bool {
	if runT, ok := any(t.t).(RunT[ET]); ok {
		return runT.Run(name, func(innerT ET) {
			innerSelf := eitherT[WT, ET]{
				t:        innerT,
				wrapFunc: t.wrapFunc,
			}
			f(t.wrapFunc(innerSelf))
		})
	}
	t.t.Logf("Run not supported by %T", t.t)
	t.t.FailNow()
	return false
}

func (t eitherT[WT, ET]) Fail() {
	if runT, ok := any(t.t).(RunT[ET]); ok {
		runT.Fail()
		return
	}
	t.t.Logf("Fail not supported by %T", t.t)
	t.t.FailNow()
}

func (t eitherT[WT, ET]) Parallel() {
	if runT, ok := any(t.t).(RunT[ET]); ok {
		runT.Parallel()
		return
	}
	t.t.Logf("Parallel not supported by %T", t.t)
	t.t.FailNow()
}

// Delegate all T methods
func (t eitherT[WT, ET]) Cleanup(f func())                          { t.t.Cleanup(f) }
func (t eitherT[WT, ET]) Setenv(key, value string)                  { t.t.Setenv(key, value) }
func (t eitherT[WT, ET]) Error(args ...interface{})                 { t.t.Error(args...) }
func (t eitherT[WT, ET]) Errorf(format string, args ...interface{}) { t.t.Errorf(format, args...) }
func (t eitherT[WT, ET]) FailNow()                                  { t.t.FailNow() }
func (t eitherT[WT, ET]) Failed() bool                              { return t.t.Failed() }
func (t eitherT[WT, ET]) Fatal(args ...interface{})                 { t.t.Fatal(args...) }
func (t eitherT[WT, ET]) Fatalf(format string, args ...interface{}) { t.t.Fatalf(format, args...) }
func (t eitherT[WT, ET]) Helper()                                   { t.t.Helper() }
func (t eitherT[WT, ET]) Log(args ...interface{})                   { t.t.Log(args...) }
func (t eitherT[WT, ET]) Logf(format string, args ...interface{})   { t.t.Logf(format, args...) }
func (t eitherT[WT, ET]) Name() string                              { return t.t.Name() }
func (t eitherT[WT, ET]) Skip(args ...interface{})                  { t.t.Skip(args...) }
func (t eitherT[WT, ET]) Skipf(format string, args ...interface{})  { t.t.Skipf(format, args...) }
func (t eitherT[WT, ET]) Skipped() bool                             { return t.t.Skipped() }

// tRunWrapper wraps any RunT[WT] to implement RunT[T]
type tRunWrapper[WT T] struct {
	inner RunT[WT]
}

func (w tRunWrapper[WT]) Run(name string, f func(T)) bool {
	return w.inner.Run(name, func(wt WT) {
		f(wt) // WT satisfies T, so this works
	})
}

func (w tRunWrapper[WT]) Fail()                                     { w.inner.Fail() }
func (w tRunWrapper[WT]) Parallel()                                 { w.inner.Parallel() }
func (w tRunWrapper[WT]) Cleanup(f func())                          { w.inner.Cleanup(f) }
func (w tRunWrapper[WT]) Setenv(key, value string)                  { w.inner.Setenv(key, value) }
func (w tRunWrapper[WT]) Error(args ...interface{})                 { w.inner.Error(args...) }
func (w tRunWrapper[WT]) Errorf(format string, args ...interface{}) { w.inner.Errorf(format, args...) }
func (w tRunWrapper[WT]) FailNow()                                  { w.inner.FailNow() }
func (w tRunWrapper[WT]) Failed() bool                              { return w.inner.Failed() }
func (w tRunWrapper[WT]) Fatal(args ...interface{})                 { w.inner.Fatal(args...) }
func (w tRunWrapper[WT]) Fatalf(format string, args ...interface{}) { w.inner.Fatalf(format, args...) }
func (w tRunWrapper[WT]) Helper()                                   { w.inner.Helper() }
func (w tRunWrapper[WT]) Log(args ...interface{})                   { w.inner.Log(args...) }
func (w tRunWrapper[WT]) Logf(format string, args ...interface{})   { w.inner.Logf(format, args...) }
func (w tRunWrapper[WT]) Name() string                              { return w.inner.Name() }
func (w tRunWrapper[WT]) Skip(args ...interface{})                  { w.inner.Skip(args...) }
func (w tRunWrapper[WT]) Skipf(format string, args ...interface{})  { w.inner.Skipf(format, args...) }
func (w tRunWrapper[WT]) Skipped() bool                             { return w.inner.Skipped() }

// AdjustSkipFrames forwards the skip frame adjustment to the inner wrapper if it supports it
func (w tRunWrapper[WT]) AdjustSkipFrames(skip int) {
	if adjuster, ok := any(w.inner).(interface{ AdjustSkipFrames(int) }); ok {
		adjuster.AdjustSkipFrames(skip)
	}
}
