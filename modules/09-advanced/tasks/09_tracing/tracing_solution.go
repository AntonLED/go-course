//go:build solution

package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"maps"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// isLowerHex: W3C требует строчные hex-символы; hex.Decode принял бы и заглавные.
func isLowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}

// ParseTraceparent разбирает заголовок traceparent.
//
//	version(2) "-" trace-id(32) "-" parent-id(16) "-" flags(2)  = 55 символов
func ParseTraceparent(s string) (SpanContext, error) {
	var sc SpanContext
	if len(s) < 55 || s[2] != '-' || s[35] != '-' || s[52] != '-' {
		return sc, ErrInvalidTraceparent
	}
	version, traceHex, spanHex, flagsHex := s[0:2], s[3:35], s[36:52], s[53:55]
	if !isLowerHex(version) || !isLowerHex(traceHex) || !isLowerHex(spanHex) || !isLowerHex(flagsHex) {
		return sc, ErrInvalidTraceparent
	}
	switch {
	case version == "ff": // запрещённая версия
		return sc, ErrInvalidTraceparent
	case version == "00" && len(s) != 55: // у версии 00 нет хвоста
		return sc, ErrInvalidTraceparent
	case version != "00" && len(s) > 55 && s[55] != '-':
		// Будущие версии могут добавлять поля через '-': разбираем известную часть.
		return sc, ErrInvalidTraceparent
	}
	_, _ = hex.Decode(sc.TraceID[:], []byte(traceHex)) // корректность уже проверена
	_, _ = hex.Decode(sc.SpanID[:], []byte(spanHex))
	flags, _ := strconv.ParseUint(flagsHex, 16, 8)
	sc.Sampled = flags&0x01 != 0
	if !sc.IsValid() {
		return SpanContext{}, ErrInvalidTraceparent
	}
	return sc, nil
}

// Traceparent форматирует SpanContext в заголовок версии 00.
func (sc SpanContext) Traceparent() string {
	flags := "00"
	if sc.Sampled {
		flags = "01"
	}
	return "00-" + sc.TraceID.String() + "-" + sc.SpanID.String() + "-" + flags
}

// Tracer создаёт спаны и хранит завершённые (in-memory exporter).
type Tracer struct {
	mu       sync.Mutex
	rnd      io.Reader
	finished []SpanData
}

// NewTracer: rnd — источник случайных байт для ID (nil → crypto/rand).
func NewTracer(rnd io.Reader) *Tracer {
	if rnd == nil {
		rnd = rand.Reader
	}
	return &Tracer{rnd: rnd}
}

// randomFill заполняет b ненулевыми случайными байтами (нулевой ID невалиден).
func (t *Tracer) randomFill(b []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for {
		if _, err := io.ReadFull(t.rnd, b); err != nil {
			panic(fmt.Sprintf("tracing: источник случайности: %v", err))
		}
		for _, x := range b {
			if x != 0 {
				return
			}
		}
	}
}

// Span — активный спан.
type Span struct {
	tracer *Tracer
	data   SpanData

	mu    sync.Mutex
	ended bool
}

type spanKey struct{}
type remoteKey struct{}

// Start начинает спан. Родитель: локальный спан из ctx → удалённый
// SpanContext из ctx → нет (новая трасса, sampled=true).
func (t *Tracer) Start(ctx context.Context, name string) (context.Context, *Span) {
	s := &Span{tracer: t, data: SpanData{Name: name, Attrs: map[string]string{}, Start: time.Now()}}
	switch {
	case SpanFromContext(ctx) != nil:
		parent := SpanFromContext(ctx).data.Context
		s.data.Context.TraceID = parent.TraceID
		s.data.Context.Sampled = parent.Sampled
		s.data.ParentID = parent.SpanID
	case remoteFromContext(ctx).IsValid():
		parent := remoteFromContext(ctx)
		s.data.Context.TraceID = parent.TraceID
		s.data.Context.Sampled = parent.Sampled
		s.data.ParentID = parent.SpanID
		s.data.Remote = true
	default:
		t.randomFill(s.data.Context.TraceID[:])
		s.data.Context.Sampled = true
	}
	t.randomFill(s.data.Context.SpanID[:])
	return context.WithValue(ctx, spanKey{}, s), s
}

