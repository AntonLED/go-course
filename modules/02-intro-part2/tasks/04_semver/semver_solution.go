//go:build solution

package semver

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
)

func invalid(s, reason string) error {
	return fmt.Errorf("semver: %q: %s: %w", s, reason, ErrInvalid)
}

func Parse(s string) (Version, error) {
	rest, ok := strings.CutPrefix(s, "v")
	if !ok {
		return Version{}, invalid(s, "нет префикса v")
	}
	var v Version

	// Build отрезаем первым: в нём тоже могут быть '-'.
	if i := strings.IndexByte(rest, '+'); i >= 0 {
		v.Build = rest[i+1:]
		rest = rest[:i]
		for _, id := range strings.Split(v.Build, ".") {
			if !validIdent(id) {
				return Version{}, invalid(s, "плохой build-идентификатор")
			}
		}
	}
	if i := strings.IndexByte(rest, '-'); i >= 0 {
		v.Pre = strings.Split(rest[i+1:], ".")
		rest = rest[:i]
		for _, id := range v.Pre {
			if !validIdent(id) || (isNumeric(id) && len(id) > 1 && id[0] == '0') {
				return Version{}, invalid(s, "плохой pre-release идентификатор")
			}
		}
	}

	parts := strings.Split(rest, ".")
	if len(parts) != 3 {
		return Version{}, invalid(s, "нужно ровно три компоненты")
	}
	nums := [3]*uint64{&v.Major, &v.Minor, &v.Patch}
	for i, p := range parts {
		if !isNumeric(p) || (len(p) > 1 && p[0] == '0') {
			return Version{}, invalid(s, "плохое число")
		}
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil { // переполнение
			return Version{}, invalid(s, err.Error())
		}
		*nums[i] = n
	}
	return v, nil
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func validIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '-') {
			return false
		}
	}
	return true
}

func (v Version) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "v%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.Pre) > 0 {
		b.WriteString("-" + strings.Join(v.Pre, "."))
	}
	if v.Build != "" {
		b.WriteString("+" + v.Build)
	}
	return b.String()
}

func Compare(a, b Version) int {
	if c := cmp.Compare(a.Major, b.Major); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Minor, b.Minor); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Patch, b.Patch); c != 0 {
		return c
	}
	// Без pre-release — старше, чем с ним.
	switch {
	case len(a.Pre) == 0 && len(b.Pre) == 0:
		return 0
	case len(a.Pre) == 0:
		return 1
	case len(b.Pre) == 0:
		return -1
	}
	for i := 0; i < min(len(a.Pre), len(b.Pre)); i++ {
		if c := compareIdent(a.Pre[i], b.Pre[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a.Pre), len(b.Pre))
}

func compareIdent(x, y string) int {
	xn, yn := isNumeric(x), isNumeric(y)
	switch {
	case xn && yn:
		// Ведущих нулей нет, поэтому сравнение длины, затем строки, корректно
		// и не боится переполнения.
		if c := cmp.Compare(len(x), len(y)); c != 0 {
			return c
		}
		return strings.Compare(x, y)
	case xn:
		return -1
	case yn:
		return 1
	}
	return strings.Compare(x, y)
}

func Max(vs ...string) (string, error) {
	var (
		best    string
		bestVer Version
	)
	for i, s := range vs {
		v, err := Parse(s)
		if err != nil {
			return "", fmt.Errorf("semver: max: %w", err)
		}
		if i == 0 || Compare(v, bestVer) > 0 {
			best, bestVer = s, v
		}
	}
	return best, nil
}

func CheckModulePath(path string, v Version) error {
	// Суффикс версии — последний элемент пути вида "v<цифры>".
	var suffix string // цифры после "v"; "" — суффикса нет
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		if d, ok := strings.CutPrefix(path[i+1:], "v"); ok && isNumeric(d) {
			suffix = d
		}
	}
	want := strconv.FormatUint(v.Major, 10)
	switch {
	case suffix == "0" || suffix == "1" || (len(suffix) > 1 && suffix[0] == '0'):
		// /v0, /v1 и суффиксы с ведущими нулями (/v02) недопустимы никогда.
		return fmt.Errorf("module %s: суффикс /v%s недопустим: %w", path, suffix, ErrMajorMismatch)
	case v.Major < 2 && suffix != "":
		return fmt.Errorf("module %s@%s: для v0/v1 суффикс не нужен: %w", path, v, ErrMajorMismatch)
	case v.Major >= 2 && suffix != want:
		return fmt.Errorf("module %s@%s: путь должен оканчиваться на /v%d: %w", path, v, v.Major, ErrMajorMismatch)
	}
	return nil
}
