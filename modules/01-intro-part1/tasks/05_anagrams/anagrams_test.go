package anagrams

import (
	"fmt"
	"slices"
	"testing"
)

func TestGroupAnagrams(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want [][]string
	}{
		{"пусто", nil, nil},
		{"классика",
			[]string{"eat", "tea", "tan", "ate", "nat", "bat"},
			[][]string{{"eat", "tea", "ate"}, {"tan", "nat"}, {"bat"}}},
		{"регистр и дубликаты",
			[]string{"Listen", "silent", "enlist", "silent", "google"},
			[][]string{{"Listen", "silent", "enlist", "silent"}, {"google"}}},
		{"кириллица",
			[]string{"кот", "ток", "кто", "Отк", "мир", "рим"},
			[][]string{{"кот", "ток", "кто", "Отк"}, {"мир", "рим"}}},
		{"пустое слово", []string{"", "a", ""}, [][]string{{"", ""}, {"a"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Несколько прогонов: порядок обхода map случаен, результат — нет.
			for range 20 {
				got := GroupAnagrams(tt.in)
				if len(got) != len(tt.want) || !slices.EqualFunc(got, tt.want, slices.Equal[[]string]) {
					t.Fatalf("GroupAnagrams(%q) = %q, ожидалось %q", tt.in, got, tt.want)
				}
			}
		})
	}
}

func TestGroupAnagramsNoAliasing(t *testing.T) {
	in := []string{"ab", "ba"}
	got := GroupAnagrams(in)
	got[0][0] = "XX"
	if in[0] != "ab" {
		t.Errorf("результат GroupAnagrams разделяет память со входом")
	}
}

func TestInvert(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 1, "d": 3, "e": 1}
	got := Invert(m)
	want := map[int][]string{1: {"a", "c", "e"}, 2: {"b"}, 3: {"d"}}
	if fmt.Sprint(got) != fmt.Sprint(want) { // fmt печатает map с сортировкой ключей
		t.Errorf("Invert(%v) = %v, ожидалось %v", m, got, want)
	}
	if len(m) != 5 {
		t.Errorf("Invert изменил исходную map: %v", m)
	}

	empty := Invert(nil)
	if empty == nil {
		t.Fatal("Invert(nil) вернул nil map — запись в неё вызовет панику")
	}
	empty[1] = []string{"x"} // не должно паниковать
}

func TestTwoSum(t *testing.T) {
	tests := []struct {
		nums   []int
		target int
		i, j   int
		ok     bool
	}{
		{[]int{2, 7, 11, 15}, 9, 0, 1, true},
		{[]int{3, 2, 4}, 6, 1, 2, true},
		{[]int{3, 3}, 6, 0, 1, true},
		{[]int{3}, 6, 0, 0, false},
		{nil, 0, 0, 0, false},
		{[]int{1, 2, 3}, 100, 0, 0, false},
		{[]int{5, 1, 5, 5}, 10, 0, 2, true},
		{[]int{1, 4, 2, 3}, 5, 0, 1, true},
		{[]int{-3, 4, 3, 90}, 0, 0, 2, true},
		{[]int{0, 4, 3, 0}, 0, 0, 3, true},
		{[]int{2, 2, 1, 3}, 4, 0, 1, true},
	}
	for _, tt := range tests {
		i, j, ok := TwoSum(tt.nums, tt.target)
		if ok != tt.ok || (ok && (i != tt.i || j != tt.j)) {
			t.Errorf("TwoSum(%v, %d) = (%d, %d, %v), ожидалось (%d, %d, %v)",
				tt.nums, tt.target, i, j, ok, tt.i, tt.j, tt.ok)
		}
	}
}
