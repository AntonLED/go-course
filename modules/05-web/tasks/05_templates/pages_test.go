package pages

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func mustRenderer(t *testing.T) *Renderer {
	t.Helper()
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	return r
}

func render(t *testing.T, r *Renderer, p Page) string {
	t.Helper()
	var sb strings.Builder
	if err := r.Render(&sb, p); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return sb.String()
}

func TestRenderList(t *testing.T) {
	r := mustRenderer(t)
	out := render(t, r, Page{
		Title: "Каталог",
		User:  "Ира",
		Items: []Item{
			{Title: "Go", URL: "https://go.dev/", Tags: []string{"go", "web"}, PriceCents: 1250},
			{Title: "Мелочь", URL: "/cheap?a=1&b=2", PriceCents: 5},
		},
	})
	wants := []string{
		"<title>Каталог</title>",
		`class="greeting"`, "Привет, Ира!",
		`<a href="https://go.dev/">Go</a>`,
		"12.50 ₽", "0.05 ₽",
		"go, web",
		`href="/cheap?a=1&amp;b=2"`,
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("в выводе нет %q:\n%s", w, out)
		}
	}
	if n := strings.Count(out, `class="item"`); n != 2 {
		t.Errorf("элементов class=\"item\": %d, ожидалось 2", n)
	}
	if strings.Contains(out, `class="empty"`) {
		t.Error("при непустом списке не должно быть class=\"empty\"")
	}
	if strings.Count(out, `class="tags"`) != 1 {
		t.Error("span.tags выводится только для элементов с тегами")
	}
}

func TestRenderEmptyAndAnon(t *testing.T) {
	r := mustRenderer(t)
	for _, items := range [][]Item{nil, {}} {
		out := render(t, r, Page{Title: "Пусто", Items: items})
		if !strings.Contains(out, `<p class="empty">Пусто</p>`) {
			t.Errorf("для пустого списка ожидалось <p class=\"empty\">Пусто</p>:\n%s", out)
		}
		if strings.Contains(out, "<ul") {
			t.Error("для пустого списка не нужен <ul>")
		}
		if strings.Contains(out, "greeting") {
			t.Error("для анонима не должно быть приветствия")
		}
	}
}

func TestRenderEscaping(t *testing.T) {
	r := mustRenderer(t)
	out := render(t, r, Page{
		Title: "<script>alert('t')</script>",
		User:  `"><img src=x onerror=alert(1)>`,
		Items: []Item{{
			Title: "<b>жирный</b>",
			URL:   "javascript:alert(document.cookie)",
			Tags:  []string{"<i>"},
		}},
	})
	bad := []string{"<script>alert", "<img src=x", "javascript:", "<b>жирный", "<i>"}
	for _, b := range bad {
		if strings.Contains(out, b) {
			t.Errorf("XSS: в выводе неэкранированное %q (text/template вместо html/template?)\n%s", b, out)
		}
	}
	if !strings.Contains(out, "&lt;script&gt;") || !strings.Contains(out, "#ZgotmplZ") {
		t.Errorf("ожидалось экранирование и #ZgotmplZ вместо javascript: URL:\n%s", out)
	}
}

func TestRenderNegativePrice(t *testing.T) {
	r := mustRenderer(t)
	var sb strings.Builder
	err := r.Render(&sb, Page{Title: "x", Items: []Item{{Title: "a", URL: "/", PriceCents: -1}}})
	if err == nil {
		t.Fatal("отрицательная цена должна давать ошибку рендеринга")
	}
}

func TestHandler(t *testing.T) {
	r := mustRenderer(t)
	tests := []struct {
		name string
		page Page
		err  error
		code int
	}{
		{"ok", Page{Title: "T", Items: []Item{{Title: "a", URL: "/a", PriceCents: 100}}}, nil, 200},
		{"loadErr", Page{}, errors.New("db down"), 500},
		{"renderErr", Page{Title: "T", Items: []Item{{Title: "a", URL: "/a", PriceCents: -5}}}, nil, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := r.Handler(func(*http.Request) (Page, error) { return tt.page, tt.err })
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
			if rec.Code != tt.code {
				t.Fatalf("код %d, ожидалось %d", rec.Code, tt.code)
			}
			body := rec.Body.String()
			if tt.code == 200 {
				if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
					t.Errorf("Content-Type = %q", ct)
				}
				if !strings.Contains(body, "1.00 ₽") {
					t.Errorf("тело: %s", body)
				}
				return
			}
			if strings.Contains(body, "<html") || strings.Contains(body, "doctype") {
				t.Errorf("при ошибке в ответ попал частичный HTML: %q", body)
			}
		})
	}
}

func TestConcurrentRender(t *testing.T) {
	r := mustRenderer(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var sb strings.Builder
			if err := r.Render(&sb, Page{Title: "x", Items: []Item{{Title: "a", URL: "/", Tags: []string{"t"}}}}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
