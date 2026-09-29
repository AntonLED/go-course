//go:build !solution

// Package atomicfile — атомарная запись файлов через временный файл и rename.
package atomicfile

import (
	"io"
	"io/fs"
)

// Write атомарно записывает файл name: читатели видят либо старое
// содержимое целиком, либо новое целиком — и никогда не видят половину.
//
// Алгоритм:
//  1. os.CreateTemp в том ЖЕ каталоге, что и name (rename атомарен только в пределах одной ФС);
//  2. fn(w) пишет данные (через bufio.Writer — не забудьте Flush);
//  3. f.Chmod(perm) — CreateTemp создаёт файл с правами 0600 (umask к Chmod не применяется);
//  4. f.Sync() — данные и права на диске до rename;
//  5. f.Close() — проверить ошибку!;
//  6. os.Rename(tmp, name).
//
// При любой ошибке (и при панике в fn) временный файл удаляется, а
// исходный файл name остаётся нетронутым.
func Write(name string, perm fs.FileMode, fn func(w io.Writer) error) (err error) {
	// TODO
	panic("TODO")
}

// WriteFile — атомарный аналог os.WriteFile.
func WriteFile(name string, data []byte, perm fs.FileMode) error {
	// TODO: через Write
	panic("TODO")
}
