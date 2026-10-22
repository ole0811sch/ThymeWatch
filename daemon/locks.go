package main

import "sync"

func WithLock(lock sync.Locker, f func()) {
	lock.Lock()
	defer lock.Unlock()
	f()
}

func WithLockReturning[T any](lock sync.Locker, f func() T) T {
	lock.Lock()
	defer lock.Unlock()
	return f()
}
