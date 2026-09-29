package miniweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func do(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errBody(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("тело ошибки не JSON: %q", rec.Body.String())
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Errorf("Content-Type ошибки = %q", rec.Header().Get("Content-Type"))
	}
	return m["error"]
}

func TestParamsAndQuery(t *testing.T) {
	r := New()
	r.GET("/users/{id}", func(c *Context) error {
		return c.JSON(200, map[string]string{"id": c.Param("id"), "sort": c.Query("sort")})
	})
	r.GET("/files/{path...}", func(c *Context) error { return c.String(200, c.Param("path")) })
	r.POST("/users", func(c *Context) error { return c.NoContent(201) })

	rec := do(r, "GET", "/users/42?sort=name", "")
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"id":"42","sort":"name"}` {
		t.Errorf("GET /users/42: %d %s", rec.Code, rec.Body)
	}
	rec = do(r, "GET", "/files/a/b/c.txt", "")
	if rec.Body.String() != "a/b/c.txt" || rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("wildcard: %q %q", rec.Body, rec.Header().Get("Content-Type"))
	}
	if rec := do(r, "POST", "/users", ""); rec.Code != 201 {
		t.Errorf("POST /users: %d", rec.Code)
	}
}

func TestNotFoundAndMethodNotAllowed(t *testing.T) {
	r := New()
	r.Use(func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			c.W.Header().Set("X-Global", "yes")
			return next(c)
		}
	})
	r.GET("/items", func(c *Context) error { return c.String(200, "items") })

	rec := do(r, "GET", "/nope", "")
	if rec.Code != 404 || errBody(t, rec) != "not found" {
		t.Errorf("404: %d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("X-Global") != "yes" {
		t.Error("глобальный middleware должен срабатывать и для 404")
	}
	rec = do(r, "DELETE", "/items", "")
	if rec.Code != 405 || errBody(t, rec) != "method not allowed" {
		t.Errorf("405: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Header().Get("Allow"), "GET") {
		t.Errorf("405 без Allow: %q", rec.Header().Get("Allow"))
	}
	if rec.Header().Get("X-Global") != "yes" {
		t.Error("глобальный middleware должен срабатывать и для 405")
	}
}

func tracer(name string) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			v, _ := c.Get("trace")
			s, _ := v.(string)
			c.Set("trace", s+name+">")
			return next(c)
		}
	}
}

func TestGroupsAndOrder(t *testing.T) {
	r := New()
	show := func(c *Context) error {
		v, _ := c.Get("trace")
		return c.String(200, fmt.Sprint(v))
	}
	r.GET("/early", show) // зарегистрирован ДО r.Use — глобальные всё равно действуют
	r.Use(tracer("global"))
	api := r.Group("/api", tracer("api"))
	v1 := api.Group("/v1/", tracer("v1"))
	v1.GET("/users/{id}", show, tracer("route"))
	api.GET("/ping", show)
	admin := api.Group("/admin")
	admin.GET("/before", show)
	admin.Use(tracer("admin"))
	admin.GET("/after", show)
	api.Group("").GET("", show)

	tests := []struct{ path, want string }{
		{"/early", "global>"},
		{"/api/v1/users/7", "global>api>v1>route>"},
		{"/api/ping", "global>api>"},
		{"/api/admin/before", "global>api>"},
		{"/api/admin/after", "global>api>admin>"},
		{"/api", "global>api>"},
	}
	for _, tt := range tests {
		rec := do(r, "GET", tt.path, "")
		if rec.Code != 200 || rec.Body.String() != tt.want {
			t.Errorf("GET %s: %d %q, ожидалось %q", tt.path, rec.Code, rec.Body.String(), tt.want)
		}
	}
	// Вложенная группа не должна «заражать» соседей через общий backing array.
	g := r.Group("/x", tracer("x"))
	a := g.Group("/a", tracer("a"))
	b := g.Group("/b", tracer("b"))
	a.GET("/", show)
	b.GET("/", show)
	if got := do(r, "GET", "/x/a/", "").Body.String(); got != "global>x>a>" {
		t.Errorf("/x/a/: %q", got)
	}
	if got := do(r, "GET", "/x/b/", "").Body.String(); got != "global>x>b>" {
		t.Errorf("/x/b/: %q", got)
	}
}

func TestErrorHandling(t *testing.T) {
	r := New()
	r.GET("/http", func(c *Context) error { return NewHTTPError(422, "bad input") })
	r.GET("/wrapped", func(c *Context) error { return fmt.Errorf("check: %w", NewHTTPError(403, "forbidden")) })
	r.GET("/plain", func(c *Context) error { return errors.New("db password=secret failed") })
	r.GET("/written", func(c *Context) error {
		_ = c.String(202, "partial")
		return errors.New("after write")
	})
	r.GET("/badjson", func(c *Context) error { return c.JSON(200, math.NaN()) })
	r.GET("/panic", func(c *Context) error { panic("boom") }, Recover())

	tests := []struct {
		path string
		code int
		msg  string
	}{
		{"/http", 422, "bad input"},
		{"/wrapped", 403, "forbidden"},
		{"/plain", 500, "internal server error"},
		{"/badjson", 500, "internal server error"},
		{"/panic", 500, "internal server error"},
	}
	for _, tt := range tests {
		rec := do(r, "GET", tt.path, "")
		if rec.Code != tt.code {
			t.Errorf("%s: код %d, ожидалось %d", tt.path, rec.Code, tt.code)
			continue
		}
		if got := errBody(t, rec); got != tt.msg {
			t.Errorf("%s: error=%q, ожидалось %q", tt.path, got, tt.msg)
		}
	}
	rec := do(r, "GET", "/written", "")
	if rec.Code != 202 || rec.Body.String() != "partial" {
		t.Errorf("ответ уже начат — ошибку нельзя дописывать: %d %q", rec.Code, rec.Body)
	}

	r.ErrorHandler = func(c *Context, err error) { _ = c.String(599, "custom: "+err.Error()) }
	rec = do(r, "GET", "/plain", "")
	if rec.Code != 599 || !strings.HasPrefix(rec.Body.String(), "custom: ") {
		t.Errorf("кастомный ErrorHandler: %d %q", rec.Code, rec.Body)
	}
}

func TestBind(t *testing.T) {
	type in struct {
		Name string `json:"name"`
	}
	r := New()
	r.POST("/echo", func(c *Context) error {
		var v in
		if err := c.Bind(&v); err != nil {
			return err
		}
		return c.JSON(201, v)
	})
	rec := do(r, "POST", "/echo", `{"name":"go"}`)
	if rec.Code != 201 || strings.TrimSpace(rec.Body.String()) != `{"name":"go"}` {
		t.Errorf("Bind ok: %d %s", rec.Code, rec.Body)
	}
	for _, body := range []string{`{"name":"go","admin":true}`, `{"name":`, `[]`} {
		rec := do(r, "POST", "/echo", body)
		if rec.Code != 400 {
			t.Errorf("Bind(%s): код %d, ожидалось 400", body, rec.Code)
		}
	}
	rec = do(r, "POST", "/echo", `{"name":"`+strings.Repeat("x", 2<<20)+`"}`)
	if rec.Code != 413 {
		t.Errorf("Bind большого тела: код %d, ожидалось 413", rec.Code)
	}
}

func TestRecoverAbort(t *testing.T) {
	h := Recover()(func(c *Context) error { panic(http.ErrAbortHandler) })
	var got any
	func() {
		defer func() { got = recover() }()
		_ = h(&Context{W: httptest.NewRecorder(), R: httptest.NewRequest("GET", "/", nil)})
	}()
	if got != http.ErrAbortHandler {
		t.Errorf("http.ErrAbortHandler должен пробрасываться, recover() = %v", got)
	}
}

func TestSetGet(t *testing.T) {
	r := New()
	r.GET("/", func(c *Context) error {
		if _, ok := c.Get("missing"); ok {
			return errors.New("Get несуществующего ключа вернул ok")
		}
		c.Set("user", "ann")
		v, ok := c.Get("user")
		if !ok || v != "ann" {
			return fmt.Errorf("Get = %v %v", v, ok)
		}
		return c.NoContent(204)
	})
	if rec := do(r, "GET", "/", ""); rec.Code != 204 {
		t.Errorf("Set/Get: %d %s", rec.Code, rec.Body)
	}
}
