package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func tag(name string, log *[]string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*log = append(*log, name+">")
			next.ServeHTTP(w, r)
			*log = append(*log, "<"+name)
		})
	}
}

func TestChainOrder(t *testing.T) {
	var log []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { log = append(log, "h") })
	Chain(h, tag("A", &log), tag("B", &log), tag("C", &log)).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	want := "A> B> C> h <C <B <A"
	if got := strings.Join(log, " "); got != want {
		t.Errorf("порядок вызовов: %q, ожидалось %q", got, want)
	}
	log = nil
	Chain(h).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if strings.Join(log, " ") != "h" {
		t.Errorf("Chain без middleware: %v", log)
	}
}

func TestStatusRecorder(t *testing.T) {
	tests := []struct {
		name   string
		h      http.HandlerFunc
		status int
		bytes  int
	}{
		{"implicit200", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("hello")) }, 200, 5},
		{"explicit", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(201)
			_, _ = w.Write([]byte("ab"))
			_, _ = w.Write([]byte("cd"))
		}, 201, 4},
		{"superfluous", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
			w.WriteHeader(500)
		}, 404, 0},
		{"nothing", func(w http.ResponseWriter, r *http.Request) {}, 200, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			sr := NewStatusRecorder(rec)
			tt.h(sr, httptest.NewRequest("GET", "/", nil))
			if sr.Status != tt.status || sr.Bytes != tt.bytes {
				t.Errorf("Status=%d Bytes=%d, ожидалось %d и %d", sr.Status, sr.Bytes, tt.status, tt.bytes)
			}
			if rec.Code != tt.status {
				t.Errorf("до клиента дошёл код %d, ожидалось %d", rec.Code, tt.status)
			}
		})
	}
}

func TestStatusRecorderUnwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := NewStatusRecorder(rec)
	if err := http.NewResponseController(sr).Flush(); err != nil {
		t.Fatalf("ResponseController.Flush через StatusRecorder: %v (нет Unwrap?)", err)
	}
	if !rec.Flushed {
		t.Error("Flush не дошёл до исходного ResponseWriter")
	}
}

func newLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, nil)), &buf
}

func records(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	dec := json.NewDecoder(buf)
	for {
		var m map[string]any
		if err := dec.Decode(&m); err == io.EOF {
			return out
		} else if err != nil {
			t.Fatalf("лог не JSON: %v", err)
		}
		out = append(out, m)
	}
}

func TestRecover(t *testing.T) {
	logger, buf := newLogger()
	h := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	func() {
		defer func() {
			if v := recover(); v != nil {
				t.Fatalf("паника вышла наружу: %v", v)
			}
		}()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))
	}()
	if rec.Code != 500 {
		t.Errorf("код %d, ожидалось 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"error"`) {
		t.Errorf("тело %q, ожидался JSON с полем error", rec.Body.String())
	}
	recs := records(t, buf)
	if len(recs) != 1 || recs[0]["msg"] != "panic" || recs[0]["level"] != "ERROR" {
		t.Errorf("ожидалась одна запись ERROR \"panic\", получено %v", recs)
	}

	// Без паники — ничего не меняется.
	ok := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	rec = httptest.NewRecorder()
	ok.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 204 {
		t.Errorf("без паники код %d, ожидалось 204", rec.Code)
	}
}

func TestRecoverAbortHandler(t *testing.T) {
	logger, _ := newLogger()
	h := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic(http.ErrAbortHandler) }))
	var got any
	func() {
		defer func() { got = recover() }()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	}()
	if got != http.ErrAbortHandler {
		t.Errorf("паника http.ErrAbortHandler должна пробрасываться, recover() = %v", got)
	}
}

var hexID = regexp.MustCompile(`^[0-9a-fA-F]{16}$`)

func TestRequestID(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = RequestIDFrom(r.Context()) }))
	tests := []struct {
		name, in string
		keep     bool
	}{
		{"valid", "abc-123_XYZ", true},
		{"empty", "", false},
		{"tooLong", strings.Repeat("a", 65), false},
		{"max", strings.Repeat("a", 64), true},
		{"injection", "x\nlevel=ERROR", false},
		{"space", "a b", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.in != "" {
				req.Header.Set(HeaderRequestID, tt.in)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			hdr := rec.Header().Get(HeaderRequestID)
			if hdr != seen {
				t.Errorf("в заголовке %q, в контексте %q — должны совпадать", hdr, seen)
			}
			if tt.keep && seen != tt.in {
				t.Errorf("валидный ID %q заменён на %q", tt.in, seen)
			}
			if !tt.keep && !hexID.MatchString(seen) {
				t.Errorf("сгенерированный ID %q, ожидалось 16 hex-символов", seen)
			}
		})
	}
	a, b := httptest.NewRecorder(), httptest.NewRecorder()
	h.ServeHTTP(a, httptest.NewRequest("GET", "/", nil))
	h.ServeHTTP(b, httptest.NewRequest("GET", "/", nil))
	if a.Header().Get(HeaderRequestID) == b.Header().Get(HeaderRequestID) {
		t.Error("два запроса получили одинаковый сгенерированный ID")
	}
	if RequestIDFrom(httptest.NewRequest("GET", "/", nil).Context()) != "" {
		t.Error("RequestIDFrom без ID должен вернуть \"\"")
	}
}

func TestLoggingWithChain(t *testing.T) {
	logger, buf := newLogger()
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("short and stout"))
	}), RequestID, Logging(logger), Recover(logger))
	req := httptest.NewRequest("POST", "/tea?x=1", nil)
	req.Header.Set(HeaderRequestID, "req-42")
	h.ServeHTTP(httptest.NewRecorder(), req)

	recs := records(t, buf)
	if len(recs) != 1 {
		t.Fatalf("ожидалась 1 запись лога, получено %d: %v", len(recs), recs)
	}
	r := recs[0]
	want := map[string]any{"msg": "request", "method": "POST", "path": "/tea", "status": float64(418), "bytes": float64(15), "request_id": "req-42"}
	for k, v := range want {
		if r[k] != v {
			t.Errorf("поле %q = %v, ожидалось %v", k, r[k], v)
		}
	}
	if _, ok := r["duration"]; !ok {
		t.Error("нет поля duration")
	}
}

func TestLoggingSeesPanicStatus(t *testing.T) {
	// Logging снаружи Recover должен увидеть 500.
	logger, buf := newLogger()
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("x") }), Logging(logger), Recover(logger))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/p", nil))
	var found bool
	for _, r := range records(t, buf) {
		if r["msg"] == "request" {
			found = true
			if r["status"] != float64(500) {
				t.Errorf("Logging записал status=%v, ожидалось 500", r["status"])
			}
		}
	}
	if !found {
		t.Error("нет записи request")
	}
}
