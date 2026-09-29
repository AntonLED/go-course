package todoapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

var fixedNow = time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC)

// syncBuffer — буфер для логов, безопасный для параллельной записи.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) records(t *testing.T) []map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []map[string]any
	dec := json.NewDecoder(bytes.NewReader(s.b.Bytes()))
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

type env struct {
	srv  *httptest.Server
	logs *syncBuffer
}

func setup(t *testing.T, store Store) *env {
	t.Helper()
	logs := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	if store == nil {
		store = NewMemStore(func() time.Time { return fixedNow })
	}
	srv := httptest.NewServer(NewServer(store, logger))
	t.Cleanup(srv.Close)
	return &env{srv: srv, logs: logs}
}

type resp struct {
	code int
	hdr  http.Header
	body []byte
}

func (e *env) do(t *testing.T, method, path, ct, body string, hdr ...string) resp {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	r, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return resp{r.StatusCode, r.Header, b}
}

const js = "application/json"

func (e *env) create(t *testing.T, title string) Todo {
	t.Helper()
	r := e.do(t, "POST", "/todos", js, fmt.Sprintf(`{"title":%q}`, title))
	if r.code != 201 {
		t.Fatalf("POST /todos: %d %s", r.code, r.body)
	}
	var td Todo
	if err := json.Unmarshal(r.body, &td); err != nil {
		t.Fatal(err)
	}
	return td
}

type problemBody struct {
	Title  string            `json:"title"`
	Status int               `json:"status"`
	Errors map[string]string `json:"errors"`
}

func expectProblem(t *testing.T, r resp, code int, field string) {
	t.Helper()
	if r.code != code {
		t.Fatalf("код %d, ожидалось %d; тело %s", r.code, code, r.body)
	}
	if ct := r.hdr.Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, ожидалось application/problem+json", ct)
	}
	var p problemBody
	if err := json.Unmarshal(r.body, &p); err != nil {
		t.Fatalf("тело не JSON: %s", r.body)
	}
	if p.Status != code || p.Title == "" {
		t.Errorf("problem = %+v", p)
	}
	if field != "" && p.Errors[field] == "" {
		t.Errorf("ожидалась ошибка по полю %q: %s", field, r.body)
	}
}

func TestHealthz(t *testing.T) {
	e := setup(t, nil)
	r := e.do(t, "GET", "/healthz", "", "")
	if r.code != 200 || !strings.Contains(string(r.body), `"ok"`) {
		t.Errorf("healthz: %d %s", r.code, r.body)
	}
}

func TestCRUD(t *testing.T) {
	e := setup(t, nil)
	r := e.do(t, "POST", "/todos", "application/json; charset=utf-8", `{"title":"  купить молоко  ","done":true}`)
	if r.code != 201 {
		t.Fatalf("create: %d %s", r.code, r.body)
	}
	if loc := r.hdr.Get("Location"); loc != "/todos/1" {
		t.Errorf("Location = %q", loc)
	}
	var td Todo
	_ = json.Unmarshal(r.body, &td)
	if td.ID != 1 || td.Title != "купить молоко" || !td.Done || !td.CreatedAt.Equal(fixedNow) {
		t.Errorf("создано %+v", td)
	}
	if !strings.Contains(string(r.body), `"created_at"`) {
		t.Errorf("нет created_at в JSON: %s", r.body)
	}

	r = e.do(t, "GET", "/todos/1", "", "")
	if r.code != 200 || !strings.Contains(string(r.body), "купить молоко") {
		t.Errorf("get: %d %s", r.code, r.body)
	}

	r = e.do(t, "PATCH", "/todos/1", js, `{"done":false}`)
	_ = json.Unmarshal(r.body, &td)
	if r.code != 200 || td.Done || td.Title != "купить молоко" {
		t.Errorf("patch done: %d %s", r.code, r.body)
	}
	r = e.do(t, "PATCH", "/todos/1", js, `{"title":"кефир"}`)
	_ = json.Unmarshal(r.body, &td)
	if r.code != 200 || td.Title != "кефир" || td.Done {
		t.Errorf("patch title: %d %s", r.code, r.body)
	}

	if r = e.do(t, "DELETE", "/todos/1", "", ""); r.code != 204 {
		t.Errorf("delete: %d", r.code)
	}
	expectProblem(t, e.do(t, "GET", "/todos/1", "", ""), 404, "")
	expectProblem(t, e.do(t, "DELETE", "/todos/1", "", ""), 404, "")
	expectProblem(t, e.do(t, "PATCH", "/todos/1", js, `{"done":true}`), 404, "")
}

