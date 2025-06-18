package ntest

import (
	"testing"

	"github.com/muir/nject/v2"
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
func RunParallelMatrix(t RunT, chain ...any) {
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
func RunMatrix(t RunT, chain ...any) {
	runMatrixTest(t, false, chain)
}

func runMatrixTest(t RunT, parallel bool, chain []any) {
	matrix, before, after := breakChain(chain)
	if matrix == nil {
		panic("matrix test requires a matrix")
	}

	var startTest func(RunT, map[string]nject.Provider, []any, []any)
	startTest = func(t RunT, matrix map[string]nject.Provider, before []any, after []any) {
		for name, subChain := range matrix {
			subChain := subChain
			t.Run(name, func(subT *testing.T) {
				// Implement ReWrapper logic here
				var reWrapped RunT
				if reWrapper, ok := t.(ReWrapper); ok {
					reWrapped = NewTestRunner(reWrapper.ReWrap(subT))
				} else {
					reWrapped = NewTestRunner(subT)
				}

				if parallel {
					reWrapped.Parallel()
				}
				matrix, newBefore, newAfter := breakChain(after)
				if matrix == nil {
					RunTest(reWrapped, combineSlices(before, []any{subChain}, after)...)
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
