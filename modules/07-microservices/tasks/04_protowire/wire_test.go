package protowire

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatalf("плохой hex в тесте: %v", err)
	}
	return b
}

var varintCases = []struct {
	v   uint64
	hex string
}{
	{0, "00"},
	{1, "01"},
	{127, "7f"},
	{128, "80 01"},
	{150, "96 01"},
	{300, "ac 02"},
	{16383, "ff 7f"},
	{16384, "80 80 01"},
	{math.MaxUint32, "ff ff ff ff 0f"},
	{1 << 63, "80 80 80 80 80 80 80 80 80 01"},
	{math.MaxUint64, "ff ff ff ff ff ff ff ff ff 01"},
}

func TestVarint(t *testing.T) {
	for _, c := range varintCases {
		want := unhex(t, c.hex)
		got := AppendVarint(nil, c.v)
		if !bytes.Equal(got, want) {
			t.Errorf("AppendVarint(%d) = % x, ожидалось % x", c.v, got, want)
		}
		if n := SizeVarint(c.v); n != len(want) {
			t.Errorf("SizeVarint(%d) = %d, ожидалось %d", c.v, n, len(want))
		}
		// с «хвостом» после varint
		v, n, err := ConsumeVarint(append(want, 0xAA, 0xBB))
		if err != nil || v != c.v || n != len(want) {
			t.Errorf("ConsumeVarint(% x) = %d, %d, %v; ожидалось %d, %d, nil", want, v, n, err, c.v, len(want))
		}
	}
	// AppendVarint дописывает, а не перезаписывает
	if got := AppendVarint([]byte{0xFF}, 1); !bytes.Equal(got, []byte{0xFF, 0x01}) {
		t.Errorf("AppendVarint должен дописывать в конец: % x", got)
	}
}

func TestConsumeVarintErrors(t *testing.T) {
	cases := []struct {
		name string
		hex  string
		err  error
	}{
		{"пусто", "", ErrTruncated},
		{"оборван", "96", ErrTruncated},
		{"оборван длинный", "ff ff ff", ErrTruncated},
		{"11 байт", "ff ff ff ff ff ff ff ff ff ff 01", ErrOverflow},
		{"10-й байт > 1", "ff ff ff ff ff ff ff ff ff 02", ErrOverflow},
	}
	for _, c := range cases {
		_, _, err := ConsumeVarint(unhex(t, c.hex))
		if !errors.Is(err, c.err) {
			t.Errorf("%s: ConsumeVarint(%s) ошибка %v, ожидалась %v", c.name, c.hex, err, c.err)
		}
	}
}

func TestZigZag(t *testing.T) {
	cases := []struct {
		v int64
		z uint64
	}{
		{0, 0}, {-1, 1}, {1, 2}, {-2, 3}, {2, 4},
		{math.MaxInt32, 4294967294}, {math.MinInt32, 4294967295},
		{math.MaxInt64, math.MaxUint64 - 1}, {math.MinInt64, math.MaxUint64},
	}
	for _, c := range cases {
		if got := EncodeZigZag(c.v); got != c.z {
			t.Errorf("EncodeZigZag(%d) = %d, ожидалось %d", c.v, got, c.z)
		}
		if got := DecodeZigZag(c.z); got != c.v {
			t.Errorf("DecodeZigZag(%d) = %d, ожидалось %d", c.z, got, c.v)
		}
	}
}

func TestTag(t *testing.T) {
	cases := []struct {
		num Number
		typ Type
		hex string
	}{
		{1, VarintType, "08"},
		{2, BytesType, "12"},
		{3, BytesType, "1a"},
		{4, VarintType, "20"},
		{5, Fixed32Type, "2d"},
		{6, Fixed64Type, "31"},
		{15, BytesType, "7a"},
		{16, VarintType, "80 01"}, // номера ≥ 16 занимают 2 байта — частые поля нумеруйте 1..15
		{100, VarintType, "a0 06"},
		{MaxValidNumber, Fixed32Type, "fd ff ff ff 0f"},
	}
	for _, c := range cases {
		want := unhex(t, c.hex)
		if got := AppendTag(nil, c.num, c.typ); !bytes.Equal(got, want) {
			t.Errorf("AppendTag(%d, %d) = % x, ожидалось % x", c.num, c.typ, got, want)
		}
		num, typ, n, err := ConsumeTag(want)
		if err != nil || num != c.num || typ != c.typ || n != len(want) {
			t.Errorf("ConsumeTag(% x) = %d, %d, %d, %v", want, num, typ, n, err)
		}
	}
	for _, bad := range []string{"00", "02", "80 80 80 80 10"} { // поле 0 и поле 2^29
		if _, _, _, err := ConsumeTag(unhex(t, bad)); !errors.Is(err, ErrInvalidField) {
			t.Errorf("ConsumeTag(%s) ошибка %v, ожидалась ErrInvalidField", bad, err)
		}
	}
}

