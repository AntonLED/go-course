# 04. Protobuf wire format руками

Задача к уроку [«gRPC + protobuf»](../../lessons/03-grpc-protobuf.md).

Protobuf обычно воспринимают как чёрный ящик: описал схему, запустил `protoc`, получил `Marshal`. Но формат на проводе удивительно прост, и понимать его полезно — например, чтобы разобраться, почему поле `int32` со значением −1 занимает 10 байт, или почему старый сервис спокойно читает сообщения с новыми полями. В этой задаче вы реализуете то, что делают `google.golang.org/protobuf/encoding/protowire` и код, сгенерированный `protoc-gen-go`, — но только на стандартной библиотеке. Байтовые векторы в тестах совпадают с выводом настоящего protobuf.

```proto
syntax = "proto3";
message User {
  int64  id   = 1;
  string name = 2;
  repeated string tags = 3;
  sint32 score = 4;
}
```

## Что сделать

Код пишется в `wire.go`. Типы `Number`, `Type`, `User` и ошибки уже есть в `types.go`.

```go
func AppendVarint(b []byte, v uint64) []byte
func SizeVarint(v uint64) int
func ConsumeVarint(b []byte) (v uint64, n int, err error)
func EncodeZigZag(v int64) uint64
func DecodeZigZag(v uint64) int64
func AppendTag(b []byte, num Number, typ Type) []byte
func ConsumeTag(b []byte) (Number, Type, int, error)
func AppendBytes(b, v []byte) []byte
func ConsumeBytes(b []byte) ([]byte, int, error)
func ConsumeFieldValue(typ Type, b []byte) (int, error)
func (u *User) Marshal() []byte
func (u *User) Unmarshal(b []byte) error
```

## Шпаргалка по формату

**Varint** хранит число группами по 7 бит, младшие группы идут первыми, а старший бит каждого байта означает «дальше есть продолжение». Например, $300 = 1\,0010\,1100_2$ кодируется как `ac 02`. Самый длинный varint занимает 10 байт.

**Ключ поля (tag)** — это `varint((field_number << 3) | wire_type)`, то есть $8 \cdot \text{field\_number} + \text{wire\_type}$. Поле 1 типа varint даёт `08`, поле 2 типа bytes — `12`, а поле 16 уже занимает 2 байта.

Wire types бывают такие:

- 0 — varint;
- 1 — fixed64 (8 байт, little-endian);
- 2 — length-delimited;
- 3 и 4 — группы (устарели);
- 5 — fixed32 (4 байта, little-endian).

У `int64` и `int32` отрицательные значения кодируются как 64-битные, поэтому всегда занимают 10 байт. Для знаковых чисел, которые часто бывают отрицательными, существуют `sint32` и `sint64` с кодированием **ZigZag**: `(n << 1) ^ (n >> 63)`. Оно чередует знаки, так что маленькие по модулю числа остаются короткими:

$$
\text{zigzag}(n) = \begin{cases} 2n, & n \ge 0 \\ -2n - 1, & n < 0 \end{cases}
$$

Поля string, bytes и вложенные сообщения пишутся как `tag, varint(len), байты`.

Наконец, proto3 не пишет скаляр, равный значению по умолчанию. Элементы `repeated string` пишутся все, даже пустые строки `""`, каждый со своим тегом.

Пример: `User{ID: 150, Name: "Ann", Tags: {"a","bc"}, Score: -2}` кодируется в
`08 96 01 | 12 03 41 6e 6e | 1a 01 61 | 1a 02 62 63 | 20 03`.

## Требования к Unmarshal

- Перед разбором `u` сбрасывается. Для скаляров побеждает последнее вхождение поля, а элементы `repeated` накапливаются. Порядок полей в данных может быть любым.
- Неизвестные поля пропускаются с помощью `ConsumeFieldValue`. Именно на этом держится совместимость схем: старый код читает новые сообщения.
- Известное поле с неверным wire type даёт `ErrWireType`, поле с номером 0 — `ErrInvalidField`, а `string` с невалидным UTF-8 — `ErrInvalidUTF8`.
- `sint32` декодируется из младших 32 бит varint.
- Строки не должны ссылаться на входной буфер; `string(b)` как раз делает копию.

## Подвохи

10-й байт varint может быть только `00` или `01`. Любое другое значение означает переполнение 64 бит, и это ошибка.

В `ConsumeBytes` длина берётся из данных и может оказаться огромной. Сравнивайте её с остатком буфера в `uint64`, пока не преобразовали в `int`, — иначе после преобразования она может стать отрицательной.

В `uint64(v<<1) ^ uint64(v>>63)` сдвиг `v>>63` для `int64` арифметический: он размножает знаковый бит, и получается либо все нули, либо все единицы. На этом и держится весь трюк.

## Запуск проверки

```bash
go test ./modules/07-microservices/tasks/04_protowire/
go test -tags solution ./modules/07-microservices/tasks/04_protowire/   # эталон
go test -bench . ./modules/07-microservices/tasks/04_protowire/
```