// Finished возвращает копию списка завершённых спанов в порядке End.
func (t *Tracer) Finished() []SpanData {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]SpanData, len(t.finished))
	for i, d := range t.finished {
		d.Attrs = maps.Clone(d.Attrs) // вызывающий не должен менять наши данные
		out[i] = d
	}
	return out
}

func (s *Span) Context() SpanContext { return s.data.Context } // неизменяем после Start

func (s *Span) SetAttr(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ended {
		s.data.Attrs[key] = value
	}
}

// End завершает спан; повторный вызов — no-op.
func (s *Span) End() {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	s.ended = true
	s.data.End = time.Now()
	d := s.data
	d.Attrs = maps.Clone(s.data.Attrs)
	s.mu.Unlock()

	s.tracer.mu.Lock()
	s.tracer.finished = append(s.tracer.finished, d)
	s.tracer.mu.Unlock()
}

// SpanFromContext возвращает текущий спан или nil.
func SpanFromContext(ctx context.Context) *Span {
	s, _ := ctx.Value(spanKey{}).(*Span)
	return s
}

func remoteFromContext(ctx context.Context) SpanContext {
	sc, _ := ctx.Value(remoteKey{}).(SpanContext)
	return sc
}

// ContextWithRemote кладёт в ctx удалённый родительский SpanContext.
func ContextWithRemote(ctx context.Context, sc SpanContext) context.Context {
	return context.WithValue(ctx, remoteKey{}, sc)
}

// Inject записывает traceparent текущего спана (или удалённого контекста).
func Inject(ctx context.Context, h http.Header) {
	if s := SpanFromContext(ctx); s != nil {
		h.Set(TraceparentHeader, s.Context().Traceparent())
		return
	}
	if sc := remoteFromContext(ctx); sc.IsValid() {
		h.Set(TraceparentHeader, sc.Traceparent())
	}
}

// Extract читает traceparent; невалидный заголовок игнорируется
// (по спецификации — начинаем новую трассу, а не отвечаем ошибкой).
func Extract(ctx context.Context, h http.Header) context.Context {
	sc, err := ParseTraceparent(h.Get(TraceparentHeader))
	if err != nil {
		return ctx
	}
	return ContextWithRemote(ctx, sc)
}

// statusRecorder запоминает код ответа.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap позволяет http.ResponseController добраться до исходного writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Middleware создаёт серверный спан "METHOD /path" на каждый запрос.
func Middleware(t *Tracer, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := Extract(r.Context(), r.Header)
		ctx, span := t.Start(ctx, r.Method+" "+r.URL.Path)
		defer span.End()
		span.SetAttr("http.method", r.Method)
		span.SetAttr("http.path", r.URL.Path)

		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r.WithContext(ctx))
		if rec.status == 0 {
			rec.status = http.StatusOK // обработчик ничего не написал
		}
		span.SetAttr("http.status_code", strconv.Itoa(rec.status))
	})
}

// Tree рисует дерево спанов: по строке на спан, отступ — 2 пробела на
// уровень. Корни — спаны без родителя или с родителем вне набора.
// Соседи сортируются по Start, при равенстве — по Name.
func Tree(spans []SpanData) string {
	ids := make(map[SpanID]bool, len(spans))
	for _, s := range spans {
		ids[s.Context.SpanID] = true
	}
	children := map[SpanID][]SpanData{}
	var roots []SpanData
	for _, s := range spans {
		if s.ParentID.IsValid() && ids[s.ParentID] {
			children[s.ParentID] = append(children[s.ParentID], s)
		} else {
			roots = append(roots, s)
		}
	}
	sortSpans := func(ss []SpanData) {
		sort.SliceStable(ss, func(i, j int) bool {
			if !ss[i].Start.Equal(ss[j].Start) {
				return ss[i].Start.Before(ss[j].Start)
			}
			return ss[i].Name < ss[j].Name
		})
	}
	var b strings.Builder
	var walk func(s SpanData, depth int)
	walk = func(s SpanData, depth int) {
		b.WriteString(strings.Repeat("  ", depth))
		b.WriteString(s.Name)
		b.WriteByte('\n')
		kids := children[s.Context.SpanID]
		sortSpans(kids)
		for _, k := range kids {
			walk(k, depth+1)
		}
	}
	sortSpans(roots)
	for _, r := range roots {
		walk(r, 0)
	}
	return b.String()
}
