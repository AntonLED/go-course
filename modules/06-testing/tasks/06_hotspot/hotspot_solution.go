//go:build solution

package hotspot

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Dedup: множество на map — O(n) вместо O(n²).
func Dedup(lines []string) []string {
	seen := make(map[string]struct{}, len(lines))
	var out []string
	for _, l := range lines {
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		out = append(out, l)
	}
	return out
}

// TopWords: подсчёт в map, слово — подстрока исходного текста (без посимвольной конкатенации),
// сортировка O(u log u).
func TopWords(text string, k int) []WordCount {
	if k <= 0 {
		return []WordCount{}
	}
	counts := make(map[string]int)
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
	start := -1
	flush := func(end int) {
		if start >= 0 {
			counts[strings.ToLower(text[start:end])]++
			start = -1
		}
	}
	for i, r := range text {
		if isWord(r) {
			if start < 0 {
				start = i
			}
		} else {
			flush(i)
		}
	}
	flush(len(text))

	res := make([]WordCount, 0, len(counts))
	for w, c := range counts {
		res = append(res, WordCount{w, c})
	}
	slices.SortFunc(res, func(a, b WordCount) int {
		if c := cmp.Compare(b.Count, a.Count); c != 0 {
			return c
		}
		return strings.Compare(a.Word, b.Word)
	})
	if k < len(res) {
		res = res[:k]
	}
	return res
}

// Report: strings.Builder с заранее посчитанным размером — O(n).
// s += ... копирует всю накопленную строку на каждой итерации — O(n²) по байтам.
func Report(lines []string) string {
	size := 0
	for _, l := range lines {
		size += len(l) + 24
	}
	var b strings.Builder
	b.Grow(size)
	for i, l := range lines {
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(": ")
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return b.String()
}
