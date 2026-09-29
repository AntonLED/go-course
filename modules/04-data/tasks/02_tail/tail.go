//go:build !solution

package tail

import "io"

// Tail возвращает последние n строк rs (без '\n' и завершающего '\r').
//
// Требования:
//   - читать с конца блоками по ChunkSize (rs.Seek + io.ReadFull / ReadAt),
//     а не весь поток: для большого файла объём чтения должен быть ~O(размер хвоста);
//   - завершающий '\n' в конце файла не порождает пустую строку;
//   - текущая позиция rs не важна — Tail сам делает Seek;
//   - n == 0 или пустой поток → пустой результат (len == 0), n < 0 → ErrNegative.
func Tail(rs io.ReadSeeker, n int) ([]string, error) {
	// TODO: реализуйте
	panic("TODO")
}
