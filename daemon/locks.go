package main

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	lock "github.com/viney-shih/go-lock"
)

func WithLock[T any](ctx context.Context, lock lock.RWMutex, f func() T) (T, error) {
	if !lock.TryLockWithContext(ctx) {
		return Zero[T](), ctx.Err()
	}
	defer lock.Unlock()
	return f(), nil
}

func WithRLock[T any](ctx context.Context, lock lock.RWMutex, f func() T) (T, error) {
	if !lock.RTryLockWithContext(ctx) {
		return Zero[T](), ctx.Err()
	}
	defer lock.RUnlock()
	return f(), nil
}

func WithLock2[T, R any](ctx context.Context, lock lock.RWMutex, f func() (T, R)) (T, R, error) {
	if !lock.TryLockWithContext(ctx) {
		return Zero[T](), Zero[R](), ctx.Err()
	}
	defer lock.Unlock()
	v1, v2 := f()
	return v1, v2, nil
}

func WithRLock2[T, R any](ctx context.Context, lock lock.RWMutex, f func() (T, R)) (T, R, error) {
	if !lock.RTryLockWithContext(ctx) {
		return Zero[T](), Zero[R](), ctx.Err()
	}
	defer lock.RUnlock()
	v1, v2 := f()
	return v1, v2, nil
}

var LockboxTrackLockSites = false

type Lockbox[T any] struct {
	LastLockedAt string
	value        T
	mutex        *lock.CASMutex
}

func MakeLockbox[T any](value T) Lockbox[T] {
	return Lockbox[T]{value: value, mutex: lock.NewCASMutex()}
}

func (lb *Lockbox[T]) Read(ctx context.Context, f func(value *T)) error {
	_, err := WithRLock(ctx, lb.mutex, func() Void {
		if LockboxTrackLockSites {
			lb.LastLockedAt = fmt.Sprintf("%+v", errors.New("RLocked"))
		}
		f(&lb.value)
		return VVoid
	})
	return err
}

func (lb *Lockbox[T]) ReadReturning[R any](ctx context.Context, f func(value *T) R) (R, error) {
	return WithRLock(ctx, lb.mutex, func() R {
		if LockboxTrackLockSites {
			lb.LastLockedAt = fmt.Sprintf("%+v", errors.New("RLocked"))
		}
		return f(&lb.value)
	})
}

func (lb *Lockbox[T]) ReadReturning2[R1, R2 any](ctx context.Context, f func(value *T) (R1, R2)) (R1, R2, error) {
	return WithRLock2(ctx, lb.mutex, func() (R1, R2) {
		if LockboxTrackLockSites {
			lb.LastLockedAt = fmt.Sprintf("%+v", errors.New("RLocked"))
		}
		return f(&lb.value)
	})
}

func (lb *Lockbox[T]) Write(ctx context.Context, f func(value *T)) error {
	_, err := WithLock(ctx, lb.mutex, func() Void {
		if LockboxTrackLockSites {
			lb.LastLockedAt = fmt.Sprintf("%+v", errors.New("WLocked"))
		}
		f(&lb.value)
		return VVoid
	})
	return err
}

func (lb *Lockbox[T]) WriteReturning[R any](ctx context.Context, f func(value *T) R) (R, error) {
	return WithLock(ctx, lb.mutex, func() R {
		if LockboxTrackLockSites {
			lb.LastLockedAt = fmt.Sprintf("%+v", errors.New("WLocked"))
		}
		return f(&lb.value)
	})
}

func (lb *Lockbox[T]) WriteReturning2[R1, R2 any](ctx context.Context, f func(value *T) (R1, R2)) (R1, R2, error) {
	return WithLock2(ctx, lb.mutex, func() (R1, R2) {
		if LockboxTrackLockSites {
			lb.LastLockedAt = fmt.Sprintf("%+v", errors.New("WLocked"))
		}
		return f(&lb.value)
	})
}
