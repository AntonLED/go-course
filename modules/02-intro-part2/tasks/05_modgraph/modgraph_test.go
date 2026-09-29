package modgraph

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const goMod = `// Модуль примера
module example.com/app

go 1.23
toolchain go1.23.4

require github.com/single/dep v1.0.0

require (
	github.com/a/lib v1.2.0
	github.com/b/lib/v2 v2.3.1 // indirect
	golang.org/x/text v0.14.0 //indirect
	github.com/c/lib v0.1.0 // just a comment
)

replace github.com/a/lib => ../a-lib-fork

replace (
	github.com/b/lib/v2 v2.3.1 => github.com/me/b-lib/v2 v2.3.2
	golang.org/x/text => golang.org/x/text v0.15.0
)

exclude github.com/bad/mod v1.0.0

retract (
	v1.0.1 // опубликовали по ошибке
)
`

func TestParseGoMod(t *testing.T) {
	f, err := ParseGoMod(goMod)
	if err != nil {
		t.Fatalf("ParseGoMod: %v", err)
	}
	want := &File{
		Module: "example.com/app",
		Go:     "1.23",
		Require: []Requirement{
			{Module{"github.com/single/dep", "v1.0.0"}, false},
			{Module{"github.com/a/lib", "v1.2.0"}, false},
			{Module{"github.com/b/lib/v2", "v2.3.1"}, true},
			{Module{"golang.org/x/text", "v0.14.0"}, true},
			{Module{"github.com/c/lib", "v0.1.0"}, false},
		},
		Replace: []Replace{
			{Module{"github.com/a/lib", ""}, Module{"../a-lib-fork", ""}},
			{Module{"github.com/b/lib/v2", "v2.3.1"}, Module{"github.com/me/b-lib/v2", "v2.3.2"}},
			{Module{"golang.org/x/text", ""}, Module{"golang.org/x/text", "v0.15.0"}},
		},
	}
	if !reflect.DeepEqual(f, want) {
		t.Errorf("ParseGoMod:\n получили %+v\n ожидалось %+v", f, want)
	}
}

func TestParseGoModMinimal(t *testing.T) {
	f, err := ParseGoMod("module m\n")
	if err != nil || f.Module != "m" || f.Go != "" || len(f.Require) != 0 || len(f.Replace) != 0 {
		t.Errorf("ParseGoMod(\"module m\") = %+v, %v", f, err)
	}
}

func TestParseGoModErrors(t *testing.T) {
	tests := []struct {
		name, src, line string
	}{
		{"неизвестная директива", "module m\nfoo bar\n", "go.mod:2:"},
		{"require без версии", "module m\n\nrequire x\n", "go.mod:3:"},
		{"require в блоке без версии", "module m\nrequire (\n  a v1.0.0\n  b\n)\n", "go.mod:4:"},
		{"replace без =>", "module m\nreplace a b\n", "go.mod:2:"},
		{"replace пустая правая часть", "module m\nreplace a =>\n", "go.mod:2:"},
		{"незакрытый блок", "module m\nrequire (\n a v1.0.0\n", "go.mod:2:"},
		{"module с двумя аргументами", "module a b\n", "go.mod:1:"},
		{"нет module", "go 1.23\n", "go.mod"},
		{"пустой файл", "", "go.mod"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseGoMod(tt.src)
			if !errors.Is(err, ErrSyntax) {
				t.Fatalf("ParseGoMod(%q) err = %v, ожидалась обёртка ErrSyntax", tt.src, err)
			}
			if !strings.HasPrefix(err.Error(), tt.line) {
				t.Errorf("сообщение %q должно начинаться с %q", err.Error(), tt.line)
			}
		})
	}
}

func m(p, v string) Module { return Module{p, v} }

