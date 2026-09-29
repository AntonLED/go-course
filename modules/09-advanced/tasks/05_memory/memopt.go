//go:build !solution

package memopt

import "bytes"

// Event содержит те же поля, что и EventOriginal. Переставьте поля так,
// чтобы unsafe.Sizeof(Event{}) был минимальным (сейчас 64 байта на amd64).
type Event struct {
	Active  bool
	ID      int64
	Kind    uint8
	Score   float64
	Flags   uint16
	Count   int32
	Deleted bool
	Name    string
}

// AppendRecord дописывает в dst строку вида
//
//	id=42 name="Bob" score=3.50 ok=true\n
//
// и НЕ аллоцирует, если в dst достаточно capacity. Никакого fmt!
func AppendRecord(dst []byte, r Record) []byte {
	panic("TODO")
}

// JoinInts склеивает числа через sep, делая не более одной аллокации.
func JoinInts(ids []int, sep string) string {
	panic("TODO")
}

// BufferPool — пул *bytes.Buffer поверх sync.Pool.
type BufferPool struct {
	// TODO
}

// NewBufferPool создаёт пул; буферы с Cap() > maxCap в пул не возвращаются.
func NewBufferPool(maxCap int) *BufferPool {
	panic("TODO")
}

// Get возвращает пустой (Len()==0) буфер.
func (p *BufferPool) Get() *bytes.Buffer {
	panic("TODO")
}

// Put возвращает буфер в пул (nil и слишком большие — игнорируются).
func (p *BufferPool) Put(b *bytes.Buffer) {
	panic("TODO")
}

// CloneHead возвращает независимую копию первых n байт b (n > len(b) —
// копия всего b) с cap == len: результат не должен удерживать в памяти b.
func CloneHead(b []byte, n int) []byte {
	panic("TODO")
}

// RemoveAt удаляет s[i], сохраняя порядок, и обнуляет освободившийся
// последний слот базового массива, чтобы GC мог собрать объект.
func RemoveAt[T any](s []T, i int) []T {
	panic("TODO")
}
