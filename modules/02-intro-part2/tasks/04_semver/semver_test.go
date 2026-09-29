package semver

import (
	"errors"
	"math/rand"
	"reflect"
	"slices"
	"testing"
)

func TestParseValid(t *testing.T) {
	tests := []struct {
		in   string
		want Version
	}{
		{"v0.0.0", Version{}},
		{"v1.2.3", Version{Major: 1, Minor: 2, Patch: 3}},
		{"v10.20.30", Version{Major: 10, Minor: 20, Patch: 30}},
		{"v1.0.0-alpha", Version{Major: 1, Pre: []string{"alpha"}}},
		{"v1.0.0-rc.1", Version{Major: 1, Pre: []string{"rc", "1"}}},
		{"v1.0.0-0.3.7", Version{Major: 1, Pre: []string{"0", "3", "7"}}},
		{"v1.0.0-x-y-z.--", Version{Major: 1, Pre: []string{"x-y-z", "--"}}},
		{"v1.0.0+20130313144700", Version{Major: 1, Build: "20130313144700"}},
		{"v1.0.0-beta+exp.sha.5114f85", Version{Major: 1, Pre: []string{"beta"}, Build: "exp.sha.5114f85"}},
		{"v1.0.0+build-1.001", Version{Major: 1, Build: "build-1.001"}},
		{"v2.0.0-rc-1+b-2", Version{Major: 2, Pre: []string{"rc-1"}, Build: "b-2"}},
		{"v1.0.0-00a.0", Version{Major: 1, Pre: []string{"00a", "0"}}}, // ведущие нули можно в буквенно-цифровых

	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := Parse(tt.in)
			if err != nil {
				t.Fatalf("Parse(%q) ошибка: %v", tt.in, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q) = %#v, ожидалось %#v", tt.in, got, tt.want)
			}
			if s := got.String(); s != tt.in {
				t.Errorf("Parse(%q).String() = %q, ожидалось исходную строку", tt.in, s)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	bad := []string{
		"", "v", "1.2.3", "v1", "v1.2", "v1.2.3.4", "v01.2.3", "v1.02.3", "v1.2.03",
		"v1.2.3-", "v1.2.3+", "v1.2.3-01", "v1.2.3-a..b", "v1.2.3-a_b", "v1.2.3+a..b",
		"v1.2.3+a b", "v-1.2.3", "v1.2.x", "V1.2.3", " v1.2.3", "v1.2.3 ",
		"v99999999999999999999.0.0", // переполнение uint64
	}
	for _, s := range bad {
		t.Run(s, func(t *testing.T) {
			_, err := Parse(s)
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("Parse(%q) err = %v, ожидалась обёртка ErrInvalid", s, err)
			}
		})
	}
}

func mustParse(t *testing.T, s string) Version {
	t.Helper()
	v, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return v
}

func TestCompareOrder(t *testing.T) {
	// Пример упорядоченного списка из спецификации semver.org + свои.
	ordered := []string{
		"v0.9.9",
		"v1.0.0-0.3.7",
		"v1.0.0-alpha",
		"v1.0.0-alpha.1",
		"v1.0.0-alpha.beta",
		"v1.0.0-beta",
		"v1.0.0-beta.2",
		"v1.0.0-beta.11",
		"v1.0.0-rc.1",
		"v1.0.0",
		"v1.0.1",
		"v1.1.0",
		"v1.10.0",
		"v2.0.0",
		"v10.0.0",
	}
	for i := range ordered {
		for j := range ordered {
			a, b := mustParse(t, ordered[i]), mustParse(t, ordered[j])
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got := Compare(a, b); got != want {
				t.Errorf("Compare(%s, %s) = %d, ожидалось %d", ordered[i], ordered[j], got, want)
			}
		}
	}
	// Build игнорируется.
	if c := Compare(mustParse(t, "v1.0.0+a"), mustParse(t, "v1.0.0+b")); c != 0 {
		t.Errorf("Compare(v1.0.0+a, v1.0.0+b) = %d, ожидалось 0", c)
	}
	// Буквенно-цифровые — по ASCII (заглавные раньше строчных), а
	// идентификатор с цифрой в начале, но с буквами, — не числовой.
	pairs := [][2]string{
		{"v1.0.0-Beta", "v1.0.0-alpha"},
		{"v1.0.0-2", "v1.0.0-1a"},
		{"v1.0.0-1-1", "v1.0.0-1a"},
		{"v1.0.0-alpha.9", "v1.0.0-alpha.10"},
	}
	for _, p := range pairs {
		a, b := mustParse(t, p[0]), mustParse(t, p[1])
		if c := Compare(a, b); c != -1 {
			t.Errorf("Compare(%s, %s) = %d, ожидалось -1", p[0], p[1], c)
		}
		if c := Compare(b, a); c != 1 {
			t.Errorf("Compare(%s, %s) = %d, ожидалось 1", p[1], p[0], c)
		}
	}
	// Длинные числовые идентификаторы сравниваются как числа.
	if c := Compare(mustParse(t, "v1.0.0-99999999999999999999999"), mustParse(t, "v1.0.0-100000000000000000000000")); c != -1 {
		t.Errorf("сравнение длинных числовых pre-release = %d, ожидалось -1", c)
	}

	// Сортировка перемешанного списка через Compare.
	shuffled := slices.Clone(ordered)
	r := rand.New(rand.NewSource(1))
	r.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	slices.SortFunc(shuffled, func(a, b string) int { return Compare(mustParse(t, a), mustParse(t, b)) })
	if !slices.Equal(shuffled, ordered) {
		t.Errorf("после сортировки %v, ожидалось %v", shuffled, ordered)
	}
}

func TestMax(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"v1.0.0"}, "v1.0.0"},
		{[]string{"v1.0.0-rc.1", "v1.0.0", "v0.99.0"}, "v1.0.0"},
		{[]string{"v1.9.0", "v1.10.0", "v1.2.0"}, "v1.10.0"}, // не строковое сравнение!
		{[]string{"v2.0.0-alpha", "v1.99.99"}, "v2.0.0-alpha"},
		{[]string{"v1.0.0+first", "v1.0.0+second"}, "v1.0.0+first"},
	}
	for _, tt := range tests {
		got, err := Max(tt.in...)
		if err != nil || got != tt.want {
			t.Errorf("Max(%v) = %q, %v; ожидалось %q", tt.in, got, err, tt.want)
		}
	}
	if _, err := Max("v1.0.0", "latest"); !errors.Is(err, ErrInvalid) {
		t.Errorf("Max с некорректной версией: err = %v, ожидалась обёртка ErrInvalid", err)
	}
}