func TestValidation(t *testing.T) {
	e := setup(t, nil)
	e.create(t, "x")
	tests := []struct {
		name, method, path, ct, body string
		code                         int
		field                        string
	}{
		{"noCT", "POST", "/todos", "text/plain", `{"title":"a"}`, 415, ""},
		{"badJSON", "POST", "/todos", js, `{"title":`, 400, ""},
		{"unknown", "POST", "/todos", js, `{"title":"a","owner":"me"}`, 400, ""},
		{"twoObjects", "POST", "/todos", js, `{"title":"a"}{"title":"b"}`, 400, ""},
		{"emptyTitle", "POST", "/todos", js, `{"title":"   "}`, 422, "title"},
		{"longTitle", "POST", "/todos", js, `{"title":"` + strings.Repeat("ж", 201) + `"}`, 422, "title"},
		{"huge", "POST", "/todos", js, `{"title":"` + strings.Repeat("a", 2<<20) + `"}`, 413, ""},
		{"badID", "GET", "/todos/abc", "", "", 400, ""},
		{"zeroID", "GET", "/todos/0", "", "", 400, ""},
		{"negID", "DELETE", "/todos/-5", "", "", 400, ""},
		{"patchEmpty", "PATCH", "/todos/1", js, `{}`, 422, ""},
		{"patchBadTitle", "PATCH", "/todos/1", js, `{"title":""}`, 422, "title"},
		{"patchWrongType", "PATCH", "/todos/1", js, `{"done":"yes"}`, 400, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectProblem(t, e.do(t, tt.method, tt.path, tt.ct, tt.body), tt.code, tt.field)
		})
	}
	// 200 рун — можно.
	e.create(t, strings.Repeat("ж", 200))
}

