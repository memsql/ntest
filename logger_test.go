package ntest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memsql/ntest"
)

var _ ntest.T = &testing.T{}

func TestPrefixLogger(t *testing.T) {
	var caught []string
	captureT := ntest.ReplaceLogger(t, func(s string) {
		t.Log("captured:", s)
		caught = append(caught, s)
	})
	extraDetail := ntest.ExtraDetailLogger(captureT, "some-prefix")
	extraDetail.Log("not-formatted", 3)
	extraDetail.Logf("formatted '%s'", "quoted")

	require.Equal(t, 2, len(caught), "len caught")
	assert.Regexp(t, `some-prefix \d\d:\d\d:\d\d not-formatted 3$`, caught[0], "unformatted")
	assert.Regexp(t, `some-prefix \d\d:\d\d:\d\d formatted 'quoted'$`, caught[1], "formatted")
}
