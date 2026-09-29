//go:build solution

package service

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ======================= контекст запроса =======================

type ctxKey int

const (
	requestIDKey ctxKey = iota
	traceKey
)

type traceInfo struct {
	TraceID, SpanID, ParentID string
	Sampled                   bool
}

func requestIDFrom(ctx context.Context) string {
	s, _ := ctx.Value(requestIDKey).(string)
	return s
}

func traceFrom(ctx context.Context) (traceInfo, bool) {
	t, ok := ctx.Value(traceKey).(traceInfo)
	return t, ok
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // crypto/rand.Read не возвращает ошибок на поддерживаемых ОС
	return hex.EncodeToString(b)
}

// ======================= логирование =======================

// ctxHandler — обёртка над slog.Handler, добавляющая к каждой записи
// request_id и trace_id из контекста. Благодаря ей любой код, вызывающий
// log.InfoContext(ctx, ...), автоматически получает корреляцию.
type ctxHandler struct{ slog.Handler }

func (h ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := requestIDFrom(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	if t, ok := traceFrom(ctx); ok {
		r.AddAttrs(slog.String("trace_id", t.TraceID), slog.String("span_id", t.SpanID))
	}
	return h.Handler.Handle(ctx, r)
}

func (h ctxHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return ctxHandler{h.Handler.WithAttrs(as)}
}
func (h ctxHandler) WithGroup(name string) slog.Handler { return ctxHandler{h.Handler.WithGroup(name)} }

func newLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(ctxHandler{slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})})
}

// ======================= метрики =======================

// metrics — минимальный реестр под нужды сервиса (полная версия — задача 08).
type metrics struct {
	mu        sync.Mutex
	requests  map[[3]string]uint64 // method, route, code
	durations map[[2]string]*histo // method, route
	keys      func() int           // gauge, вычисляемый при экспорте
}

var buckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

type histo struct {
	counts [12]uint64 // len(buckets)+1, некумулятивные
	sum    float64
}

func newMetrics(keys func() int) *metrics {
	return &metrics{requests: map[[3]string]uint64{}, durations: map[[2]string]*histo{}, keys: keys}
}

func (m *metrics) observe(method, route string, code int, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[[3]string{method, route, strconv.Itoa(code)}]++
	k := [2]string{method, route}
	h := m.durations[k]
	if h == nil {
		h = &histo{}
		m.durations[k] = h
	}
	h.counts[sort.SearchFloat64s(buckets, d.Seconds())]++
	h.sum += d.Seconds()
}

var labelEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

func fmtFloat(v float64) string {
	if math.IsInf(v, 1) {
		return "+Inf"
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func (m *metrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	bw := bufio.NewWriter(w)
	defer bw.Flush()

	m.mu.Lock()
	defer m.mu.Unlock()

	fmt.Fprintln(bw, "# HELP http_request_duration_seconds Длительность HTTP-запросов.")
	fmt.Fprintln(bw, "# TYPE http_request_duration_seconds histogram")
	hkeys := make([][2]string, 0, len(m.durations))
	for k := range m.durations {
		hkeys = append(hkeys, k)
	}
	sort.Slice(hkeys, func(i, j int) bool { return hkeys[i][0]+"\xff"+hkeys[i][1] < hkeys[j][0]+"\xff"+hkeys[j][1] })
	for _, k := range hkeys {
		h := m.durations[k]
		ls := fmt.Sprintf(`method="%s",route="%s"`, labelEscaper.Replace(k[0]), labelEscaper.Replace(k[1]))
		var cum uint64
		for i, c := range h.counts {
			cum += c
			le := math.Inf(1)
			if i < len(buckets) {
				le = buckets[i]
			}
			fmt.Fprintf(bw, "http_request_duration_seconds_bucket{%s,le=\"%s\"} %d\n", ls, fmtFloat(le), cum)
		}
		fmt.Fprintf(bw, "http_request_duration_seconds_sum{%s} %s\n", ls, fmtFloat(h.sum))
		fmt.Fprintf(bw, "http_request_duration_seconds_count{%s} %d\n", ls, cum)
	}

	fmt.Fprintln(bw, "# HELP http_requests_total Всего HTTP-запросов.")
	fmt.Fprintln(bw, "# TYPE http_requests_total counter")
	rkeys := make([][3]string, 0, len(m.requests))
	for k := range m.requests {
		rkeys = append(rkeys, k)
	}
	sort.Slice(rkeys, func(i, j int) bool { return strings.Join(rkeys[i][:], "\xff") < strings.Join(rkeys[j][:], "\xff") })
	for _, k := range rkeys {
		fmt.Fprintf(bw, "http_requests_total{method=\"%s\",route=\"%s\",code=\"%s\"} %d\n",
			labelEscaper.Replace(k[0]), labelEscaper.Replace(k[1]), k[2], m.requests[k])
	}

	fmt.Fprintln(bw, "# HELP kv_keys Количество ключей в хранилище.")
	fmt.Fprintln(bw, "# TYPE kv_keys gauge")
	fmt.Fprintf(bw, "kv_keys %d\n", m.keys())
}

// ======================= middleware =======================

// statusWriter запоминает код ответа.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) code() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

var requestIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// requestID: берём входящий X-Request-ID (если он безопасен — он попадёт
// в логи!), иначе генерируем. Возвращаем его клиенту.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if !requestIDRe.MatchString(id) {
			id = randomHex(8)
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

var traceparentRe = regexp.MustCompile(`^00-([0-9a-f]{32})-([0-9a-f]{16})-([0-9a-f]{2})$`)

// tracing: продолжаем входящую трассу W3C (traceparent) или начинаем новую.
// Полная версия со спанами — задача 09.
func tracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := traceInfo{SpanID: randomHex(8), Sampled: true}
		m := traceparentRe.FindStringSubmatch(r.Header.Get("traceparent"))
		if m != nil && m[1] != strings.Repeat("0", 32) && m[2] != strings.Repeat("0", 16) {
			flags, _ := strconv.ParseUint(m[3], 16, 8)
			t.TraceID, t.ParentID, t.Sampled = m[1], m[2], flags&1 == 1
		} else {
			t.TraceID = randomHex(16)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), traceKey, t)))
	})
}

// accessLog пишет строку лога на каждый запрос и восстанавливается после паники.
func accessLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler { // намеренный обрыв — пробрасываем
					panic(p)
				}
				log.ErrorContext(r.Context(), "panic", "panic", fmt.Sprint(p))
				if sw.status == 0 {
					http.Error(sw, "internal error", http.StatusInternalServerError)
				}
			}
			log.InfoContext(r.Context(), "http request",
				"method", r.Method, "path", r.URL.Path, "status", sw.code(),
				"duration_ms", float64(time.Since(start).Microseconds())/1000)
		}()
		next.ServeHTTP(sw, r)
	})
}

// instrument считает метрики. Должен стоять прямо перед ServeMux: mux
// записывает сработавший шаблон в r.Pattern того *Request, который получил.
// Шаблон ("GET /kv/{key}") вместо пути — защита от взрыва кардинальности.
func instrument(m *metrics, mux http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		defer func() {
			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			code := sw.code()
			if p := recover(); p != nil {
				code = http.StatusInternalServerError
				defer panic(p) // метрику учли — пусть дальше разбирается accessLog
			}
			m.observe(r.Method, route, code, time.Since(start))
		}()
		mux.ServeHTTP(sw, r)
	})
}
