package allocs

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"math/rand"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"
)

// raceEnabled: детектор гонок добавляет свои аллокации (и sync.Pool под -race
// намеренно теряет объекты), поэтому лимиты аллокаций проверяем только без -race.
var raceEnabled = func() bool {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, s := range bi.Settings {
		if s.Key == "-race" && s.Value == "true" {
			return true
		}
	}
	return false
}()

func checkAllocs(t *testing.T, name string, limit float64, f func()) {
	t.Helper()
	if raceEnabled {
		t.Logf("%s: проверка аллокаций пропущена под -race", name)
		return
	}
	if got := testing.AllocsPerRun(200, f); got > limit {
		t.Errorf("%s: %.0f аллокаций на вызов, лимит %.0f", name, got, limit)
	}
}

// --- эталоны (заведомо правильные, но медленные) ---

func refJoin(xs []int, sep string) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, sep)
}

func refRecord(r Record) string {
	return fmt.Sprintf("id=%d name=%q score=%s active=%t tags=%s\n",
		r.ID, r.Name, strconv.FormatFloat(r.Score, 'f', -1, 64), r.Active, strings.Join(r.Tags, "|"))
}

func TestJoinInts(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	cases := [][]int{nil, {}, {0}, {-1}, {math.MaxInt64, math.MinInt64}, {1, 22, 333}}
	for i := 0; i < 300; i++ {
		xs := make([]int, rng.Intn(20))
		for j := range xs {
			xs[j] = int(rng.Int63()) >> rng.Intn(63)
			if rng.Intn(2) == 0 {
				xs[j] = -xs[j]
			}
		}
		cases = append(cases, xs)
	}
	for _, xs := range cases {
		for _, sep := range []string{"", ",", ", ", " | "} {
			if got, want := JoinInts(xs, sep), refJoin(xs, sep); got != want {
				t.Fatalf("JoinInts(%v, %q) = %q, ожидалось %q", xs, sep, got, want)
			}
		}
	}
	big := make([]int, 1000)
	for i := range big {
		big[i] = i * 7919
	}
	checkAllocs(t, "JoinInts(1000 чисел)", 1, func() { _ = JoinInts(big, ", ") })
	checkAllocs(t, "JoinInts(пусто)", 0, func() { _ = JoinInts(nil, ", ") })
}

func TestSumCSV(t *testing.T) {
	tests := []struct {
		in   string
		want int64
		bad  bool
	}{
		{"", 0, false},
		{"  ", 0, false},
		{"42", 42, false},
		{"1,2,3", 6, false},
		{" 1 ,\t-2 , +3 ", 2, false},
		{"9223372036854775807", math.MaxInt64, false},
		{"1,,2", 0, true},
		{"1,2,", 0, true},
		{",1", 0, true},
		{"1, x", 0, true},
		{"1 2", 0, true},
		{"99999999999999999999", 0, true},
		{"0x10", 0, true},
	}
	for _, tt := range tests {
		got, err := SumCSV(tt.in)
		if tt.bad {
			if err == nil {
				t.Errorf("SumCSV(%q) = %d, ожидалась ошибка", tt.in, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("SumCSV(%q) = %d, %v; ожидалось %d", tt.in, got, err, tt.want)
		}
	}
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(strconv.Itoa(i - 250))
	}
	line := sb.String()
	if got, err := SumCSV(line); err != nil || got != -250 {
		t.Fatalf("SumCSV(-250..249) = %d, %v", got, err)
	}
	checkAllocs(t, "SumCSV(500 чисел)", 0, func() { _, _ = SumCSV(line) })
}

func TestAppendRecord(t *testing.T) {
	recs := []Record{
		{},
		{ID: 42, Name: "Ann", Score: 3.5, Active: true, Tags: []string{"a", "b", "c"}},
		{ID: -7, Name: `He said "hi"\n`, Score: -0.001, Tags: []string{"x"}},
		{ID: math.MaxInt64, Name: "Привет\t🚀\x00", Score: 1e21, Active: true},
		{Name: "\xff", Score: math.Inf(-1)},
		{Score: math.NaN(), Tags: []string{"", ""}},
	}
	for _, r := range recs {
		got := string(AppendRecord([]byte("prefix|"), r))
		if want := "prefix|" + refRecord(r); got != want {
			t.Errorf("AppendRecord(%+v)\n получено %q\nожидалось %q", r, got, want)
		}
	}
	r := recs[1]
	buf := make([]byte, 0, 256)
	checkAllocs(t, "AppendRecord", 0, func() { buf = AppendRecord(buf[:0], r) })
}

func TestRender(t *testing.T) {
	var out bytes.Buffer
	var writes int
	w := writerFunc(func(p []byte) (int, error) { writes++; return out.Write(p) })
	cases := []struct {
		user  string
		items []string
		want  string
	}{
		{"ann", []string{"a", "b", "c"}, "ann: a, b, c\n"},
		{"bob", nil, "bob: \n"},
		{"", []string{"x"}, ": x\n"},
		{"кот", []string{"рыба", "молоко"}, "кот: рыба, молоко\n"},
	}
	for _, c := range cases {
		out.Reset()
		writes = 0
		if err := Render(w, c.user, c.items); err != nil {
			t.Fatal(err)
		}
		if out.String() != c.want || writes != 1 {
			t.Errorf("Render(%q, %q) = %q за %d вызовов Write, ожидалось %q за 1", c.user, c.items, out.String(), writes, c.want)
		}
	}
	// Ошибка writer'а возвращается.
	errW := writerFunc(func(p []byte) (int, error) { return 0, io.ErrShortWrite })
	if err := Render(errW, "a", nil); err != io.ErrShortWrite {
		t.Errorf("ошибка Write: %v", err)
	}
	items := []string{"alpha", "beta", "gamma", "delta"}
	checkAllocs(t, "Render", 0, func() { _ = Render(io.Discard, "user", items) })
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// --- бенчмарки: go test -bench . -benchmem ./modules/06-testing/tasks/05_allocs/ ---

var sinkS string
var sinkI int64
var sinkB []byte

func BenchmarkJoinInts(b *testing.B) {
	xs := make([]int, 100)
	for i := range xs {
		xs[i] = i * 12345
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkS = JoinInts(xs, ",")
	}
}

func BenchmarkSumCSV(b *testing.B) {
	line := strings.Repeat("123, -45, 6789, ", 50) + "0"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkI, _ = SumCSV(line)
	}
}

func BenchmarkAppendRecord(b *testing.B) {
	r := Record{ID: 42, Name: "Ann", Score: 3.5, Active: true, Tags: []string{"a", "b", "c"}}
	buf := make([]byte, 0, 256)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = AppendRecord(buf[:0], r)
	}
	sinkB = buf
}

func BenchmarkRender(b *testing.B) {
	items := []string{"alpha", "beta", "gamma", "delta"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Render(io.Discard, "user", items)
	}
}
