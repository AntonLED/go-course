//go:build solution

package memopt

import (
	"bytes"
	"strconv"
	"strings"
	"sync"
)

// Event: поля отсортированы по убыванию выравнивания/размера — так
// компилятору не нужно вставлять padding между ними.
//
//	string(16) + int64(8) + float64(8) + int32(4) + uint16(2) + uint8 + bool + bool = 41
//	→ округление до кратного 8 = 48 байт (было 64).
type Event struct {
	Name    string
	ID      int64
	Score   float64
	Count   int32
	Flags   uint16
	Kind    uint8
	Active  bool
	Deleted bool
}

// AppendRecord дописывает запись в dst без аллокаций: strconv.Append*
// пишут прямо в срез, fmt не используется (упаковка аргументов в interface{}
// и рефлексия внутри fmt приводят к аллокациям).
func AppendRecord(dst []byte, r Record) []byte {
	dst = append(dst, "id="...)
	dst = strconv.AppendInt(dst, r.ID, 10)
	dst = append(dst, " name="...)
	dst = strconv.AppendQuote(dst, r.Name)
	dst = append(dst, " score="...)
	dst = strconv.AppendFloat(dst, r.Score, 'f', 2, 64)
	dst = append(dst, " ok="...)
	dst = strconv.AppendBool(dst, r.OK)
	return append(dst, '\n')
}

// JoinInts: сначала точно считаем итоговую длину, затем один Grow.
// strings.Builder.String() не копирует данные — итого 1 аллокация.
func JoinInts(ids []int, sep string) string {
	if len(ids) == 0 {
		return ""
	}
	n := len(sep) * (len(ids) - 1)
	for _, id := range ids {
		n += intLen(id)
	}
	var sb strings.Builder
	sb.Grow(n)           // единственная аллокация
	var scratch [20]byte // на стеке: не убегает в кучу
	for i, id := range ids {
		if i > 0 {
			sb.WriteString(sep)
		}
		sb.Write(strconv.AppendInt(scratch[:0], int64(id), 10))
	}
	// Builder.String() отдаёт внутренний буфер без копирования
	// (через unsafe.String), поэтому второй аллокации нет.
	return sb.String()
}

// intLen — количество символов в десятичной записи x (с минусом).
func intLen(x int) int {
	n := 1
	if x < 0 {
		n++ // знак; -x может переполниться для MinInt, поэтому делим сам x
	}
	for x >= 10 || x <= -10 {
		x /= 10
		n++
	}
	return n
}

// BufferPool — пул *bytes.Buffer поверх sync.Pool.
type BufferPool struct {
	pool   sync.Pool
	maxCap int
}

// NewBufferPool создаёт пул; буферы с Cap() > maxCap в пул не возвращаются.
func NewBufferPool(maxCap int) *BufferPool {
	return &BufferPool{
		maxCap: maxCap,
		pool:   sync.Pool{New: func() any { return new(bytes.Buffer) }},
	}
}

// Get возвращает пустой буфер.
func (p *BufferPool) Get() *bytes.Buffer {
	return p.pool.Get().(*bytes.Buffer)
}

// Put сбрасывает буфер и возвращает его в пул. Огромные буферы выбрасываем:
// иначе один запрос на 100 МБ навсегда «раздует» пул.
func (p *BufferPool) Put(b *bytes.Buffer) {
	if b == nil || b.Cap() > p.maxCap {
		return
	}
	b.Reset()
	p.pool.Put(b)
}

// CloneHead возвращает независимую копию первых n байт.
// b[:n] удерживал бы весь базовый массив b (классическая утечка через подслайс).
func CloneHead(b []byte, n int) []byte {
	n = max(0, min(n, len(b)))
	out := make([]byte, n)
	copy(out, b[:n])
	return out
}

// RemoveAt удаляет s[i] и зануляет хвост, как slices.Delete (Go 1.22+).
func RemoveAt[T any](s []T, i int) []T {
	copy(s[i:], s[i+1:])
	var zero T
	s[len(s)-1] = zero // иначе указатель остаётся в массиве за пределами len
	return s[:len(s)-1]
}
