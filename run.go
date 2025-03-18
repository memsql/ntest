package ntest

import (
	"testing"

	"github.com/muir/nject/v2"
	"github.com/stretchr/testify/require"
)

// RunTest provides the basic framework for running a test.
//
// If running a testing.T test, pass that. If running a Ginkgo test, pass ginkgo.GinkgoT().
func RunTest(t T, chain ...interface{}) {
	tseq := nject.Sequence("T",
		func() T { return t },
	)
	if testingT, ok := t.(*testing.T); ok {
		tseq = tseq.Append("realT",
			func() *testing.T { return testingT },
		)
	}
	err := nject.Run(t.Name(),
		tseq,
		func(inner func() error, t *testing.T) {
			err := inner()
			require.NoErrorf(t, err, "setup for test %s failed", t.Name())
		},
		nject.Sequence("user-chain", chain...),
		nject.NonFinal(nject.Shun(func(inner func()) error { inner(); return nil })),
	)
	if err != nil && err.Error() != nject.DetailedError(err) {
		t.Logf("nject detailed error: %s", nject.DetailedError(err))
	}
	require.NoErrorf(t, err, "invalid injection chain for %s", t.Name())
}
