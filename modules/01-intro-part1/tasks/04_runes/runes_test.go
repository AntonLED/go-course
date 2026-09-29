package runes

import (
	"testing"
	"unicode/utf8"
)

func TestReverse(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"a", "a"},
		{"hello", "olleh"},
		{"Привет", "тевирП"},
		{"Hello, 世界!", "!界世 ,olleH"},
		{"ёжик🦔", "🦔кижё"},
		{"a\xffb", "b�a"},
	}
	for _, tt := range tests {
		got := Reverse(tt.in)
		if got != tt.want {
			t.Errorf("Reverse(%q) = %q, ожидалось %q", tt.in, got, tt.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("Reverse(%q) вернул невалидный UTF-8: %q", tt.in, got)
		}
	}
}

func TestIsPalindrome(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", true},
		{"!!!", true},
		{"a", true},
		{"abba", true},
		{"abca", false},
		{"A man, a plan, a canal: Panama", true},
		{"А роза упала на лапу Азора", true},
		{"Ёж", false},
		{"12321", true},
		{"123 21", true},
		{"ab", false},
		{"Ωmega", false},
	}
	for _, tt := range tests {
		if got := IsPalindrome(tt.in); got != tt.want {
			t.Errorf("IsPalindrome(%q) = %v, ожидалось %v", tt.in, got, tt.want)
		}
	}
}

func TestCompress(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"a", "a"},
		{"aaabb", "a3b2"},
		{"aaabcc", "a3bc2"},
		{"abc", "abc"},
		{"aaaaaaaaaaaa", "a12"},
		{"ёёж", "ё2ж"},
		{"世世世界", "世3界"},
		{"aAa", "aAa"},
		{"  x", " 2x"},
	}
	for _, tt := range tests {
		if got := Compress(tt.in); got != tt.want {
			t.Errorf("Compress(%q) = %q, ожидалось %q", tt.in, got, tt.want)
		}
	}
}

func TestCaesar(t *testing.T) {
	tests := []struct {
		in    string
		shift int
		want  string
	}{
		{"", 5, ""},
		{"abc", 1, "bcd"},
		{"xyz", 3, "abc"},
		{"XYZ", 3, "ABC"},
		{"Hello, World!", 13, "Uryyb, Jbeyq!"},
		{"abc", -1, "zab"},
		{"abc", 26, "abc"},
		{"abc", 53, "bcd"},
		{"abc", -27, "zab"},
		{"Привет, Go!", 2, "Привет, Iq!"},
	}
	for _, tt := range tests {
		if got := Caesar(tt.in, tt.shift); got != tt.want {
			t.Errorf("Caesar(%q, %d) = %q, ожидалось %q", tt.in, tt.shift, got, tt.want)
		}
	}
	// Шифрование обратимо.
	src := "The quick brown fox, съешь же ещё"
	for _, k := range []int{1, 7, -30, 100} {
		if got := Caesar(Caesar(src, k), -k); got != src {
			t.Errorf("Caesar(Caesar(s, %d), %d) = %q, ожидалось %q", k, -k, got, src)
		}
	}
}

func BenchmarkCompress(b *testing.B) {
	s := "aaaaabbbbbcccccdddddeeeeeжжжжжззззз"
	for i := 0; i < b.N; i++ {
		Compress(s)
	}
}
