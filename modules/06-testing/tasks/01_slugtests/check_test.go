package slugtests

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unicode"
)

// spyTB — фейковый testing.TB: перехватывает ошибки проверки, не роняя настоящий тест.
// Встраивание testing.TB даёт реализацию «приватного» метода интерфейса и остальных методов.
type spyTB struct {
	testing.TB
	mu      sync.Mutex
	failed  bool
	skipped bool
	msgs    []string
}

func (s *spyTB) record(fail bool, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fail {
		s.failed = true
	}
	s.msgs = append(s.msgs, msg)
}

func (s *spyTB) Helper()                           {}
func (s *spyTB) Log(args ...any)                   { s.record(false, fmt.Sprint(args...)) }
func (s *spyTB) Logf(format string, args ...any)   { s.record(false, fmt.Sprintf(format, args...)) }
func (s *spyTB) Error(args ...any)                 { s.record(true, fmt.Sprint(args...)) }
func (s *spyTB) Errorf(format string, args ...any) { s.record(true, fmt.Sprintf(format, args...)) }
func (s *spyTB) Fail()                             { s.record(true, "Fail()") }
func (s *spyTB) FailNow()                          { s.record(true, "FailNow()"); runtime.Goexit() }
func (s *spyTB) Fatal(args ...any)                 { s.record(true, fmt.Sprint(args...)); runtime.Goexit() }
func (s *spyTB) Fatalf(format string, args ...any) {
	s.record(true, fmt.Sprintf(format, args...))
	runtime.Goexit()
}
func (s *spyTB) Failed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed
}
func (s *spyTB) SkipNow() {
	s.mu.Lock()
	s.skipped = true
	s.mu.Unlock()
	runtime.Goexit()
}
func (s *spyTB) Skip(args ...any)                 { s.SkipNow() }
func (s *spyTB) Skipf(format string, args ...any) { s.SkipNow() }
func (s *spyTB) Skipped() bool                    { return s.skipped }

// runCheck запускает проверку в отдельной горутине: Fatal/FailNow делают runtime.Goexit,
// что завершит только её, а не наш тест.
func runCheck(t *testing.T, impl func(string) string) *spyTB {
	spy := &spyTB{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if v := recover(); v != nil {
				spy.record(true, fmt.Sprintf("panic: %v", v))
			}
		}()
		CheckSlugify(spy, impl)
	}()
	<-done
	return spy
}

// mutant — Slugify с «переключателями» багов.
type mutant struct {
	noLower          bool // не переводит в нижний регистр
	asciiLower       bool // понижает только A-Z
	noCollapse       bool // каждый разделитель → отдельный дефис
	keepLeading      bool // дефис в начале
	keepTrailing     bool // дефис в конце
	asciiOnly        bool // не-ASCII буквы считает разделителями
	digitsAsSep      bool // цифры считает разделителями
	apostropheSep    bool // апостроф — разделитель
	typographicSep   bool // ’ — разделитель (удаляется только ')
	underscoreWord   bool // '_' — часть слова
	byteTruncate     bool // обрезает по байтам
	noTruncate       bool // не обрезает
	noTrimAfterCut   bool // не убирает дефис после обрезки
	maxLen           int  // 0 → MaxLen
	dropFirstRuneCap bool // Title-case: первая буква остаётся заглавной
}

