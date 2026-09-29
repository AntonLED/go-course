# Урок 3. gRPC + protobuf

gRPC — RPC-фреймворк, созданный в Google. Контракт сервиса описывается в файле `.proto`, сообщения сериализуются в Protocol Buffers, а в качестве транспорта используется HTTP/2. Из одного `.proto` генерируется типизированный клиент и интерфейс сервера для десятка языков, поэтому сервис на Go и клиент на Java или Python договариваются через один и тот же файл.

В этом уроке мы спустимся от синтаксиса `.proto` до отдельных байтов на проводе, а затем посмотрим, как поверх HTTP/2 устроены вызовы, статусы и дедлайны.

## proto3: синтаксис

Вот `.proto`-файл, в котором собраны почти все конструкции, встречающиеся на практике:

```proto
syntax = "proto3";

package inventory.v1;                                   // пространство имён в протоколе
option go_package = "example.com/shop/gen/inventory/v1;inventoryv1";

import "google/protobuf/timestamp.proto";

message Item {
  string sku = 1;
  int32  qty = 2;
  repeated string tags = 3;              // список
  map<string, string> attrs = 4;         // на проводе = repeated message {key=1; value=2}
  google.protobuf.Timestamp updated_at = 5;
  optional int32 discount = 6;           // явное присутствие поля (proto3 ≥ 3.15)
  oneof source { string warehouse = 7; string supplier = 8; }
  Status status = 9;

  reserved 10, 12 to 14;                 // номера удалённых полей — нельзя переиспользовать
  reserved "legacy_price";
}

enum Status {
  STATUS_UNSPECIFIED = 0;                // первый элемент enum обязан быть 0
  STATUS_ACTIVE = 1;
}

service Inventory {
  rpc GetItem(GetItemRequest) returns (Item);                      // unary
  rpc WatchStock(WatchRequest) returns (stream StockEvent);        // server streaming
  rpc UploadItems(stream Item) returns (UploadSummary);            // client streaming
  rpc Sync(stream SyncMessage) returns (stream SyncMessage);       // bidirectional
}
```

Скалярные типы protobuf отображаются в типы Go так:

| protobuf | Go |
|---|---|
| `int32`, `sint32`, `sfixed32` | `int32` |
| `int64`, `sint64`, `sfixed64` | `int64` |
| `uint32`, `fixed32` | `uint32` |
| `bool` | `bool` |
| `string` | `string` (обязан быть валидным UTF-8) |
| `bytes` | `[]byte` |
| `double` | `float64` |
| `float` | `float32` |

Вложенное сообщение превращается в указатель `*T`, `repeated` — в срез `[]T`, а `map` — в `map[K]V`.

Обратите внимание на значения по умолчанию: они вообще не передаются по сети. Из-за этого `qty = 0` и «qty не задан» неотличимы, если поле не объявлено как `optional`. У `optional`-поля в Go появляется тип `*int32`, и `nil` означает, что значения нет. До появления `optional` в proto3 для «необязательных» чисел использовали обёртки вроде `google.protobuf.Int32Value`.

## Номера полей и эволюция схемы

Самое важное, что нужно знать о protobuf: на проводе нет имён полей, есть только их **номера**. Из этого факта следуют все правила эволюции схемы.

Номер поля закреплён за ним навсегда. Если поле удалили, его номер и имя нужно пометить `reserved`, чтобы никто случайно не отдал их новому полю. Добавлять новые поля, наоборот, безопасно: старый код пропустит их как неизвестные, а при пересылке сообщения даже сохранит. Переименовать поле тоже можно, на бинарный формат это не влияет. Но имя используется в JSON- и текстовом представлении, поэтому переименование сломает их и всё, что опирается на имена полей.

Тип поля иногда можно сменить без потери совместимости. Типы `int32`, `int64`, `uint32`, `uint64` и `bool` взаимозаменяемы, потому что все кодируются одним varint (при сужении значение усекается). `string` и `bytes` совместимы, если байты — валидный UTF-8. Одиночное поле строки или сообщения можно превратить в `repeated`. А вот `int32` и `sint32` несовместимы, потому что кодируют числа по-разному; то же касается `fixed32` и `int32`.

