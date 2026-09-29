//go:build solution

package slugtests

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// CheckSlugify — табличная проверка: по нескольку кейсов на каждое правило спецификации.
func CheckSlugify(t testing.TB, slugify func(string) string) {
	t.Helper()
	cases := []struct {
		name, in, want string
	}{
		// базовые
		{"empty", "", ""},
		{"simple", "hello", "hello"},
		{"lower", "Hello World", "hello-world"},
		// правило 1: юникод и цифры
		{"cyrillic", "Привет, Мир", "привет-мир"},
		{"greek", "ΣΟΦΙΑ", "σοφια"},
		{"digits", "Go 1.23 Release", "go-1-23-release"},
		{"onlyDigits", "2024", "2024"},
		// правило 2: апострофы
		{"apostrophe", "Don't Stop", "dont-stop"},
		{"typographic", "Rock’n’Roll", "rocknroll"},
		{"onlyApostrophes", "'’'", ""},
		// правило 3: схлопывание разделителей
		{"collapse", "a  --  b", "a-b"},
		{"punct", "a!@#$%^&*()b", "a-b"},
		{"underscore", "snake_case_name", "snake-case-name"},
		{"emoji", "go 🚀 fast", "go-fast"},
		{"tabsNewlines", "a\t\nb", "a-b"},
		// правило 4: края
		{"leading", "  --hello", "hello"},
		{"trailing", "hello!!!  ", "hello"},
		{"onlySeparators", " - _ !", ""},
		// правило 5: длина в рунах
		{"exactMax", strings.Repeat("a", MaxLen), strings.Repeat("a", MaxLen)},
		{"overMax", strings.Repeat("a", MaxLen+5), strings.Repeat("a", MaxLen)},
		{"runesNotBytes", strings.Repeat("я", MaxLen+1), strings.Repeat("я", MaxLen)},
		{"cutAtDash", strings.Repeat("a", MaxLen-1) + " bcd", strings.Repeat("a", MaxLen-1)},
		{"cutBeforeDash", strings.Repeat("a", MaxLen) + " b", strings.Repeat("a", MaxLen)},
	}
	for _, c := range cases {
		got := slugify(c.in)
		if got != c.want {
			t.Errorf("%s: slugify(%q) = %q, ожидалось %q", c.name, c.in, got, c.want)
		}
	}

	// Свойства, которые должны выполняться для любого входа.
	for _, in := range []string{"Съешь же ещё этих мягких французских булок, да выпей чаю", "x--y__z", "  Ünïcödé  "} {
		got := slugify(in)
		switch {
		case !utf8.ValidString(got):
			t.Errorf("slugify(%q) = %q — невалидный UTF-8", in, got)
		case utf8.RuneCountInString(got) > MaxLen:
			t.Errorf("slugify(%q) длиннее %d рун", in, MaxLen)
		case strings.HasPrefix(got, "-") || strings.HasSuffix(got, "-") || strings.Contains(got, "--"):
			t.Errorf("slugify(%q) = %q — лишние дефисы", in, got)
		case slugify(got) != got:
			t.Errorf("slugify не идемпотентна на %q: %q → %q", in, got, slugify(got))
		}
	}
}
