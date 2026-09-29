# 06. unsafe: zero-copy, заголовки срезов, смещения полей

Урок: [unsafe](../../lessons/05-unsafe.md)

Пакет `unsafe` позволяет обойти систему типов Go: превратить `[]byte` в строку без копирования, прочитать поле по смещению, посмотреть на массив `float32` как на массив `uint32`. Это даёт выигрыш в горячем коде, но цена ошибки — не паника, а тихо испорченная память или падение GC через час работы. В этой задаче вы реализуете несколько типовых приёмов и научитесь делать их так, чтобы `go vet` и `checkptr` не нашли, к чему придраться.

Пакет называется `unsafex`. Все функции пишутся в `unsafex.go`, ошибки уже есть в `types.go`.

| Функция | Что сделать |
|---|---|
| `BytesToString(b []byte) string` | zero-copy (`unsafe.String` + `unsafe.SliceData`); пустой вход → `""` |
| `StringToBytes(s string) []byte` | zero-copy (`unsafe.Slice` + `unsafe.StringData`); `""` → `nil`; `len == cap == len(s)` |
| `SliceLenCap[T](s []T) (len, cap int)` | прочитать поля заголовка среза через конверсию `*[]T → *yourHeader` (без встроенных `len`/`cap`) |
| `Float64Bits(f) uint64` | `*(*uint64)(unsafe.Pointer(&f))` — как в `math` |
| `ReadField[T](base unsafe.Pointer, off uintptr) T` | прочитать значение по смещению (используйте `unsafe.Add`) |
| `Overlap(a, b []byte) bool` | пересекаются ли `a[:len(a)]` и `b[:len(b)]` (cap не учитывается; пустые — не пересекаются) |
| `Reinterpret[To, From](s []From) ([]To, error)` | взгляд на ту же память как на `[]To` без копирования |

## Reinterpret

- `To` и `From` могут быть только числовыми типами: `int*`, `uint*`, `float*`, `uintptr`. Для остальных возвращается `ErrNotPOD`; проверяйте через `reflect.TypeFor[T]().Kind()`. Переинтерпретировать память как указатели — прямой путь к краху GC, который начнёт считать случайные числа адресами.
- Размер данных в байтах, `len(s)*Sizeof(From)`, должен делиться на `Sizeof(To)`, то есть $\text{len}(s) \cdot \text{Sizeof(From)} \equiv 0 \pmod{\text{Sizeof(To)}}$. Иначе — `ErrSize`.
- Адрес начала должен быть кратен `Alignof(To)`, иначе — `ErrAlign`.
- Пустой вход даёт `nil, nil`. У результата `cap == len`.

## Обязательные правила

Не сохраняйте `uintptr` в переменные, чтобы потом превратить обратно в `unsafe.Pointer`. На это ругается `go vet` (`possible misuse of unsafe.Pointer`), и не зря: пока адрес лежит в целочисленной переменной, GC не знает, что на объект кто-то ссылается, и может его передвинуть или собрать. Для арифметики над указателями есть `unsafe.Add`.

Не используйте устаревшие `reflect.SliceHeader` и `StringHeader`.

Тесты запускаются под `-race`, а он включает `-d=checkptr`, так что неправильные конверсии указателей упадут прямо в тестах.

## Когда zero-copy — это UB

Строки в Go неизменяемы, и на этом держатся ключи map и интернированные строки. Если вызвать `BytesToString(b)`, а потом выполнить `b[0] = 'x'`, строка «изменится» у вас на глазах, и инвариант будет нарушен. Обратная ситуация ещё жёстче: `StringToBytes("literal")[0] = 'x'` пишет в read-only память, и процесс получает `SIGSEGV`.

## Проверка
```
go test ./modules/09-advanced/tasks/06_unsafe/
go test -race ./modules/09-advanced/tasks/06_unsafe/   # checkptr
go vet ./modules/09-advanced/tasks/06_unsafe/
```
