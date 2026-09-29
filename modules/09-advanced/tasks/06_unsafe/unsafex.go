//go:build !solution

package unsafex

import "unsafe"

// BytesToString возвращает строку, разделяющую память с b (без копирования).
// После вызова b нельзя изменять — строки в Go неизменяемы.
func BytesToString(b []byte) string {
	panic("TODO") // подсказка: unsafe.String + unsafe.SliceData
}

// StringToBytes возвращает срез, разделяющий память с s (без копирования).
// Результат нельзя изменять (память строки может быть read-only). "" → nil.
func StringToBytes(s string) []byte {
	panic("TODO") // подсказка: unsafe.Slice + unsafe.StringData
}

// SliceLenCap читает len и cap прямо из заголовка среза через unsafe
// (без встроенных len/cap) — упражнение на раскладку runtime-структур.
func SliceLenCap[T any](s []T) (length, capacity int) {
	panic("TODO")
}

// Float64Bits — аналог math.Float64bits через конверсию *float64 → *uint64.
func Float64Bits(f float64) uint64 {
	panic("TODO")
}

// ReadField читает значение типа T по смещению off от base
// (off обычно получают через unsafe.Offsetof).
func ReadField[T any](base unsafe.Pointer, off uintptr) T {
	panic("TODO") // подсказка: unsafe.Add, а не uintptr-арифметика
}

// Overlap сообщает, пересекаются ли области памяти a[0:len(a)] и b[0:len(b)].
func Overlap(a, b []byte) bool {
	panic("TODO")
}

// Reinterpret переинтерпретирует память среза []From как []To без копирования.
// Оба типа — числовые (int*, uint*, float*, uintptr), иначе ErrNotPOD.
// Суммарный размер должен делиться на Sizeof(To) (ErrSize), адрес начала —
// быть выровнен под To (ErrAlign). Пустой вход → nil, nil.
func Reinterpret[To, From any](s []From) ([]To, error) {
	panic("TODO")
}