func TestPagination(t *testing.T) {
	e := setup(t, nil)
	for i := 1; i <= 25; i++ {
		e.create(t, fmt.Sprintf("t%d", i))
	}
	type page struct {
		Items  []Todo `json:"items"`
		Total  int    `json:"total"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
	}
	get := func(q string) (page, resp) {
		r := e.do(t, "GET", "/todos"+q, "", "")
		var p page
		if r.code == 200 {
			if err := json.Unmarshal(r.body, &p); err != nil {
				t.Fatal(err)
			}
		}
		return p, r
	}
	p, _ := get("")
	if len(p.Items) != 20 || p.Total != 25 || p.Limit != 20 || p.Offset != 0 || p.Items[0].Title != "t1" {
		t.Errorf("по умолчанию: %d элементов, total=%d limit=%d offset=%d", len(p.Items), p.Total, p.Limit, p.Offset)
	}
	p, _ = get("?limit=10&offset=20")
	if len(p.Items) != 5 || p.Items[0].Title != "t21" || p.Limit != 10 || p.Offset != 20 {
		t.Errorf("limit=10 offset=20: %+v", p)
	}
	_, r := get("?offset=100")
	if r.code != 200 || !strings.Contains(string(r.body), `"items":[]`) {
		t.Errorf("за концом списка ожидался items: [] (не null): %s", r.body)
	}
	p, _ = get("?limit=100")
	if len(p.Items) != 25 {
		t.Errorf("limit=100: %d", len(p.Items))
	}
	for _, q := range []string{"?limit=0", "?limit=101", "?limit=abc", "?offset=-1", "?limit=5&offset=x"} {
		_, r := get(q)
		expectProblem(t, r, 400, "")
	}
	_, r = get("?limit=0")
	if !strings.Contains(string(r.body), `"limit"`) {
		t.Errorf("ожидалось errors.limit: %s", r.body)
	}
}

func TestEmptyList(t *testing.T) {
	e := setup(t, nil)
	r := e.do(t, "GET", "/todos", "", "")
	if r.code != 200 || !strings.Contains(string(r.body), `"items":[]`) || !strings.Contains(string(r.body), `"total":0`) {
		t.Errorf("пустой список: %d %s", r.code, r.body)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	e := setup(t, nil)
	if r := e.do(t, "PUT", "/todos/1", js, `{}`); r.code != 405 {
		t.Errorf("PUT /todos/1: %d, ожидалось 405", r.code)
	}
}

// brokenStore — фейк хранилища: ошибки и паники по заказу.
type brokenStore struct {
	*MemStore
	listErr error
	panicOn string
}

func (b *brokenStore) List(ctx context.Context, off, lim int) ([]Todo, int, error) {
	if b.listErr != nil {
		return nil, 0, b.listErr
	}
	return b.MemStore.List(ctx, off, lim)
}

func (b *brokenStore) Get(ctx context.Context, id int64) (Todo, error) {
	if b.panicOn == "get" {
		panic("nil map write, конечно же")
	}
	return b.MemStore.Get(ctx, id)
}

func TestStoreErrorsAndPanics(t *testing.T) {
	bs := &brokenStore{MemStore: NewMemStore(nil), listErr: errors.New("pg: password authentication failed for user admin")}
	e := setup(t, bs)
	r := e.do(t, "GET", "/todos", "", "")
	expectProblem(t, r, 500, "")
	if strings.Contains(string(r.body), "password") {
		t.Error("500 не должен раскрывать детали внутренней ошибки")
	}

	bs.panicOn = "get"
	r = e.do(t, "GET", "/todos/1", "", "", "X-Request-ID", "trace-me")
	expectProblem(t, r, 500, "")
	if r.hdr.Get("X-Request-ID") != "trace-me" {
		t.Errorf("X-Request-ID в ответе на панику = %q", r.hdr.Get("X-Request-ID"))
	}
	var panicLogged bool
	for _, rec := range e.logs.records(t) {
		if rec["msg"] == "panic" && rec["level"] == "ERROR" {
			panicLogged = true
		}
	}
	if !panicLogged {
		t.Error("паника должна логироваться (ERROR \"panic\")")
	}
	// Сервер жив.
	if r := e.do(t, "GET", "/healthz", "", ""); r.code != 200 {
		t.Errorf("после паники healthz: %d", r.code)
	}
}

func TestRequestIDAndLogging(t *testing.T) {
	e := setup(t, nil)
	r := e.do(t, "GET", "/todos/42", "", "", "X-Request-ID", "abc-1")
	if r.hdr.Get("X-Request-ID") != "abc-1" {
		t.Errorf("X-Request-ID не прокинут: %q", r.hdr.Get("X-Request-ID"))
	}
	r2 := e.do(t, "GET", "/healthz", "", "", "X-Request-ID", "bad id with spaces")
	gen := r2.hdr.Get("X-Request-ID")
	if gen == "" || gen == "bad id with spaces" {
		t.Errorf("невалидный X-Request-ID надо заменить сгенерированным, получено %q", gen)
	}
	var found bool
	for _, rec := range e.logs.records(t) {
		if rec["msg"] == "request" && rec["request_id"] == "abc-1" {
			found = true
			if rec["method"] != "GET" || rec["path"] != "/todos/42" || rec["status"] != float64(404) {
				t.Errorf("запись лога: %v", rec)
			}
		}
	}
	if !found {
		t.Errorf("нет записи \"request\" с request_id=abc-1: %v", e.logs.records(t))
	}
}

func TestConcurrentCreates(t *testing.T) {
	e := setup(t, nil)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("POST", e.srv.URL+"/todos", strings.NewReader(`{"title":"x"}`))
			req.Header.Set("Content-Type", js)
			r, err := e.srv.Client().Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			r.Body.Close()
		}()
	}
	wg.Wait()
	r := e.do(t, "GET", "/todos?limit=100", "", "")
	if !strings.Contains(string(r.body), `"total":20`) {
		t.Errorf("после 20 параллельных создаёт: %s", r.body)
	}
}
