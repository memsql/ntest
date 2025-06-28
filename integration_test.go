package ntest_test

import (
	"testing"

	"github.com/memsql/ntest"
)

// TestReplaceLoggerIntegration tests ReplaceLogger behavior in a real go test environment
// The real part of this test is inside Makefile
func TestReplaceLoggerIntegration(t *testing.T) {
	// Use ReplaceLogger directly (no BufferedLogger)
	logger := ntest.ReplaceLogger(t, func(s string) {
		t.Helper()
		t.Log("PREFIX " + s)
	})

	logger.Log("test message from user code")
}
