package tree

import "cmp"

type node[K cmp.Ordered, V any] struct {
	key         K
	val         V
	left, right *node[K, V]
}

// Tree — несбалансированное двоичное дерево поиска. Нулевое значение —
// пустое дерево, готовое к работе.
type Tree[K cmp.Ordered, V any] struct {
	root *node[K, V]
	size int
}
