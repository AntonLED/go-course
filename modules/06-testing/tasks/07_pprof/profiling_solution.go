//go:build solution

package profiling

import (
	"bufio"
	"bytes"
	"cmp"
	"fmt"
	"io"
	"runtime"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
)

// CPUProfile профилирует выполнение work.
func CPUProfile(w io.Writer, work func()) error {
	// В процессе может идти только один CPU-профиль (глобальный SIGPROF-таймер).
	if err := pprof.StartCPUProfile(w); err != nil {
		return fmt.Errorf("start cpu profile: %w", err)
	}
	defer pprof.StopCPUProfile() // сбрасывает остаток данных в w, даже если work паникует
	work()
	return nil
}

// HeapProfile пишет профиль кучи. Статистика heap-профиля обновляется на GC,
// поэтому без runtime.GC() свежие аллокации в нём не видны.
func HeapProfile(w io.Writer) error {
	runtime.GC()
	return pprof.Lookup("heap").WriteTo(w, 0) // debug=0 — gzip-protobuf для go tool pprof
}

// ParseGoroutineProfile разбирает вывод pprof.Lookup("goroutine").WriteTo(w, 1).
//
//	goroutine profile: total 6
//	3 @ 0x474cee 0x40ee85 ...
//	#	0x5b8804	main.worker+0x24	/app/main.go:3
//	...
func ParseGoroutineProfile(r io.Reader) ([]FuncCount, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	if !sc.Scan() {
		return nil, fmt.Errorf("%w: пустой ввод", ErrMalformed)
	}
	head := sc.Text()
	totalStr, ok := strings.CutPrefix(head, "goroutine profile: total ")
	if !ok {
		return nil, fmt.Errorf("%w: заголовок %q", ErrMalformed, head)
	}
	total, err := strconv.Atoi(strings.TrimSpace(totalStr))
	if err != nil {
		return nil, fmt.Errorf("%w: total %q", ErrMalformed, totalStr)
	}

	counts := map[string]int{}
	sum := 0
	curCount := -1   // число горутин в текущей записи; -1 — записи нет
	var first string // первая функция записи (запасной вариант)
	assigned := false
	finish := func() {
		if curCount >= 0 && !assigned && first != "" {
			counts[first] += curCount
		}
		curCount, first, assigned = -1, "", false
	}

	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.TrimSpace(line) == "":
			finish()
		case strings.HasPrefix(line, "# labels:"):
			// метки pprof.Do — пропускаем
		case strings.HasPrefix(line, "#"):
			if curCount < 0 {
				return nil, fmt.Errorf("%w: кадр вне записи: %q", ErrMalformed, line)
			}
			f := strings.Fields(line) // "#", "0x...", "pkg.Func+0x1c", "file:line"
			if len(f) < 3 {
				return nil, fmt.Errorf("%w: кадр %q", ErrMalformed, line)
			}
			fn := f[2]
			if i := strings.LastIndex(fn, "+0x"); i >= 0 {
				fn = fn[:i]
			}
			if first == "" {
				first = fn
			}
			// Горутина «стоит» в первой функции, не относящейся к рантайму.
			if !assigned && !strings.HasPrefix(fn, "runtime.") {
				counts[fn] += curCount
				assigned = true
			}
		default:
			finish()
			nStr, _, ok := strings.Cut(line, " @")
			n, err := strconv.Atoi(nStr)
			if !ok || err != nil || n <= 0 {
				return nil, fmt.Errorf("%w: строка %q", ErrMalformed, line)
			}
			curCount = n
			sum += n
		}
	}
	finish()
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if sum != total {
		return nil, fmt.Errorf("%w: сумма записей %d != total %d", ErrMalformed, sum, total)
	}

	res := make([]FuncCount, 0, len(counts))
	for fn, c := range counts {
		res = append(res, FuncCount{fn, c})
	}
	slices.SortFunc(res, func(a, b FuncCount) int {
		if c := cmp.Compare(b.Count, a.Count); c != 0 {
			return c
		}
		return strings.Compare(a.Func, b.Func)
	})
	return res, nil
}

// GoroutineTop — топ функций по числу горутин в текущем процессе.
func GoroutineTop(n int) ([]FuncCount, error) {
	var buf bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&buf, 1); err != nil {
		return nil, err
	}
	res, err := ParseGoroutineProfile(&buf)
	if err != nil {
		return nil, err
	}
	if n >= 0 && n < len(res) {
		res = res[:n]
	}
	return res, nil
}