func TestCheckModulePath(t *testing.T) {
	tests := []struct {
		path, ver string
		ok        bool
	}{
		{"github.com/a/b", "v0.1.0", true},
		{"github.com/a/b", "v1.5.0", true},
		{"github.com/a/b/v2", "v2.0.0", true},
		{"github.com/a/b/v3", "v3.1.4-rc.1", true},
		{"github.com/a/b", "v2.0.0", false},
		{"github.com/a/b/v2", "v3.0.0", false},
		{"github.com/a/b/v2", "v1.0.0", false},
		{"github.com/a/b/v1", "v1.0.0", false},
		{"github.com/a/b/v0", "v0.1.0", false},
		{"example.com/vendor", "v1.0.0", true}, // "vendor" — не суффикс версии
		{"example.com/v2x", "v2.0.0", false},
		{"mod/v12", "v12.0.0", true},
		{"github.com/a/b/v02", "v2.0.0", false}, // ведущий ноль в суффиксе
		{"github.com/a/b/v2", "v2.0.0+build", true},
	}
	for _, tt := range tests {
		err := CheckModulePath(tt.path, mustParse(t, tt.ver))
		if tt.ok && err != nil {
			t.Errorf("CheckModulePath(%q, %s) = %v, ожидалось nil", tt.path, tt.ver, err)
		}
		if !tt.ok && !errors.Is(err, ErrMajorMismatch) {
			t.Errorf("CheckModulePath(%q, %s) = %v, ожидалась обёртка ErrMajorMismatch", tt.path, tt.ver, err)
		}
	}
}

func BenchmarkCompare(b *testing.B) {
	x, _ := Parse("v1.0.0-beta.11")
	y, _ := Parse("v1.0.0-beta.2")
	for i := 0; i < b.N; i++ {
		Compare(x, y)
	}
}
