package gen

import (
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestMapFilterReduce(t *testing.T) {
	got := Map([]int{1, 2, 3}, strconv.Itoa) // инференс T=int, U=string
	if !slices.Equal(got, []string{"1", "2", "3"}) {
		t.Errorf("Map(Itoa) = %q", got)
	}
	if got := Map([]int(nil), func(int) int { return 0 }); len(got) != 0 {
		t.Errorf("Map(nil) = %v, ожидалось пусто", got)
	}

	src := []int{1, 2, 3, 4, 5, 6}
	orig := slices.Clone(src)
	even := Filter(src, func(x int) bool { return x%2 == 0 })
	if !slices.Equal(even, []int{2, 4, 6}) {
		t.Errorf("Filter(even) = %v", even)
	}
	if !slices.Equal(src, orig) {
		t.Errorf("Filter испортил исходный срез: %v, было %v", src, orig)
	}
	even = append(even, 100)
	if !slices.Equal(src, orig) {
		t.Errorf("результат Filter разделяет массив с исходным: %v", src)
	}

	words := []string{"go", "is", "fun"}
	total := Reduce(words, 0, func(acc int, s string) int { return acc + len(s) })
	if total != 7 {
		t.Errorf("Reduce(len) = %d, ожидалось 7", total)
	}
	joined := Reduce(words, "", func(acc, s string) string { return acc + "/" + s })
	if joined != "/go/is/fun" {
		t.Errorf("Reduce(concat) = %q — порядок должен быть слева направо", joined)
	}
}

type Celsius float64
type ID uint8

func TestSum(t *testing.T) {
	if got := Sum([]int{1, 2, 3}); got != 6 {
		t.Errorf("Sum(int) = %v", got)
	}
	if got := Sum([]Celsius{20.5, 1.5}); got != 22 {
		t.Errorf("Sum(Celsius) = %v — нужен ~float64 в ограничении", got)
	}
	if got := Sum([]ID{200, 100}); got != 44 { // переполнение uint8: 300 % 256
		t.Errorf("Sum([]ID{200,100}) = %v, ожидалось 44 (переполнение uint8)", got)
	}
	if got := Sum[float64](nil); got != 0 {
		t.Errorf("Sum(nil) = %v", got)
	}
}

func TestMinMax(t *testing.T) {
	lo, hi, ok := MinMax([]int{3, -1, 7, 7, 0})
	if lo != -1 || hi != 7 || !ok {
		t.Errorf("MinMax(int) = %v, %v, %v", lo, hi, ok)
	}
	s1, s2, ok := MinMax([]string{"pear", "apple", "Zebra"})
	if s1 != "Zebra" || s2 != "pear" || !ok {
		t.Errorf("MinMax(string) = %q, %q, %v (сравнение по байтам: 'Z' < 'a')", s1, s2, ok)
	}
	if _, _, ok := MinMax([]float64{}); ok {
		t.Errorf("MinMax(пусто): ok = true")
	}
	nan := math.NaN()
	f1, f2, _ := MinMax([]float64{3, nan, 1})
	if !math.IsNaN(f1) || f2 != 3 {
		t.Errorf("MinMax({3, NaN, 1}) = %v, %v; ожидалось NaN, 3 (как cmp.Compare)", f1, f2)
	}
	f1, f2, _ = MinMax([]float64{nan, 5, 2})
	if !math.IsNaN(f1) || f2 != 5 {
		t.Errorf("MinMax({NaN, 5, 2}) = %v, %v; ожидалось NaN, 5", f1, f2)
	}
}

func TestGroupByUniq(t *testing.T) {
	words := []string{"go", "rust", "c", "java", "zig", "d"}
	got := GroupBy(words, func(s string) int { return len(s) })
	want := map[int][]string{1: {"c", "d"}, 2: {"go"}, 3: {"zig"}, 4: {"rust", "java"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GroupBy(len) = %v, ожидалось %v", got, want)
	}
	if u := Uniq([]string{"b", "a", "b", "c", "a"}); !slices.Equal(u, []string{"b", "a", "c"}) {
		t.Errorf("Uniq = %v", u)
	}
	type point struct{ X, Y int }
	if u := Uniq([]point{{1, 2}, {1, 2}, {2, 1}}); len(u) != 2 {
		t.Errorf("Uniq(struct) = %v", u)
	}
}

func TestSet(t *testing.T) {
	var zero Set[string] // нулевое значение пригодно
	if zero.Has("x") || zero.Len() != 0 {
		t.Errorf("пустое множество: Has/Len")
	}
	zero.Remove("x")
	zero.Add("x", "y", "x")
	if zero.Len() != 2 || !zero.Has("x") {
		t.Errorf("после Add: Len = %d", zero.Len())
	}

	a := NewSet(1, 2, 3, 4)
	b := NewSet(3, 4, 5)
	if got := Sorted(a.Union(b)); !slices.Equal(got, []int{1, 2, 3, 4, 5}) {
		t.Errorf("Union = %v", got)
	}
	if got := Sorted(a.Intersect(b)); !slices.Equal(got, []int{3, 4}) {
		t.Errorf("Intersect = %v", got)
	}
	if got := Sorted(a.Difference(b)); !slices.Equal(got, []int{1, 2}) {
		t.Errorf("Difference = %v", got)
	}
	if got := Sorted(b.Difference(a)); !slices.Equal(got, []int{5}) {
		t.Errorf("b.Difference(a) = %v", got)
	}
	// Операции не меняют аргументы.
	if !slices.Equal(Sorted(a), []int{1, 2, 3, 4}) || !slices.Equal(Sorted(b), []int{3, 4, 5}) {
		t.Errorf("операции изменили исходные множества: a=%v b=%v", Sorted(a), Sorted(b))
	}
	// Результат не должен разделять map с исходным.
	u := a.Union(b)
	u.Add(100)
	if a.Has(100) {
		t.Errorf("Union вернул множество, разделяющее map с исходным")
	}
	var empty Set[int]
	if got := empty.Union(&empty); got.Len() != 0 {
		t.Errorf("Union пустых = %v", Sorted(got))
	}
	got := empty.Union(a)
	got.Add(-1)
	if a.Has(-1) {
		t.Errorf("empty.Union(a) разделяет map с a")
	}
	a.Remove(1)
	if a.Has(1) || a.Len() != 3 {
		t.Errorf("Remove не сработал")
	}
}

func TestStack(t *testing.T) {
	var s Stack[*strings.Builder]
	if _, ok := s.Pop(); ok {
		t.Errorf("Pop из пустого стека: ok = true")
	}
	if _, ok := s.Peek(); ok {
		t.Errorf("Peek пустого стека: ok = true")
	}
	b1, b2 := &strings.Builder{}, &strings.Builder{}
	s.Push(b1)
	s.Push(b2)
	if top, ok := s.Peek(); !ok || top != b2 || s.Len() != 2 {
		t.Errorf("Peek = %p, %v; Len = %d", top, ok, s.Len())
	}
	if x, _ := s.Pop(); x != b2 {
		t.Errorf("Pop вернул не последний элемент")
	}
	// Освобождённый слот должен быть обнулён, чтобы не удерживать b2 от GC
	// (проверяем, только если массив тот же и слот ещё в пределах cap).
	if n := len(s.items); cap(s.items) > n && s.items[:n+1][n] != nil {
		t.Errorf("Pop не обнулил освобождённый слот: стек удерживает ссылку от GC")
	}
	if x, _ := s.Pop(); x != b1 {
		t.Errorf("Pop вернул не тот элемент")
	}
	if s.Len() != 0 {
		t.Errorf("Len после Pop = %d", s.Len())
	}

	var ints Stack[int]
	for i := range 100 {
		ints.Push(i)
	}
	for i := 99; i >= 0; i-- {
		if x, ok := ints.Pop(); !ok || x != i {
			t.Fatalf("Pop = %d, %v; ожидалось %d", x, ok, i)
		}
	}
}

func BenchmarkSetIntersect(b *testing.B) {
	x, y := NewSet[int](), NewSet[int]()
	for i := range 10000 {
		x.Add(i)
		if i%10 == 0 {
			y.Add(i)
		}
	}
	for i := 0; i < b.N; i++ {
		x.Intersect(y)
	}
}
