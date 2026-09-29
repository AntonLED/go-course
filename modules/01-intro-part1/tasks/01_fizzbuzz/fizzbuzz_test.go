package fizzbuzz

import (
	"slices"
	"testing"
)

func TestFizzBuzz(t *testing.T) {
	tests := []struct {
		n    int
		want []string
	}{
		{-3, []string{}},
		{0, []string{}},
		{1, []string{"1"}},
		{5, []string{"1", "2", "Fizz", "4", "Buzz"}},
		{15, []string{"1", "2", "Fizz", "4", "Buzz", "Fizz", "7", "8", "Fizz", "Buzz", "11", "Fizz", "13", "14", "FizzBuzz"}},
	}
	for _, tt := range tests {
		got := FizzBuzz(tt.n)
		if got == nil {
			t.Errorf("FizzBuzz(%d) вернул nil, ожидался пустой (не nil) срез", tt.n)
			continue
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("FizzBuzz(%d) = %q, ожидалось %q", tt.n, got, tt.want)
		}
	}
	if got := FizzBuzz(30); len(got) != 30 {
		t.Errorf("len(FizzBuzz(30)) = %d, ожидалось 30", len(got))
	} else if got[29] != "FizzBuzz" || got[24] != "Buzz" || got[26] != "Fizz" {
		t.Errorf("FizzBuzz(30): неверные элементы 25/27/30: %q %q %q", got[24], got[26], got[29])
	}
}

func TestColumns(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		cols  int
		want  string
	}{
		{"пусто", nil, 3, ""},
		{"cols=0", []string{"a"}, 0, ""},
		{"cols<0", []string{"a"}, -1, ""},
		{"ровно", []string{"1", "2", "Fizz", "4"}, 2, "   1    2\nFizz    4\n"},
		{"неполная строка", []string{"1", "2", "Fizz"}, 2, "   1    2\nFizz\n"},
		{"одна колонка", []string{"a", "bb"}, 1, " a\nbb\n"},
		{"cols больше len", []string{"a", "bb"}, 5, " a bb\n"},
		{"кириллица", []string{"я", "ёж", "кот"}, 3, "  я  ёж кот\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Columns(tt.items, tt.cols); got != tt.want {
				t.Errorf("Columns(%q, %d) = %q, ожидалось %q", tt.items, tt.cols, got, tt.want)
			}
		})
	}
}

func BenchmarkFizzBuzz(b *testing.B) {
	for i := 0; i < b.N; i++ {
		FizzBuzz(1000)
	}
}
