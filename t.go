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

type runner interface {
	T
	Run(string, func(*testing.T)) bool
}

// RunWithReWrap is a helper that runs a subtest and automatically handles ReWrap logic.
// This should be used instead of calling t.Run in tests that use
// ReplaceLogger, BufferedLogger, or ExtraDetailLogger. If running a test with a
// wrapped T that supports ReWrap, use RunWithReWrap instead of .Run(). It can
// also be used with Ts that do not support ReWrap.
func RunWithReWrap(t T, name string, f func(T)) bool {
	runT, ok := t.(runner)
	if !ok {
		t.Logf("Run not supported by %T", t)
		t.Fail()
		return false
	}
	return runT.Run(name, func(subT *testing.T) {
		var reWrapped T
		if reWrapper, ok := t.(ReWrapper); ok {
			reWrapped = reWrapper.ReWrap(subT)
		} else {
			reWrapped = subT
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
