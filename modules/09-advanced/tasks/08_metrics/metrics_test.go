package metrics

import (
	"errors"
	"io"
	"math"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func text(t *testing.T, r *Registry) string {
	t.Helper()
	var b strings.Builder
	if err := r.WriteText(&b); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	return b.String()
}

func TestExposition(t *testing.T) {
	r := NewRegistry()
	reqs, err := r.NewCounter("http_requests_total", "Всего HTTP-запросов.", "method", "code")
	if err != nil {
		t.Fatal(err)
	}
	up, _ := r.NewGauge("up", "Сервис жив.")
	inflight, _ := r.NewGauge("http_in_flight", "Запросы в обработке.", "path")
	lat, err := r.NewHistogram("http_duration_seconds", "Латентность.", []float64{0.1, 0.5, 1}, "method")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = r.NewCounter("aaa_unused_total", "Серий ещё нет.", "x")

	reqs.WithLabelValues("GET", "200").Inc()
	reqs.WithLabelValues("GET", "200").Add(2)
	reqs.WithLabelValues("POST", "500").Inc()
	reqs.WithLabelValues("GET", "404").Add(0.5)
	up.WithLabelValues().Set(1)
	g := inflight.WithLabelValues(`/a"b\c` + "\n")
	g.Inc()
	g.Inc()
	g.Dec()
	lat.WithLabelValues("GET").Observe(0.1) // граница включительно: le="0.1"
	lat.WithLabelValues("GET").Observe(0.3)
	lat.WithLabelValues("GET").Observe(7)

	want := `# HELP aaa_unused_total Серий ещё нет.
# TYPE aaa_unused_total counter
# HELP http_duration_seconds Латентность.
# TYPE http_duration_seconds histogram
http_duration_seconds_bucket{method="GET",le="0.1"} 1
http_duration_seconds_bucket{method="GET",le="0.5"} 2
http_duration_seconds_bucket{method="GET",le="1"} 2
http_duration_seconds_bucket{method="GET",le="+Inf"} 3
http_duration_seconds_sum{method="GET"} 7.4
http_duration_seconds_count{method="GET"} 3
# HELP http_in_flight Запросы в обработке.
# TYPE http_in_flight gauge
http_in_flight{path="/a\"b\\c\n"} 1
# HELP http_requests_total Всего HTTP-запросов.
# TYPE http_requests_total counter
http_requests_total{method="GET",code="200"} 3
http_requests_total{method="GET",code="404"} 0.5
http_requests_total{method="POST",code="500"} 1
# HELP up Сервис жив.
# TYPE up gauge
up 1
`
	if got := text(t, r); got != want {
		t.Errorf("WriteText:\n--- получено ---\n%s--- ожидалось ---\n%s", got, want)
	}
}

func TestUnlabeledStartsAtZero(t *testing.T) {
	r := NewRegistry()
	_, _ = r.NewCounter("jobs_total", "line1\nline2 \\ back")
	_, _ = r.NewHistogram("size_bytes", "h", []float64{10, math.Inf(1)})
	want := `# HELP jobs_total line1\nline2 \\ back
# TYPE jobs_total counter
jobs_total 0
# HELP size_bytes h
# TYPE size_bytes histogram
size_bytes_bucket{le="10"} 0
size_bytes_bucket{le="+Inf"} 0
size_bytes_sum 0
size_bytes_count 0
`
	if got := text(t, r); got != want {
		t.Errorf("WriteText:\n--- получено ---\n%s--- ожидалось ---\n%s", got, want)
	}
}

func TestDefaultBuckets(t *testing.T) {
	r := NewRegistry()
	h, _ := r.NewHistogram("d", "h", nil)
	h.WithLabelValues().Observe(0.2)
	out := text(t, r)
	if strings.Count(out, "d_bucket") != len(DefBuckets)+1 || !strings.Contains(out, `d_bucket{le="0.25"} 1`) {
		t.Errorf("ожидались DefBuckets + +Inf:\n%s", out)
	}
}

func TestValidation(t *testing.T) {
	r := NewRegistry()
	if _, err := r.NewCounter("ok_total", "h"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		f    func() error
		want error
	}{
		{"дубликат", func() error { _, err := r.NewGauge("ok_total", "h"); return err }, ErrDuplicate},
		{"имя с дефисом", func() error { _, err := r.NewCounter("bad-name", "h"); return err }, ErrInvalidName},
		{"имя с цифры", func() error { _, err := r.NewCounter("1abc", "h"); return err }, ErrInvalidName},
		{"пустое имя", func() error { _, err := r.NewCounter("", "h"); return err }, ErrInvalidName},
		{"лейбл с двоеточием", func() error { _, err := r.NewCounter("a", "h", "a:b"); return err }, ErrInvalidName},
		{"лейбл __", func() error { _, err := r.NewCounter("b", "h", "__name"); return err }, ErrInvalidName},
		{"дубль лейбла", func() error { _, err := r.NewCounter("c", "h", "x", "x"); return err }, ErrInvalidName},
		{"le в гистограмме", func() error { _, err := r.NewHistogram("d", "h", nil, "le"); return err }, ErrInvalidName},
		{"бакеты не по возрастанию", func() error { _, err := r.NewHistogram("e", "h", []float64{1, 1}); return err }, ErrBuckets},
		{"бакеты убывают", func() error { _, err := r.NewHistogram("f", "h", []float64{2, 1}); return err }, ErrBuckets},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.f(); !errors.Is(err, tc.want) {
				t.Errorf("ошибка = %v, ожидалось %v", err, tc.want)
			}
		})
	}
	// Двоеточие в имени метрики допустимо (recording rules).
	if _, err := r.NewCounter("job:requests:rate5m", "h"); err != nil {
		t.Errorf("двоеточие в имени метрики допустимо: %v", err)
	}
}

