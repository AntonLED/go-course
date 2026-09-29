# 08. Мини-фреймворк поверх net/http

Пакет `miniweb`. Урок: [Популярные фреймворки для HTTP](../../lessons/06-frameworks.md).

Фреймворки вроде chi, gin и echo дают роутер с группами, цепочки middleware, удобный `Context` и обработку ошибок, которые хендлер просто возвращает. Внешние зависимости в курсе недоступны, но это даже к лучшему: мы соберём свой echo-подобный фреймворк на `http.ServeMux` из Go 1.22, где сопоставление маршрутов с методами и параметрами уже есть в стандартной библиотеке. После этого магия настоящих фреймворков перестанет быть магией.

## API

```go
type HandlerFunc func(c *Context) error              // types.go
type Middleware func(next HandlerFunc) HandlerFunc   // types.go
type HTTPError struct{ Code int; Message string }    // types.go, NewHTTPError(code, msg)

func New() *Router
func (r *Router) Use(mw ...Middleware)               // глобальные
func (r *Router) ServeHTTP(w, req)
r.ErrorHandler func(c *Context, err error)           // по умолчанию DefaultErrorHandler

// RouteGroup (Router встраивает корневую *RouteGroup, поэтому r.GET/r.Group тоже работают)
func (g *RouteGroup) Group(prefix string, mw ...Middleware) *RouteGroup
func (g *RouteGroup) Use(mw ...Middleware)
func (g *RouteGroup) Handle(method, path string, h HandlerFunc, mw ...Middleware)
GET / POST / PUT / PATCH / DELETE (path, h, mw...)

// Context
c.W, c.R; Param(name); Query(name); Written(); JSON(code, v); String(code, s);
NoContent(code); Bind(dst); Set(key, v); Get(key)

func DefaultErrorHandler(c *Context, err error)
func Recover() Middleware
```

## Правила

Маршрут регистрируется в `ServeMux` как `"METHOD " + prefix + path`, а `Param` — это просто `r.PathValue`. Префиксы групп склеиваются аккуратно: `Group("/api").Group("/v1/").GET("/users/{id}")` даёт `GET /api/v1/users/{id}`, а `Group("/api").GET("")` — `GET /api`.

Middleware выполняются в таком порядке: сначала глобальные (`Router.Use`), затем middleware групп от внешней к внутренней, затем middleware самого маршрута и в конце хендлер.

`Router.Use` действует на все запросы, в том числе на маршруты, зарегистрированные до его вызова, и на ответы 404/405. `RouteGroup.Use`, напротив, действует только на маршруты, зарегистрированные после вызова.

Вложенные группы не должны портить друг другу списки middleware. Здесь поджидает классический подвох с `append` и общим backing array.

Если маршрут не найден, ответ — `404 {"error":"not found"}`. Если путь есть, но метод не подходит, ответ — `405 {"error":"method not allowed"}` с заголовком `Allow`. Подсказка: `mux.Handler(req)` возвращает пустой `pattern`, если маршрут не найден, а какой именно ответ (404 или 405) дал бы mux, можно узнать, вызвав возвращённый хендлер на «пробном» `ResponseWriter`.

Если хендлер вернул ошибку, а ответ ещё не начат, вызывается `ErrorHandler`. `DefaultErrorHandler` для `*HTTPError` (в том числе обёрнутой через `%w`) отвечает её кодом и телом `{"error": Message}`, а для любой другой ошибки — `500 {"error":"internal server error"}` без деталей. Если ответ уже начат, дописывать ничего нельзя.

`JSON` сначала сериализует значение и при ошибке возвращает её, ничего не записав в ответ. `Bind` читает не больше 1 MiB и использует `DisallowUnknownFields`; его ошибки превращаются в `*HTTPError` с кодом 400, а при превышении размера — 413.

`Recover()` превращает панику в ошибку, но `http.ErrAbortHandler` пробрасывает дальше.

## Запуск проверки

```
go test -race ./modules/05-web/tasks/08_miniweb/
```
