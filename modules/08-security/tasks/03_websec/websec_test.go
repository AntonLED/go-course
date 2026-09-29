package websec

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var ok = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
})

func TestSecurityHeaders(t *testing.T) {
	h := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// заголовки должны быть выставлены ДО вызова next
		if w.Header().Get("X-Frame-Options") == "" {
			t.Error("заголовки нужно выставлять до вызова next")
		}
		w.WriteHeader(201)
	}))
	want := map[string]string{
		"Content-Security-Policy": "default-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "http://example.com/", nil))
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, ожидалось %q", k, got, v)
		}
	}
	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS по HTTP не выставляется, а получено %q", got)
	}

	req := httptest.NewRequest("GET", "https://example.com/", nil)
	req.TLS = &tls.ConnectionState{}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Strict-Transport-Security"); got != "max-age=63072000; includeSubDomains" {
		t.Errorf("HSTS по HTTPS = %q", got)
	}
}

func TestSessionCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	SetSessionCookie(rec, "s3cr3t", 2*time.Hour)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("ожидалась 1 cookie, получено %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != SessionCookieName || c.Value != "s3cr3t" || c.Path != "/" || c.Domain != "" {
		t.Errorf("cookie %+v", c)
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("HttpOnly=%v Secure=%v SameSite=%v; ожидалось true, true, Lax", c.HttpOnly, c.Secure, c.SameSite)
	}
	if c.MaxAge != 7200 {
		t.Errorf("MaxAge = %d, ожидалось 7200", c.MaxAge)
	}
}

func csrfCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == CSRFCookieName {
			return c
		}
	}
	return nil
}

func TestCSRFIssuesToken(t *testing.T) {
	var seen string
	h := CSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = CSRFToken(r) }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/form", nil))
	c := csrfCookie(t, rec)
	if c == nil {
		t.Fatal("GET без cookie должен выдать CSRF-cookie")
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil || len(raw) != 32 {
		t.Errorf("токен %q должен быть 32 байтами в base64 RawURL", c.Value)
	}
	if c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Errorf("атрибуты CSRF-cookie: %+v", c)
	}
	if seen != c.Value {
		t.Errorf("CSRFToken(r) = %q, ожидался токен из cookie %q", seen, c.Value)
	}

	// существующая валидная cookie не перевыпускается
	req := httptest.NewRequest("GET", "/form", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: c.Value})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if csrfCookie(t, rec) != nil || seen != c.Value {
		t.Error("при наличии валидной cookie новый токен выдавать не нужно")
	}
	// невалидная cookie заменяется
	req = httptest.NewRequest("GET", "/form", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "short"})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if nc := csrfCookie(t, rec); nc == nil || nc.Value == "short" {
		t.Error("невалидную CSRF-cookie нужно заменить")
	}
}

func TestCSRFProtection(t *testing.T) {
	token := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	other := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))
	called := false
	h := CSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))

	type tc struct {
		name, method, cookie, header, form, origin string
		want                                       int
	}
	cases := []tc{
		{"GET без токена", "GET", "", "", "", "", 200},
		{"HEAD без токена", "HEAD", "", "", "", "", 200},
		{"POST с совпадающим заголовком", "POST", token, token, "", "", 200},
		{"POST с полем формы", "POST", token, "", token, "", 200},
		{"POST своего Origin", "PUT", token, token, "", "http://example.com", 200},
		{"POST без cookie", "POST", "", token, "", "", 403},
		{"POST без заголовка", "POST", token, "", "", "", 403},
		{"POST с чужим токеном", "DELETE", token, other, "", "", 403},
		{"пустой токен и пустая cookie", "POST", "", "", "", "", 403},
		{"чужой Origin", "POST", token, token, "", "https://evil.com", 403},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			called = false
			var req *http.Request
			if c.form != "" {
				req = httptest.NewRequest(c.method, "http://example.com/transfer", strings.NewReader(url.Values{CSRFFormField: {c.form}}.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			} else {
				req = httptest.NewRequest(c.method, "http://example.com/transfer", nil)
			}
			if c.cookie != "" {
				req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: c.cookie})
			}
			if c.header != "" {
				req.Header.Set(CSRFHeaderName, c.header)
			}
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("код %d, ожидался %d", rec.Code, c.want)
			}
			if (c.want == 200) != called {
				t.Errorf("next вызван = %v, ожидалось %v", called, c.want == 200)
			}
		})
	}
}