func mustPanic(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: ожидалась паника", name)
		}
	}()
	f()
}

func TestPanics(t *testing.T) {
	r := NewRegistry()
	c, _ := r.NewCounter("c_total", "h", "a", "b")
	mustPanic(t, "мало значений лейблов", func() { c.WithLabelValues("x") })
	mustPanic(t, "много значений лейблов", func() { c.WithLabelValues("x", "y", "z") })
	mustPanic(t, "отрицательный Add у counter", func() { c.WithLabelValues("x", "y").Add(-1) })
}

func TestSameSeries(t *testing.T) {
	r := NewRegistry()
	c, _ := r.NewCounter("c_total", "h", "a")
	if c.WithLabelValues("x") != c.WithLabelValues("x") {
		t.Error("одинаковые значения лейблов должны давать одну серию")
	}
	// Склейка значений не должна давать коллизий: ("a,b") и ("a","b") — разные серии.
	c2, _ := r.NewCounter("c2_total", "h", "x", "y")
	if c2.WithLabelValues("a,b", "") == c2.WithLabelValues("a", ",b") {
		t.Error("разные наборы значений лейблов дали одну серию (коллизия ключа)")
	}
}

func TestConcurrent(t *testing.T) {
	r := NewRegistry()
	c, _ := r.NewCounter("c_total", "h", "worker")
	g, _ := r.NewGauge("g", "h")
	h, _ := r.NewHistogram("h", "h", []float64{1})
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				c.WithLabelValues("shared").Inc()
				g.WithLabelValues().Add(0.5)
				h.WithLabelValues().Observe(2)
				if i%100 == 0 {
					_ = r.WriteText(io.Discard) // экспорт параллельно с записью
				}
			}
		}()
	}
	wg.Wait()
	out := text(t, r)
	for _, want := range []string{`c_total{worker="shared"} 8000`, "g 4000", `h_bucket{le="+Inf"} 8000`, "h_sum 16000", "h_count 8000"} {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("ожидалась строка %q в\n%s", want, out)
		}
	}
}

func TestHandler(t *testing.T) {
	r := NewRegistry()
	c, _ := r.NewCounter("hits_total", "h")
	c.WithLabelValues().Inc()
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if ct := rec.Header().Get("Content-Type"); ct != ContentType {
		t.Errorf("Content-Type = %q, ожидалось %q", ct, ContentType)
	}
	if !strings.Contains(rec.Body.String(), "hits_total 1\n") {
		t.Errorf("тело: %s", rec.Body.String())
	}
}

func BenchmarkCounterInc(b *testing.B) {
	r := NewRegistry()
	c, _ := r.NewCounter("c_total", "h", "m")
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.WithLabelValues("GET").Inc()
		}
	})
}
