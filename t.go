package ntest

import (
	"fmt"
	"time"
)

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

type logWrappedT struct {
	T
	logger func(string)
}

// ReplaceLogger creates a T that is wrapped so that the logger is
// overridden with the provided function.
func ReplaceLogger(t T, logger func(string)) T {
	return logWrappedT{
		T:      t,
		logger: logger,
	}
}

func (t logWrappedT) Log(args ...interface{}) {
	line := fmt.Sprintln(args...)
	t.logger(line[0 : len(line)-1])
}

func (t logWrappedT) Logf(format string, args ...interface{}) {
	t.logger(fmt.Sprintf(format, args...))
}

// ExtraDetailLogger creates a T that wraps the logger to add both a
// prefix and a timestamp to each line that is logged.
func ExtraDetailLogger(t T, prefix string) T {
	return ReplaceLogger(t, func(s string) {
		t.Log(prefix, time.Now().Format("15:04:05"), s)
	})
}
