package kv_test

import (
	"errors"
	"flag"
	"fmt"
	"maps"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kv "gocourse/modules/06-testing/tasks/02_kvfuzz"
)

// go test -run TestEncodeGolden -update — перезаписать эталонные файлы.
var update = flag.Bool("update", false, "перезаписать golden-файлы в testdata/")

func TestDecodeTable(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[string]string
		err  error
	}{
		{"empty", "", map[string]string{}, nil},
		{"spaces", "   ", map[string]string{}, nil},
		{"one", "a=1", map[string]string{"a": "1"}, nil},
		{"many", "b=2 a=1  c=x.y/z", map[string]string{"a": "1", "b": "2", "c": "x.y/z"}, nil},
		{"quoted", `msg="hello world" n=5`, map[string]string{"msg": "hello world", "n": "5"}, nil},
		{"emptyQuoted", `a=""`, map[string]string{"a": ""}, nil},
		{"escapes", `q="say \"hi\"\\n"`, map[string]string{"q": `say "hi"\n`}, nil},
		{"unicode", `city="Санкт-Петербург"`, map[string]string{"city": "Санкт-Петербург"}, nil},
		{"invalidUTF8", `b="\xff\xfe"`, map[string]string{"b": "\xff\xfe"}, nil},
		{"keyChars", "a.b-c_D9=1", map[string]string{"a.b-c_D9": "1"}, nil},
		{"leadTrail", "  a=1  ", map[string]string{"a": "1"}, nil},

		{"noEq", "abc", nil, kv.ErrSyntax},
		{"noEqSecond", "a=1 b", nil, kv.ErrSyntax},
		{"emptyKey", "=1", nil, kv.ErrInvalidKey},
		{"badKey", "a b=1", nil, kv.ErrSyntax},
		{"badKeyChar", "ключ=1", nil, kv.ErrInvalidKey},
		{"emptyBare", "a= b=1", nil, kv.ErrSyntax},
		{"emptyBareEnd", "a=", nil, kv.ErrSyntax},
		{"unclosed", `a="abc`, nil, kv.ErrSyntax},
		{"unclosedEsc", `a="abc\"`, nil, kv.ErrSyntax},
		{"badEscape", `a="\q"`, nil, kv.ErrSyntax},
		{"garbageAfterQuote", `a="x"y`, nil, kv.ErrSyntax},
		{"quoteInBare", `a=x"y`, nil, kv.ErrSyntax},
		{"eqInBare", "a=b=c", nil, kv.ErrSyntax},
		{"tab", "a=1\tb=2", nil, kv.ErrSyntax},
		{"dup", "a=1 a=2", nil, kv.ErrDuplicateKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := kv.Decode(tt.in)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("kv.Decode(%q): ошибка %v, ожидалась %v", tt.in, err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("kv.Decode(%q): неожиданная ошибка %v", tt.in, err)
			}
			if got == nil || !maps.Equal(got, tt.want) {
				t.Errorf("kv.Decode(%q) = %#v, ожидалось %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestEncodeErrors(t *testing.T) {
	for _, k := range []string{"", "a b", "a=b", "ключ", "a\""} {
		if _, err := kv.Encode(map[string]string{k: "v"}); !errors.Is(err, kv.ErrInvalidKey) {
			t.Errorf("Encode с ключом %q: ошибка %v, ожидалась kv.ErrInvalidKey", k, err)
		}
	}
}

// goldenCases — входы для golden-теста. Порядок важен: он же порядок строк в файле.
var goldenCases = []struct {
	name string
	m    map[string]string
}{
	{"nil", nil},
	{"simple", map[string]string{"b": "2", "a": "1", "c": "3"}},
	{"quoting", map[string]string{"msg": "hello world", "empty": "", "q": `he said "hi"`, "bs": `C:\dir`}},
	{"special", map[string]string{"eq": "a=b", "tab": "a\tb", "nl": "line1\nline2", "ctrl": "\x00\x7f"}},
	{"unicode", map[string]string{"ru": "привет", "emoji": "🚀", "bad": "\xff"}},
	{"bare", map[string]string{"path": "/usr/local/bin", "url": "http://x.io/?a&b", "neg": "-1.5e10"}},
}

func TestEncodeGolden(t *testing.T) {
	var b strings.Builder
	for _, c := range goldenCases {
		got, err := kv.Encode(c.m)
		if err != nil {
			t.Fatalf("kv.Encode(%s): %v", c.name, err)
		}
		fmt.Fprintf(&b, "%s: %s\n", c.name, got)
	}
	path := filepath.Join("testdata", "encode.golden")
	if *update {
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if b.String() != string(want) {
		t.Errorf("вывод Encode не совпадает с %s\n--- получено:\n%s--- ожидалось:\n%s", path, b.String(), want)
	}
}

func TestRoundTripRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	alphabet := []string{"a", "Z", "0", " ", "\"", "\\", "=", "\n", "\t", "я", "\xff", "'", "-", "\x00", "🚀"}
	keyAlpha := "abcXYZ019_.-"
	for i := 0; i < 2000; i++ {
		m := map[string]string{}
		for n := rng.Intn(5); n > 0; n-- {
			var k, v strings.Builder
			for l := 1 + rng.Intn(6); l > 0; l-- {
				k.WriteByte(keyAlpha[rng.Intn(len(keyAlpha))])
			}
			for l := rng.Intn(8); l > 0; l-- {
				v.WriteString(alphabet[rng.Intn(len(alphabet))])
			}
			m[k.String()] = v.String()
		}
		s, err := kv.Encode(m)
		if err != nil {
			t.Fatalf("kv.Encode(%#v): %v", m, err)
		}
		got, err := kv.Decode(s)
		if err != nil {
			t.Fatalf("kv.Decode(kv.Encode(%#v)) = kv.Decode(%q): %v", m, s, err)
		}
		if !maps.Equal(got, m) {
			t.Fatalf("round trip: %#v → %q → %#v", m, s, got)
		}
	}
}

// FuzzDecode: Decode не паникует на любом входе, а успешно разобранное переживает Encode→Decode.
// Запуск фаззинга: go test -fuzz=FuzzDecode -fuzztime=30s ./modules/06-testing/tasks/02_kvfuzz/
// Без -fuzz выполняются только seed-входы (и файлы из testdata/fuzz/FuzzDecode, если есть).
func FuzzDecode(f *testing.F) {
	for _, s := range []string{"", "a=1", `a="x y" b=2`, `a="\xff"`, "a=1 a=2", `a="`, `k="\u0000"`, "  x=y  "} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		m, err := kv.Decode(s)
		if err != nil {
			return
		}
		enc, err := kv.Encode(m)
		if err != nil {
			t.Fatalf("kv.Decode(%q) ok, но kv.Encode(%#v): %v", s, m, err)
		}
		m2, err := kv.Decode(enc)
		if err != nil || !maps.Equal(m, m2) {
			t.Fatalf("kv.Decode(%q)=%#v → Encode=%q → Decode=%#v, %v", s, m, enc, m2, err)
		}
	})
}

// FuzzRoundTrip: любая пара (валидный ключ, произвольное значение) переживает Encode→Decode.
func FuzzRoundTrip(f *testing.F) {
	f.Add("k", "v")
	f.Add("key", `with "quotes" and \ slashes`)
	f.Add("x", "\xff\x00\n")
	f.Add("a.b", "")
	f.Fuzz(func(t *testing.T, k, v string) {
		m := map[string]string{k: v}
		enc, err := kv.Encode(m)
		if err != nil {
			if !errors.Is(err, kv.ErrInvalidKey) {
				t.Fatalf("Encode: неожиданная ошибка %v", err)
			}
			return
		}
		got, err := kv.Decode(enc)
		if err != nil || !maps.Equal(got, m) {
			t.Fatalf("round trip %q=%q → %q → %#v, %v", k, v, enc, got, err)
		}
	})
}

func ExampleEncode() {
	s, _ := kv.Encode(map[string]string{"user": "ann", "msg": "hello world", "n": "5"})
	fmt.Println(s)
	// Output: msg="hello world" n=5 user=ann
}
