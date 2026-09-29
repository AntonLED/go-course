//go:build !solution

// Package runes — задача к уроку «Работа со строками».
package runes

// Reverse переворачивает строку по рунам (не по байтам!).
// Некорректные байты UTF-8 заменяются на U+FFFD (как при конвертации в []rune).
func Reverse(s string) string {
	// TODO: реализуйте
	panic("TODO")
}

// IsPalindrome сообщает, является ли строка палиндромом, если учитывать
// только буквы и цифры (unicode.IsLetter/IsDigit) и игнорировать регистр.
// Пустая строка (и строка без букв/цифр) — палиндром.
func IsPalindrome(s string) bool {
	// TODO: реализуйте
	panic("TODO")
}

// Compress выполняет RLE-сжатие по рунам: серия из n > 1 одинаковых рун
// записывается как руна + n, одиночная руна — как есть.
// "aaabcc" → "a3bc2", "ёёж" → "ё2ж".
func Compress(s string) string {
	// TODO: реализуйте (strings.Builder, strconv)
	panic("TODO")
}

// Caesar сдвигает латинские буквы a–z и A–Z на shift позиций по кругу
// (регистр сохраняется), остальные символы (включая кириллицу) не меняются.
// shift может быть отрицательным и больше 26.
func Caesar(s string, shift int) string {
	// TODO: реализуйте (strings.Map)
	panic("TODO")
}
