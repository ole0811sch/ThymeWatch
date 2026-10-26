package main

import "context"

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
