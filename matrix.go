package ntest

import (
	"testing"

	"github.com/muir/nject"
)

// RunParallelMatrix uses t.Run() to fork into multiple threads of execution for each
// sub-test before any chains are evaluated. This forces the chains to share
// nothing between them. RunParallelMatrix does not provide any default injectors
// other than a *testing.T that comes from a named provider (named "testing.T")
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
func RunParallelMatrix(t *testing.T, chain ...any) {
	t.Parallel()
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
func RunMatrix(t *testing.T, chain ...any) {
	runMatrixTest(t, false, chain)
}

func runMatrixTest(t *testing.T, parallel bool, chain []any) {
	breakChain := func(t *testing.T, chain []any) (matrix map[string]nject.Provider, before []any, after []any) {
		for i, injector := range chain {
			matrix, ok := injector.(map[string]nject.Provider)
			if ok {
				return matrix, chain[:i], chain[i+1:]
			}
		}
		return nil, nil, chain
	}
	testingT := func(t *testing.T) []any {
		return []any{nject.Provide("testing.T", func() *testing.T { return t })}
	}

	matrix, before, after := breakChain(t, chain)
	if matrix == nil {
		t.Log("No matrix found in matrix testing, perhaps the specifier is in a Sequence? (not allowed)")
		t.Fail()
		return
	}

	var startTest func(t *testing.T, matrix map[string]nject.Provider, before []any, after []any)
	startTest = func(t *testing.T, matrix map[string]nject.Provider, before []any, after []any) {
		for name, subChain := range matrix {
			subChain := subChain
			t.Run(name, func(t *testing.T) {
				if parallel {
					t.Parallel()
				}
				matrix, newBefore, newAfter := breakChain(t, after)
				if matrix == nil {
					RunTest(t, combineSlices(testingT(t), before, []any{subChain}, after)...)
				} else {
					startTest(t, matrix, combineSlices(before, newBefore, []any{subChain}), newAfter)
				}
			})
		}
	}
	startTest(t, matrix, before, after)
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