Номер поля влияет и на размер сообщения. Номера от 1 до 15 кодируются в тег длиной 1 байт, поэтому их стоит отдавать самым частым полям. Номера от 16 до 2047 занимают 2 байта. Диапазон 19000–19999 зарезервирован самим protobuf, а максимальный номер поля равен $2^{29} - 1$. Откуда берутся эти границы, станет ясно в следующем разделе.

Следить за совместимостью вручную утомительно, поэтому проверку автоматизируют: `buf breaking --against '.git#branch=main'` сравнит схему с веткой `main` и сообщит о ломающих изменениях.

## Wire format

Сериализованное сообщение — это просто последовательность пар «ключ–значение». Ключ, он же **tag**, объединяет номер поля $f$ и тип кодирования значения $w$ (wire type) и записывается как varint. В спецификации и в коде это выглядит как `varint((field_number << 3) | wire_type)`, а математически

$$
\text{tag} = f \cdot 2^3 + w, \qquad 0 \le w \le 7.
$$

Под wire type отведены три младших бита, а всё остальное занимает номер поля. Тип кодирования нужен парсеру, чтобы понять, сколько байт занимает значение, даже если он ничего не знает о поле:

| wire type | Что | Типы |
|---|---|---|
| 0 VARINT | base-128 varint | int32, int64, uint*, sint*, bool, enum |
| 1 I64 | 8 байт little-endian | fixed64, sfixed64, double |
| 2 LEN | varint длина + байты | string, bytes, сообщения, packed repeated |
| 3/4 SGROUP/EGROUP | устарело (proto2) | — |
| 5 I32 | 4 байта little-endian | fixed32, sfixed32, float |

Теперь понятны границы номеров полей. Один байт varint вмещает 7 бит, то есть значения меньше $2^7 = 128$. Тег поля 15 равен $15 \cdot 8 + w \le 127$ и помещается в байт, а тег поля 16 уже не меньше 128 и требует двух байт. Два байта вмещают значения меньше $2^{14}$, и это даёт номера до $2^{14}/2^3 - 1 = 2047$. Максимум $2^{29} - 1$ получается потому, что тег хранится как 32-битное значение, из которого три бита отданы под wire type: $32 - 3 = 29$.

### Varint

**Varint** кодирует целое число переменным количеством байт. Число режется на группы по 7 бит, младшие группы идут первыми, а старший бит каждого байта служит флагом «дальше есть продолжение». Если байты $b_0, b_1, \dots, b_k$ несут 7-битные группы, то

$$
n = \sum_{i=0}^{k} b_i \cdot 128^i .
$$

Разберём число 150, в двоичном виде `1001 0110`. Оно раскладывается как $150 = 22 + 1 \cdot 128$: младшая группа 22 с флагом продолжения даёт байт `96`, старшая группа 1 даёт `01`, и в итоге получаем `96 01`. Точно так же $300 = 44 + 2 \cdot 128$ кодируется как `ac 02`. Самое длинное значение, 64-битное, занимает $\lceil 64/7 \rceil = 10$ байт.

![Varint режет число на 7-битные группы, пишет младшую первой и ставит старший бит 1 во всех байтах, кроме последнего: 150 кодируется как `96 01`, 300 как `ac 02`](img/varint.svg)

### Отрицательные числа и ZigZag

С отрицательными числами у varint беда. Отрицательные `int32` и `int64` кодируются как 64-битное дополнение до двух, в котором старшие биты заполнены единицами, поэтому такое значение всегда занимает 10 байт, даже если это просто −1.

Для полей, где отрицательные значения обычны, есть типы `sint32` и `sint64` с кодированием **ZigZag**. Оно нумерует числа по возрастанию модуля, чередуя знак: $0, -1, 1, -2, 2, \dots$ отображаются в $0, 1, 2, 3, 4, \dots$ Математически

