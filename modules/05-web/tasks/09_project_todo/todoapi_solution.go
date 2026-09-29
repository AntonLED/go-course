//go:build solution

package todoapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxBody      = 1 << 20
	defaultLimit = 20
	maxLimit     = 100
	maxTitle     = 200
)

// ---------- ответы ----------

type problem struct {
	Title  string            `json:"title"`
	Status int               `json:"status"`
	Detail string            `json:"detail,omitempty"`
	Errors map[string]string `json:"errors,omitempty"`
}

func (p *problem) Error() string { return p.Detail }

func newProblem(status int, detail string) *problem {
	return &problem{Title: http.StatusText(status), Status: status, Detail: detail}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		writeProblem(w, newProblem(http.StatusInternalServerError, ""))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(b, '\n'))
}

func writeProblem(w http.ResponseWriter, p *problem) {
	b, _ := json.Marshal(p)
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_, _ = w.Write(b)
}

// ---------- сервер ----------

type server struct {
	store  Store
	logger *slog.Logger
}

// NewServer собирает REST API.
func NewServer(store Store, logger *slog.Logger) http.Handler {
	s := &server{store: store, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /todos", s.handle(s.create))
	mux.HandleFunc("GET /todos", s.handle(s.list))
	mux.HandleFunc("GET /todos/{id}", s.handle(s.get))
	mux.HandleFunc("PATCH /todos/{id}", s.handle(s.patch))
	mux.HandleFunc("DELETE /todos/{id}", s.handle(s.delete))

	// RequestID — самый внешний, чтобы ID был и в логах, и в ответе на панику;
	// Logging снаружи Recover — чтобы видеть 500.
	return requestID(s.logging(s.recoverer(mux)))
}

// handle переводит ошибку хендлера в ответ: единая точка формирования ошибок.
func (s *server) handle(fn func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := fn(w, r)
		if err == nil {
			return
		}
		var p *problem
		switch {
		case errors.As(err, &p):
		case errors.Is(err, ErrNotFound):
			p = newProblem(http.StatusNotFound, "задача не найдена")
		default:
			s.logger.Error("internal error", "err", err, "request_id", requestIDFrom(r.Context()))
			p = newProblem(http.StatusInternalServerError, "")
		}
		writeProblem(w, p)
	}
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, newProblem(http.StatusBadRequest, "id должен быть положительным целым")
	}
	return id, nil
}

func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		return newProblem(http.StatusUnsupportedMediaType, "ожидается application/json")
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return newProblem(http.StatusRequestEntityTooLarge, "слишком большое тело")
		}
		return newProblem(http.StatusBadRequest, "некорректный JSON: "+err.Error())
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return newProblem(http.StatusBadRequest, "ожидается ровно один JSON-объект")
	}
	return nil
}

func validateTitle(title string, errs map[string]string) string {
	title = strings.TrimSpace(title)
	if n := utf8.RuneCountInString(title); n == 0 || n > maxTitle {
		errs["title"] = "от 1 до 200 символов"
	}
	return title
}

func unprocessable(errs map[string]string) error {
	p := newProblem(http.StatusUnprocessableEntity, "ошибка валидации")
	p.Errors = errs
	return p
}

func (s *server) create(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Title string `json:"title"`
		Done  bool   `json:"done"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	errs := map[string]string{}
	title := validateTitle(in.Title, errs)
	if len(errs) > 0 {
		return unprocessable(errs)
	}
	t, err := s.store.Create(r.Context(), Todo{Title: title, Done: in.Done})
	if err != nil {
		return err
	}
	w.Header().Set("Location", "/todos/"+strconv.FormatInt(t.ID, 10))
	writeJSON(w, http.StatusCreated, t)
	return nil
}

func queryInt(r *http.Request, name string, def, lo, hi int, errs map[string]string) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < lo || v > hi {
		errs[name] = "должно быть целым в диапазоне [" + strconv.Itoa(lo) + ", " + strconv.Itoa(hi) + "]"
		return def
	}
	return v
}

func (s *server) list(w http.ResponseWriter, r *http.Request) error {
	errs := map[string]string{}
	limit := queryInt(r, "limit", defaultLimit, 1, maxLimit, errs)
	offset := queryInt(r, "offset", 0, 0, 1<<31-1, errs)
	if len(errs) > 0 {
		p := newProblem(http.StatusBadRequest, "некорректные параметры пагинации")
		p.Errors = errs
		return p
	}
	items, total, err := s.store.List(r.Context(), offset, limit)
	if err != nil {
		return err
	}
	if items == nil {
		items = []Todo{} // в JSON — [], а не null
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
	return nil
}

func (s *server) get(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	t, err := s.store.Get(r.Context(), id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, t)
	return nil
}

func (s *server) patch(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	// Указатели отличают «поле не передано» от «передано нулевое значение».
	var in struct {
		Title *string `json:"title"`
		Done  *bool   `json:"done"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	errs := map[string]string{}
	if in.Title == nil && in.Done == nil {
		errs["body"] = "нужно хотя бы одно поле: title или done"
	}
	var title string
	if in.Title != nil {
		title = validateTitle(*in.Title, errs)
	}
	if len(errs) > 0 {
		return unprocessable(errs)
	}
	t, err := s.store.Get(r.Context(), id)
	if err != nil {
		return err
	}
	if in.Title != nil {
		t.Title = title
	}
	if in.Done != nil {
		t.Done = *in.Done
	}
	if t, err = s.store.Update(r.Context(), t); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, t)
	return nil
}

func (s *server) delete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.store.Delete(r.Context(), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---------- middleware ----------

type ctxKey struct{}

func requestIDFrom(ctx context.Context) string {
	s, _ := ctx.Value(ctxKey{}).(string)
	return s
}

func validID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, c := range []byte(s) {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !validID(id) {
			var b [8]byte
			_, _ = rand.Read(b[:])
			id = hex.EncodeToString(b[:])
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status, s.wrote = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.status, s.wrote = http.StatusOK, true
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func (s *server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start),
			"request_id", requestIDFrom(r.Context()))
	})
}

func (s *server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.logger.Error("panic", "value", v, "request_id", requestIDFrom(r.Context()))
				writeProblem(w, newProblem(http.StatusInternalServerError, ""))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
