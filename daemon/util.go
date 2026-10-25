package main

func Todo[T any]() T {
	panic("Not implemented")
}

func Zero[T any]() T {
	var zero T
	return zero
}

type Void = struct{}

var VVoid = struct{}{}
