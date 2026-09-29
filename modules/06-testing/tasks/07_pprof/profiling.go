//go:build !solution

package profiling

import (
	"errors"
	"io"
)

// CPUProfile снимает CPU-профиль на время выполнения work и пишет его в w
// (pprof.StartCPUProfile / StopCPUProfile). work вызывается ровно один раз.
// Если профилирование уже идёт — вернуть ошибку, НЕ вызывая work.
func CPUProfile(w io.Writer, work func()) error {
	// TODO
	return errors.New("TODO")
}

// HeapProfile пишет в w профиль кучи (pprof.Lookup("heap")) в бинарном формате,
// предварительно вызвав runtime.GC(), чтобы статистика была актуальной.
func HeapProfile(w io.Writer) error {
	// TODO
	return errors.New("TODO")
}

// ParseGoroutineProfile разбирает текстовый профиль горутин (WriteTo(w, 1)) и
// возвращает, сколько горутин находится в каждой функции (формат и правила — в README).
func ParseGoroutineProfile(r io.Reader) ([]FuncCount, error) {
	// TODO
	return nil, errors.New("TODO")
}

// GoroutineTop снимает профиль горутин текущего процесса и возвращает n самых «населённых» функций.
func GoroutineTop(n int) ([]FuncCount, error) {
	// TODO
	return nil, errors.New("TODO")
}
