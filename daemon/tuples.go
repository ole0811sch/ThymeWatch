package main

type Pair[T, U any] struct {
	First  T
	Second U
}

type Triple[T, U, V any] struct {
	First  T
	Second U
	Third  V
}

func MakePair[T, U any](first T, second U) Pair[T, U] {
	return Pair[T, U]{first, second}
}

func MakeTriple[T, U, V any](first T, second U, third V) Triple[T, U, V] {
	return Triple[T, U, V]{first, second, third}
}
