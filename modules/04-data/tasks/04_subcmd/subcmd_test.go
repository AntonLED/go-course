package subcmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
)

type cliResult struct {
	code           int
	stdout, stderr string
}

func runTextkit(stdin io.Reader, args ...string) cliResult {
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	var out, errb bytes.Buffer
	code := Run(args, stdin, &out, &errb)
	return cliResult{code, out.String(), errb.String()}
}

func TestRunOK(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"count всё", "hello world\nпривет\n", []string{"count"}, "lines=2 words=3 bytes=25\n"},
		{"count -words", "a  b\tc", []string{"count", "-words"}, "words=3\n"},
		{"count -bytes -lines", "a\nb", []string{"count", "-bytes", "-lines"}, "lines=1 bytes=3\n"},
		{"count пусто", "", []string{"count"}, "lines=0 words=0 bytes=0\n"},
		{"repeat по умолчанию", "", []string{"repeat", "hi"}, "hi hi\n"},
		{"repeat -n -sep", "", []string{"repeat", "-n", "3", "-sep", ", ", "go", "go"}, "go go, go go, go go\n"},
		{"repeat -n 0", "", []string{"repeat", "-n=0", "x"}, "\n"},
		{"upper", "Привет, world!\nещё\n", []string{"upper"}, "ПРИВЕТ, WORLD!\nЕЩЁ\n"},
		{"upper без \\n в конце", "abc", []string{"upper"}, "ABC"},
		{"version", "", []string{"version"}, "textkit 1.2.0\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runTextkit(strings.NewReader(tt.stdin), tt.args...)
			if r.code != ExitOK {
				t.Fatalf("Run(%q) = %d, ожидалось 0; stderr: %s", tt.args, r.code, r.stderr)
			}
			if r.stdout != tt.want {
				t.Errorf("Run(%q) stdout = %q, ожидалось %q", tt.args, r.stdout, tt.want)
			}
			if r.stderr != "" {
				t.Errorf("Run(%q) stderr должен быть пуст, получено %q", tt.args, r.stderr)
			}
		})
	}
}

func TestRunUpperLarge(t *testing.T) {
	in := strings.Repeat("ё", 100_000) // одна длинная строка, многобайтовые руны
	r := runTextkit(iotest.HalfReader(strings.NewReader(in)), "upper")
	if r.code != 0 || r.stdout != strings.Repeat("Ё", 100_000) {
		t.Errorf("upper большой строки: код %d, длина вывода %d", r.code, len(r.stdout))
	}
}

func TestRunJSON(t *testing.T) {
	r := runTextkit(strings.NewReader("a b\n"), "-json", "count", "-lines", "-words")
	var m map[string]int
	if err := json.Unmarshal([]byte(r.stdout), &m); err != nil {
		t.Fatalf("stdout не JSON: %q (%v)", r.stdout, err)
	}
	if !reflect.DeepEqual(m, map[string]int{"lines": 1, "words": 2}) {
		t.Errorf("count JSON = %v", m)
	}

	r = runTextkit(nil, "-json", "repeat", "-n", "2", "x")
	if strings.TrimSpace(r.stdout) != `["x","x"]` {
		t.Errorf("repeat JSON = %q", r.stdout)
	}
	r = runTextkit(nil, "-json", "repeat", "-n", "0", "x")
	if strings.TrimSpace(r.stdout) != `[]` {
		t.Errorf("repeat -n 0 JSON = %q, ожидалось [] (а не null)", r.stdout)
	}
	r = runTextkit(nil, "-json", "version")
	if strings.TrimSpace(r.stdout) != `{"version":"1.2.0"}` {
		t.Errorf("version JSON = %q", r.stdout)
	}
}

func TestRunUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"без аргументов", nil},
		{"только глобальный флаг", []string{"-json"}},
		{"неизвестная команда", []string{"frobnicate"}},
		{"неизвестный глобальный флаг", []string{"-x", "count"}},
		{"глобальный флаг после команды", []string{"count", "-json"}},
		{"repeat без текста", []string{"repeat"}},
		{"repeat отрицательный n", []string{"repeat", "-n", "-1", "x"}},
		{"repeat n не число", []string{"repeat", "-n", "много", "x"}},
		{"count с аргументом", []string{"count", "file.txt"}},
		{"upper с аргументом", []string{"upper", "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runTextkit(nil, tt.args...)
			if r.code != ExitUsage {
				t.Errorf("Run(%q) = %d, ожидалось %d (ExitUsage)", tt.args, r.code, ExitUsage)
			}
			if r.stdout != "" {
				t.Errorf("Run(%q): при ошибке stdout должен быть пуст, получено %q", tt.args, r.stdout)
			}
			if r.stderr == "" {
				t.Errorf("Run(%q): сообщение об ошибке должно быть в stderr", tt.args)
			}
		})
	}
	if r := runTextkit(nil); !strings.Contains(r.stderr, "usage: textkit") {
		t.Errorf("без аргументов в stderr должна быть справка, получено %q", r.stderr)
	}
}

func TestRunHelp(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"-help"}, {"count", "-h"}, {"repeat", "--help"}} {
		r := runTextkit(nil, args...)
		if r.code != ExitOK || r.stdout != "" || !strings.Contains(r.stderr, "usage") {
			t.Errorf("Run(%q) = %d, stdout=%q, stderr=%q; ожидалось 0 и справка в stderr", args, r.code, r.stdout, r.stderr)
		}
	}
}

func TestRunReadError(t *testing.T) {
	boom := errors.New("stdin сломан")
	for _, cmd := range []string{"count", "upper"} {
		r := runTextkit(io.MultiReader(strings.NewReader("abc\n"), iotest.ErrReader(boom)), cmd)
		if r.code != ExitFailure {
			t.Errorf("%s с ошибкой чтения: код %d, ожидалось %d", cmd, r.code, ExitFailure)
		}
		if !strings.Contains(r.stderr, boom.Error()) {
			t.Errorf("%s: stderr = %q, должен содержать причину", cmd, r.stderr)
		}
	}
}
