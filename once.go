//go:build go1.21

package ntest

import "sync"

var onceFunc = sync.OnceFunc
