package processor

import (
	"iter"
)

func Reverse[T any](xs []T) iter.Seq2[int, T] {
	return func(yield func(index int, value T) bool) {
		for i := len(xs) - 1; i >= 0; i-- {
			if !yield(i, xs[i]) {
				break
			}
		}
	}
}
