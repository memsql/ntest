package ntest_test

import (
	"context"
	"testing"

	"github.com/memsql/ntest"
)

func TestCancel(t *testing.T) {
	t.Parallel()
	ntest.RunTest(t, context.Background, ntest.AutoCancel, func(
		ctx context.Context,
		cancel ntest.Cancel,
	) {
		select {
		case <-ctx.Done():
			t.Fatal("context is cancelled before cancel()")
		default:
		}
		cancel()
		select {
		case <-ctx.Done():
			t.Log("context is cancelled")
		default:
			t.Fatal("context is not cancelled after cancel()")
		}
	})
}