$$
\operatorname{zz}(n) =
\begin{cases}
2n, & n \ge 0,\\
-2n - 1, & n < 0.
\end{cases}
$$

В коде то же самое записывают без ветвлений: `(n << 1) ^ (n >> 63)`, а для `sint32` сдвиг вправо делается на 31, `(n << 1) ^ (n >> 31)`. Сдвиг вправо здесь арифметический: `n >> 63` даёт 0 для неотрицательных чисел и все единицы для отрицательных, и XOR с этой маской как раз инвертирует биты.

![ZigZag нумерует числа по возрастанию модуля, чередуя знак, поэтому −1 в `sint32` занимает 1 байт, а в `int32` — 10](img/zigzag.svg)

### Сообщение целиком

Соберём всё вместе на примере `message Test { int32 a = 1; string b = 2; }` со значениями `a=150` и `b="testing"`:

```
08 96 01                 поле 1, varint, 150
12 07 74 65 73 74 69 6e 67  поле 2, LEN 7, "testing"
```

Тег `08` — это $1 \cdot 8 + 0$, поле 1 с типом VARINT, за ним уже знакомые `96 01`. Тег `12` в шестнадцатеричной записи равен 18, то есть $2 \cdot 8 + 2$: поле 2 с типом LEN. Дальше идёт длина `07` и семь байт строки.

![Сообщение `Test` побайтово: tag `08` означает поле 1 и VARINT, `96 01` — это 150; tag `12` означает поле 2 и LEN, `07` — длина строки](img/proto-wire-test.svg)

Повторяющиеся числовые скаляры в proto3 по умолчанию упаковываются (**packed repeated**). Например, `repeated int32 xs = 4` со значениями `[1, 2, 300]` кодируется одним LEN-полем: `22 04 01 02 ac 02`. Тег `22` — это $4 \cdot 8 + 2 = 34$, длина 4 байта, а внутри подряд идут varint-ы 1, 2 и 300. Парсер при этом обязан принимать обе формы: и упакованную, и непакованную, где тег повторяется перед каждым элементом.

![Packed-форма кладёт все значения в одно LEN-поле с одним тегом; непакованная повторяет тег перед каждым элементом, и парсер обязан понимать обе](img/packed-repeated.svg)

Вложенные сообщения тоже кодируются как LEN: после тега идёт длина, а затем байты вложенного сообщения. Значит, длину вложенного сообщения нужно знать до того, как начать его писать. Поэтому сгенерированный код сначала вычисляет `Size()` и только потом сериализует.

### Свойства формата, о которых спрашивают

Из устройства формата вытекает несколько свойств, которые любят проверять на собеседованиях. Поля в сообщении могут идти в любом порядке. Если скалярное поле встретилось дважды, побеждает последнее значение; именно поэтому два сообщения можно слить простой конкатенацией их байтов. Неизвестные поля парсер пропускает, ориентируясь на wire type.

Наконец, сериализация protobuf не каноническая. Порядок элементов `map` не фиксирован, а байты одного и того же сообщения могут отличаться между версиями библиотек и между языками. Опция `proto.MarshalOptions{Deterministic: true}` фиксирует порядок только в пределах одной сборки. Поэтому байты protobuf не годятся ни как стабильный ключ кэша, ни как вход для подписи.

## Кодогенерация: protoc и buf

Классический способ получить Go-код из `.proto` — компилятор `protoc` с двумя плагинами:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       api/inventory/v1/inventory.proto
```

Плагин `protoc-gen-go` генерирует типы сообщений (`inventory.pb.go`), а `protoc-gen-go-grpc` — клиент и интерфейс сервера (`inventory_grpc.pb.go`).

В новых проектах всё чаще используют **buf** — современную обёртку над этим процессом. Модуль и правила линтера описываются в `buf.yaml`, плагины — в `buf.gen.yaml`, а работа сводится к командам `buf generate`, `buf lint` и `buf breaking`. Зависимости buf умеет подтягивать из Buf Schema Registry.

## Сервер и клиент на grpc-go

Посмотрим, как выглядит реализация сервиса `Inventory` и вызов его с клиента:

```go
type server struct {
    inventoryv1.UnimplementedInventoryServer // обязательное встраивание: forward compatibility
}

