//go:build solution

package tree

import (
	"cmp"
	"iter"
)

func (t *Tree[K, V]) Put(k K, v V) {
	// Итеративная вставка: на вырожденном дереве (отсортированные ключи)
	// рекурсия была бы глубиной O(n).
	link := &t.root
	for *link != nil {
		n := *link
		switch c := cmp.Compare(k, n.key); {
		case c < 0:
			link = &n.left
		case c > 0:
			link = &n.right
		default:
			n.val = v
			return
		}
	}
	*link = &node[K, V]{key: k, val: v}
	t.size++
}

func (t *Tree[K, V]) Get(k K) (V, bool) {
	n := t.root
	for n != nil {
		switch c := cmp.Compare(k, n.key); {
		case c < 0:
			n = n.left
		case c > 0:
			n = n.right
		default:
			return n.val, true
		}
	}
	var zero V
	return zero, false
}

func (t *Tree[K, V]) Len() int { return t.size }

// walk возвращает false, если потребитель попросил остановиться. Этот
// флаг «пробрасывается» вверх по рекурсии — так мы не вызовем yield
// повторно после false.
func (n *node[K, V]) walk(yield func(K, V) bool) bool {
	if n == nil {
		return true
	}
	return n.left.walk(yield) && yield(n.key, n.val) && n.right.walk(yield)
}

func (n *node[K, V]) walkBack(yield func(K, V) bool) bool {
	if n == nil {
		return true
	}
	return n.right.walkBack(yield) && yield(n.key, n.val) && n.left.walkBack(yield)
}

func (n *node[K, V]) walkRange(lo, hi K, yield func(K, V) bool) bool {
	if n == nil {
		return true
	}
	// Левое поддерево имеет смысл, только если n.key > lo; правое — если n.key < hi.
	if cmp.Less(lo, n.key) && !n.left.walkRange(lo, hi, yield) {
		return false
	}
	if cmp.Compare(lo, n.key) <= 0 && cmp.Less(n.key, hi) && !yield(n.key, n.val) {
		return false
	}
	if cmp.Less(n.key, hi) {
		return n.right.walkRange(lo, hi, yield)
	}
	return true
}

func (t *Tree[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) { t.root.walk(yield) }
}

func (t *Tree[K, V]) Backward() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) { t.root.walkBack(yield) }
}

func (t *Tree[K, V]) Keys() iter.Seq[K] {
	return func(yield func(K) bool) {
		for k := range t.All() {
			if !yield(k) {
				return
			}
		}
	}
}

func (t *Tree[K, V]) Range(lo, hi K) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) { t.root.walkRange(lo, hi, yield) }
}

func MergeSorted[T cmp.Ordered](a, b iter.Seq[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		nextA, stopA := iter.Pull(a)
		defer stopA()
		nextB, stopB := iter.Pull(b)
		defer stopB()

		va, okA := nextA()
		vb, okB := nextB()
		for okA || okB {
			// Берём из a, если b кончилась или va <= vb (стабильность).
			if okA && (!okB || cmp.Compare(va, vb) <= 0) {
				if !yield(va) {
					return
				}
				va, okA = nextA()
			} else {
				if !yield(vb) {
					return
				}
				vb, okB = nextB()
			}
		}
	}
}
