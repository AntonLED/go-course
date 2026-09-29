package seq

import (
	"iter"
	"reflect"
	"slices"
	"testing"
)

// counted — источник 0..n-1, считающий выданные элементы и отмечающий
// завершение (срабатывает defer внутри итератора).
type counted struct {
	produced int
	finished bool
}

func (c *counted) seq(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		defer func() { c.finished = true }()
		for i := 0; i < n; i++ {
			c.produced++
			if !yield(i) {
				return
			}
		}
	}
}

// assertStops проверяет, что после первого false от yield итератор больше
// не вызывает yield (иначе range-over-func паникует).
func assertStops[T any](t *testing.T, name string, s iter.Seq[T]) {
	t.Helper()
	calls := 0
	s(func(T) bool {
		calls++
		return false
	})
	if calls > 1 {
		t.Errorf("%s: yield вызван %d раз после возврата false", name, calls)
	}
	// И через обычный range с break — рантайм паникует при нарушении.
	for range s {
		break
	}
}

// assertStops2 — то же для iter.Seq2.
func assertStops2[K, V any](t *testing.T, name string, s iter.Seq2[K, V]) {
	t.Helper()
	calls := 0
	s(func(K, V) bool {
		calls++
		return false
	})
	if calls > 1 {
		t.Errorf("%s: yield вызван %d раз после возврата false", name, calls)
	}
	for range s {
		break
	}
}

func TestFibonacci(t *testing.T) {
	got := slices.Collect(Take(Fibonacci(), 10))
	want := []int{0, 1, 1, 2, 3, 5, 8, 13, 21, 34}
	if !slices.Equal(got, want) {
		t.Errorf("Take(Fibonacci, 10) = %v, ожидалось %v", got, want)
	}
	var last int
	for v := range Fibonacci() {
		if v > 1000 {
			last = v
			break
		}
	}
	if last != 1597 {
		t.Errorf("первое число Фибоначчи > 1000 = %d, ожидалось 1597", last)
	}
	assertStops(t, "Fibonacci", Fibonacci())
}

func TestFilterMap(t *testing.T) {
	even := Filter(Fibonacci(), func(x int) bool { return x%2 == 0 })
	got := slices.Collect(Take(even, 5))
	if !slices.Equal(got, []int{0, 2, 8, 34, 144}) {
		t.Errorf("чётные Фибоначчи = %v", got)
	}
	sq := Map(slices.Values([]int{1, 2, 3}), func(x int) string { return string(rune('a' + x)) })
	if got := slices.Collect(sq); !slices.Equal(got, []string{"b", "c", "d"}) {
		t.Errorf("Map = %v", got)
	}
	assertStops(t, "Filter", Filter(Fibonacci(), func(int) bool { return true }))
	assertStops(t, "Map", Map(Fibonacci(), func(x int) int { return x }))

	// Ранний выход должен останавливать источник.
	var c counted
	for v := range Map(c.seq(100), func(x int) int { return x * 10 }) {
		if v == 30 {
			break
		}
	}
	if c.produced != 4 || !c.finished {
		t.Errorf("после break на 4-м элементе источник выдал %d, finished=%v", c.produced, c.finished)
	}
}

func TestTake(t *testing.T) {
	tests := []struct {
		src, n, wantLen, wantProduced int
	}{
		{10, 3, 3, 3}, // ровно 3, без запроса 4-го
		{2, 5, 2, 2},
		{5, 0, 0, 0},
		{5, -1, 0, 0},
		{0, 3, 0, 0},
	}
	for _, tt := range tests {
		var c counted
		got := slices.Collect(Take(c.seq(tt.src), tt.n))
		if len(got) != tt.wantLen {
			t.Errorf("Take(src=%d, %d) = %v", tt.src, tt.n, got)
		}
		if c.produced != tt.wantProduced {
			t.Errorf("Take(src=%d, %d): источник выдал %d элементов, ожидалось %d", tt.src, tt.n, c.produced, tt.wantProduced)
		}
	}
	assertStops(t, "Take", Take(Fibonacci(), 5))
}

func TestEnumerate(t *testing.T) {
	var idx []int
	var vals []string
	for i, v := range Enumerate(slices.Values([]string{"a", "b", "c"})) {
		idx = append(idx, i)
		vals = append(vals, v)
		if i == 1 {
			break
		}
	}
	if !slices.Equal(idx, []int{0, 1}) || !slices.Equal(vals, []string{"a", "b"}) {
		t.Errorf("Enumerate с break: %v %v", idx, vals)
	}
	assertStops2(t, "Enumerate", Enumerate(Fibonacci()))
}

func TestZip(t *testing.T) {
	var ca, cb counted
	var got [][2]any
	for a, b := range Zip(ca.seq(3), Map(cb.seq(10), func(x int) string { return string(rune('x' + x%3)) })) {
		got = append(got, [2]any{a, b})
	}
	want := [][2]any{{0, "x"}, {1, "y"}, {2, "z"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Zip = %v, ожидалось %v", got, want)
	}
	if !ca.finished || !cb.finished {
		t.Errorf("Zip не остановил источники (stop не вызван): a=%v b=%v", ca.finished, cb.finished)
	}

	// Ранний выход потребителя.
	ca, cb = counted{}, counted{}
	for a := range Zip(ca.seq(100), cb.seq(100)) {
		if a == 2 {
			break
		}
	}
	if !ca.finished || !cb.finished {
		t.Errorf("после break источники Zip не завершены: a=%v b=%v", ca.finished, cb.finished)
	}

	// Бесконечная + конечная.
	n := 0
	for range Zip(Fibonacci(), slices.Values([]int{1, 2})) {
		n++
	}
	if n != 2 {
		t.Errorf("Zip(бесконечная, 2 элемента) дал %d пар", n)
	}
	assertStops2(t, "Zip", Zip(Fibonacci(), Fibonacci()))
}

func TestChunk(t *testing.T) {
	got := slices.Collect(Chunk(slices.Values([]int{1, 2, 3, 4, 5, 6, 7}), 3))
	want := [][]int{{1, 2, 3}, {4, 5, 6}, {7}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Chunk(1..7, 3) = %v, ожидалось %v (чанки не должны разделять память!)", got, want)
	}
	if got := slices.Collect(Chunk(slices.Values([]int{}), 2)); len(got) != 0 {
		t.Errorf("Chunk(пусто) = %v", got)
	}
	got = slices.Collect(Take(Chunk(Fibonacci(), 2), 2))
	if !reflect.DeepEqual(got, [][]int{{0, 1}, {1, 2}}) {
		t.Errorf("Take(Chunk(Fibonacci, 2), 2) = %v", got)
	}
	assertStops(t, "Chunk", Chunk(Fibonacci(), 4))

	defer func() {
		if recover() == nil {
			t.Errorf("Chunk(s, 0) должен паниковать")
		}
	}()
	for range Chunk(Fibonacci(), 0) {
		break
	}
}

func BenchmarkPipeline(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sum := 0
		for v := range Take(Filter(Map(Fibonacci(), func(x int) int { return x % 1000 }), func(x int) bool { return x%2 == 0 }), 100) {
			sum += v
		}
		_ = sum
	}
}
