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

type Lockbox[T any] struct {
	value T
	mutex sync.RWMutex
}

func MakeLockbox[T any](value T) Lockbox[T] {
	return Lockbox[T]{value: value}
}

func (lb *Lockbox[T]) Read(f func(value *T)) {
	lb.mutex.RLock()
	defer lb.mutex.RUnlock()
	f(&lb.value)
}

func (lb *Lockbox[T]) ReadReturning[R any](f func(value *T) R) R {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()
	return f(&lb.value)
}

func (lb *Lockbox[T]) ReadReturning2[R1 any, R2 any](f func(value *T) (R1, R2)) (R1, R2) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()
	return f(&lb.value)
}

func (lb *Lockbox[T]) Write(f func(value *T)) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()
	f(&lb.value)
}

func (lb *Lockbox[T]) WriteReturning[R any](f func(value *T) R) R {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()
	return f(&lb.value)
}

func (lb *Lockbox[T]) WriteReturning2[R1 any, R2 any](f func(value *T) (R1, R2)) (R1, R2) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()
	return f(&lb.value)
}
