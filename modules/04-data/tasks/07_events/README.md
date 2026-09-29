# Задача 07. JSON: полиморфные события, свой `Duration`, потоковый разбор

В реальных системах через JSON часто передают поток событий разных типов: вход пользователя, покупку, сессию. Отличаются они полем `"type"`, а набор остальных полей у каждого свой. Стандартный `encoding/json` такую полиморфию напрямую не поддерживает, поэтому её приходится собирать вручную. Заодно мы научим `time.Duration` сериализоваться по-человечески и разберём большой JSON-массив потоково, не загружая его в память.

Пакет `events`. Типы (`Duration`, `Event`, `Login`, `Purchase`, `Session`, ошибки) лежат в `types.go`, реализовать нужно в `events.go`:

```go
func (d Duration) MarshalJSON() ([]byte, error)   // Duration(90*time.Second) → "1m30s"
func (d *Duration) UnmarshalJSON(b []byte) error  // "1m30s" или число секунд: 90, 1.5
func Marshal(e Event) ([]byte, error)             // {"type":"login","user":...}
func Unmarshal(data []byte) (Event, error)        // диспетчеризация по "type"
func Stream(r io.Reader, fn func(Event) error) error
```

## Формат

```json
[
  {"type":"login","user":"ann","at":"2024-05-01T10:00:00Z","ip":"10.0.0.1"},
  {"type":"purchase","user":"bob","amount_cents":1999,"items":["book"]},
  {"type":"session","user":"ann","length":"1h2m0s"}
]
```

`Marshal` выдаёт плоский объект: `"type"` идёт первым, остальные поля — в порядке объявления в структуре, с учётом `omitempty`.

`Unmarshal` возвращает значение конкретного типа (`Login`, а не `*Login`). Если поля `type` нет, возвращается `ErrNoType`, если тип неизвестен — `ErrUnknownType`; обе ошибки проверяются через `errors.Is`. Неизвестные поля считаются ошибкой (`Decoder.DisallowUnknownFields`), и `"amount_cents": 1.5` — тоже ошибка.

`Stream` читает JSON-массив потоково. Сначала `dec.Token()`, от которого мы ждём `[`, затем цикл `for dec.More() { dec.Decode(&raw) }`, после него закрывающая `]` и проверка, что дальше идёт `io.EOF`. Ошибку из `fn` нужно вернуть как есть и прекратить чтение. Тест подаёт 6 МБ через `io.Pipe` и проверяет, что после остановки на 3-м элементе прочитано не больше 64 КиБ.

## Подвохи

Встроенный интерфейс (`struct{ Type string; Event }`) пакет json не «разворачивает», и в результате получится `"Event":{...}`. Встраивать нужно конкретный тип, например `*Login`.

Если включить `DisallowUnknownFields` и декодировать сразу в `Login`, поле `"type"` окажется «неизвестным». Выручает обёртка ``struct{ Type string `json:"type"`; *Login }``.

`MarshalJSON` объявляется на значении, а `UnmarshalJSON` — на указателе. Если объявить `MarshalJSON` на указателе, то поле-значение `Duration` в структуре, переданной по значению, закодируется как число наносекунд.

Число, декодированное в `any`, всегда имеет тип `float64`. А $10^{300}$ секунд (`1e300`) не влезает в `int64` наносекунд — такой ввод тоже нужно отвергнуть.

Не уходите в рекурсию: `json.Marshal(d)` внутри `(d Duration) MarshalJSON` вызовет сам себя.

## Проверка

```
cd go-course  # корень курса
go test ./modules/04-data/tasks/07_events/
go test -tags solution ./modules/04-data/tasks/07_events/   # эталон
```