func (s *server) GetItem(ctx context.Context, req *inventoryv1.GetItemRequest) (*inventoryv1.Item, error) {
    item, ok := s.store.Get(req.GetSku()) // геттеры безопасны для nil
    if !ok {
        return nil, status.Errorf(codes.NotFound, "sku %q not found", req.GetSku())
    }
    return item, nil
}

func (s *server) WatchStock(req *inventoryv1.WatchRequest, stream grpc.ServerStreamingServer[inventoryv1.StockEvent]) error {
    for ev := range s.events(stream.Context()) {
        if err := stream.Send(ev); err != nil {
            return err
        }
    }
    return nil // → grpc-status: 0
}

func main() {
    lis, _ := net.Listen("tcp", ":50051")
    s := grpc.NewServer(grpc.ChainUnaryInterceptor(logging, auth))
    inventoryv1.RegisterInventoryServer(s, &server{})
    reflection.Register(s) // для grpcurl
    s.Serve(lis)
}

// клиент
conn, err := grpc.NewClient("dns:///inventory:50051",
    grpc.WithTransportCredentials(insecure.NewCredentials()))
defer conn.Close()
client := inventoryv1.NewInventoryClient(conn)
ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
defer cancel()
item, err := client.GetItem(ctx, &inventoryv1.GetItemRequest{Sku: "A"})
if status.Code(err) == codes.NotFound { ... }
```

Пара деталей в этом коде заслуживает внимания. Встраивание `UnimplementedInventoryServer` обязательно: когда в `.proto` добавят новый метод, ваш код продолжит компилироваться, а новый метод будет возвращать `Unimplemented`. Геттеры вроде `req.GetSku()` безопасно вызывать даже на `nil`-сообщении.

Клиент создаётся через `grpc.NewClient` (появился в версии 1.63). Он не устанавливает соединение сразу, соединение ленивое; старый `grpc.Dial` объявлен устаревшим. Создавайте одно долгоживущее `*grpc.ClientConn` на каждый целевой сервис и переиспользуйте его во всём процессе. Внутри оно держит HTTP/2-соединения к адресам цели и само балансирует между ними.

gRPC поддерживает четыре типа RPC. Unary — обычный вызов «запрос–ответ». При server streaming сервер отправляет сообщения через `stream.Send`, а клиент читает их `Recv` до `io.EOF`. При client streaming всё наоборот: сервер читает `Recv` до `io.EOF` и затем отвечает одним сообщением через `SendAndClose`. При bidi-стриминге оба потока независимы, и каждая сторона пишет и читает когда хочет.

![Четыре типа RPC из сервиса `Inventory`: сообщения идут по одному HTTP/2-потоку, а поток закрывается трейлерами с `grpc-status`](img/rpc-types.svg)

## HTTP/2 и фрейминг gRPC

Каждый вызов gRPC — это отдельный HTTP/2-**поток** (stream) внутри общего TCP-соединения. Фреймы разных потоков перемешиваются, и медленный вызов не задерживает остальные. Так HTTP/2 даёт мультиплексирование без head-of-line blocking на уровне HTTP.

![Три вызова в одном TCP-соединении: фреймы разных потоков перемешаны и помечены своим stream id, поэтому долгий `WatchStock` не блокирует `GetItem`](img/http2-streams.svg)

Так выглядит unary-вызов на уровне фреймов:

```
HEADERS  :method POST  :path /inventory.v1.Inventory/GetItem
         content-type: application/grpc   te: trailers   grpc-timeout: 200m
         authorization: Bearer ...        (metadata)
