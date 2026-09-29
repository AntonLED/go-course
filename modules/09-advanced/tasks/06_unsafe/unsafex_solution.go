//go:build solution

package unsafex

import (
	"reflect"
	"unsafe"
)

// BytesToString: строка указывает на тот же массив, что и b.
// Корректно, только пока b не изменяется (иначе «неизменяемая» строка
// поменяется у всех, кто её держит, — например, ключ в map).
func BytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// StringToBytes: срез указывает на байты строки. Запись в него — UB:
// литералы лежат в read-only сегменте, запись вызовет SIGSEGV.
func StringToBytes(s string) []byte {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// sliceHeader повторяет runtime.slice (см. reflect.SliceHeader, который
// устарел именно из-за неправильного использования).
type sliceHeader struct {
	data unsafe.Pointer
	len  int
	cap  int
}

// SliceLenCap: шаблон №1 из документации unsafe — конверсия *T1 → *T2
// между типами с совместимой раскладкой памяти.
func SliceLenCap[T any](s []T) (length, capacity int) {
	h := (*sliceHeader)(unsafe.Pointer(&s))
	return h.len, h.cap
}

// Float64Bits: тоже шаблон №1 (именно так устроен math.Float64bits).
func Float64Bits(f float64) uint64 {
	return *(*uint64)(unsafe.Pointer(&f))
}

// ReadField: unsafe.Add сохраняет значение типа unsafe.Pointer, поэтому GC
// всё время «видит» указатель. Выражение
// unsafe.Pointer(uintptr(base)+off) допустимо только одним выражением
// (шаблон №3), а хранить uintptr в переменной — ошибка.
func ReadField[T any](base unsafe.Pointer, off uintptr) T {
	return *(*T)(unsafe.Add(base, off))
}

// Overlap сравнивает адреса как числа. Превращать uintptr обратно в
// указатель здесь не нужно — поэтому это безопасно.
func Overlap(a, b []byte) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	a0 := uintptr(unsafe.Pointer(unsafe.SliceData(a)))
	b0 := uintptr(unsafe.Pointer(unsafe.SliceData(b)))
	return a0 < b0+uintptr(len(b)) && b0 < a0+uintptr(len(a))
}

func isPOD(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// Reinterpret переинтерпретирует []From как []To без копирования.
func Reinterpret[To, From any](s []From) ([]To, error) {
	if !isPOD(reflect.TypeFor[To]()) || !isPOD(reflect.TypeFor[From]()) {
		return nil, ErrNotPOD
	}
	if len(s) == 0 {
		return nil, nil
	}
	var from From
	var to To
	total := uintptr(len(s)) * unsafe.Sizeof(from)
	if total%unsafe.Sizeof(to) != 0 {
		return nil, ErrSize
	}
	p := unsafe.Pointer(unsafe.SliceData(s))
	if uintptr(p)%unsafe.Alignof(to) != 0 {
		return nil, ErrAlign
	}
	// cap тоже урезаем до len: append к результату не должен «заглянуть»
	// в память за границами исходных данных под видом To.
	return unsafe.Slice((*To)(p), total/unsafe.Sizeof(to)), nil
}
