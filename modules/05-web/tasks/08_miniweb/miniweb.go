//go:build !solution

package miniweb

import (
	"errors"
	"net/http"
)

// Context — всё, что нужно обработчику, в одном объекте.
// W должен быть обёрткой, которая помнит, начат ли ответ (см. Written).
type Context struct {
	W http.ResponseWriter
	R *http.Request
	// TODO: служебные поля
}

// Param — параметр пути ({id}, {path...}). Подсказка: r.PathValue.
func (c *Context) Param(name string) string { return "" }

// Query — параметр строки запроса.
func (c *Context) Query(name string) string { return "" }

// Written — был ли уже вызван WriteHeader/Write.
func (c *Context) Written() bool { return false }

// JSON: сначала Marshal (ошибка → вернуть её, ничего не записав), затем
// Content-Type "application/json; charset=utf-8", код, тело.
func (c *Context) JSON(code int, v any) error { return errors.New("TODO") }

// String: Content-Type "text/plain; charset=utf-8", код, тело.
func (c *Context) String(code int, s string) error { return errors.New("TODO") }

// NoContent: только код.
func (c *Context) NoContent(code int) error { return errors.New("TODO") }

// Bind: JSON-тело ≤1 MiB, DisallowUnknownFields. Ошибка → *HTTPError 400 (413 при превышении).
func (c *Context) Bind(dst any) error { return errors.New("TODO") }

// Set/Get — значения на время запроса.
func (c *Context) Set(key string, v any)      {}
func (c *Context) Get(key string) (any, bool) { return nil, false }

// RouteGroup — маршруты с общим префиксом и middleware.
type RouteGroup struct {
	// TODO
}

// Router — корень фреймворка. Методы Handle/GET/POST/.../Group доступны через встроенный *RouteGroup.
type Router struct {
	*RouteGroup
	// ErrorHandler вызывается, если обработчик вернул ошибку, а ответ ещё не начат.
	ErrorHandler func(c *Context, err error)
	// TODO
}

// New создаёт роутер с ErrorHandler = DefaultErrorHandler.
func New() *Router {
	// TODO
	return &Router{RouteGroup: &RouteGroup{}, ErrorHandler: DefaultErrorHandler}
}

// Use (Router) — глобальные middleware: оборачивают все запросы, включая 404/405,
// и действуют в том числе на уже зарегистрированные маршруты.
func (r *Router) Use(mw ...Middleware) {}

// Use (Group) — middleware группы; действует на маршруты, зарегистрированные после вызова.
func (g *RouteGroup) Use(mw ...Middleware) {}

// Group — вложенная группа: префикс склеивается, middleware наследуются.
func (g *RouteGroup) Group(prefix string, mw ...Middleware) *RouteGroup { return g }

// Handle регистрирует маршрут "METHOD prefix+path". Порядок middleware:
// глобальные → группы (от внешней к внутренней) → маршрута → обработчик.
func (g *RouteGroup) Handle(method, path string, h HandlerFunc, mw ...Middleware) {}

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

// ServeHTTP: найденный маршрут — вызвать; не найден — 404 {"error":"not found"};
// другой метод — 405 {"error":"method not allowed"} с заголовком Allow.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	http.NotFound(w, req)
}

// DefaultErrorHandler: *HTTPError (в т.ч. обёрнутая) → её код и {"error": Message};
// любая другая ошибка → 500 {"error":"internal server error"}.
func DefaultErrorHandler(c *Context, err error) {}

// Recover превращает панику обработчика в ошибку (→ 500). http.ErrAbortHandler пробрасывается.
func Recover() Middleware {
	return func(next HandlerFunc) HandlerFunc { return next }
}
