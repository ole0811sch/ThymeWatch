package main

import (
	"context"
	"log"
)

func Todo[T any]() T {
	panic("Not implemented")
}

func Zero[T any]() T {
	var zero T
	return zero
}

type Void = struct{}

var VVoid = struct{}{}

func DerivedFromTwoContexts(ctxA context.Context, ctxB context.Context) (context.Context, context.CancelFunc) {
	ctxCpy, cancelCpy := context.WithCancel(ctxA)
	ctxJoined, cancelJoined := context.WithCancel(ctxB)
	context.AfterFunc(ctxCpy, cancelJoined)
	return ctxJoined, cancelCpy
}

func Ignoring2ndArg[R any, T any, S any](f func(t T) S) func(T, R) S {
	return func(t T, r R) S {
		return f(t)
	}
}

func LogIfError(prefix string, err error) {
	if err != nil {
		log.Printf("%v: %v\n", prefix, err)
	}
}
