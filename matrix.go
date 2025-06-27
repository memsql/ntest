package ntest

import (
	"testing"

	"github.com/muir/nject/v2"
)

// RunParallelMatrix uses t.Run() to fork into multiple threads of execution for each
// sub-test before any chains are evaluated. This forces the chains to share
// nothing between them. RunParallelMatrix does not provide any default injectors
// other than a *testing.T that comes from a named provider (named "testing.T"). That
// injector is only present if the RunT argument was actually a *testing.T
//
// A matrix is a specific type: map[string]nject.Provider. Add those to the
// chain to trigger matrix testing.
//
// t.Parallel() is used for each t.Run()
//
// A warning about t.Parallel(): inner tests wait until outer tests finish.
// See https://go.dev/play/p/ZDaw054HeIN
//
// Matrix values must be direct arguments to RunMatrix -- they will not be extracted
// from nject.Sequences. RunParallelMatrix will fail if there is no matrix provided.
//
// The provided T must support Run()
func RunParallelMatrix(t T, chain ...any) {
	runMatrixTest(t, true, chain)
}

// RunMatrix uses t.Run() separate execution for each
// sub-test before any chains are evaluated. This forces the chains to share
// nothing between them. RunMatrix does not provide any default injectors
// other than a *testing.T that comes from a named provider (named "testing.T")
//
// A matrix is a specific type: map[string]nject.Provider. Add those to the
// chain to trigger matrix testing.
//
// Matrix values must be direct arguments to RunMatrix -- they will not be extracted
// from nject.Sequences. RunMatrix will fail if there is no matrix provided.
//
// The provided T must support Run()
func RunMatrix(t T, chain ...any) {
	runMatrixTest(t, false, chain)
}

func runMatrixTest(t T, parallel bool, chain []any) {
	_, ok := t.(runner)
	if !ok {
		t.Logf("Run not supported by %T", t)
		t.Fail()
		return
	}
	matrix, before, after := breakChain(chain)
	if matrix == nil {
		t.Log("FAIL: matrix test requires a matrix")
		t.Fail()
		return
	}

	var startTest func(T, map[string]nject.Provider, []any, []any)
	startTest = func(t T, matrix map[string]nject.Provider, before []any, after []any) {
		for name, subChain := range matrix {
			subChain := subChain
			RunWithReWrap(t, name, func(reWrapped T) {
				if parallel {
					Parallel(reWrapped)
				}
				testingT := func(tInner T) []any {
					if tt, ok := tInner.(*testing.T); ok {
						return []any{nject.Provide("testing.T", func() *testing.T { return tt })}
					}
					return []any{}
				}
				matrix, newBefore, newAfter := breakChain(after)
				if matrix == nil {
					RunTest(reWrapped, combineSlices(testingT(t), before, []any{subChain}, after)...)
				} else {
					startTest(reWrapped, matrix, combineSlices(before, newBefore, []any{subChain}), newAfter)
				}
			})
		}
	}
	startTest(t, matrix, before, after)
}

func breakChain(chain []any) (matrix map[string]nject.Provider, before []any, after []any) {
	for i, item := range chain {
		if m, ok := item.(map[string]nject.Provider); ok {
			// Found the matrix, split the chain
			before = chain[:i]
			after = chain[i+1:]
			return m, before, after
		}
	}
	// No matrix found
	return nil, chain, nil
}

func combineSlices[T any](first []T, more ...[]T) []T {
	if len(more) == 0 {
		return first
	}
	total := len(first)
	for _, m := range more {
		total += len(m)
	}
	if total == len(first) {
		return first
	}
	combined := make([]T, len(first), total)
	copy(combined, first)
	for _, m := range more {
		combined = append(combined, m...)
	}
	return combined
}
