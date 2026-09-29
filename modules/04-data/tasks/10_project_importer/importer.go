//go:build !solution

package importer

import "io"

// Run — вся утилита: main сводится к
//
//	os.Exit(importer.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
//
//	usage: importer -db DSN [-format csv|jsonl] [-dry-run] [-strict] [FILE]
//
// Полное ТЗ — в README.md. Рекомендуемое разбиение:
//
//	parseArgs(args) (options, error)                 — FlagSet, валидация аргументов
//	openInput(path, stdin) (io.Reader, io.Closer, error) — файл/stdin + автоопределение gzip
//	readCSV(r, emit) / readJSONL(r, emit) error      — потоковое чтение, emit(line, raw)
//	validate(raw) (Product, error)                   — правила полей, parsePrice("12.34") → 1234
//	productRepo{tx}.Upsert(ctx, p) (inserted bool, err error)
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// TODO
	return -1
}
