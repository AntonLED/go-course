package tree

import (
	"iter"
	"math"
	"math/rand"
	"slices"
	"testing"
)

func build(keys ...int) *Tree[int, string] {
	var t Tree[int, string]
	for _, k := range keys {
		t.Put(k, string(rune('a'+k%26)))
	}
	return &t
}

func TestPutGet(t *testing.T) {
	var tr Tree[string, int]
	if _, ok := tr.Get("x"); ok || tr.Len() != 0 {
		t.Fatalf("пустое дерево: Get/Len")
	}
	tr.Put("b", 1)
	tr.Put("a", 2)
	tr.Put("c", 3)
	tr.Put("b", 10) // замена
	if tr.Len() != 3 {
		t.Errorf("Len = %d, ожидалось 3 (замена не увеличивает размер)", tr.Len())
	}
	if v, ok := tr.Get("b"); !ok || v != 10 {
		t.Errorf("Get(b) = %d, %v; ожидалось 10, true", v, ok)
	}
	if _, ok := tr.Get("z"); ok {
		t.Errorf("Get(z) нашёл несуществующий ключ")
	}
}

func TestAllBackward(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	keys := r.Perm(200)
	tr := build(keys...)
	got := slices.Collect(tr.Keys())
	want := slices.Sorted(slices.Values(keys))
	if !slices.Equal(got, want) {
		t.Fatalf("Keys не по возрастанию: %v", got[:10])
	}
	var back []int
	for k, v := range tr.Backward() {
		if v != string(rune('a'+k%26)) {
			t.Fatalf("Backward: значение %q для ключа %d", v, k)
		}
		back = append(back, k)
	}
	slices.Reverse(want)
	if !slices.Equal(back, want) {
		t.Errorf("Backward не по убыванию")
	}
	var empty Tree[int, int]
	for range empty.All() {
		t.Fatalf("обход пустого дерева не должен вызывать yield")
	}
}

// assertStops2: после false yield не вызывается повторно ни на какой глубине.
func assertStops2[K, V any](t *testing.T, name string, s iter.Seq2[K, V], stopAfter int) {
	t.Helper()
	calls := 0
	s(func(K, V) bool {
		calls++
		return calls < stopAfter
	})
	if calls != stopAfter {
		t.Errorf("%s: остановка после %d-го: yield вызван %d раз", name, stopAfter, calls)
	}
}

func TestEarlyExit(t *testing.T) {
	tr := build(50, 25, 75, 10, 30, 60, 90, 5, 15, 27, 35)
	for stop := 1; stop <= tr.Len(); stop++ {
		assertStops2(t, "All", tr.All(), stop)
		assertStops2(t, "Backward", tr.Backward(), stop)
	}
	assertStops2(t, "Range", tr.Range(0, 100), 3)

	// break в range: рантайм паникует, если итератор продолжит.
	var got []int
	for k := range tr.All() {
		got = append(got, k)
		if k == 27 {
			break
		}
	}
	if !slices.Equal(got, []int{5, 10, 15, 25, 27}) {
		t.Errorf("break на 27: %v", got)
	}
	n := 0
	for range tr.Keys() {
		n++
		if n == 2 {
			break
		}
	}

	// Вырожденное дерево (отсортированная вставка) большой глубины.
	var deep Tree[int, int]
	for i := range 3000 {
		deep.Put(i, i)
	}
	for k := range deep.All() {
		if k == 2990 {
			break
		}
	}
}

func TestRange(t *testing.T) {
	tr := build(50, 25, 75, 10, 30, 60, 90, 5, 15, 27, 35)
	tests := []struct {
		lo, hi int
		want   []int
	}{
		{0, 100, []int{5, 10, 15, 25, 27, 30, 35, 50, 60, 75, 90}},
		{15, 50, []int{15, 25, 27, 30, 35}}, // hi не включается
		{26, 29, []int{27}},
		{51, 59, nil},
		{50, 50, nil},
		{90, 1000, []int{90}},
		{60, 10, nil}, // lo > hi
	}
	for _, tt := range tests {
		var got []int
		for k := range tr.Range(tt.lo, tt.hi) {
			got = append(got, k)
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("Range(%d, %d) = %v, ожидалось %v", tt.lo, tt.hi, got, tt.want)
		}
	}
}

