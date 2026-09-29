//go:build solution

package wordcount

import (
	"bufio"
	"errors"
	"io"
	"unicode"
)

// Count потоково считает статистику по r.
//
// Решение — конечный автомат поверх bufio.Reader.ReadRune: память O(1)
// независимо от длины строк, а слова и руны корректно «склеиваются» через
// границы чанков, которые отдаёт нижележащий Reader.
func Count(r io.Reader) (Counts, error) {
	var c Counts
	br := bufio.NewReader(r)
	inWord := false
	lineLen := 0   // длина текущей строки в рунах
	pendingCR := 0 // подряд идущие '\r' в хвосте: последний не считаем, если за ним '\n'
	lineOpen := false
	for {
		ch, size, err := br.ReadRune()
		if err != nil {
			if lineOpen { // последняя строка без '\n'
				c.Lines++
				c.MaxLineLen = max(c.MaxLineLen, lineLen+pendingCR)
			}
			if errors.Is(err, io.EOF) {
				return c, nil
			}
			return c, err
		}
		c.Bytes += size
		c.Runes++

		if unicode.IsSpace(ch) {
			inWord = false
		} else if !inWord {
			inWord = true
			c.Words++
		}

		switch ch {
		case '\n':
			c.Lines++
			// Только один '\r' непосредственно перед '\n' — часть CRLF;
			// предыдущие '\r' («a\r\r\n») — обычные символы строки.
			c.MaxLineLen = max(c.MaxLineLen, lineLen+max(pendingCR-1, 0))
			lineLen, pendingCR, lineOpen = 0, 0, false
		case '\r':
			lineOpen = true
			pendingCR++
		default:
			lineOpen = true
			lineLen += pendingCR + 1
			pendingCR = 0
		}
	}
}
