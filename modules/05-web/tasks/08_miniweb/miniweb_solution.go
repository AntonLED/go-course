//go:build solution

package miniweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ---------- Context ----------

// respWriter запоминает, начат ли ответ: после этого обработчик ошибок молчит.
type respWriter struct {
	http.ResponseWriter
	written bool
}

func (w *respWriter) WriteHeader(code int) { w.written = true; w.ResponseWriter.WriteHeader(code) }
func (w *respWriter) Write(b []byte) (int, error) {
	w.written = true
	return w.ResponseWriter.Write(b)
}
func (w *respWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Context — всё, что нужно обработчику, в одном объекте.
type Context struct {
	W     http.ResponseWriter
	R     *http.Request
	rw    *respWriter
	store map[string]any
}

func newContext(w http.ResponseWriter, r *http.Request) *Context {
	rw := &respWriter{ResponseWriter: w}
	return &Context{W: rw, R: r, rw: rw}
}

// Param — параметр пути ({id}, {path...}).
func (c *Context) Param(name string) string { return c.R.PathValue(name) }

// Query — параметр строки запроса.
func (c *Context) Query(name string) string { return c.R.URL.Query().Get(name) }

// Written сообщает, начат ли уже ответ.
func (c *Context) Written() bool { return c.rw.written }

// JSON сериализует v и отправляет ответ.
func (c *Context) JSON(code int, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err // ещё ничего не записано — обработчик ошибок ответит 500
	}
	c.W.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.W.WriteHeader(code)
	_, err = c.W.Write(b)
	return err
}

// String отправляет текст.
func (c *Context) String(code int, s string) error {
	c.W.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.W.WriteHeader(code)
	_, err := c.W.Write([]byte(s))
	return err
}

// NoContent отправляет только код.
func (c *Context) NoContent(code int) error {
	c.W.WriteHeader(code)
	return nil
}

// Bind декодирует JSON-тело (≤1 MiB, без неизвестных полей) в dst.
func (c *Context) Bind(dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(c.W, c.R.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return NewHTTPError(http.StatusRequestEntityTooLarge, "body too large")
		}
		return NewHTTPError(http.StatusBadRequest, "invalid JSON: "+err.Error())
	}
	return nil
}

// Set/Get — хранилище значений на время запроса (для middleware → handler).
func (c *Context) Set(key string, v any) {
	if c.store == nil {
		c.store = make(map[string]any)
	}
	c.store[key] = v
}

func (c *Context) Get(key string) (any, bool) {
	v, ok := c.store[key]
	return v, ok
}

// ---------- Router и группы ----------

// RouteGroup — набор маршрутов с общим префиксом и middleware.
type RouteGroup struct {
	router *Router
	prefix string
	mws    []Middleware
}

// Router — корень: ServeMux Go 1.22 делает сопоставление, мы — всё остальное.
type Router struct {
	*RouteGroup
	mux    *http.ServeMux
	global []Middleware
	// ErrorHandler вызывается, если обработчик вернул ошибку и ответ ещё не начат.
	ErrorHandler func(c *Context, err error)
}

// New создаёт роутер.
func New() *Router {
	r := &Router{mux: http.NewServeMux(), ErrorHandler: DefaultErrorHandler}
	r.RouteGroup = &RouteGroup{router: r}
	return r
}

// Use добавляет глобальные middleware: они оборачивают все запросы, включая 404/405,
// и действуют на уже зарегистрированные маршруты.
func (r *Router) Use(mw ...Middleware) { r.global = append(r.global, mw...) }

// Use для группы действует на маршруты, зарегистрированные после вызова.
func (g *RouteGroup) Use(mw ...Middleware) { g.mws = append(g.mws, mw...) }

