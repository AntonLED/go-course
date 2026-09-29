//go:build !solution

package tracing

import (
	"context"
	"io"
	"net/http"
)

// ParseTraceparent разбирает заголовок "00-<32 hex>-<16 hex>-<2 hex>".
func ParseTraceparent(s string) (SpanContext, error) {
	panic("TODO")
}

// Traceparent форматирует SpanContext в заголовок версии 00.
func (sc SpanContext) Traceparent() string {
	panic("TODO")
}

// Tracer создаёт спаны и хранит завершённые (in-memory exporter).
type Tracer struct {
	// TODO
}

// NewTracer: rnd — источник случайных байт для ID (nil → crypto/rand).
func NewTracer(rnd io.Reader) *Tracer {
	panic("TODO")
}

// Span — активный спан.
type Span struct {
	// TODO
}

// Start начинает спан — дочерний к спану из ctx, либо к удалённому
// SpanContext из ctx, либо корень новой трассы.
func (t *Tracer) Start(ctx context.Context, name string) (context.Context, *Span) {
	panic("TODO")
}

// Finished возвращает копию списка завершённых спанов в порядке End.
func (t *Tracer) Finished() []SpanData {
	panic("TODO")
}

func (s *Span) Context() SpanContext      { panic("TODO") }
func (s *Span) SetAttr(key, value string) { panic("TODO") }
func (s *Span) End()                      { panic("TODO") } // повторный End — no-op

// SpanFromContext возвращает текущий спан или nil.
func SpanFromContext(ctx context.Context) *Span {
	panic("TODO")
}

// ContextWithRemote кладёт в ctx удалённый родительский SpanContext.
func ContextWithRemote(ctx context.Context, sc SpanContext) context.Context {
	panic("TODO")
}

// Inject записывает traceparent текущего спана (или удалённого контекста) в h.
func Inject(ctx context.Context, h http.Header) {
	panic("TODO")
}

// Extract читает traceparent из h; валидный — кладёт в ctx как удалённый.
func Extract(ctx context.Context, h http.Header) context.Context {
	panic("TODO")
}

// Middleware создаёт серверный спан на каждый запрос.
func Middleware(t *Tracer, next http.Handler) http.Handler {
	panic("TODO")
}

// Tree рисует дерево спанов (см. README).
func Tree(spans []SpanData) string {
	panic("TODO")
}
