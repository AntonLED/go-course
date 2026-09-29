//go:build solution

package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

// Chain оборачивает h так, что mws[0] — самый внешний слой.
// Chain(h, A, B) == A(B(h)): запрос проходит A → B → h.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// StatusRecorder запоминает код ответа и число записанных байт.
type StatusRecorder struct {
	http.ResponseWriter
	Status      int
	Bytes       int
	wroteHeader bool
}

// NewStatusRecorder оборачивает w.
func NewStatusRecorder(w http.ResponseWriter) *StatusRecorder {
	return &StatusRecorder{ResponseWriter: w, Status: http.StatusOK}
}

func (r *StatusRecorder) WriteHeader(code int) {
	if r.wroteHeader { // повторный вызов — superfluous, запоминаем только первый
		return
	}
	r.wroteHeader = true
	r.Status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *StatusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK) // неявный 200, как у настоящего ResponseWriter
	}
	n, err := r.ResponseWriter.Write(b)
	r.Bytes += n
	return n, err
}

// Unwrap позволяет http.ResponseController добраться до Flush/Hijack/SetWriteDeadline.
func (r *StatusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Recover перехватывает панику обработчика и отвечает 500.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				// ErrAbortHandler — легальный способ оборвать ответ; net/http обработает сам.
				if v == http.ErrAbortHandler {
					panic(v)
				}
				logger.Error("panic", "value", v, "path", r.URL.Path, "request_id", RequestIDFrom(r.Context()))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"internal server error"}`))
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func validRequestID(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// RequestID берёт валидный X-Request-ID из запроса или генерирует новый,
// кладёт его в контекст и в заголовок ответа.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if !validRequestID(id) { // не доверяем входу: лог-инъекции, мегабайтные ID
			id = newRequestID()
		}
		w.Header().Set(HeaderRequestID, id)
		ctx := context.WithValue(r.Context(), ctxKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFrom достаёт ID из контекста ("" если нет).
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Logging пишет одну запись на запрос после его завершения.
func Logging(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := NewStatusRecorder(w)
			next.ServeHTTP(rec, r)
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.Status,
				"bytes", rec.Bytes,
				"duration", time.Since(start),
				"request_id", RequestIDFrom(r.Context()),
			)
		})
	}
}