func TestSafeJoin(t *testing.T) {
	root := filepath.FromSlash("/srv/static")
	good := []struct{ in, want string }{
		{"index.html", "/srv/static/index.html"},
		{"/img/logo.png", "/srv/static/img/logo.png"},
		{"a/../b.txt", "/srv/static/b.txt"},
		{"./a//b", "/srv/static/a/b"},
		{"/etc/passwd", "/srv/static/etc/passwd"},
		{"...", "/srv/static/..."},
	}
	for _, c := range good {
		got, err := SafeJoin(root, c.in)
		if err != nil || got != filepath.FromSlash(c.want) {
			t.Errorf("SafeJoin(%q) = %q, %v; ожидалось %q", c.in, got, err, c.want)
		}
	}
	bad := []string{"", "/", "..", "../etc/passwd", "a/../../etc/passwd", "/../../x", "img/../../../x", "a\x00.png", "a/b/../../.."}
	for _, in := range bad {
		if got, err := SafeJoin(root, in); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("SafeJoin(%q) = %q, %v; ожидалась ErrUnsafePath", in, got, err)
		}
	}
	// классическая ошибка — проверка через HasPrefix: /srv/static2 начинается с /srv/static
	if got, err := SafeJoin(root, "../static2/secret"); err == nil {
		t.Errorf("SafeJoin(../static2/secret) = %q: выход в соседний каталог с общим префиксом", got)
	}
}

func TestCORS(t *testing.T) {
	h := CORS([]string{"https://app.example.com", "null"}, ok)
	do := func(method, origin, reqMethod string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "https://api.example.com/items", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if reqMethod != "" {
			req.Header.Set("Access-Control-Request-Method", reqMethod)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := do("GET", "https://app.example.com", "")
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" ||
		rec.Header().Get("Access-Control-Allow-Credentials") != "true" || rec.Body.String() != "ok" {
		t.Errorf("разрешённый Origin: заголовки %v, тело %q", rec.Header(), rec.Body)
	}
	if !strings.Contains(strings.Join(rec.Header().Values("Vary"), ","), "Origin") {
		t.Error("нет Vary: Origin")
	}

	for _, evil := range []string{"https://evil.com", "https://app.example.com.evil.com", "http://app.example.com", "null"} {
		rec = do("GET", evil, "")
		if v := rec.Header().Get("Access-Control-Allow-Origin"); v != "" {
			t.Errorf("Origin %q: Access-Control-Allow-Origin = %q, ожидалось отсутствие", evil, v)
		}
		if rec.Body.String() != "ok" {
			t.Errorf("Origin %q: обычный запрос должен дойти до next (браузер сам не отдаст ответ скрипту)", evil)
		}
		if !strings.Contains(strings.Join(rec.Header().Values("Vary"), ","), "Origin") {
			t.Errorf("Origin %q: нет Vary: Origin", evil)
		}
	}

	rec = do("OPTIONS", "https://app.example.com", "PUT")
	if rec.Code != http.StatusNoContent || rec.Body.String() == "ok" {
		t.Errorf("preflight: код %d тело %q; ожидалось 204 без вызова next", rec.Code, rec.Body)
	}
	if rec.Header().Get("Access-Control-Allow-Methods") != "GET, POST, PUT, DELETE" ||
		rec.Header().Get("Access-Control-Allow-Headers") != "Content-Type, X-CSRF-Token" ||
		rec.Header().Get("Access-Control-Max-Age") != "600" ||
		rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Errorf("preflight заголовки: %v", rec.Header())
	}
	rec = do("OPTIONS", "https://evil.com", "DELETE")
	if rec.Code != http.StatusForbidden || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("preflight чужого Origin: код %d, ACAO %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
	// OPTIONS без Access-Control-Request-Method — не preflight
	if rec = do("OPTIONS", "https://app.example.com", ""); rec.Body.String() != "ok" {
		t.Error("обычный OPTIONS должен дойти до next")
	}
}

func TestGreetingEscapes(t *testing.T) {
	var b strings.Builder
	if err := Greeting(&b, `<script>alert("x")</script>`, "javascript:alert(1)"); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if strings.Contains(out, "<script>") || !strings.Contains(out, "&lt;script&gt;") {
		t.Errorf("имя не экранировано: %s", out)
	}
	if strings.Contains(out, "javascript:") || !strings.Contains(out, `href="#ZgotmplZ"`) {
		t.Errorf("javascript:-ссылка не обезврежена: %s", out)
	}

	b.Reset()
	_ = Greeting(&b, "Анна", `https://example.com/?q=a"onmouseover="x`)
	out = b.String()
	if !strings.HasPrefix(out, "<p>Hello, Анна!</p><a href=\"https://example.com/?q=a") || strings.Contains(out, `"onmouseover`) {
		t.Errorf("атрибут не экранирован: %s", out)
	}
}
