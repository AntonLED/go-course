//go:build solution

// Package slicex — задача к уроку «Работа с массивами и срезами».
package slicex

import "slices"

// Unique возвращает новый срез без повторов, сохраняя порядок.
func Unique(s []int) []int {
	if s == nil {
		return nil
	}
	seen := make(map[int]struct{}, len(s))
	res := make([]int, 0, len(s)) // не nil даже при len(s) == 0
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		res = append(res, v)
	}
	return res
}

// RotateLeft — сдвиг на месте через три разворота:
// rev(s[:k]), rev(s[k:]), rev(s) — O(n) времени, O(1) памяти.
func RotateLeft(s []int, k int) {
	n := len(s)
	if n == 0 {
		return // иначе k % 0 → panic: integer divide by zero
	}
	k %= n
	if k < 0 { // в Go остаток имеет знак делимого: -1 % 5 == -1
		k += n
	}
	if k == 0 {
		return
	}
	slices.Reverse(s[:k])
	slices.Reverse(s[k:])
	slices.Reverse(s)
}

// Chunk делит s на куски, ограничивая cap каждого куска его длиной.
func Chunk(s []int, size int) [][]int {
	if size <= 0 {
		panic("slicex: Chunk size must be positive")
	}
	if len(s) == 0 {
		return nil
	}
	res := make([][]int, 0, (len(s)+size-1)/size)
	for lo := 0; lo < len(s); lo += size {
		hi := min(lo+size, len(s))
		// Третий индекс ограничивает cap: append к куску будет вынужден
		// выделить новый массив вместо того, чтобы затереть s[hi].
		res = append(res, s[lo:hi:hi])
	}
	return res
}

// Insert вставляет vs перед индексом i.
func Insert(s []int, i int, vs ...int) []int {
	n, m := len(s), len(vs)
	if i < 0 || i > n {
		panic("slicex: Insert index out of range")
	}
	if m == 0 {
		return s
	}
	if n+m > cap(s) {
		// Места нет — собираем результат в новом массиве, vs читается
		// до какой-либо записи, поэтому aliasing не страшен.
		res := make([]int, n+m, max(n+m, 2*n))
		copy(res, s[:i])
		copy(res[i:], vs)
		copy(res[i+m:], s[i:])
		return res
	}
	// Места хватает — будем сдвигать хвост s на месте. Если vs указывает
	// в тот же массив, сдвиг затрёт его содержимое, поэтому копируем vs.
	vs = slices.Clone(vs)
	s = s[:n+m]
	copy(s[i+m:], s[i:n]) // copy корректно работает с перекрытием
	copy(s[i:], vs)
	return s
}

// FilterInPlace — классический приём «запись по второму указателю».
func FilterInPlace(s []int, keep func(int) bool) []int {
	res := s[:0] // тот же backing array, len 0
	for _, v := range s {
		if keep(v) {
			res = append(res, v) // никогда не выйдет за cap(s)
		}
	}
	// Обнуляем хвост: для указателей это важно, чтобы GC мог освободить память.
	clear(s[len(res):])
	return res
}
