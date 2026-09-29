# Задача 02. Свои io.Reader и io.Writer

Пакет `streams`. Интерфейсы `io.Reader` и `io.Writer` состоят из одного метода каждый, и именно поэтому на них держится половина стандартной библиотеки: сжатие, шифрование, сеть и файлы собираются из маленьких обёрток, как из конструктора. В этой задаче вы напишете несколько таких обёрток сами. Реализуйте в `streams.go`:

```go
func NewRot13Reader(r io.Reader) io.Reader           // ROT13 для латиницы, остальное без изменений
func NewCountingWriter(w io.Writer) *CountingWriter  // считает записанные байты
func (c *CountingWriter) Write(p []byte) (int, error)
func (c *CountingWriter) Count() int64
func LimitReader(r io.Reader, n int64) io.Reader     // своя версия io.LimitReader
func MultiWriter(ws ...io.Writer) io.Writer          // своя версия io.MultiWriter
```

Пример:

```go
r := NewRot13Reader(strings.NewReader("Hello, World!"))
b, _ := io.ReadAll(r) // "Uryyb, Jbeyq!"
```

## Контракт io.Reader (его проверяет `testing/iotest.TestReader`)

У `Read` есть правила, которые легко нарушить, если не читать документацию внимательно.

- `Read` может вернуть одновременно `n > 0` и `err != nil`, в том числе `io.EOF`. Поэтому сначала обработайте `n` прочитанных байт и только потом смотрите на ошибку.
- Преобразовывать можно только `p[:n]`, а не весь буфер `p`.
- `LimitReader` не должен читать из источника больше `n` байт. Для этого обрежьте буфер (`p = p[:remaining]`) до вызова `Read`. При `n <= 0` он сразу возвращает `0, io.EOF`.

## Подвохи

`CountingWriter` считает фактически записанные байты, даже если нижележащий writer вернул ошибку.

`MultiWriter` при ошибке любого из writer'ов сразу её возвращает. Если writer записал меньше `len(p)` байт и ошибки не вернул, результатом будет `io.ErrShortWrite`. Вызванный без аргументов, `MultiWriter` ведёт себя как `io.Discard`.

И помните, что кириллица в UTF-8 кодируется несколькими байтами: байты со значением от `0x80` и выше ROT13 не трогает.

## Проверка

```
go test ./modules/02-intro-part2/tasks/02_streams/
```
