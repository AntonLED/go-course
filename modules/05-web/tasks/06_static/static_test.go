package static

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

var mtime = time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":       {Data: []byte("<h1>home</h1>"), ModTime: mtime},
		"css/app.css":      {Data: []byte("body{}"), ModTime: mtime},
		"js/app.js":        {Data: []byte("console.log(1)"), ModTime: mtime},
		"docs/index.html":  {Data: []byte("<h1>docs</h1>"), ModTime: mtime},
		"empty/readme.txt": {Data: []byte("x"), ModTime: mtime},
		".env":             {Data: []byte("SECRET=1"), ModTime: mtime},
		".git/config":      {Data: []byte("[core]"), ModTime: mtime},
	}
}

func newServer(t *testing.T) http.Handler {
	t.Helper()
	h, err := FileServer(testFS())
	if err != nil || h == nil {
		t.Fatalf("FileServer: %v", err)
	}
	return h
}

func do(h http.Handler, method, target string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var etagRe = regexp.MustCompile(`^"[^"]+"$`)

func TestServeFiles(t *testing.T) {
	h := newServer(t)
	tests := []struct {
		path, body, ctype, cache string
	}{
		{"/", "<h1>home</h1>", "text/html", "no-cache"},
		{"/index.html", "<h1>home</h1>", "text/html", "no-cache"},
		{"/css/app.css", "body{}", "text/css", "public, max-age=86400"},
		{"/js/app.js", "console.log(1)", "javascript", "public, max-age=86400"},
		{"/docs/", "<h1>docs</h1>", "text/html", "no-cache"},
		{"/docs", "<h1>docs</h1>", "text/html", "no-cache"},
		{"/css/../css/app.css", "body{}", "text/css", "public, max-age=86400"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := do(h, "GET", tt.path, nil)
			if rec.Code != 200 || rec.Body.String() != tt.body {
				t.Fatalf("код %d тело %q, ожидалось 200 %q", rec.Code, rec.Body.String(), tt.body)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, tt.ctype) {
				t.Errorf("Content-Type = %q, ожидалось содержащее %q", ct, tt.ctype)
			}
			if cc := rec.Header().Get("Cache-Control"); cc != tt.cache {
				t.Errorf("Cache-Control = %q, ожидалось %q", cc, tt.cache)
			}
			if et := rec.Header().Get("ETag"); !etagRe.MatchString(et) {
				t.Errorf("ETag = %q, ожидался сильный ETag в кавычках", et)
			}
		})
	}
}

func TestNotFoundAndHidden(t *testing.T) {
	h := newServer(t)
	for _, p := range []string{"/nope.txt", "/.env", "/.git/config", "/empty/", "/css/", "/../.env"} {
		if rec := do(h, "GET", p, nil); rec.Code != 404 {
			t.Errorf("GET %s: код %d, ожидалось 404 (тело %q)", p, rec.Code, rec.Body.String())
		}
	}
}

func TestMethods(t *testing.T) {
	h := newServer(t)
	rec := do(h, "POST", "/index.html", nil)
	if rec.Code != 405 || !strings.Contains(rec.Header().Get("Allow"), "GET") {
		t.Errorf("POST: код %d, Allow %q", rec.Code, rec.Header().Get("Allow"))
	}
	rec = do(h, "HEAD", "/css/app.css", nil)
	if rec.Code != 200 || rec.Body.Len() != 0 || rec.Header().Get("ETag") == "" {
		t.Errorf("HEAD: код %d, тело %d байт, ETag %q", rec.Code, rec.Body.Len(), rec.Header().Get("ETag"))
	}
}

func TestETagConditional(t *testing.T) {
	h := newServer(t)
	first := do(h, "GET", "/css/app.css", nil)
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("нет ETag")
	}
	if again := do(h, "GET", "/css/app.css", nil).Header().Get("ETag"); again != etag {
		t.Errorf("ETag нестабилен: %q vs %q", etag, again)
	}
	if other := do(h, "GET", "/js/app.js", nil).Header().Get("ETag"); other == etag {
		t.Error("у разных файлов одинаковый ETag")
	}
	tests := []struct {
		name, inm string
		code      int
	}{
		{"match", etag, 304},
		{"list", `"zzz", ` + etag, 304},
		{"weak", "W/" + etag, 304},
		{"star", "*", 304},
		{"miss", `"deadbeef"`, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(h, "GET", "/css/app.css", map[string]string{"If-None-Match": tt.inm})
			if rec.Code != tt.code {
				t.Fatalf("If-None-Match %s: код %d, ожидалось %d", tt.inm, rec.Code, tt.code)
			}
			if tt.code == 304 {
				if rec.Body.Len() != 0 {
					t.Error("304 не должен иметь тела")
				}
				if rec.Header().Get("ETag") != etag {
					t.Error("304 должен содержать ETag")
				}
				// RFC 9110 §15.4.5: 304 обязан содержать те же Cache-Control/ETag, что и 200.
				if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=86400" {
					t.Errorf("304: Cache-Control = %q, ожидалось как у 200", cc)
				}
			}
		})
	}
}

func TestSameContentSameETag(t *testing.T) {
	fs := fstest.MapFS{
		"a.txt": {Data: []byte("same")},
		"b.txt": {Data: []byte("same")},
	}
	h, err := FileServer(fs)
	if err != nil {
		t.Fatal(err)
	}
	a := do(h, "GET", "/a.txt", nil).Header().Get("ETag")
	b := do(h, "GET", "/b.txt", nil).Header().Get("ETag")
	if a == "" || a != b {
		t.Errorf("ETag должен зависеть только от содержимого: %q vs %q", a, b)
	}
}

func TestEmbeddedAssets(t *testing.T) {
	h, err := FileServer(Assets())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "Привет из embed!") {
		t.Errorf("embed: код %d, тело %q", resp.StatusCode, body)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/css/app.css", nil)
	req.Header.Set("If-None-Match", resp.Header.Get("ETag"))
	resp2, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Errorf("ETag index.html не должен подходить к app.css: код %d", resp2.StatusCode)
	}
}
