package atomicfile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func readFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(b)
}

// onlyFiles проверяет, что в каталоге ровно ожидаемые файлы (нет мусорных temp).
func onlyFiles(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("файлы в каталоге: %q, ожидалось %q (временные файлы должны удаляться)", got, want)
	}
}

func TestWriteFileNewAndOverwrite(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "config.json")
	if err := WriteFile(name, []byte(`{"v":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, name); got != `{"v":1}` {
		t.Errorf("содержимое = %q", got)
	}
	if err := WriteFile(name, []byte(`{"v":2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, name); got != `{"v":2}` {
		t.Errorf("после перезаписи = %q", got)
	}
	if err := WriteFile(name, nil, 0o644); err != nil || readFile(t, name) != "" {
		t.Errorf("пустые данные: %v", err)
	}
	onlyFiles(t, dir, "config.json")
}

func TestWritePerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("права POSIX")
	}
	dir := t.TempDir()
	for _, perm := range []os.FileMode{0o640, 0o600, 0o755} {
		name := filepath.Join(dir, fmt.Sprintf("f%o", perm))
		if err := WriteFile(name, []byte("x"), perm); err != nil {
			t.Fatal(err)
		}
		st, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != perm {
			t.Errorf("права = %v, ожидалось %v (CreateTemp создаёт 0600 — нужен Chmod)", st.Mode().Perm(), perm)
		}
	}
}

func TestWriteStreaming(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "big.txt")
	err := Write(name, 0o644, func(w io.Writer) error {
		for i := range 10000 {
			if _, err := fmt.Fprintf(w, "line %d\n", i); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(name)
	if n := bytes.Count(b, []byte("\n")); n != 10000 {
		t.Errorf("строк = %d, ожидалось 10000 (забыт Flush?)", n)
	}
}

func TestWriteFailureKeepsOriginal(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(name, []byte("старое"), 0o644); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("генерация упала")
	err := Write(name, 0o644, func(w io.Writer) error {
		io.WriteString(w, "новое, но не до конца")
		return boom
	})
	if !errors.Is(err, boom) {
		t.Errorf("ожидалась ошибка %v, получено %v", boom, err)
	}
	if got := readFile(t, name); got != "старое" {
		t.Errorf("исходный файл изменился: %q", got)
	}
	onlyFiles(t, dir, "data.txt")
}

func TestWritePanicCleansUp(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "p.txt")
	func() {
		defer func() {
			if recover() == nil {
				t.Error("паника из fn должна пролететь наружу")
			}
		}()
		Write(name, 0o644, func(w io.Writer) error { panic("ой") })
	}()
	onlyFiles(t, dir)
}

func TestWriteErrors(t *testing.T) {
	dir := t.TempDir()
	if err := WriteFile(filepath.Join(dir, "no", "such", "dir", "f"), []byte("x"), 0o644); err == nil {
		t.Error("несуществующий каталог: ожидалась ошибка")
	}
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(filepath.Join(sub, "target"), 0o755)
	os.WriteFile(filepath.Join(sub, "target", "keep"), nil, 0o644)
	// Цель — непустой каталог: rename должен упасть, мусор — убран.
	if err := WriteFile(filepath.Join(sub, "target"), []byte("x"), 0o644); err == nil {
		t.Error("цель — каталог: ожидалась ошибка")
	}
	onlyFiles(t, sub, "target")
}

func TestWriteRelativeName(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := WriteFile("rel.txt", []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, "rel.txt")); got != "ok" {
		t.Errorf("содержимое = %q", got)
	}
}

func TestConcurrentWriters(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "shared.txt")
	payload := func(i int) []byte { return bytes.Repeat([]byte{byte('a' + i)}, 64*1024) }
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := WriteFile(name, payload(i), 0o644); err != nil {
				t.Error(err)
			}
		}()
	}
	// Параллельный читатель никогда не должен видеть «смесь».
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			b, err := os.ReadFile(name)
			if err != nil {
				continue // файла ещё может не быть
			}
			if len(b) != 64*1024 || bytes.Count(b, b[:1]) != len(b) {
				t.Errorf("читатель увидел неполный/смешанный файл: len=%d", len(b))
				return
			}
		}
	}()
	wg.Wait()
	close(stop)
	<-done
	onlyFiles(t, dir, "shared.txt")
}