func TestBytes(t *testing.T) {
	got := AppendBytes(nil, []byte("hi"))
	if !bytes.Equal(got, []byte{0x02, 'h', 'i'}) {
		t.Errorf("AppendBytes(hi) = % x", got)
	}
	v, n, err := ConsumeBytes([]byte{0x02, 'h', 'i', 'X'})
	if err != nil || string(v) != "hi" || n != 3 {
		t.Errorf("ConsumeBytes = %q, %d, %v", v, n, err)
	}
	for _, bad := range []string{"", "05 41", "ff ff ff ff ff ff ff ff 7f 41"} {
		if _, _, err := ConsumeBytes(unhex(t, bad)); !errors.Is(err, ErrTruncated) {
			t.Errorf("ConsumeBytes(%s) ошибка %v, ожидалась ErrTruncated", bad, err)
		}
	}
}

func TestConsumeFieldValue(t *testing.T) {
	cases := []struct {
		typ  Type
		hex  string
		n    int
		want error
	}{
		{VarintType, "96 01 ff", 2, nil},
		{Fixed32Type, "01 02 03 04 05", 4, nil},
		{Fixed64Type, "01 02 03 04 05 06 07 08", 8, nil},
		{BytesType, "02 68 69 00", 3, nil},
		{Fixed32Type, "01 02 03", 0, ErrTruncated},
		{Fixed64Type, "01", 0, ErrTruncated},
		{StartGroupType, "00", 0, ErrUnsupported},
		{6, "00", 0, ErrWireType},
		{7, "00", 0, ErrWireType},
	}
	for _, c := range cases {
		n, err := ConsumeFieldValue(c.typ, unhex(t, c.hex))
		if !errors.Is(err, c.want) || (c.want == nil && n != c.n) {
			t.Errorf("ConsumeFieldValue(%d, %s) = %d, %v; ожидалось %d, %v", c.typ, c.hex, n, err, c.n, c.want)
		}
	}
}

// Векторы совпадают с выводом настоящего protobuf (SerializeToString / protoc).
var userCases = []struct {
	name string
	u    User
	hex  string
}{
	{"пустое", User{}, ""},
	{"все поля", User{ID: 150, Name: "Ann", Tags: []string{"a", "bc"}, Score: -2},
		"08 96 01  12 03 41 6e 6e  1a 01 61  1a 02 62 63  20 03"},
	{"отрицательный int64 = 10 байт", User{ID: -1}, "08 ff ff ff ff ff ff ff ff ff 01"},
	{"sint32 1", User{Score: 1}, "20 02"},
	{"sint32 -1", User{Score: -1}, "20 01"},
	{"sint32 max", User{Score: math.MaxInt32}, "20 fe ff ff ff 0f"},
	{"sint32 min", User{Score: math.MinInt32}, "20 ff ff ff ff 0f"},
	{"пустой элемент repeated пишется", User{Tags: []string{"", "x"}}, "1a 00 1a 01 78"},
	{"кириллица: длина в байтах", User{Name: "Привет"}, "12 0c d0 9f d1 80 d0 b8 d0 b2 d0 b5 d1 82"},
	{"длина 200 — varint из 2 байт", User{Name: strings.Repeat("x", 200)}, "12 c8 01" + strings.Repeat("78", 200)},
}

func TestUserMarshal(t *testing.T) {
	for _, c := range userCases {
		t.Run(c.name, func(t *testing.T) {
			want := unhex(t, c.hex)
			got := c.u.Marshal()
			if !bytes.Equal(got, want) {
				t.Errorf("Marshal(%+v)\n  = % x\nожидалось % x", c.u, got, want)
			}
		})
	}
}

