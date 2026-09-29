//go:build solution

package tail

import (
	"bytes"
	"io"
)

// Tail возвращает последние n строк rs (без '\n' и завершающего '\r').
//
// Идём от конца потока блоками по ChunkSize, приклеивая каждый блок спереди
// к уже прочитанному хвосту, пока в хвосте не наберётся n+1 перевод строки
// (или пока не дойдём до начала). Байты, раньше нужной границы, не читаем.
func Tail(rs io.ReadSeeker, n int) ([]string, error) {
	if n < 0 {
		return nil, ErrNegative
	}
	if n == 0 {
		return nil, nil
	}
	size, err := rs.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	var buf []byte // прочитанный хвост файла: байты [pos, size)
	pos := size
	// Сколько '\n' нужно: n разделителей между строками + возможный
	// завершающий '\n' в самом конце файла.
	need := n
	for pos > 0 {
		step := min(int64(ChunkSize), pos)
		pos -= step
		if _, err := rs.Seek(pos, io.SeekStart); err != nil {
			return nil, err
		}
		chunk := make([]byte, step, int(step)+len(buf))
		if _, err := io.ReadFull(rs, chunk); err != nil {
			return nil, err
		}
		buf = append(chunk, buf...)

		cnt := bytes.Count(buf, []byte{'\n'})
		if bytes.HasSuffix(buf, []byte{'\n'}) {
			cnt-- // финальный перевод строки не отделяет новую строку
		}
		if cnt >= need {
			break
		}
	}

	if size == 0 {
		return nil, nil
	}
	buf = bytes.TrimSuffix(buf, []byte{'\n'})
	lines := bytes.Split(buf, []byte{'\n'})
	if pos > 0 {
		lines = lines[1:] // первая «строка» блока может быть обрезана
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = string(bytes.TrimSuffix(l, []byte{'\r'}))
	}
	return out, nil
}
