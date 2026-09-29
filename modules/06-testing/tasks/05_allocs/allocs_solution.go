//go:build solution

package allocs

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
)

// JoinInts: одна аллокация — буфер strings.Builder нужного размера.
func JoinInts(xs []int, sep string) string {
	if len(xs) == 0 {
		return ""
	}
	// Верхняя оценка: 20 символов на int64 со знаком + разделители.
	var b strings.Builder
	b.Grow(len(xs)*20 + (len(xs)-1)*len(sep))
	var tmp [24]byte // на стеке: strconv.AppendInt сюда не аллоцирует
	for i, x := range xs {
		if i > 0 {
			b.WriteString(sep)
		}
		b.Write(strconv.AppendInt(tmp[:0], int64(x), 10))
	}
	return b.String() // Builder отдаёт свой буфер без копирования
}

var errEmptyField = errors.New("SumCSV: empty field")

// SumCSV без strings.Split: идём по строке индексами, подстроки не аллоцируют.
func SumCSV(s string) (int64, error) {
	if strings.Trim(s, " \t") == "" {
		return 0, nil
	}
	var sum int64
	for {
		field := s
		i := strings.IndexByte(s, ',')
		if i >= 0 {
			field = s[:i]
		}
		field = strings.Trim(field, " \t")
		if field == "" {
			return 0, errEmptyField
		}
		// ParseInt на успешном пути не аллоцирует; ошибка (*NumError) — аллоцирует, но это редкий путь.
		n, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			return 0, err
		}
		sum += n
		if i < 0 {
			return sum, nil
		}
		s = s[i+1:]
	}
}

// AppendRecord: только strconv.Append* — они пишут прямо в dst.
func AppendRecord(dst []byte, r Record) []byte {
	dst = append(dst, "id="...)
	dst = strconv.AppendInt(dst, r.ID, 10)
	dst = append(dst, " name="...)
	dst = strconv.AppendQuote(dst, r.Name)
	dst = append(dst, " score="...)
	dst = strconv.AppendFloat(dst, r.Score, 'f', -1, 64)
	dst = append(dst, " active="...)
	dst = strconv.AppendBool(dst, r.Active)
	dst = append(dst, " tags="...)
	for i, t := range r.Tags {
		if i > 0 {
			dst = append(dst, '|')
		}
		dst = append(dst, t...)
	}
	return append(dst, '\n')
}

// bufPool хранит *bytes.Buffer: указатель кладётся в interface{} без аллокации
// (в отличие от []byte, который пришлось бы упаковывать — это и ловит линтер SA6002).
var bufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// Render собирает строку в переиспользуемом буфере и пишет её одним Write.
func Render(w io.Writer, user string, items []string) error {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		// Не возвращаем в пул гигантские буферы — иначе один большой запрос
		// навсегда раздует память.
		if buf.Cap() <= 64<<10 {
			bufPool.Put(buf)
		}
	}()
	buf.WriteString(user)
	buf.WriteString(": ")
	for i, it := range items {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(it)
	}
	buf.WriteByte('\n')
	_, err := w.Write(buf.Bytes())
	return err
}