DATA     00 | 00 00 00 05 | 0a 03 41 42 43      ← Length-Prefixed-Message
---- ответ ----
HEADERS  :status 200  content-type: application/grpc
DATA     00 | 00 00 00 0c | ...
HEADERS  grpc-status: 0  grpc-message: ...       ← трейлеры (END_STREAM)
```

Каждое сообщение внутри DATA снабжено пятибайтным префиксом: 1 байт compressed-flag и 4 байта длины в big-endian, а за ними идёт сам protobuf. Итоговый статус вызова передаётся в **трейлерах**, то есть в заголовках, которые приходят после тела. Статус становится известен только в самом конце, особенно при стриминге, поэтому HTTP-статус ответа почти всегда 200.

По умолчанию входящее сообщение не может быть больше 4 МБ. На сервере лимит меняется опцией `grpc.MaxRecvMsgSize`, на клиенте — `grpc.MaxCallRecvMsgSize`.

![Unary-вызов во фреймах HTTP/2 и разбор Length-Prefixed-Message из DATA: флаг сжатия, длина 5 в big-endian и protobuf `GetItemRequest{sku: "ABC"}`](img/grpc-frame.svg)

## Metadata, статусы, дедлайны

### Metadata

**Metadata** в gRPC — это HTTP/2-заголовки. Ключи пишутся в нижнем регистре, а ключи с суффиксом `-bin` содержат бинарные значения, которые на проводе передаются в base64. Вот как с ними работают на клиенте и на сервере:

```go
ctx = metadata.AppendToOutgoingContext(ctx, "x-request-id", id)        // клиент
md, _ := metadata.FromIncomingContext(ctx); md.Get("x-request-id")     // сервер
grpc.SetHeader(ctx, metadata.Pairs("x-served-by", host))               // заголовок ответа
var hdr metadata.MD; client.GetItem(ctx, req, grpc.Header(&hdr))       // прочитать на клиенте
```

### Коды статуса

gRPC определяет 17 кодов статуса (`codes.*`): `OK 0, Canceled 1, Unknown 2, InvalidArgument 3, DeadlineExceeded 4, NotFound 5, AlreadyExists 6, PermissionDenied 7, ResourceExhausted 8, FailedPrecondition 9, Aborted 10, OutOfRange 11, Unimplemented 12, Internal 13, Unavailable 14, DataLoss 15, Unauthenticated 16`.

Ретраить обычно можно `Unavailable`, а иногда ещё `ResourceExhausted` и `Aborted`. Остальные коды, как правило, детерминированы, и повтор вернёт ту же ошибку. Учтите, что обычная `error`, возвращённая из обработчика, превращается у клиента в `Unknown`. Если нужно передать структурированные подробности, к статусу прикрепляют детали: `st, _ := status.New(codes.InvalidArgument, "bad sku").WithDetails(&errdetails.BadRequest{...}); return nil, st.Err()`.

### Дедлайны

Дедлайн клиента передаётся серверу в заголовке `grpc-timeout` и на сервере автоматически становится дедлайном `ctx`. Если обработчик делает исходящие вызовы с этим же контекстом, они наследуют оставшуюся часть бюджета. Отмена работает похожим образом: когда клиент отменяет вызов, по сети уходит `RST_STREAM`, и на сервере срабатывает `ctx.Done()`.

Отсюда простое правило: на клиенте дедлайн нужно ставить всегда. Без него вызов может висеть бесконечно.

![Клиент ставит 200 мс; каждый следующий вызов получает в `grpc-timeout` только остаток, так что вся цепочка упирается в один абсолютный дедлайн](img/deadline-propagation.svg)

## Интерсепторы

Интерсепторы — это middleware для gRPC. Unary-интерсептор получает контекст, запрос, информацию о методе и следующий обработчик в цепочке:

```go
func logging(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
    start := time.Now()
    resp, err := handler(ctx, req)
    slog.InfoContext(ctx, "rpc", "method", info.FullMethod, "code", status.Code(err), "dur", time.Since(start))
    return resp, err
}
// stream: func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error
// клиент: grpc.WithChainUnaryInterceptor(...)
```

В `ChainUnaryInterceptor(a, b, c)` самым внешним оказывается `a`: запрос проходит через него первым, а ответ — последним. Через интерсепторы обычно делают логирование, метрики, трейсинг (otelgrpc), recovery от паник, аутентификацию и валидацию.

![`ChainUnaryInterceptor(logging, auth)`: запрос проходит интерсепторы снаружи внутрь, ответ — обратно, и каждый может ответить сам, не вызывая следующий](img/interceptors.svg)

## Экосистема

Вокруг gRPC сложилась богатая экосистема. **grpc-gateway** генерирует REST↔gRPC-прокси по аннотациям в `.proto` вида `option (google.api.http) = { get: "/v1/items/{sku}" };`, так что один сервис можно выставить наружу и как REST API.

Если зарегистрировать на сервере **reflection** (`reflection.Register(s)`), он начнёт отдавать собственную схему. Это нужно утилите grpcurl и Postman, чтобы вызывать методы без `.proto`-файлов под рукой:

  ```bash
  grpcurl -plaintext localhost:50051 list
  grpcurl -plaintext -d '{"sku":"A"}' localhost:50051 inventory.v1.Inventory/GetItem
  ```

Для проверки здоровья есть стандартный сервис `grpc.health.v1.Health` (пакет `google.golang.org/grpc/health`). А для браузеров, которые не умеют работать с HTTP/2-трейлерами напрямую, существуют grpc-web и Connect.

## Типичные ошибки

Самая дорогая ошибка в protobuf — переиспользовать номер удалённого поля: старые клиенты будут читать новое поле как старое и получат мусор. Менее опасная, но частая — выбрать `int32` для отрицательных значений, где каждое займёт 10 байт, вместо `sint32`.

В gRPC-коде чаще всего забывают дедлайн на клиенте, и вызов висит вечно. Другая классика — создавать новый `grpc.ClientConn` на каждый запрос вместо одного долгоживущего. Если обработчик возвращает `fmt.Errorf`, клиент увидит `Unknown`, поэтому ошибки с осмысленным кодом нужно оборачивать через `status.Error`. И наконец, ошибки gRPC сравнивают не через `==`, а через `status.Code(err)`.

## Вопросы с собеседований

1. Что такое tag в protobuf и как он кодируется? Сколько байт займёт тег поля с номером 16?
2. Почему `int32(-1)` занимает 10 байт и что такое ZigZag?
3. Как безопасно удалить поле из сообщения?
4. Где в gRPC передаётся статус вызова и почему HTTP-статус почти всегда 200?
5. Чем отличаются четыре типа RPC и когда нужен bidi-стриминг?
6. Как дедлайн клиента попадает на сервер?

## Ссылки

- [protobuf.dev: Encoding](https://protobuf.dev/programming-guides/encoding/), [Language Guide (proto3)](https://protobuf.dev/programming-guides/proto3/)
- [gRPC over HTTP/2 (PROTOCOL-HTTP2.md)](https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-HTTP2.md)
- [grpc-go](https://pkg.go.dev/google.golang.org/grpc), [codes](https://pkg.go.dev/google.golang.org/grpc/codes), [status](https://pkg.go.dev/google.golang.org/grpc/status)
- [pkg.go.dev/google.golang.org/protobuf/encoding/protowire](https://pkg.go.dev/google.golang.org/protobuf/encoding/protowire)
- [buf.build/docs](https://buf.build/docs/)

## Практика

- [04_protowire](../tasks/04_protowire/) — varint, zigzag, теги и кодек сообщения `User` руками.
- [05_grpcframe](../tasks/05_grpcframe/) — 5-байтный фрейминг и мини-RPC с мультиплексированием, дедлайнами и отменой.
- [06_project_orders](../tasks/06_project_orders/) — Задание 7.
