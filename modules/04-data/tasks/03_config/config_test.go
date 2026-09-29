package config

import (
	"errors"
	"flag"
	"reflect"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLevel(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want Level
	}{{"debug", LevelDebug}, {"INFO", LevelInfo}, {"Warning", LevelWarn}, {"warn", LevelWarn}, {" error ", LevelError}} {
		var l Level
		if err := l.Set(tt.in); err != nil || l != tt.want {
			t.Errorf("Level.Set(%q) = %v, %v; ожидалось %v", tt.in, l, err, tt.want)
		}
	}
	var l Level
	if err := l.Set("verbose"); err == nil {
		t.Error("Level.Set(\"verbose\") должен вернуть ошибку")
	}
	for l, want := range map[Level]string{LevelDebug: "debug", LevelInfo: "info", LevelWarn: "warn", LevelError: "error", 7: "Level(7)"} {
		if got := l.String(); got != want {
			t.Errorf("Level(%d).String() = %q, ожидалось %q", int(l), got, want)
		}
	}
	var _ flag.Value = (*Level)(nil)
	var _ flag.Value = (*StringList)(nil)
}

func TestStringList(t *testing.T) {
	var s StringList
	s.Set("a")
	s.Set("b, c,,")
	s.Set("")
	if !reflect.DeepEqual([]string(s), []string{"a", "b", "c"}) {
		t.Errorf("StringList = %q, ожидалось [a b c]", []string(s))
	}
	if got := s.String(); got != "a,b,c" {
		t.Errorf("String() = %q", got)
	}
	var nilList *StringList
	if got := nilList.String(); got != "" {
		t.Errorf("(*StringList)(nil).String() = %q, ожидалось пусто (контракт flag.Value)", got)
	}
}

func TestParse(t *testing.T) {
	def := Config{Addr: ":8080", Timeout: 5 * time.Second, Level: LevelInfo}
	with := func(f func(*Config)) Config { c := def; f(&c); return c }
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want Config
	}{
		{"по умолчанию", nil, nil, def},
		{"флаги", []string{"-addr", "localhost:9000", "-timeout=1m30s", "-level", "debug", "-v"}, nil,
			with(func(c *Config) {
				c.Addr, c.Timeout, c.Level, c.Verbose = "localhost:9000", 90*time.Second, LevelDebug, true
			})},
		{"двойной дефис", []string{"--addr=:1", "--v"}, nil, with(func(c *Config) { c.Addr, c.Verbose = ":1", true })},
		{"теги", []string{"-tag", "a", "-tag=b,c"}, nil, with(func(c *Config) { c.Tags = []string{"a", "b", "c"} })},
		{"позиционные", []string{"-v", "x.txt", "y.txt"}, nil, with(func(c *Config) { c.Verbose, c.Files = true, []string{"x.txt", "y.txt"} })},
		{"флаг после позиционного — тоже позиционный", []string{"x.txt", "-v"}, nil,
			with(func(c *Config) { c.Files = []string{"x.txt", "-v"} })},
		{"-- завершает флаги", []string{"--", "-v"}, nil, with(func(c *Config) { c.Files = []string{"-v"} })},
		{"окружение", nil, map[string]string{"APP_ADDR": ":7000", "APP_TIMEOUT": "250ms", "APP_LEVEL": "error"},
			with(func(c *Config) { c.Addr, c.Timeout, c.Level = ":7000", 250*time.Millisecond, LevelError })},
		{"флаги важнее окружения", []string{"-level=warn", "-addr", ":1"}, map[string]string{"APP_ADDR": ":7000", "APP_LEVEL": "error"},
			with(func(c *Config) { c.Addr, c.Level = ":1", LevelWarn })},
		{"-v=false", []string{"-v=false"}, nil, def},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.args, env(tt.env))
			if err != nil {
				t.Fatalf("Parse(%q): неожиданная ошибка %v", tt.args, err)
			}
			if len(got.Tags) == 0 {
				got.Tags = nil
			}
			if len(got.Files) == 0 {
				got.Files = nil
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q) =\n %+v\nожидалось\n %+v", tt.args, got, tt.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{"неизвестный флаг", []string{"-port", "80"}, nil},
		{"плохой уровень", []string{"-level", "loud"}, nil},
		{"плохая длительность", []string{"-timeout", "5"}, nil}, // без единиц — ошибка
		{"нулевой таймаут", []string{"-timeout", "0s"}, nil},
		{"отрицательный таймаут", []string{"-timeout=-1s"}, nil},
		{"флаг без значения", []string{"-addr"}, nil},
		{"плохой APP_TIMEOUT", nil, map[string]string{"APP_TIMEOUT": "soon"}},
		{"плохой APP_LEVEL", nil, map[string]string{"APP_LEVEL": "loud"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.args, env(tt.env)); err == nil {
				t.Errorf("Parse(%q, %v): ожидалась ошибка", tt.args, tt.env)
			}
		})
	}
	// Ошибка в окружении должна называть переменную.
	for name, val := range map[string]string{"APP_TIMEOUT": "soon", "APP_LEVEL": "loud"} {
		_, err := Parse(nil, env(map[string]string{name: val}))
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("Parse с %s=%q: ошибка %v должна упоминать имя переменной %s", name, val, err, name)
		}
	}
	for _, h := range []string{"-h", "-help", "--help"} {
		if _, err := Parse([]string{h}, env(nil)); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("Parse(%q): ожидалась flag.ErrHelp, получено %v", h, err)
		}
	}
}
