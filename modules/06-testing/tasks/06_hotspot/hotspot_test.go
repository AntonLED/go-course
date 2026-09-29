package hotspot

import (
	"fmt"
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"
)

// --- эталоны: медленные, но очевидно правильные ---

func refDedup(lines []string) []string {
	var out []string
	for _, l := range lines {
		if !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

func refTopWords(text string, k int) []WordCount {
	counts := map[string]int{}
	for _, w := range strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		var b strings.Builder
		for _, r := range w {
			b.WriteRune(unicode.ToLower(r))
		}
		counts[b.String()]++
	}
	var res []WordCount
	for w, c := range counts {
		res = append(res, WordCount{w, c})
	}
	slices.SortFunc(res, func(a, b WordCount) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Word, b.Word)
	})
	return res[:min(max(k, 0), len(res))]
}

func refReport(lines []string) string {
	var b strings.Builder
	for i, l := range lines {
		fmt.Fprintf(&b, "%d: %s\n", i+1, l)
	}
	return b.String()
}

// --- корректность на маленьких входах ---

func TestDedupCorrect(t *testing.T) {
	tests := [][]string{nil, {}, {"a"}, {"a", "a"}, {"b", "a", "b", "c", "a"}, {"", "", "x", ""}}
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 200; i++ {
		in := make([]string, rng.Intn(30))
		for j := range in {
			in[j] = strconv.Itoa(rng.Intn(10))
		}
		tests = append(tests, in)
	}
	for _, in := range tests {
		orig := slices.Clone(in)
		if got, want := Dedup(in), refDedup(in); !slices.Equal(got, want) {
			t.Fatalf("Dedup(%q) = %q, ожидалось %q", in, got, want)
		}
		if !slices.Equal(in, orig) {
			t.Fatalf("Dedup изменил входной срез")
		}
	}
}

func TestTopWordsCorrect(t *testing.T) {
	tests := []struct {
		text string
		k    int
		want []WordCount
	}{
		{"", 3, nil},
		{"a b a", 0, nil},
		{"a b a", -1, nil},
		{"Go go GO! gopher, go.", 2, []WordCount{{"go", 4}, {"gopher", 1}}},
		{"b a c b a", 10, []WordCount{{"a", 2}, {"b", 2}, {"c", 1}}},
		{"Мама мыла раму, МАМА!", 1, []WordCount{{"мама", 2}}},
		{"x1 x1 2024 2024 2024", 5, []WordCount{{"2024", 3}, {"x1", 2}}},
		{"don't \xff stop", 5, []WordCount{{"don", 1}, {"stop", 1}, {"t", 1}}},
	}
	for _, tt := range tests {
		got := TopWords(tt.text, tt.k)
		if !slices.Equal(got, tt.want) {
			t.Errorf("TopWords(%q, %d) = %v, ожидалось %v", tt.text, tt.k, got, tt.want)
		}
		if got == nil {
			t.Errorf("TopWords(%q, %d) вернул nil, ожидался пустой срез", tt.text, tt.k)
		}
	}
	rng := rand.New(rand.NewSource(3))
	words := []string{"Go", "go", "ёж", "Ёж", "a1", "x", "Zeta", "zeta", "42"}
	seps := []string{" ", ", ", "!\n", "—", "\t"}
	for i := 0; i < 200; i++ {
		var b strings.Builder
		for j := rng.Intn(40); j > 0; j-- {
			b.WriteString(words[rng.Intn(len(words))])
			b.WriteString(seps[rng.Intn(len(seps))])
		}
		k := rng.Intn(8)
		if got, want := TopWords(b.String(), k), refTopWords(b.String(), k); !slices.Equal(got, want) {
			t.Fatalf("TopWords(%q, %d) = %v, ожидалось %v", b.String(), k, got, want)
		}
	}
}

func TestReportCorrect(t *testing.T) {
	for _, in := range [][]string{nil, {""}, {"a"}, {"первая", "", "третья"}} {
		if got, want := Report(in), refReport(in); got != want {
			t.Errorf("Report(%q) = %q, ожидалось %q", in, got, want)
		}
	}
}

// --- производительность: большие входы с ЩЕДРЫМ лимитом ---
// Линейное решение укладывается в десятки миллисекунд (под -race — в пару сотен),
// квадратичное — работает минуты.

func within(t *testing.T, limit time.Duration, name string, f func()) {
	t.Helper()
	done := make(chan struct{})
	start := time.Now()
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
		t.Logf("%s: %v", name, time.Since(start).Round(time.Millisecond))
	case <-time.After(limit):
		// Горутина продолжит работу в фоне, но тест уже провален.
		t.Fatalf("%s: дольше %v — похоже, алгоритм квадратичный", name, limit)
	}
}

func TestDedupFast(t *testing.T) {
	const n = 200_000
	in := make([]string, n)
	for i := range in {
		in[i] = "line-" + strconv.Itoa(i%(n/2))
	}
	var out []string
	within(t, 3*time.Second, "Dedup(200k)", func() { out = Dedup(in) })
	if len(out) != n/2 || out[0] != "line-0" || out[len(out)-1] != "line-"+strconv.Itoa(n/2-1) {
		t.Errorf("Dedup(200k): %d строк", len(out))
	}
}

func TestTopWordsFast(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 300_000; i++ {
		fmt.Fprintf(&b, "Word%d ", i%30_000)
		if i%7 == 0 {
			b.WriteString("common, ")
		}
	}
	text := b.String()
	var got []WordCount
	within(t, 3*time.Second, "TopWords(300k слов)", func() { got = TopWords(text, 3) })
	want := []WordCount{{"common", 42858}, {"word0", 10}, {"word1", 10}}
	if !slices.Equal(got, want) {
		t.Errorf("TopWords = %v, ожидалось %v", got, want)
	}
}

func TestReportFast(t *testing.T) {
	lines := make([]string, 200_000)
	for i := range lines {
		lines[i] = "some log line with payload"
	}
	var out string
	within(t, 3*time.Second, "Report(200k строк)", func() { out = Report(lines) })
	if !strings.HasPrefix(out, "1: some") || !strings.HasSuffix(out, "200000: some log line with payload\n") {
		t.Errorf("Report: неверный результат (%d байт)", len(out))
	}
}

// Бенчмарки для профилирования:
//
//	go test -run XXX -bench TopWords -cpuprofile cpu.out ./modules/06-testing/tasks/06_hotspot/
//	go tool pprof -top cpu.out
var sink any

func BenchmarkDedup(b *testing.B) {
	in := make([]string, 5000)
	for i := range in {
		in[i] = strconv.Itoa(i % 2500)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sink = Dedup(in)
	}
}

func BenchmarkTopWords(b *testing.B) {
	text := strings.Repeat("the quick brown fox jumps over the lazy dog ", 2000)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sink = TopWords(text, 5)
	}
}

func BenchmarkReport(b *testing.B) {
	lines := make([]string, 5000)
	for i := range lines {
		lines[i] = "log line"
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sink = Report(lines)
	}
}
