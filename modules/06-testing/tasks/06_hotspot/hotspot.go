//go:build !solution

package hotspot

import (
	"slices"
	"strconv"
	"unicode"
)

// Функции ниже корректны, но на больших входах работают квадратичное время.
// Найдите горячие точки (профилировщиком или глазами) и перепишите, сохранив поведение.

// Dedup возвращает строки без повторов, сохраняя порядок первых вхождений.
func Dedup(lines []string) []string {
	var out []string
	for _, l := range lines {
		if !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// TopWords возвращает k самых частых слов текста.
// Слово — максимальная последовательность букв/цифр (unicode), приводится к нижнему регистру.
// Сортировка: по убыванию Count, при равенстве — по возрастанию Word. k <= 0 → пустой результат.
func TopWords(text string, k int) []WordCount {
	counts := []WordCount{}
	add := func(w string) {
		for i := range counts {
			if counts[i].Word == w {
				counts[i].Count++
				return
			}
		}
		counts = append(counts, WordCount{w, 1})
	}
	word := ""
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			word += string(unicode.ToLower(r))
			continue
		}
		if word != "" {
			add(word)
			word = ""
		}
	}
	if word != "" {
		add(word)
	}
	// пузырьковая сортировка
	for i := 0; i < len(counts); i++ {
		for j := 0; j < len(counts)-1-i; j++ {
			a, b := counts[j], counts[j+1]
			if a.Count < b.Count || a.Count == b.Count && a.Word > b.Word {
				counts[j], counts[j+1] = b, a
			}
		}
	}
	if k <= 0 {
		return []WordCount{}
	}
	if k > len(counts) {
		k = len(counts)
	}
	return counts[:k]
}

// Report строит отчёт: для каждой строки "<номер с 1>: <строка>\n".
func Report(lines []string) string {
	s := ""
	for i, l := range lines {
		s += strconv.Itoa(i+1) + ": " + l + "\n"
	}
	return s
}
