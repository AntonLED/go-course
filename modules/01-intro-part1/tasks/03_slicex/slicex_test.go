package slicex

import (
	"slices"
	"testing"
)

func TestUnique(t *testing.T) {
	tests := []struct {
		in, want []int
	}{
		{[]int{1, 2, 1, 3, 2, 4}, []int{1, 2, 3, 4}},
		{[]int{5, 5, 5}, []int{5}},
		{[]int{3, 1, 2}, []int{3, 1, 2}},
		{[]int{0, 0, -1, 0}, []int{0, -1}},
	}
	for _, tt := range tests {
		orig := slices.Clone(tt.in)
		got := Unique(tt.in)
		if !slices.Equal(got, tt.want) {
			t.Errorf("Unique(%v) = %v, ожидалось %v", orig, got, tt.want)
		}
		if !slices.Equal(tt.in, orig) {
			t.Errorf("Unique испортил вход: было %v, стало %v", orig, tt.in)
		}
	}
	if got := Unique(nil); got != nil {
		t.Errorf("Unique(nil) = %#v, ожидался nil", got)
	}
	if got := Unique([]int{}); got == nil || len(got) != 0 {
		t.Errorf("Unique([]int{}) = %#v, ожидался пустой не-nil срез", got)
	}
}

func TestUniqueNoAliasing(t *testing.T) {
	in := []int{1, 2, 3}
	got := Unique(in)
	got[0] = 100
	if in[0] != 1 {
		t.Errorf("результат Unique разделяет память со входом: in = %v", in)
	}
}

func TestRotateLeft(t *testing.T) {
	tests := []struct {
		in   []int
		k    int
		want []int
	}{
		{[]int{1, 2, 3, 4, 5}, 2, []int{3, 4, 5, 1, 2}},
		{[]int{1, 2, 3, 4, 5}, 0, []int{1, 2, 3, 4, 5}},
		{[]int{1, 2, 3, 4, 5}, 5, []int{1, 2, 3, 4, 5}},
		{[]int{1, 2, 3, 4, 5}, 7, []int{3, 4, 5, 1, 2}},
		{[]int{1, 2, 3, 4, 5}, -1, []int{5, 1, 2, 3, 4}},
		{[]int{1, 2, 3, 4, 5}, -12, []int{4, 5, 1, 2, 3}},
		{[]int{1}, 100, []int{1}},
		{[]int{}, 3, []int{}},
		{nil, -3, nil},
	}
	for _, tt := range tests {
		s := slices.Clone(tt.in)
		RotateLeft(s, tt.k)
		if !slices.Equal(s, tt.want) {
			t.Errorf("RotateLeft(%v, %d) → %v, ожидалось %v", tt.in, tt.k, s, tt.want)
		}
	}
}

func TestRotateLeftInPlace(t *testing.T) {
	backing := []int{1, 2, 3, 4, 5, 6}
	s := backing[1:4] // [2 3 4]
	RotateLeft(s, 1)
	if want := []int{1, 3, 4, 2, 5, 6}; !slices.Equal(backing, want) {
		t.Errorf("RotateLeft должен работать на месте и не выходить за len: backing = %v, ожидалось %v", backing, want)
	}
	big := make([]int, 1000)
	allocs := testing.AllocsPerRun(10, func() { RotateLeft(big, 333) })
	if allocs > 0 {
		t.Errorf("RotateLeft выделяет память: %.0f аллокаций, ожидалось 0", allocs)
	}
}

func TestChunk(t *testing.T) {
	s := []int{1, 2, 3, 4, 5, 6, 7}
	got := Chunk(s, 3)
	want := [][]int{{1, 2, 3}, {4, 5, 6}, {7}}
	if !slices.EqualFunc(got, want, slices.Equal[[]int]) {
		t.Fatalf("Chunk(%v, 3) = %v, ожидалось %v", s, got, want)
	}
	if &got[0][0] != &s[0] {
		t.Errorf("куски должны ссылаться на память s (без копирования)")
	}
	// append к первому куску не должен затереть s[3] и второй кусок.
	got[0] = append(got[0], 100)
	if s[3] != 4 || got[1][0] != 4 {
		t.Errorf("append к куску испортил исходный срез: s = %v, chunk[1] = %v", s, got[1])
	}

	if got := Chunk([]int{1, 2}, 5); !slices.EqualFunc(got, [][]int{{1, 2}}, slices.Equal[[]int]) {
		t.Errorf("Chunk([1 2], 5) = %v, ожидалось [[1 2]]", got)
	}
	if got := Chunk(nil, 2); len(got) != 0 {
		t.Errorf("Chunk(nil, 2) = %v, ожидался пустой результат", got)
	}
	for _, size := range []int{0, -1} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Chunk(s, %d) должен паниковать", size)
				}
			}()
			Chunk([]int{1}, size)
		}()
	}
}