func TestUserUnmarshal(t *testing.T) {
	for _, c := range userCases {
		t.Run(c.name, func(t *testing.T) {
			u := User{ID: 999, Name: "мусор", Tags: []string{"old"}}
			if err := u.Unmarshal(unhex(t, c.hex)); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !reflect.DeepEqual(u.Tags, c.u.Tags) && !(len(u.Tags) == 0 && len(c.u.Tags) == 0) {
				t.Errorf("Tags = %q, ожидалось %q", u.Tags, c.u.Tags)
			}
			if u.ID != c.u.ID || u.Name != c.u.Name || u.Score != c.u.Score {
				t.Errorf("Unmarshal = %+v, ожидалось %+v (старое содержимое должно сбрасываться)", u, c.u)
			}
		})
	}
}

func TestUserUnmarshalSpecial(t *testing.T) {
	cases := []struct {
		name string
		hex  string
		want User
	}{
		{"неизвестные поля пропускаются",
			"08 01  2d 01 02 03 04  31 01 02 03 04 05 06 07 08  3a 02 68 69  a0 06 2a  12 01 41",
			User{ID: 1, Name: "A"}},
		{"последнее значение скаляра побеждает", "08 01 08 02 12 01 61 12 01 62", User{ID: 2, Name: "b"}},
		{"порядок полей произвольный", "20 03 1a 01 78 08 07 1a 01 79", User{ID: 7, Score: -2, Tags: []string{"x", "y"}}},
		{"sint32 из 10-байтного varint — младшие 32 бита", "20 ff ff ff ff ff ff ff ff ff 01", User{Score: math.MinInt32}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var u User
			if err := u.Unmarshal(unhex(t, c.hex)); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !reflect.DeepEqual(u, c.want) {
				t.Errorf("Unmarshal(%s) = %+v, ожидалось %+v", c.hex, u, c.want)
			}
		})
	}
}

func TestUserUnmarshalErrors(t *testing.T) {
	cases := []struct {
		name string
		hex  string
		err  error
	}{
		{"оборванный varint", "08 96", ErrTruncated},
		{"длина больше данных", "12 05 41", ErrTruncated},
		{"оборванный тег", "a0", ErrTruncated},
		{"поле 0", "00 01", ErrInvalidField},
		{"id как bytes", "0a 01 00", ErrWireType},
		{"name как varint", "10 01", ErrWireType},
		{"wire type 7", "0f", ErrWireType},
		{"неизвестное поле с wire type 7", "3f 00", ErrWireType},
		{"неизвестная группа", "3b", ErrUnsupported},
		{"оборванный fixed64 у неизвестного поля", "31 01 02", ErrTruncated},
		{"невалидный UTF-8", "12 01 ff", ErrInvalidUTF8},
		{"невалидный UTF-8 в tags", "1a 02 c3 28", ErrInvalidUTF8},
		{"переполнение varint", "08 ff ff ff ff ff ff ff ff ff ff 01", ErrOverflow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var u User
			if err := u.Unmarshal(unhex(t, c.hex)); !errors.Is(err, c.err) {
				t.Errorf("Unmarshal(%s) ошибка %v, ожидалась %v", c.hex, err, c.err)
			}
		})
	}
}

func TestUserNoAliasing(t *testing.T) {
	buf := User{Name: "abc", Tags: []string{"t"}}.marshalCopy()
	var u User
	if err := u.Unmarshal(buf); err != nil {
		t.Fatal(err)
	}
	for i := range buf {
		buf[i] = 'Z'
	}
	if u.Name != "abc" || u.Tags[0] != "t" {
		t.Errorf("строки ссылаются на входной буфер: %+v", u)
	}
}

func (u User) marshalCopy() []byte { return bytes.Clone(u.Marshal()) }

func TestRoundTrip(t *testing.T) {
	us := []User{
		{ID: math.MinInt64, Name: "ё", Tags: []string{"🙂", ""}, Score: math.MinInt32},
		{ID: math.MaxInt64, Score: math.MaxInt32},
	}
	for _, u := range us {
		var got User
		if err := got.Unmarshal(u.Marshal()); err != nil || !reflect.DeepEqual(got, u) {
			t.Errorf("roundtrip %+v → %+v, %v", u, got, err)
		}
	}
}

func BenchmarkMarshal(b *testing.B) {
	u := User{ID: 150, Name: "Ann", Tags: []string{"a", "bc"}, Score: -2}
	for i := 0; i < b.N; i++ {
		_ = u.Marshal()
	}
}

func BenchmarkUnmarshal(b *testing.B) {
	data := (&User{ID: 150, Name: "Ann", Tags: []string{"a", "bc"}, Score: -2}).Marshal()
	var u User
	for i := 0; i < b.N; i++ {
		_ = u.Unmarshal(data)
	}
}
