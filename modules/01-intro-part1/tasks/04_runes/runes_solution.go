//go:build solution

// Package runes — задача к уроку «Работа со строками».
package runes

import (
	"strconv"
	"strings"
	"unicode"
)

// Reverse переворачивает строку по рунам.
func Reverse(s string) string {
	// []rune(s) декодирует UTF-8; битые байты превращаются в U+FFFD.
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

// IsPalindrome проверяет палиндром по буквам и цифрам без учёта регистра.
func IsPalindrome(s string) bool {
	r := make([]rune, 0, len(s))
	for _, c := range s { // range по строке идёт по рунам
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			r = append(r, unicode.ToLower(c))
		}
	}
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		if r[i] != r[j] {
			return false
		}
	}
	return true
}

// Compress — RLE по рунам.
func Compress(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	var prev rune
	n := 0
	flush := func() {
		if n == 0 {
			return
		}
		b.WriteRune(prev)
		if n > 1 {
			b.WriteString(strconv.Itoa(n))
		}
	}
	for _, c := range s {
		if n > 0 && c == prev {
			n++
			continue
		}
		flush()
		prev, n = c, 1
	}
	flush()
	return b.String()
}

// Caesar — шифр Цезаря для латиницы.
func Caesar(s string, shift int) string {
	// Нормализуем сдвиг в [0, 26): в Go -1 % 26 == -1.
	shift = (shift%26 + 26) % 26
	return strings.Map(func(c rune) rune {
		switch {
		case c >= 'a' && c <= 'z':
			return 'a' + (c-'a'+rune(shift))%26
		case c >= 'A' && c <= 'Z':
			return 'A' + (c-'A'+rune(shift))%26
		}
		return c
	}, s)
}
