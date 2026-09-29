package unsafex

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"unsafe"
)

func TestBytesToString(t *testing.T) {
	b := []byte("привет, мир")
	s := BytesToString(b)
	if s != "привет, мир" {
		t.Fatalf("BytesToString = %q", s)
	}
	if unsafe.StringData(s) != unsafe.SliceData(b) {
		t.Error("BytesToString скопировал данные, ожидался zero-copy")
	}
	if BytesToString(nil) != "" || BytesToString([]byte{}) != "" {
		t.Error("пустой вход должен давать пустую строку")
	}
	// Демонстрация «почему это опасно»: изменение b меняет строку.
	b[0] = 'P'
	if s[0] != 'P' {
		t.Error("строка должна разделять память со срезом")
	}
}

func TestStringToBytes(t *testing.T) {
	s := "hello, " + string([]byte{'g', 'o'}) // не литерал: в куче
	b := StringToBytes(s)
	if string(b) != "hello, go" || len(b) != len(s) || cap(b) != len(s) {
		t.Fatalf("StringToBytes = %q len=%d cap=%d", b, len(b), cap(b))
	}
	if unsafe.SliceData(b) != unsafe.StringData(s) {
		t.Error("StringToBytes скопировал данные, ожидался zero-copy")
	}
	if StringToBytes("") != nil {
		t.Error(`StringToBytes("") должен вернуть nil`)
	}
	if a := testing.AllocsPerRun(100, func() { _ = BytesToString(StringToBytes(s)) }); a != 0 {
		t.Errorf("конверсии аллоцируют: %.1f", a)
	}
}

func TestSliceLenCap(t *testing.T) {
	s := make([]int64, 3, 10)
	if l, c := SliceLenCap(s); l != 3 || c != 10 {
		t.Errorf("SliceLenCap = (%d,%d), ожидалось (3,10)", l, c)
	}
	sub := s[2:5:7]
	if l, c := SliceLenCap(sub); l != 3 || c != 5 {
		t.Errorf("SliceLenCap(s[2:5:7]) = (%d,%d), ожидалось (3,5)", l, c)
	}
	if l, c := SliceLenCap[string](nil); l != 0 || c != 0 {
		t.Errorf("SliceLenCap(nil) = (%d,%d)", l, c)
	}
}

func TestFloat64Bits(t *testing.T) {
	for _, f := range []float64{0, -0.0, 1, -1.5, math.Pi, math.Inf(1), math.MaxFloat64, math.SmallestNonzeroFloat64} {
		if got, want := Float64Bits(f), math.Float64bits(f); got != want {
			t.Errorf("Float64Bits(%v) = %#x, ожидалось %#x", f, got, want)
		}
	}
	if got := Float64Bits(math.Copysign(0, -1)); got != 1<<63 {
		t.Errorf("Float64Bits(-0) = %#x, ожидалось 0x8000000000000000", got)
	}
}

type record struct {
	flag  bool
	id    int64
	name  string
	score float32
}

func TestReadField(t *testing.T) {
	r := &record{flag: true, id: 42, name: "go", score: 2.5}
	base := unsafe.Pointer(r)
	if got := ReadField[bool](base, unsafe.Offsetof(r.flag)); !got {
		t.Error("flag прочитан неверно")
	}
	if got := ReadField[int64](base, unsafe.Offsetof(r.id)); got != 42 {
		t.Errorf("id = %d, ожидалось 42", got)
	}
	if got := ReadField[string](base, unsafe.Offsetof(r.name)); got != "go" {
		t.Errorf("name = %q, ожидалось go", got)
	}
	if got := ReadField[float32](base, unsafe.Offsetof(r.score)); got != 2.5 {
		t.Errorf("score = %v, ожидалось 2.5", got)
	}
	arr := [4]uint16{10, 20, 30, 40}
	if got := ReadField[uint16](unsafe.Pointer(&arr), 3*unsafe.Sizeof(arr[0])); got != 40 {
		t.Errorf("arr[3] = %d, ожидалось 40", got)
	}
}

func TestOverlap(t *testing.T) {
	buf := make([]byte, 100)
	other := make([]byte, 100)
	tests := []struct {
		name string
		a, b []byte
		want bool
	}{
		{"один и тот же", buf, buf, true},
		{"вложенный", buf, buf[10:20], true},
		{"пересекаются", buf[0:50], buf[49:60], true},
		{"соседние", buf[0:50], buf[50:60], false},
		{"cap не учитывается", buf[0:10], buf[10:20], false},
		{"разные массивы", buf, other, false},
		{"пустой", buf[5:5], buf, false},
		{"nil", nil, buf, false},
	}
	for _, tc := range tests {
		if got := Overlap(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: Overlap = %v, ожидалось %v", tc.name, got, tc.want)
		}
		if got := Overlap(tc.b, tc.a); got != tc.want {
			t.Errorf("%s (симметрия): Overlap = %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
}

func TestReinterpret(t *testing.T) {
	u32 := []uint32{0x04030201, 0x08070605}
	b, err := Reinterpret[byte](u32)
	if err != nil {
		t.Fatalf("Reinterpret[byte]: %v", err)
	}
	if len(b) != 8 || cap(b) != 8 {
		t.Fatalf("len=%d cap=%d, ожидалось 8/8", len(b), cap(b))
	}
	if binary.NativeEndian.Uint32(b) != 0x04030201 {
		t.Errorf("байты не совпадают с памятью исходного среза: %v", b)
	}
	b[0] = 0xff
	if u32[0]&0xff != 0xff {
		t.Error("результат должен разделять память с исходным срезом")
	}

	u64, err := Reinterpret[uint64](u32)
	if err != nil || len(u64) != 1 {
		t.Fatalf("Reinterpret[uint64]([2]uint32) = %v, %v", u64, err)
	}

	if _, err := Reinterpret[uint64](u32[:1]); !errors.Is(err, ErrSize) {
		t.Errorf("4 байта в uint64: err = %v, ожидалось ErrSize", err)
	}
	raw := make([]byte, 64)
	start := 0
	for uintptr(unsafe.Pointer(&raw[start]))%8 == 0 {
		start++ // ищем заведомо невыровненный адрес
	}
	if _, err := Reinterpret[uint64](raw[start : start+8]); !errors.Is(err, ErrAlign) {
		t.Errorf("невыровненный адрес: err = %v, ожидалось ErrAlign", err)
	}
	if _, err := Reinterpret[*int]([]uint64{1}); !errors.Is(err, ErrNotPOD) {
		t.Errorf("[]uint64 → []*int: err = %v, ожидалось ErrNotPOD", err)
	}
	if _, err := Reinterpret[byte]([]string{"x"}); !errors.Is(err, ErrNotPOD) {
		t.Errorf("[]string → []byte: err = %v, ожидалось ErrNotPOD", err)
	}
	if got, err := Reinterpret[float64]([]uint64{}); got != nil || err != nil {
		t.Errorf("пустой вход: %v, %v", got, err)
	}
	f, _ := Reinterpret[float64]([]uint64{math.Float64bits(1.25)})
	if f[0] != 1.25 {
		t.Errorf("uint64→float64 = %v, ожидалось 1.25", f[0])
	}
}

func BenchmarkBytesToString(b *testing.B) {
	data := make([]byte, 1024)
	for i := 0; i < b.N; i++ {
		_ = BytesToString(data)
	}
}

func BenchmarkStringConversionCopy(b *testing.B) {
	data := make([]byte, 1024)
	var s string
	for i := 0; i < b.N; i++ {
		s = string(data)
	}
	_ = s
}
