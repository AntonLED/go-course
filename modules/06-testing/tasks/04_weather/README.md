# 04. Клиент погодного API и его тестирование

Пакет `weather`. Урок: [Мокирование и тестирование API](../../lessons/02-mocking.md).

Почти любой сервис ходит в чужие API, и клиентский код для них обычно тестируют хуже всего: ходить в настоящий API из тестов медленно, нестабильно, а иногда и платно. В этой задаче мы напишем клиент к погодному API и увидим, как проверить все его ветки без сети. Тесты используют три техники из урока: фейковый HTTP-сервер (`httptest.NewServer`), фейковый транспорт (`RoundTripper`-функция, никакой сети) и ручной фейк интерфейса `Provider`.

Типы, ошибки-сентинелы, `RateLimitError`, `APIError`, `Client` и интерфейс `Provider` лежат в `types.go`.

## Что сделать

```go
func (c *Client) Current(ctx context.Context, city string) (Weather, error)
func Umbrella(ctx context.Context, p Provider, city string) (bool, error)
```

### Current

Если `city` после `TrimSpace` пуст, метод возвращает `ErrEmptyCity`, не делая HTTP-запроса.

Запрос выглядит как `GET {BaseURL}/v1/current?city=<город>&units=metric`, причём лишний `/` в конце `BaseURL` не должен ломать путь. Параметры собирайте через `url.Values`: название города может содержать `&`, пробелы и кириллицу.

В запросе должны быть заголовки `X-API-Key: <APIKey>` и `Accept: application/json`. Запрос создаётся с `ctx` через `http.NewRequestWithContext`. Если `HTTP == nil`, используйте свой клиент с таймаутом, а не `DefaultClient`.

Ответы обрабатываются так:

| Код | Результат |
|---|---|
| 200 | JSON `{"city","temp_c","condition","updated_at"(RFC 3339)}` → `Weather`; лишние поля игнорировать; битый JSON → ошибка |
| 404 | `ErrCityNotFound` |
| 401, 403 | `ErrUnauthorized` |
| 429 | `*RateLimitError{RetryAfter}` из заголовка `Retry-After` в секундах (нет/мусор → 0); `errors.Is(err, ErrRateLimited)` |
| прочее | `*APIError{Status, Message}`: поле `"error"` из JSON-тела, иначе текст тела без пробелов по краям (не больше 200 байт) |

Тело ответа при любом исходе дочитывается и закрывается ровно один раз. Читать его нужно с ограничением размера.

Ошибка транспорта оборачивается через `%w`, так чтобы `errors.Is(err, context.DeadlineExceeded)` работал.

### Umbrella

`Umbrella` — потребитель, который зависит только от интерфейса `Provider`. Зонт нужен, если погода `rain`, `drizzle` или `thunderstorm`. Ошибку провайдера нужно вернуть обёрнутой.

## Запуск проверки

```
go test -race ./modules/06-testing/tasks/04_weather/
```
