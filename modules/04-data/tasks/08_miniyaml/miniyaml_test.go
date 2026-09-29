package miniyaml

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	src := `# конфиг сервиса
---
name: billing
version: 1.10          # внимание: это float 1.1!
version_str: "1.10"
port: 8080
debug: false
ratio: 0.75
country: no            # YAML 1.2: строка "no", а не false
empty:
nothing: null
tilde: ~
url: http://example.com/a#frag
time: 12:30
quote: 'it''s'
escaped: "tab\there \"q\""
hash: "value # not a comment"
negative: -42
exp: 1e3
db:
  host: localhost
  port: 5432
  pool:
    max: 10
    idle_timeout: 30s
tags:
  - api
  - "42"
  - 42
features: [auth, "b, c", 3, true]
none: []
obj: {}
servers:
- name: a
  weight: 1
- name: b
  weight: 2
  labels:
    - x
matrix:
  -
    - 1
    - 2
`
	got, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := map[string]any{
		"name":        "billing",
		"version":     1.1,
		"version_str": "1.10",
		"port":        8080,
		"debug":       false,
		"ratio":       0.75,
		"country":     "no",
		"empty":       nil,
		"nothing":     nil,
		"tilde":       nil,
		"url":         "http://example.com/a#frag",
		"time":        "12:30",
		"quote":       "it's",
		"escaped":     "tab\there \"q\"",
		"hash":        "value # not a comment",
		"negative":    -42,
		"exp":         1000.0,
		"db": map[string]any{
			"host": "localhost",
			"port": 5432,
			"pool": map[string]any{"max": 10, "idle_timeout": "30s"},
		},
		"tags":     []any{"api", "42", 42},
		"features": []any{"auth", "b, c", 3, true},
		"none":     []any{},
		"obj":      map[string]any{},
		"servers": []any{
			map[string]any{"name": "a", "weight": 1},
			map[string]any{"name": "b", "weight": 2, "labels": []any{"x"}},
		},
		"matrix": []any{[]any{1, 2}},
	}
	for k, w := range want {
		if g, ok := got[k]; !ok || !reflect.DeepEqual(g, w) {
			t.Errorf("%s = %#v (есть: %v), ожидалось %#v", k, g, ok, w)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("лишний ключ %q = %#v", k, got[k])
		}
	}
}

func TestScalars(t *testing.T) {
	tests := []struct {
		in   string
		want any
	}{
		{"42", 42}, {"+7", 7}, {"-0", 0}, {"3.14", 3.14}, {"-.5", -0.5}, {"1.", 1.0}, {"2E-2", 0.02},
		{"true", true}, {"True", true}, {"FALSE", false},
		{"yes", "yes"}, {"on", "on"}, {"off", "off"}, {"No", "No"}, {"y", "y"},
		{"null", nil}, {"~", nil}, {"Null", nil},
		{"0x1F", 31}, {"0o17", 15}, {"0777", 777}, {"-0x1F", "-0x1F"}, {"0x", "0x"}, {"0o8", "0o8"}, {"0b101", "0b101"},
		{"1_000", "1_000"}, {"Inf", "Inf"}, {"NaN", "NaN"}, {"+-5", "+-5"}, {"1e", "1e"}, {".", "."},
		{"'single'", "single"}, {`"\u043f\u0440\u0438"`, "при"}, {`""`, ""}, {"''", ""},
		{"hello world", "hello world"}, {"привет", "привет"}, {"99999999999999999999", 1e20},
	}
	for _, tt := range tests {
		got, err := Parse("v: " + tt.in)
		if err != nil {
			t.Errorf("v: %s → ошибка %v", tt.in, err)
			continue
		}
		if !reflect.DeepEqual(got["v"], tt.want) {
			t.Errorf("v: %s → %#v (%T), ожидалось %#v (%T)", tt.in, got["v"], got["v"], tt.want, tt.want)
		}
	}
	for in, sign := range map[string]int{".inf": 1, "+.Inf": 1, ".INF": 1, "-.inf": -1, "-.Inf": -1} {
		got, _ := Parse("v: " + in)
		if f, _ := got["v"].(float64); !math.IsInf(f, sign) {
			t.Errorf("v: %s → %#v, ожидалась бесконечность со знаком %d", in, got["v"], sign)
		}
	}
	for _, in := range []string{".nan", ".NaN", ".NAN"} {
		got, _ := Parse("v: " + in)
		if f, ok := got["v"].(float64); !ok || !math.IsNaN(f) {
			t.Errorf("v: %s → %#v, ожидался float64 NaN", in, got["v"])
		}
	}
}

func TestEmptyAndMisc(t *testing.T) {
	for _, src := range []string{"", "\n\n", "# только комментарий\n", "---\n"} {
		got, err := Parse(src)
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("Parse(%q) = %#v, %v; ожидалась пустая (не nil) map", src, got, err)
		}
	}
	got, err := Parse("a: 1\r\nb:\r\n  c: 2\r\n")
	if err != nil || !reflect.DeepEqual(got, map[string]any{"a": 1, "b": map[string]any{"c": 2}}) {
		t.Errorf("CRLF: %#v, %v", got, err)
	}
	got, err = Parse(`"quoted key": 1` + "\n'k: 2': 2")
	if err != nil || !reflect.DeepEqual(got, map[string]any{"quoted key": 1, "k: 2": 2}) {
		t.Errorf("ключи в кавычках: %#v, %v", got, err)
	}
	got, err = Parse("list:\n  -\n  - x\n")
	if err != nil || !reflect.DeepEqual(got, map[string]any{"list": []any{nil, "x"}}) {
		t.Errorf("пустой элемент списка: %#v, %v", got, err)
	}
	got, err = Parse("a:\n    deep: 1\nb: 2")
	if err != nil || !reflect.DeepEqual(got, map[string]any{"a": map[string]any{"deep": 1}, "b": 2}) {
		t.Errorf("отступ 4: %#v, %v", got, err)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name, src, line string
	}{
		{"табуляция", "a:\n\tb: 1", "строка 2"},
		{"дубль ключа", "a: 1\nb: 2\na: 3", "строка 3"},
		{"лишний отступ", "a: 1\n  b: 2", "строка 2"},
		{"корень-список", "- a\n- b", "строка 1"},
		{"отступ в начале", "  a: 1", "строка 1"},
		{"нет двоеточия", "a: 1\njust text", "строка 2"},
		{"смешение map и list", "a:\n  b: 1\n  - c", "строка 3"},
		{"кривой отступ", "a:\n    b: 1\n  c: 2", "строка 3"},
		{"якорь", "base: &b 1\nother: *b", "строка 1"},
		{"многострочный", "text: |\n  hello", "строка 1"},
		{"flow map", "a: {b: 1}", "строка 1"},
		{"вложенный flow", "a: [1, [2]]", "строка 1"},
		{"незакрытая кавычка", `a: "oops`, "строка 1"},
		{"незакрытый список", "a: [1, 2", "строка 1"},
		{"одинокая {", "a: {", "строка 1"},
		{"- - в строке", "a:\n  - - x", "строка 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.src)
			if err == nil {
				t.Fatalf("Parse(%q): ожидалась ошибка", tt.src)
			}
			if !strings.Contains(err.Error(), tt.line) {
				t.Errorf("Parse(%q): ошибка %q должна указывать %q", tt.src, err, tt.line)
			}
		})
	}
}
