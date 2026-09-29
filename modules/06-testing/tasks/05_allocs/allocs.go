//go:build !solution

package allocs

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Все функции ниже работают правильно, но аллоцируют слишком много.
// Задача — переписать их, сохранив результат байт в байт (см. README и лимиты в тестах).

// JoinInts соединяет числа через sep: JoinInts([]int{1, -2, 3}, ", ") == "1, -2, 3".
// Лимит: не более 1 аллокации.
func JoinInts(xs []int, sep string) string {
	parts := make([]string, 0, len(xs))
	for _, x := range xs {
		parts = append(parts, strconv.Itoa(x))
	}
	return strings.Join(parts, sep)
}

// SumCSV складывает целые числа, разделённые запятыми; вокруг чисел допустимы пробелы и табы.
// "" → 0. Пустое поле ("1,,2") или не число → ошибка. Лимит на успешном пути: 0 аллокаций.
func SumCSV(s string) (int64, error) {
	if strings.Trim(s, " \t") == "" {
		return 0, nil
	}
	var sum int64
	for _, f := range strings.Split(s, ",") {
		n, err := strconv.ParseInt(strings.Trim(f, " \t"), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("SumCSV: %w", err)
		}
		sum += n
	}
	return sum, nil
}

// AppendRecord дописывает к dst строку вида
//
//	id=42 name="Ann \"A\"" score=3.5 active=true tags=a|b|c\n
//
// score — strconv.FormatFloat(x, 'f', -1, 64), name — в кавычках как strconv.Quote.
// Лимит: 0 аллокаций, если в dst хватает ёмкости.
func AppendRecord(dst []byte, r Record) []byte {
	s := fmt.Sprintf("id=%d name=%q score=%s active=%t tags=%s\n",
		r.ID, r.Name, strconv.FormatFloat(r.Score, 'f', -1, 64), r.Active, strings.Join(r.Tags, "|"))
	return append(dst, s...)
}

// Render пишет в w строку "user: item1, item2, ...\n" одним вызовом w.Write.
// Лимит: 0 аллокаций в устоявшемся режиме (подсказка: sync.Pool с *bytes.Buffer).
func Render(w io.Writer, user string, items []string) error {
	_, err := fmt.Fprintf(w, "%s: %s\n", user, strings.Join(items, ", "))
	return err
}
