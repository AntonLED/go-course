package redact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"testing/slogtest"
	"time"
)

// TestSlogtest — официальный набор проверок корректности slog.Handler.
func TestSlogtest(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, &Options{Level: slog.LevelDebug})
	results := func() []map[string]any {
		var ms []map[string]any
		for _, line := range bytes.Split(buf.Bytes(), []byte{'\n'}) {
			if len(line) == 0 {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(line, &m); err != nil {
				t.Fatalf("строка не является JSON: %q: %v", line, err)
			}
			ms = append(ms, m)
		}
		return ms
	}
	if err := slogtest.TestHandler(h, results); err != nil {
		t.Error(err)
	}
}

// logLine логирует одну запись с нулевым временем (ключ time не выводится).
func logLine(t *testing.T, h slog.Handler, level slog.Level, msg string, args ...any) {
	t.Helper()
	r := slog.NewRecord(time.Time{}, level, msg, 0)
	r.Add(args...)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}
}

type userID int

func (u userID) LogValue() slog.Value {
	return slog.GroupValue(slog.Int("id", int(u)), slog.String("token", "leak-me"))
}

func TestExactOutput(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, nil)
	var hh slog.Handler = h.WithAttrs([]slog.Attr{slog.String("svc", "api")}).WithGroup("req").WithAttrs([]slog.Attr{slog.String("id", "r1")})
	logLine(t, hh, slog.LevelWarn, "готово",
		"status", 200, "dur", 1500*time.Millisecond, "ok", true, "ratio", 0.25,
		"err", errors.New("oops"), "password", "hunter2", slog.Group("empty"),
		slog.Group("", slog.String("inlined", "yes")),
		"user", userID(7), "tags", []string{"a", "b"})
	want := `{"level":"WARN","msg":"готово","svc":"api","req":{"id":"r1","status":200,"dur":"1.5s","ok":true,"ratio":0.25,` +
		`"err":"oops","password":"[REDACTED]","inlined":"yes","user":{"id":7,"token":"[REDACTED]"},"tags":["a","b"]}}` + "\n"
	if got := buf.String(); got != want {
		t.Errorf("вывод:\n получено  %s ожидалось %s", got, want)
	}
}

func TestRedaction(t *testing.T) {
	tests := []struct {
		name   string
		opts   *Options
		args   []any
		secret string
	}{
		{"регистр не важен", nil, []any{"Password", "p1"}, "p1"},
		{"в группе", nil, []any{slog.Group("auth", "token", "t0k3n")}, "t0k3n"},
		{"ключ-группа целиком", nil, []any{slog.Group("secret", "a", "s1", "b", "s2")}, "s1"},
		{"Authorization", nil, []any{"authorization", "Bearer xyz"}, "Bearer xyz"},
		{"свои ключи", &Options{RedactKeys: []string{"card"}}, []any{"CARD", "4111"}, "4111"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			slog.New(NewHandler(&buf, tc.opts)).Info("m", tc.args...)
			if strings.Contains(buf.String(), tc.secret) {
				t.Errorf("секрет %q утёк: %s", tc.secret, buf.String())
			}
			if !strings.Contains(buf.String(), Redacted) {
				t.Errorf("ожидалась маска %s: %s", Redacted, buf.String())
			}
		})
	}
	// Свои ключи заменяют дефолтные.
	var buf bytes.Buffer
	slog.New(NewHandler(&buf, &Options{RedactKeys: []string{"card"}})).Info("m", "password", "visible")
	if !strings.Contains(buf.String(), "visible") {
		t.Errorf("RedactKeys заменяет список по умолчанию: %s", buf.String())
	}
	// Маскирование атрибутов из WithAttrs.
	buf.Reset()
	slog.New(NewHandler(&buf, nil)).With("token", "abc").WithGroup("g").Info("m", "x", 1)
	if strings.Contains(buf.String(), "abc") {
		t.Errorf("секрет из With утёк: %s", buf.String())
	}
}

func TestLevel(t *testing.T) {
	var buf bytes.Buffer
	lv := new(slog.LevelVar)
	lv.Set(slog.LevelWarn)
	l := slog.New(NewHandler(&buf, &Options{Level: lv}))
	l.Info("скрыто")
	l.Error("видно")
	if strings.Contains(buf.String(), "скрыто") || !strings.Contains(buf.String(), "видно") {
		t.Errorf("фильтрация по уровню: %s", buf.String())
	}
	lv.Set(slog.LevelDebug) // LevelVar меняется на лету
	l.Debug("debug теперь видно")
	if !strings.Contains(buf.String(), "debug теперь видно") {
		t.Error("Level должен читаться при каждом вызове (LevelVar)")
	}
	if NewHandler(&buf, nil).Enabled(context.Background(), slog.LevelDebug) {
		t.Error("по умолчанию уровень Info")
	}
}

func TestSiblingsIndependent(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(NewHandler(&buf, nil)).With("a", 1).With("b", 2)
	l1 := base.With("c", 3)
	l2 := base.With("d", 4)
	l1.Info("one")
	l2.Info("two")
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 || strings.Contains(lines[0], `"d"`) || strings.Contains(lines[1], `"c"`) {
		t.Errorf("производные логгеры не должны влиять друг на друга:\n%s", buf.String())
	}
}

func TestConcurrent(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(NewHandler(&buf, nil))
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub := l.With("g", g)
			for i := 0; i < 100; i++ {
				sub.Info("msg", "i", i)
			}
		}()
	}
	wg.Wait()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 800 {
		t.Fatalf("строк %d, ожидалось 800", len(lines))
	}
	for _, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Fatalf("строки перемешались: %q", line)
		}
	}
}
