package findfiles

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func data(n int) []byte { return []byte(strings.Repeat("x", n)) }

var testFS = fstest.MapFS{
	"README.md":              {Data: data(10)},
	"main.go":                {Data: data(100)},
	".env":                   {Data: data(5)},
	".git/config":            {Data: data(50)},
	".git/objects/ab/cdef":   {Data: data(500)},
	"cmd/app/main.go":        {Data: data(300)},
	"cmd/app/main_test.go":   {Data: data(20)},
	"internal/db/db.go":      {Data: data(1000)},
	"internal/db/.keep":      {Data: nil},
	"internal/db/schema.sql": {Data: data(40)},
	"docs/empty":             {Mode: fs.ModeDir},
	"link.go":                {Data: []byte("main.go"), Mode: fs.ModeSymlink},
}

func paths(fs []File) []string {
	out := []string{}
	for _, f := range fs {
		out = append(out, f.Path)
	}
	return out
}

func TestFind(t *testing.T) {
	tests := []struct {
		name string
		root string
		opt  Options
		want []string
	}{
		{"все", ".", Options{}, []string{".env", ".git/config", ".git/objects/ab/cdef", "README.md",
			"cmd/app/main.go", "cmd/app/main_test.go", "internal/db/.keep", "internal/db/db.go", "internal/db/schema.sql", "main.go"}},
		{"*.go", ".", Options{Pattern: "*.go"}, []string{"cmd/app/main.go", "cmd/app/main_test.go", "internal/db/db.go", "main.go"}},
		{"без скрытых", ".", Options{SkipHidden: true}, []string{"README.md", "cmd/app/main.go", "cmd/app/main_test.go",
			"internal/db/db.go", "internal/db/schema.sql", "main.go"}},
		{"MinSize", ".", Options{MinSize: 300, SkipHidden: true}, []string{"cmd/app/main.go", "internal/db/db.go"}},
		{"MinSize включительно", ".", Options{MinSize: 100, Pattern: "main*.go"}, []string{"cmd/app/main.go", "main.go"}},
		{"MaxDepth 1", ".", Options{MaxDepth: 1, SkipHidden: true}, []string{"README.md", "main.go"}},
		{"MaxDepth 3", ".", Options{MaxDepth: 3, Pattern: "*.go"}, []string{"cmd/app/main.go", "cmd/app/main_test.go", "internal/db/db.go", "main.go"}},
		{"MaxDepth 2", ".", Options{MaxDepth: 2, Pattern: "*.go"}, []string{"main.go"}},
		{"подкаталог", "internal", Options{MaxDepth: 2}, []string{"internal/db/.keep", "internal/db/db.go", "internal/db/schema.sql"}},
		{"подкаталог MaxDepth 1", "internal/db", Options{MaxDepth: 1, SkipHidden: true}, []string{"internal/db/db.go", "internal/db/schema.sql"}},
		{"скрытый root не пропускается", ".git", Options{SkipHidden: true}, []string{".git/config", ".git/objects/ab/cdef"}},
		{"шаблон с классом", ".", Options{Pattern: "[Rr]*"}, []string{"README.md"}},
		{"ничего", ".", Options{Pattern: "*.rs"}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Find(testFS, tt.root, tt.opt)
			if err != nil {
				t.Fatalf("Find(%q, %+v): %v", tt.root, tt.opt, err)
			}
			if !reflect.DeepEqual(paths(got), tt.want) {
				t.Errorf("Find(%q, %+v) =\n %q\nожидалось\n %q", tt.root, tt.opt, paths(got), tt.want)
			}
		})
	}
	got, _ := Find(testFS, "cmd", Options{})
	if len(got) != 2 || got[0].Size != 300 || got[1].Size != 20 {
		t.Errorf("размеры: %+v", got)
	}
}

func TestFindErrors(t *testing.T) {
	if _, err := Find(testFS, ".", Options{Pattern: "[a-"}); !errors.Is(err, path.ErrBadPattern) {
		t.Errorf("плохой шаблон: ожидалось path.ErrBadPattern, получено %v", err)
	}
	if _, err := Find(fstest.MapFS{}, ".", Options{Pattern: "[a-"}); !errors.Is(err, path.ErrBadPattern) {
		t.Errorf("плохой шаблон на пустой FS: ожидалось path.ErrBadPattern, получено %v", err)
	}
	if _, err := Find(testFS, "nope", Options{}); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("несуществующий root: ожидалось fs.ErrNotExist, получено %v", err)
	}
	for _, bad := range []string{"/abs", "a/../b", "cmd/", ""} {
		if _, err := Find(testFS, bad, Options{}); err == nil {
			t.Errorf("root %q: ожидалась ошибка (fs.ValidPath)", bad)
		}
	}
}

func TestFindDirFS(t *testing.T) {
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "a", "b", "x.txt"), data(7), 0o644))
	must(os.WriteFile(filepath.Join(dir, "y.txt"), data(3), 0o644))
	got, err := Find(os.DirFS(dir), ".", Options{Pattern: "*.txt"})
	must(err)
	want := []File{{"a/b/x.txt", 7}, {"y.txt", 3}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Find(os.DirFS) = %+v, ожидалось %+v (пути в fs.FS — всегда со слэшем)", got, want)
	}
}

func TestTree(t *testing.T) {
	fsys := fstest.MapFS{
		"a.txt":         {},
		"dir/b.txt":     {},
		"dir/sub/c.txt": {},
		"dir/sub/d.txt": {},
		"dir/z":         {},
		"empty":         {Mode: fs.ModeDir},
		"z.go":          {},
	}
	want := `.
├── a.txt
├── dir
│   ├── b.txt
│   ├── sub
│   │   ├── c.txt
│   │   └── d.txt
│   └── z
├── empty
└── z.go
`
	got, err := Tree(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("Tree =\n%s\nожидалось\n%s", got, want)
	}

	got, err = Tree(fsys, "dir/sub")
	if err != nil || got != "dir/sub\n├── c.txt\n└── d.txt\n" {
		t.Errorf("Tree(dir/sub) = %q, %v", got, err)
	}
	got, err = Tree(fsys, "empty")
	if err != nil || got != "empty\n" {
		t.Errorf("Tree(empty) = %q, %v", got, err)
	}
	if _, err := Tree(fsys, "missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Tree(missing): ожидалось fs.ErrNotExist, получено %v", err)
	}
}

func TestMapFSValid(t *testing.T) {
	// fstest.TestFS проверяет, что наша тестовая FS сама по себе корректна.
	if err := fstest.TestFS(testFS, "main.go", "cmd/app/main.go"); err != nil {
		t.Fatal(err)
	}
}
