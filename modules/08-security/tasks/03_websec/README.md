# 03. Web security: заголовки, cookie, CSRF, CORS, path traversal, XSS

Задача к уроку [«Безопасность в HTTP (HTTPS)»](../../lessons/02-http-security.md).

Большинство взломов веб-приложений — это не хитрые эксплойты, а давно известные дыры: забытый заголовок, cookie без `HttpOnly`, CORS, пропускающий любой Origin, или путь к файлу, из которого можно выйти через `..`. В этой задаче вы напишете набор middleware и функций, закрывающих самые частые из них.

## Что сделать

Код пишется в `websec.go`. Константы и `ErrUnsafePath` уже есть в `types.go`.

```go
func SecurityHeaders(next http.Handler) http.Handler
func SetSessionCookie(w http.ResponseWriter, value string, maxAge time.Duration)
func CSRF(next http.Handler) http.Handler
func CSRFToken(r *http.Request) string
func SafeJoin(root, userPath string) (string, error)
func CORS(allowedOrigins []string, next http.Handler) http.Handler
func Greeting(w io.Writer, name, homepage string) error
```

### SecurityHeaders

Middleware ставит заголовки:

- CSP `default-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'`;
- `X-Content-Type-Options: nosniff`;
- `X-Frame-Options: DENY`;
- `Referrer-Policy: no-referrer`;
- HSTS `max-age=63072000; includeSubDomains` — только для HTTPS-запросов, то есть когда `r.TLS != nil`.

### SetSessionCookie

Сессионная cookie называется `__Host-session` и ставится с атрибутами `Path=/`, `HttpOnly`, `Secure`, `SameSite=Lax`; `MaxAge` задаётся в секундах.

### CSRF (double submit cookie)

Идея защиты в том, что чужой сайт может заставить браузер отправить вашу cookie, но не может её прочитать и продублировать в заголовке. Алгоритм такой:

1. Если у запроса нет валидной cookie `__Host-csrf`, сгенерируйте 32 байта из `crypto/rand`, закодируйте их в `base64.RawURLEncoding` и поставьте cookie с атрибутами `Secure` и `SameSite=Strict`, но без `HttpOnly`.
2. Запросы GET, HEAD, OPTIONS и TRACE пропускаются без проверки.
3. Для остальных методов: если есть заголовок `Origin`, его host должен совпадать с `r.Host`. Токен из заголовка `X-CSRF-Token` (или из поля формы `csrf_token`) должен совпасть с cookie из запроса; сравнивайте через `subtle.ConstantTimeCompare`. Если что-то не сошлось — `403`.
4. Обработчик получает текущий токен через `CSRFToken(r)`.

### SafeJoin

`SafeJoin` превращает путь из URL в путь внутри `root`. Ведущие `/` отбрасываются, а выход за пределы `root` через `..`, пустой путь и байт NUL дают `ErrUnsafePath`. Вам помогут `filepath.IsLocal` и `filepath.Join`.

### CORS

Разрешён только `Origin`, точно совпадающий с одним из элементов списка; значение `"null"` не разрешается никогда. Для разрешённого Origin ставятся `Access-Control-Allow-Origin: <origin>` и `Allow-Credentials: true`. Заголовок `Vary: Origin` ставится всегда. На preflight-запрос middleware отвечает `204` или `403` и не вызывает `next`.

### Greeting

`Greeting` выводит `<p>Hello, NAME!</p><a href="HOMEPAGE">homepage</a>` через `html/template`.

## Подвохи

Проверка `strings.HasPrefix(filepath.Join(root, p), root)` выглядит как защита от path traversal, но не является ею: `/srv/static2` тоже начинается с `/srv/static`.

Сравнение секретов через `a == b` открывает timing-атаку: время сравнения зависит от длины совпавшего префикса, и токен можно подобрать посимвольно.

Обратите внимание на разницу между двумя cookie. CSRF-cookie должна читаться из JS, иначе SPA не сможет положить токен в заголовок. Сессионная, наоборот, обязана быть `HttpOnly`, чтобы её не утащил XSS.

`Access-Control-Allow-Origin: *` несовместим с `credentials`, а «эхо» любого пришедшего Origin — открытая дыра. Проверка по префиксу тоже не годится: `https://app.example.com.evil.com` не должен её проходить.

Без `Vary: Origin` CDN закэширует ответ, сформированный для одного Origin, и отдаст его другому.

`text/template` ничего не экранирует. `html/template` экранирует с учётом контекста и заменяет URL вида `javascript:` на `#ZgotmplZ`.

HSTS, отправленный по обычному HTTP, бессмыслен: браузер его просто игнорирует.

## Запуск проверки

```bash
go test -race ./modules/08-security/tasks/03_websec/
go test -race -tags solution ./modules/08-security/tasks/03_websec/   # эталон
```
