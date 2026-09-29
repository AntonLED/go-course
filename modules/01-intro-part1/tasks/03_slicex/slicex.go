//go:build !solution

// Package slicex — задача к уроку «Работа с массивами и срезами».
package slicex

// Unique возвращает новый срез без повторов, сохраняя порядок первых вхождений.
// Входной срез не изменяется. Unique(nil) == nil, для пустого не-nil среза —
// пустой не-nil срез.
func Unique(s []int) []int {
	// TODO: реализуйте
	panic("TODO")
}

// RotateLeft циклически сдвигает элементы s влево на k позиций НА месте
// (без выделения нового среза того же размера). Отрицательное k — сдвиг вправо.
// k может быть больше len(s). Пустой срез — ничего не делать (и не паниковать).
func RotateLeft(s []int, k int) {
	// TODO: реализуйте
	panic("TODO")
}

// Chunk делит s на подряд идущие куски длины size (последний может быть короче).
// Куски ссылаются на память s (без копирования), но append к любому куску
// НЕ должен портить соседние куски и сам s.
// size <= 0 → panic. Пустой s → nil.
func Chunk(s []int, size int) [][]int {
	// TODO: реализуйте (подсказка: full slice expression s[low:high:max])
	panic("TODO")
}

// Insert вставляет vs в s перед индексом i и возвращает результат (как append).
// 0 <= i <= len(s), иначе panic. Должна корректно работать, даже если vs —
// это часть самого s (например, Insert(s, 1, s[2:]...)) и у s хватает capacity.
func Insert(s []int, i int, vs ...int) []int {
	// TODO: реализуйте
	panic("TODO")
}

// FilterInPlace оставляет в s только элементы, для которых keep возвращает true,
// сохраняя порядок, переиспользуя backing array s (без новых аллокаций).
// «Хвост» s[len(result):len(s)] должен быть обнулён.
func FilterInPlace(s []int, keep func(int) bool) []int {
	// TODO: реализуйте
	panic("TODO")
}
