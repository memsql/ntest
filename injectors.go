package ntest

import (
	"context"
)

// This file contains example injectors that may be useful

// Cancel is the injected type for the function type that will cancel
// a Context that has been augmented with AutoCancel.
type Cancel func()

// AutoCancel adjusts context.Context so that it will be cancelled
// when the test finishes. It can be cancelled early by calling
// the returned Cancel function.
func AutoCancel(ctx context.Context, t T) (context.Context, Cancel) {
	ctx, cancel := context.WithCancel(ctx)
	onlyOnce := onceFunc(cancel)
	t.Cleanup(onlyOnce)
	return ctx, onlyOnce
}
