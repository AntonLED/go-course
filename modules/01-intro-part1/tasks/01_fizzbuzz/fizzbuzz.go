//go:build !solution

// Package fizzbuzz — разминочная задача урока «Осознанное знакомство с Go».
package fizzbuzz

// FizzBuzz возвращает срез из n строк для чисел 1..n:
// кратные 15 → "FizzBuzz", кратные 3 → "Fizz", кратные 5 → "Buzz",
// остальные — само число (strconv.Itoa). При n <= 0 — пустой срез.
func FizzBuzz(n int) []string {
	// TODO: реализуйте
	panic("TODO")
}

// Columns раскладывает items в таблицу по cols элементов в строке.
// Каждая ячейка выравнивается вправо по ширине самого длинного элемента
// (ширина считается в символах-рунах), ячейки разделяются одним пробелом,
// каждая строка заканчивается '\n'. При cols <= 0 или пустом items — "".
func Columns(items []string, cols int) string {
	// TODO: реализуйте (подсказка: fmt.Fprintf(&b, "%*s", width, s), strings.Builder)
	panic("TODO")
}