func (m mutant) slugify(s string) string {
	maxLen := MaxLen
	if m.maxLen != 0 {
		maxLen = m.maxLen
	}
	isWord := func(r rune) bool {
		if m.underscoreWord && r == '_' {
			return true
		}
		if m.asciiOnly && r > unicode.MaxASCII {
			return false
		}
		if unicode.IsDigit(r) {
			return !m.digitsAsSep
		}
		return unicode.IsLetter(r)
	}
	lower := func(r rune) rune {
		switch {
		case m.noLower:
			return r
		case m.asciiLower:
			if r >= 'A' && r <= 'Z' {
				return r + 32
			}
			return r
		}
		return unicode.ToLower(r)
	}
	var out []rune
	pending := false
	first := true
	for _, r := range s {
		isApos := (r == '\'' && !m.apostropheSep) || (r == '’' && !m.apostropheSep && !m.typographicSep)
		switch {
		case isApos:
		case isWord(r):
			if pending && (len(out) > 0 || m.keepLeading) {
				out = append(out, '-')
			}
			pending = false
			if first && m.dropFirstRuneCap {
				out = append(out, r)
			} else {
				out = append(out, lower(r))
			}
			first = false
		default:
			if m.noCollapse && len(out) > 0 {
				out = append(out, '-')
				continue
			}
			pending = true
		}
	}
	if pending && m.keepTrailing && len(out) > 0 {
		out = append(out, '-')
	}
	if m.noCollapse {
		for len(out) > 0 && out[len(out)-1] == '-' && !m.keepTrailing {
			out = out[:len(out)-1]
		}
	}
	res := string(out)
	switch {
	case m.noTruncate:
	case m.byteTruncate:
		if len(res) > maxLen {
			res = res[:maxLen]
		}
	default:
		if len(out) > maxLen {
			res = string(out[:maxLen])
		}
	}
	if !m.noTrimAfterCut && !m.keepTrailing {
		res = strings.TrimRight(res, "-")
	}
	return res
}

var mutants = map[string]mutant{
	"не переводит в нижний регистр":      {noLower: true},
	"понижает регистр только у ASCII":    {asciiLower: true},
	"не схлопывает разделители":          {noCollapse: true},
	"оставляет дефис в начале":           {keepLeading: true},
	"оставляет дефис в конце":            {keepTrailing: true},
	"выбрасывает не-ASCII буквы":         {asciiOnly: true},
	"выбрасывает цифры":                  {digitsAsSep: true},
	"апостроф превращает в дефис":        {apostropheSep: true},
	"не удаляет типографский апостроф ’": {typographicSep: true},
	"считает '_' частью слова":           {underscoreWord: true},
	"обрезает по байтам, а не по рунам":  {byteTruncate: true},
	"не обрезает длину":                  {noTruncate: true},
	"оставляет дефис после обрезки":      {noTrimAfterCut: true},
	"обрезает до 31 руны (off-by-one)":   {maxLen: MaxLen - 1},
	"не понижает регистр первой буквы":   {dropFirstRuneCap: true},
}

func TestMutantsAreReallyDifferent(t *testing.T) {
	// Самопроверка тестового стенда: нулевой мутант совпадает с эталоном.
	for _, in := range []string{"", "Hello, World!", "  Don't Привет__x 1.2 ", strings.Repeat("ab ", 20)} {
		if got, want := (mutant{}).slugify(in), Slugify(in); got != want {
			t.Fatalf("стенд: mutant{}(%q) = %q, Slugify = %q", in, got, want)
		}
	}
}

func TestCheckPassesOnCorrect(t *testing.T) {
	spy := runCheck(t, Slugify)
	if spy.failed {
		t.Fatalf("CheckSlugify нашла ошибки в ПРАВИЛЬНОЙ реализации:\n%s", strings.Join(spy.msgs, "\n"))
	}
	if spy.skipped {
		t.Fatal("CheckSlugify не должна вызывать Skip")
	}
	var calls int
	runCheck(t, func(s string) string { calls++; return Slugify(s) })
	if calls < 5 {
		t.Errorf("CheckSlugify вызвала slugify всего %d раз — маловато для проверки", calls)
	}
}

func TestCheckCatchesMutants(t *testing.T) {
	for name, m := range mutants {
		t.Run(name, func(t *testing.T) {
			spy := runCheck(t, m.slugify)
			if !spy.failed {
				t.Errorf("мутант «%s» не пойман — добавьте тест-кейс, который его отличает", name)
			}
		})
	}
}
