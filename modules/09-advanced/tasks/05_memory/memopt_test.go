package memopt

import (
	"bytes"
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unsafe"
)

func fieldSet(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		out = append(out, f.Name+" "+f.Type.String())
	}
	sort.Strings(out)
	return out
}

func TestEventLayout(t *testing.T) {
	orig, ev := reflect.TypeFor[EventOriginal](), reflect.TypeFor[Event]()
	if !reflect.DeepEqual(fieldSet(orig), fieldSet(ev)) {
		t.Fatalf("набор полей Event должен совпадать с EventOriginal:\n %v\n %v", fieldSet(ev), fieldSet(orig))
	}
	// Минимально возможный размер: сумма размеров полей, округлённая
	// вверх до максимального выравнивания.
	var sum, maxAlign uintptr
	for i := 0; i < orig.NumField(); i++ {
		sum += orig.Field(i).Type.Size()
		maxAlign = max(maxAlign, uintptr(orig.Field(i).Type.Align()))
	}
	minSize := (sum + maxAlign - 1) / maxAlign * maxAlign
	if got := unsafe.Sizeof(Event{}); got != minSize {
		t.Errorf("unsafe.Sizeof(Event{}) = %d, ожидалось %d (исходный: %d)", got, minSize, unsafe.Sizeof(EventOriginal{}))
	}
}

func TestAppendRecord(t *testing.T) {
	tests := []struct {
		r    Record
		want string
	}{
		{Record{ID: 42, Name: "Bob", Score: 3.5, OK: true}, "id=42 name=\"Bob\" score=3.50 ok=true\n"},
		{Record{ID: -7, Name: `a"b\c`, Score: 0.125, OK: false}, "id=-7 name=\"a\\\"b\\\\c\" score=0.12 ok=false\n"},
		{Record{ID: math.MaxInt64, Name: "Привет\n", Score: -1}, "id=9223372036854775807 name=\"Привет\\n\" score=-1.00 ok=false\n"},
	}
	for _, tc := range tests {
		got := string(AppendRecord([]byte("> "), tc.r))
		if got != "> "+tc.want {
			t.Errorf("AppendRecord(%+v) = %q, ожидалось %q", tc.r, got, "> "+tc.want)
		}
	}
}

func TestAppendRecordNoAllocs(t *testing.T) {
	buf := make([]byte, 0, 256)
	r := Record{ID: 123456789, Name: "Alice", Score: 98.765, OK: true}
	allocs := testing.AllocsPerRun(100, func() {
		buf = AppendRecord(buf[:0], r)
	})
	if allocs != 0 {
		t.Errorf("AppendRecord: %.1f аллокаций на вызов, ожидалось 0", allocs)
	}
}

func TestJoinInts(t *testing.T) {
	tests := []struct {
		ids  []int
		sep  string
		want string
	}{
		{nil, ",", ""},
		{[]int{0}, ",", "0"},
		{[]int{1, -20, 300}, ", ", "1, -20, 300"},
		{[]int{math.MinInt64, math.MaxInt64}, "|", "-9223372036854775808|9223372036854775807"},
		{[]int{9, 10, 99, 100, -9, -10}, "", "91099100-9-10"},
	}
	for _, tc := range tests {
		if got := JoinInts(tc.ids, tc.sep); got != tc.want {
			t.Errorf("JoinInts(%v, %q) = %q, ожидалось %q", tc.ids, tc.sep, got, tc.want)
		}
	}
	ids := make([]int, 1000)
	for i := range ids {
		ids[i] = i * 7919
	}
	if a := testing.AllocsPerRun(50, func() { _ = JoinInts(ids, ",") }); a > 1 {
		t.Errorf("JoinInts: %.1f аллокаций, ожидалось не больше 1", a)
	}
}

func TestBufferPool(t *testing.T) {
	p := NewBufferPool(1024)
	b := p.Get()
	if b == nil || b.Len() != 0 {
		t.Fatal("Get должен возвращать пустой ненулевой буфер")
	}
	b.WriteString("грязные данные")
	p.Put(b)
	p.Put(nil) // не паникует
	for i := 0; i < 10; i++ {
		if b := p.Get(); b.Len() != 0 {
			t.Fatalf("Get вернул непустой буфер: %q", b.String())
		}
	}

	big := bytes.NewBuffer(make([]byte, 0, 1<<20))
	for i := 0; i < 10; i++ {
		p.Put(big)
		if got := p.Get(); got == big || got.Cap() > 1024 {
			t.Fatal("буфер с Cap() > maxCap не должен возвращаться в пул")
		}
	}
}

func TestBufferPoolConcurrent(t *testing.T) {
	p := NewBufferPool(4096)
	done := make(chan struct{})
	for g := 0; g < 8; g++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for i := 0; i < 200; i++ {
				b := p.Get()
				if b.Len() != 0 {
					t.Error("непустой буфер из пула")
					return
				}
				b.WriteString(strings.Repeat("x", i))
				p.Put(b)
			}
		}()
	}
	for g := 0; g < 8; g++ {
		<-done
	}
}

func TestCloneHead(t *testing.T) {
	big := bytes.Repeat([]byte("abcdef"), 1000)
	head := CloneHead(big, 4)
	if string(head) != "abcd" {
		t.Fatalf("CloneHead = %q, ожидалось abcd", head)
	}
	if cap(head) != len(head) {
		t.Errorf("cap = %d, ожидалось %d — результат удерживает лишнюю память", cap(head), len(head))
	}
	big[0] = 'Z'
	if head[0] != 'a' {
		t.Error("результат разделяет память с исходным срезом")
	}
	if got := CloneHead([]byte("ab"), 10); string(got) != "ab" {
		t.Errorf("CloneHead(n > len) = %q", got)
	}
	if got := CloneHead(nil, 3); len(got) != 0 {
		t.Errorf("CloneHead(nil) = %q", got)
	}
}

func TestRemoveAt(t *testing.T) {
	a, b, c := new(int), new(int), new(int)
	s := []*int{a, b, c}
	s = RemoveAt(s, 0)
	if len(s) != 2 || s[0] != b || s[1] != c {
		t.Fatalf("RemoveAt: порядок нарушен: %v", s)
	}
	if tail := s[:3][2]; tail != nil {
		t.Error("освободившийся слот не обнулён — объект не будет собран GC")
	}
	ints := RemoveAt([]int{1, 2, 3, 4}, 3)
	if !reflect.DeepEqual(ints, []int{1, 2, 3}) {
		t.Errorf("RemoveAt(последний) = %v", ints)
	}
}

func BenchmarkAppendRecord(b *testing.B) {
	buf := make([]byte, 0, 256)
	r := Record{ID: 1, Name: "bench", Score: 1.5}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = AppendRecord(buf[:0], r)
	}
}