func TestInsert(t *testing.T) {
	tests := []struct {
		s    []int
		i    int
		vs   []int
		want []int
	}{
		{[]int{1, 2, 3}, 0, []int{9}, []int{9, 1, 2, 3}},
		{[]int{1, 2, 3}, 3, []int{9, 8}, []int{1, 2, 3, 9, 8}},
		{[]int{1, 2, 3}, 1, []int{7, 8}, []int{1, 7, 8, 2, 3}},
		{[]int{1, 2, 3}, 1, nil, []int{1, 2, 3}},
		{nil, 0, []int{1}, []int{1}},
	}
	for _, tt := range tests {
		got := Insert(slices.Clone(tt.s), tt.i, tt.vs...)
		if !slices.Equal(got, tt.want) {
			t.Errorf("Insert(%v, %d, %v) = %v, ожидалось %v", tt.s, tt.i, tt.vs, got, tt.want)
		}
	}
}

func TestInsertWithCapacity(t *testing.T) {
	s := make([]int, 3, 10)
	copy(s, []int{1, 2, 3})
	got := Insert(s, 1, 7)
	if want := []int{1, 7, 2, 3}; !slices.Equal(got, want) {
		t.Fatalf("Insert с запасом cap = %v, ожидалось %v", got, want)
	}
	if &got[0] != &s[0] {
		t.Errorf("при достаточной capacity Insert должен переиспользовать backing array (как append)")
	}
}

func TestInsertAliasing(t *testing.T) {
	// vs — часть самого s, и места хватает: наивный сдвиг затрёт vs.
	s := make([]int, 4, 10)
	copy(s, []int{1, 2, 3, 4})
	got := Insert(s, 1, s[2:]...)
	if want := []int{1, 3, 4, 2, 3, 4}; !slices.Equal(got, want) {
		t.Errorf("Insert(s, 1, s[2:]...) = %v, ожидалось %v", got, want)
	}

	s = make([]int, 3, 10)
	copy(s, []int{1, 2, 3})
	got = Insert(s, 0, s...)
	if want := []int{1, 2, 3, 1, 2, 3}; !slices.Equal(got, want) {
		t.Errorf("Insert(s, 0, s...) = %v, ожидалось %v", got, want)
	}

	// Без запаса capacity.
	s = []int{1, 2, 3}
	got = Insert(s, 2, s[:2]...)
	if want := []int{1, 2, 1, 2, 3}; !slices.Equal(got, want) {
		t.Errorf("Insert([1 2 3], 2, s[:2]...) = %v, ожидалось %v", got, want)
	}
}

func TestInsertPanics(t *testing.T) {
	for _, i := range []int{-1, 4} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Insert(s, %d, ...) при len=3 должен паниковать", i)
				}
			}()
			Insert([]int{1, 2, 3}, i, 0)
		}()
	}
}

func TestFilterInPlace(t *testing.T) {
	s := []int{1, 2, 3, 4, 5, 6}
	even := func(x int) bool { return x%2 == 0 }
	got := FilterInPlace(s, even)
	if want := []int{2, 4, 6}; !slices.Equal(got, want) {
		t.Fatalf("FilterInPlace = %v, ожидалось %v", got, want)
	}
	if &got[0] != &s[0] {
		t.Errorf("FilterInPlace должен переиспользовать backing array")
	}
	if want := []int{2, 4, 6, 0, 0, 0}; !slices.Equal(s, want) {
		t.Errorf("хвост не обнулён: исходный срез = %v, ожидалось %v", s, want)
	}
	if got := FilterInPlace(nil, even); len(got) != 0 {
		t.Errorf("FilterInPlace(nil) = %v, ожидался пустой", got)
	}
	big := make([]int, 1000)
	allocs := testing.AllocsPerRun(10, func() { FilterInPlace(big, even) })
	if allocs > 0 {
		t.Errorf("FilterInPlace выделяет память: %.0f аллокаций", allocs)
	}
}

func BenchmarkUnique(b *testing.B) {
	s := make([]int, 10000)
	for i := range s {
		s[i] = i % 100
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Unique(s)
	}
}
