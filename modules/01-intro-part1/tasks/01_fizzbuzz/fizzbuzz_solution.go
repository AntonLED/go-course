//go:build solution

// Package fizzbuzz — разминочная задача урока «Осознанное знакомство с Go».
package fizzbuzz

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// FizzBuzz возвращает срез из n строк для чисел 1..n.
func FizzBuzz(n int) []string {
	if n <= 0 {
		return []string{}
	}
	// Заранее знаем размер — выделяем память один раз.
	res := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		switch {
		case i%15 == 0: // проверка на 15 должна идти первой
			res = append(res, "FizzBuzz")
		case i%3 == 0:
			res = append(res, "Fizz")
		case i%5 == 0:
			res = append(res, "Buzz")
		default:
			res = append(res, strconv.Itoa(i))
		}
	}
	return res
}

// Columns раскладывает items в таблицу по cols элементов в строке.
func Columns(items []string, cols int) string {
	if cols <= 0 || len(items) == 0 {
		return ""
	}
	width := 0
	for _, s := range items {
		// len(s) — это байты; для кириллицы нужна длина в рунах.
		width = max(width, utf8.RuneCountInString(s))
	}
	var b strings.Builder
	for i, s := range items {
		if i%cols != 0 {
			b.WriteByte(' ')
		}
		// fmt считает ширину %*s в рунах, а не в байтах.
		fmt.Fprintf(&b, "%*s", width, s)
		if i%cols == cols-1 || i == len(items)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