// Классический пример из статьи Russ Cox «Minimal Version Selection».
func rscGraph() (Module, Graph) {
	main := m("A", "")
	g := Graph{
		main:             {m("B", "v1.2.0"), m("C", "v1.2.0")},
		m("B", "v1.2.0"): {m("D", "v1.3.0")},
		m("C", "v1.2.0"): {m("D", "v1.4.0")},
		m("D", "v1.3.0"): {m("E", "v1.2.0")},
		m("D", "v1.4.0"): {m("E", "v1.2.0")},
		m("D", "v1.5.0"): {m("F", "v1.0.0")}, // недостижима — не влияет
		m("E", "v1.2.0"): {},
		m("C", "v1.3.0"): {m("F", "v1.1.0")}, // недостижима
	}
	return main, g
}

func TestBuildList(t *testing.T) {
	main, g := rscGraph()
	got := BuildList(main, g)
	want := []Module{main, m("B", "v1.2.0"), m("C", "v1.2.0"), m("D", "v1.4.0"), m("E", "v1.2.0")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BuildList = %v, ожидалось %v", got, want)
	}
}

func TestBuildListTricky(t *testing.T) {
	main := m("app", "")
	g := Graph{
		main: {m("x", "v1.0.0"), m("y", "v1.0.0")},
		// Старая версия x тянет z v1.9.0, новая (выбранная) — z v1.1.0.
		// MVS всё равно берёт v1.9.0: учитываются все достижимые версии.
		m("x", "v1.0.0"): {m("z", "v1.9.0")},
		m("y", "v1.0.0"): {m("x", "v1.1.0")},
		m("x", "v1.1.0"): {m("z", "v1.1.0")},
		// Цикл и требование на главный модуль.
		m("z", "v1.9.0"):  {m("y", "v1.0.0"), m("app", "v0.5.0")},
		m("z", "v1.1.0"):  {m("w", "v1.10.0")},
		m("w", "v1.10.0"): {m("w", "v1.9.0")}, // v1.10.0 > v1.9.0 (не строковое сравнение)
	}
	got := BuildList(main, g)
	want := []Module{main, m("w", "v1.10.0"), m("x", "v1.1.0"), m("y", "v1.0.0"), m("z", "v1.9.0")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BuildList = %v, ожидалось %v", got, want)
	}

	if got := BuildList(main, nil); !reflect.DeepEqual(got, []Module{main}) {
		t.Errorf("BuildList(пустой граф) = %v, ожидалось [%v]", got, main)
	}
}

func TestWhy(t *testing.T) {
	main, g := rscGraph()
	got, err := Why(main, g, "E")
	want := []Module{main, m("B", "v1.2.0"), m("D", "v1.3.0"), m("E", "v1.2.0")}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Why(E) = %v, %v; ожидалось %v", got, err, want)
	}
	got, err = Why(main, g, "C")
	if err != nil || !reflect.DeepEqual(got, []Module{main, m("C", "v1.2.0")}) {
		t.Errorf("Why(C) = %v, %v", got, err)
	}
	got, err = Why(main, g, "A")
	if err != nil || !reflect.DeepEqual(got, []Module{main}) {
		t.Errorf("Why(главный) = %v, %v; ожидалось [%v]", got, err, main)
	}
	_, err = Why(main, g, "F") // F есть в графе, но недостижима
	if !errors.Is(err, ErrNotRequired) {
		t.Errorf("Why(F) err = %v, ожидалась обёртка ErrNotRequired", err)
	}
}

func TestWhyCycle(t *testing.T) {
	main := m("app", "")
	g := Graph{
		main:             {m("a", "v1.0.0")},
		m("a", "v1.0.0"): {m("b", "v1.0.0")},
		m("b", "v1.0.0"): {m("a", "v1.0.0"), m("c", "v1.0.0")},
	}
	got, err := Why(main, g, "c")
	want := []Module{main, m("a", "v1.0.0"), m("b", "v1.0.0"), m("c", "v1.0.0")}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Why(c) = %v, %v; ожидалось %v", got, err, want)
	}
	if _, err := Why(main, g, "nope"); !errors.Is(err, ErrNotRequired) {
		t.Errorf("Why(nope) err = %v", err)
	}
}
