# 03. BasicAuth и CORS

Пакет `authcors`. Урок: [Роутинг и middleware](../../lessons/02-routing-middleware.md).

Два middleware из этой задачи стоят на границе сервиса. BasicAuth — простейшая аутентификация, которой до сих пор закрывают админки и внутренние эндпоинты, и даже в ней легко допустить уязвимость. CORS решает, каким сайтам браузер позволит читать ответы вашего API. Правила CORS кажутся запутанными, пока не реализуешь их сам, поэтому ниже они расписаны по шагам.

## BasicAuth

```go
func BasicAuth(realm string, users map[string]string) func(http.Handler) http.Handler
func UserFrom(ctx context.Context) (string, bool)
```

Логин и пароль берутся через `r.BasicAuth()`. Пароль сравнивается за постоянное время: `subtle.ConstantTimeCompare(sha256(got), sha256(want))`. Обычное `==` на строках выходит на первом несовпадающем байте, и по времени ответа атакующий может подбирать пароль побайтно — это timing-атака.

При неудаче (нет заголовка, схема не Basic, неизвестный пользователь, неверный пароль) middleware отвечает `401` с заголовком `WWW-Authenticate: Basic realm="<realm>", charset="UTF-8"`, и следующий обработчик не вызывается. При успехе имя пользователя кладётся в контекст, откуда его достаёт `UserFrom`.

## CORS

```go
func CORS(cfg CORSConfig) func(http.Handler) http.Handler   // CORSConfig — в types.go
```

Origin считается разрешённым, если он точно совпадает с одним из элементов списка или в списке есть `"*"`. Правила обработки такие:

1. Всегда добавляйте `Vary: Origin`: ответ зависит от Origin, и кэши должны об этом знать.
2. Если заголовка `Origin` нет, просто вызовите next.
3. Если Origin не разрешён, то на preflight-запрос нужно ответить `403`, не вызывая next и не добавляя CORS-заголовков. Любой другой запрос передаётся в next, но без CORS-заголовков: браузер сам не отдаст такой ответ скрипту.
4. Если Origin разрешён, `Access-Control-Allow-Origin` равен `"*"`, когда в списке есть `"*"` и `AllowCredentials == false`; в остальных случаях это эхо значения Origin. При `AllowCredentials` добавляется ещё `Access-Control-Allow-Credentials: true`.
5. Preflight — это `OPTIONS` с заголовком `Access-Control-Request-Method`, и next для него не вызывается. Если запрошенный метод не входит в `AllowedMethods` (сравнение без учёта регистра), ответ — `403` без CORS-заголовков. Если какой-то из `Access-Control-Request-Headers` (они перечислены через запятую, сравнение без учёта регистра) не разрешён, ответ тоже `403`. Иначе ответ `204` с заголовками `Access-Control-Allow-Methods` (методы в верхнем регистре через `", "`), `Access-Control-Allow-Headers` (`AllowedHeaders` через `", "`, если список не пуст) и `Access-Control-Max-Age` в секундах (если `MaxAge > 0`).
6. `OPTIONS` без `Access-Control-Request-Method` — обычный запрос: к нему применяется пункт 4, и вызывается next.

## Подвохи

Комбинацию `"*"` и credentials браузер отвергает. В этом случае нужно возвращать эхо Origin вместе с `Vary: Origin`.

CORS защищает пользователя в браузере, а не ваш сервер: curl'у эти заголовки безразличны. Авторизацию CORS не заменяет.

## Запуск проверки

```
go test -race ./modules/05-web/tasks/03_authcors/
```
