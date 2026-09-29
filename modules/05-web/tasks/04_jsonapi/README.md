# 04. JSON API: строгий декодер и Problem Details

Пакет `jsonapi`. Урок: [Работа с запросами и ответами](../../lessons/03-requests-responses.md).

Хендлер, который принимает JSON, выглядит тривиально ровно до первого странного клиента. Кто-то пришлёт гигабайтное тело, кто-то забудет `Content-Type`, кто-то опечатается в имени поля, и оно молча проигнорируется. Здесь мы напишем строгий декодер, который отвечает на каждую такую ситуацию правильным кодом, и будем отдавать ошибки в стандартном формате Problem Details (RFC 9457), чтобы клиенту было что показать пользователю.

Типы `Problem`, `Validator`, `CreateUser`, `User`, `ErrConflict` и `CreateFunc` лежат в `types.go`.

## Что сделать

```go
func WriteJSON(w http.ResponseWriter, status int, v any) error
func WriteProblem(w http.ResponseWriter, p *Problem)
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error // nil или *Problem
func (c *CreateUser) Validate() map[string]string
func NewCreateUserHandler(create CreateFunc, logger *slog.Logger) http.Handler
```

`DecodeJSON` возвращает `nil` или `*Problem` с таким кодом:

| Ситуация | Код |
|---|---|
| `Content-Type` не `application/json` (`; charset=utf-8` допустим — используйте `mime.ParseMediaType`) | 415 |
| тело больше `maxBytes` (`http.MaxBytesReader` + `errors.As(err, *http.MaxBytesError)`) | 413 |
| пустое тело, синтаксическая ошибка, обрыв, неверный тип (`Errors[поле]`), неизвестное поле (`DisallowUnknownFields`), что-то после первого объекта | 400 |
| `dst` реализует `Validator` и вернул непустую карту → `Problem.Errors` | 422 |

`WriteJSON` сначала сериализует значение в память, и если `json.Marshal` вернул ошибку, в `w` не должно попасть ничего. Почему это важно: если сначала выставить заголовки и вызвать `WriteHeader`, а потом кодировать, код ответа уже не поменять, и клиент получит 200 с пустым телом вместо 500.

`WriteProblem` выставляет `Content-Type: application/problem+json` и код `p.Status`. Пустой `Type` заменяется на `about:blank`, пустой `Title` — на `http.StatusText`.

`Validate` проверяет три поля. `name` после `TrimSpace` должно содержать от 1 до 50 рун (именно рун, а не байт). `email` разбирается через `net/mail.ParseAddress`, причём `addr.Address` должен совпасть с исходной строкой, так что `"Anna <a@b.c>"` не принимается. `age` должен лежать в диапазоне от 18 до 150 включительно.

`NewCreateUserHandler` декодирует тело через `DecodeJSON` с лимитом 1 MiB, вызывает `create` и при успехе отвечает `201` с заголовком `Location: /users/{id}`. На `ErrConflict` он отвечает 409, на любую другую ошибку — 500 без подробностей (подробности уходят в лог). У всех ошибок `Instance = r.URL.Path`.

## Подвохи

`dec.Decode` читает ровно один объект, поэтому `{"a":1}{"b":2}` пройдёт проверку, если не вызвать `Decode` второй раз.

`DisallowUnknownFields` отдаёт нетипизированную ошибку вида `json: unknown field "x"`, и распознавать её придётся по тексту.

Заголовки, выставленные после `WriteHeader`, молча игнорируются.

## Запуск проверки

```
go test -race ./modules/05-web/tasks/04_jsonapi/
```
