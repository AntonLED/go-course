//go:build solution

// Package atomicfile — атомарная запись файлов через временный файл и rename.
package atomicfile

import (
	"bufio"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Write атомарно записывает файл name.
func Write(name string, perm fs.FileMode, fn func(w io.Writer) error) (err error) {
	dir, base := filepath.Split(name)
	if dir == "" {
		dir = "."
	}
	// Временный файл — рядом с целевым: rename между файловыми системами
	// не атомарен (и вообще вернёт ошибку EXDEV).
	f, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()

	// Уборка при любом неуспехе, в том числе при панике в fn: defer
	// выполняется при раскрутке стека, а паника летит дальше.
	ok := false
	defer func() {
		if !ok {
			f.Close() // повторный Close безопасен, ошибку игнорируем
			os.Remove(tmp)
		}
	}()

	bw := bufio.NewWriter(f)
	if err := fn(bw); err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	// CreateTemp создаёт файл с правами 0600. Chmod (к нему umask не
	// применяется) делаем ДО Sync, чтобы fsync сохранил на диск и данные,
	// и новые права: иначе после сбоя питания файл мог бы остаться с 0600.
	if err := f.Chmod(perm); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	// Ошибка Close при записи важна: на NFS и некоторых ФС отложенная
	// запись может провалиться именно здесь.
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, name); err != nil {
		return err
	}
	ok = true

	// Для полной надёжности (переживает выключение питания) нужно ещё
	// fsync каталога, чтобы на диск попала сама запись о переименовании.
	// На Windows открыть каталог так нельзя — ошибку не считаем фатальной.
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

// WriteFile — атомарный аналог os.WriteFile.
func WriteFile(name string, data []byte, perm fs.FileMode) error {
	return Write(name, perm, func(w io.Writer) error {
		_, err := w.Write(data) // io.Writer обязан вернуть ошибку при n < len(data)
		return err
	})
}