type tracked struct {
	vals     []int
	finished bool
}

func (tr *tracked) seq() iter.Seq[int] {
	return func(yield func(int) bool) {
		defer func() { tr.finished = true }()
		for _, v := range tr.vals {
			if !yield(v) {
				return
			}
		}
	}
}

func TestMergeSorted(t *testing.T) {
	tests := []struct{ a, b, want []int }{
		{[]int{1, 4, 7}, []int{2, 3, 8, 9}, []int{1, 2, 3, 4, 7, 8, 9}},
		{nil, []int{1, 2}, []int{1, 2}},
		{[]int{1, 2}, nil, []int{1, 2}},
		{nil, nil, nil},
		{[]int{1, 1, 3}, []int{1, 3}, []int{1, 1, 1, 3, 3}},
	}
	for _, tt := range tests {
		got := slices.Collect(MergeSorted(slices.Values(tt.a), slices.Values(tt.b)))
		if !slices.Equal(got, tt.want) {
			t.Errorf("MergeSorted(%v, %v) = %v, ожидалось %v", tt.a, tt.b, got, tt.want)
		}
	}

	// Слияние ключей двух деревьев.
	t1, t2 := build(10, 5, 20), build(7, 15, 1)
	got := slices.Collect(MergeSorted(t1.Keys(), t2.Keys()))
	if !slices.Equal(got, []int{1, 5, 7, 10, 15, 20}) {
		t.Errorf("MergeSorted(деревья) = %v", got)
	}

	// Ранний выход должен остановить оба источника.
	a := &tracked{vals: []int{1, 3, 5, 7}}
	b := &tracked{vals: []int{2, 4, 6, 8}}
	for v := range MergeSorted(a.seq(), b.seq()) {
		if v == 3 {
			break
		}
	}
	if !a.finished || !b.finished {
		t.Errorf("после break источники не остановлены: a=%v b=%v", a.finished, b.finished)
	}
	calls := 0
	MergeSorted(slices.Values([]int{1, 2}), slices.Values([]int{3}))(func(int) bool { calls++; return false })
	if calls != 1 {
		t.Errorf("MergeSorted: yield вызван %d раз после false", calls)
	}

	fa := []float64{1, 2}
	fb := []float64{1, 2}
	if got := slices.Collect(MergeSorted(slices.Values(fa), slices.Values(fb))); !slices.Equal(got, []float64{1, 1, 2, 2}) {
		t.Errorf("MergeSorted(float) = %v", got)
	}

	// При равенстве первым идёт элемент из a. -0.0 == +0.0 для cmp.Compare,
	// но их можно различить по знаку.
	negZero := math.Copysign(0, -1)
	got2 := slices.Collect(MergeSorted(slices.Values([]float64{negZero}), slices.Values([]float64{0})))
	if len(got2) != 2 || !math.Signbit(got2[0]) || math.Signbit(got2[1]) {
		t.Errorf("MergeSorted([-0], [+0]): при равенстве первым должен идти элемент из a")
	}
	got2 = slices.Collect(MergeSorted(slices.Values([]float64{0}), slices.Values([]float64{negZero})))
	if len(got2) != 2 || math.Signbit(got2[0]) || !math.Signbit(got2[1]) {
		t.Errorf("MergeSorted([+0], [-0]): при равенстве первым должен идти элемент из a")
	}
}

func BenchmarkAll(b *testing.B) {
	tr := build(rand.New(rand.NewSource(1)).Perm(10000)...)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for range tr.All() {
		}
	}
}
