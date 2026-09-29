//go:build solution

// Package anagrams — задача к уроку «Работа с картами (map)».
package anagrams

import (
	"slices"
	"strings"
)

// key строит канонический ключ анаграммы: отсортированные руны в нижнем регистре.
func key(w string) string {
	r := []rune(strings.ToLower(w))
	slices.Sort(r)
	return string(r)
}

// GroupAnagrams группирует слова-анаграммы в детерминированном порядке.
func GroupAnagrams(words []string) [][]string {
	// map хранит не саму группу, а её индекс в срезе groups:
	// так порядок групп задаётся срезом, а не случайным обходом map.
	idx := make(map[string]int, len(words))
	groups := make([][]string, 0)
	for _, w := range words {
		k := key(w)
		if gi, ok := idx[k]; ok { // comma-ok: отличаем «нет ключа» от индекса 0
			groups[gi] = append(groups[gi], w)
			continue
		}
		idx[k] = len(groups)
		groups = append(groups, []string{w})
	}
	return groups
}

// Invert переворачивает map.
func Invert(m map[string]int) map[int][]string {
	res := make(map[int][]string, len(m)) // make — не nil, писать безопасно
	for k, v := range m {
		res[v] = append(res[v], k) // append к nil-срезу из «пустого» значения работает
	}
	for _, keys := range res {
		// keys разделяет backing array со значением в map — сортировка на месте видна в map.
		slices.Sort(keys)
	}
	return res
}

// TwoSum — один проход с map «значение → первый индекс».
func TwoSum(nums []int, target int) (i, j int, ok bool) {
	first := make(map[int]int, len(nums))
	for j, v := range nums {
		if i, found := first[target-v]; found {
			return i, j, true
		}
		if _, seen := first[v]; !seen { // запоминаем только первое вхождение → минимальный i
			first[v] = j
		}
	}
	return 0, 0, false
}