// Group создаёт вложенную группу.
func (g *RouteGroup) Group(prefix string, mw ...Middleware) *RouteGroup {
	return &RouteGroup{
		router: g.router,
		prefix: joinPath(g.prefix, prefix),
		// Полная копия: append к общему backing array испортил бы соседние группы.
		mws: append(append([]Middleware(nil), g.mws...), mw...),
	}
}

func joinPath(prefix, p string) string {
	if p == "" {
		if prefix == "" {
			return "/"
		}
		return prefix
	}
	return strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(p, "/")
}

// chain: mws[0] — внешний слой.
func chain(h HandlerFunc, mws []Middleware) HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// Handle регистрирует маршрут.
func (g *RouteGroup) Handle(method, path string, h HandlerFunc, mw ...Middleware) {
	r := g.router
	local := make([]Middleware, 0, len(g.mws)+len(mw))
	local = append(append(local, g.mws...), mw...)
	final := chain(h, local) // групповые и маршрутные — фиксируем при регистрации
	pattern := method + " " + joinPath(g.prefix, path)
	r.mux.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
		r.serve(w, req, final)
	})
}

func (g *RouteGroup) GET(p string, h HandlerFunc, mw ...Middleware) {
	g.Handle(http.MethodGet, p, h, mw...)
}
func (g *RouteGroup) POST(p string, h HandlerFunc, mw ...Middleware) {
	g.Handle(http.MethodPost, p, h, mw...)
}
func (g *RouteGroup) PUT(p string, h HandlerFunc, mw ...Middleware) {
	g.Handle(http.MethodPut, p, h, mw...)
}
func (g *RouteGroup) PATCH(p string, h HandlerFunc, mw ...Middleware) {
	g.Handle(http.MethodPatch, p, h, mw...)
}
func (g *RouteGroup) DELETE(p string, h HandlerFunc, mw ...Middleware) {
	g.Handle(http.MethodDelete, p, h, mw...)
}

// serve применяет глобальные middleware (на момент запроса) и обрабатывает ошибку.
func (r *Router) serve(w http.ResponseWriter, req *http.Request, h HandlerFunc) {
	c := newContext(w, req)
	if err := chain(h, r.global)(c); err != nil && !c.Written() {
		r.ErrorHandler(c, err)
	}
}

// probe — минимальный ResponseWriter, чтобы узнать, что ответил бы ServeMux (404 или 405).
type probe struct {
	h    http.Header
	code int
}

func (p *probe) Header() http.Header         { return p.h }
func (p *probe) Write(b []byte) (int, error) { return len(b), nil }
func (p *probe) WriteHeader(code int)        { p.code = code }

// ServeHTTP делает Router http.Handler-ом.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	h, pattern := r.mux.Handler(req)
	if pattern == "" {
		p := &probe{h: http.Header{}, code: http.StatusOK}
		h.ServeHTTP(p, req)
		switch p.code {
		case http.StatusNotFound:
			r.serve(w, req, func(c *Context) error { return NewHTTPError(http.StatusNotFound, "not found") })
			return
		case http.StatusMethodNotAllowed:
			allow := p.h.Get("Allow")
			r.serve(w, req, func(c *Context) error {
				c.W.Header().Set("Allow", allow)
				return NewHTTPError(http.StatusMethodNotAllowed, "method not allowed")
			})
			return
		}
	}
	// Маршрут найден (или редирект) — пусть ServeMux заполнит PathValue и вызовет его.
	r.mux.ServeHTTP(w, req)
}

// DefaultErrorHandler: *HTTPError → его код и сообщение; иначе 500 без деталей.
func DefaultErrorHandler(c *Context, err error) {
	var he *HTTPError
	if errors.As(err, &he) {
		_ = c.JSON(he.Code, map[string]string{"error": he.Message})
		return
	}
	_ = c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
}

// Recover превращает панику в ошибку (и, как следствие, в 500).
func Recover() Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) (err error) {
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler {
						panic(v)
					}
					err = fmt.Errorf("panic: %v", v)
				}
			}()
			return next(c)
		}
	}
}
