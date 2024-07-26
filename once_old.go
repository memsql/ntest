//go:build !go1.21

package ntest

import "sync"

func onceFunc(f func()) func() {
	var once sync.Once
	return func() {
		once.Do(f)
	}
}
