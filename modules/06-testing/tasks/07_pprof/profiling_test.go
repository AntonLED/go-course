package profiling

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const fixture = `goroutine profile: total 9
4 @ 0x474cee 0x40ee85 0x40ea32 0x5b8805 0x47c221
#	0x5b8804	example.com/app/worker.(*Pool).run+0x24	/app/worker/pool.go:31

2 @ 0x474cee 0x44a1c5 0x47c221
#	0x44a1c4	runtime.gopark+0xc4	/usr/local/go/src/runtime/proc.go:424
#	0x47c220	runtime.goexit+0x0	/usr/local/go/src/runtime/asm_amd64.s:1700

1 @ 0x4363b1 0x473b9d 0x5213b1 0x5b86dd 0x47c221
#	0x5213b0	runtime/pprof.writeRuntimeProfile+0xb0	/usr/local/go/src/runtime/pprof/pprof.go:796
#	0x5b86dc	main.main+0x13c				/app/main.go:8

1 @ 0x474cee 0x40ee85 0x5b87a5 0x47c221
# labels: {"handler":"upload"}
#	0x40ee84	runtime.chanrecv1+0x14	/usr/local/go/src/runtime/chan.go:489
#	0x5b87a4	example.com/app/worker.(*Pool).run+0x24	/app/worker/pool.go:31

1 @ 0x474cee 0x47c221
#	0x4f0f30	net/http.(*conn).serve+0x430		/usr/local/go/src/net/http/server.go:2102
`

func TestParseFixture(t *testing.T) {
	got, err := ParseGoroutineProfile(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("ParseGoroutineProfile: %v", err)
	}
	want := []FuncCount{
		{"example.com/app/worker.(*Pool).run", 5},
		{"runtime.gopark", 2},
		{"net/http.(*conn).serve", 1},
		{"runtime/pprof.writeRuntimeProfile", 1},
	}
	if !slices.Equal(got, want) {
		t.Errorf("получено\n%v\nожидалось\n%v", got, want)
	}
}

func TestParseMalformed(t *testing.T) {
	tests := map[string]string{
		"empty":       "",
		"noHeader":    "1 @ 0x1\n#\t0x1\tmain.f+0x1\tf.go:1\n",
		"badTotal":    "goroutine profile: total many\n",
		"badCount":    "goroutine profile: total 1\nx @ 0x1\n#\t0x1\tmain.f+0x1\tf.go:1\n",
		"frameFirst":  "goroutine profile: total 1\n#\t0x1\tmain.f+0x1\tf.go:1\n",
		"sumMismatch": "goroutine profile: total 5\n2 @ 0x1\n#\t0x1\tmain.f+0x1\tf.go:1\n",
		"shortFrame":  "goroutine profile: total 1\n1 @ 0x1\n#\t0x1\n",
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseGoroutineProfile(strings.NewReader(in)); !errors.Is(err, ErrMalformed) {
				t.Errorf("ожидалась ErrMalformed, получено %v", err)
			}
		})
	}
	if got, err := ParseGoroutineProfile(strings.NewReader("goroutine profile: total 0\n")); err != nil || len(got) != 0 {
		t.Errorf("пустой профиль: %v %v", got, err)
	}
}

// parkedWorker — функция, в которой «застрянут» горутины для живого теста.
func parkedWorker(ch <-chan struct{}, wg *sync.WaitGroup) {
	wg.Done()
	<-ch
}

func TestGoroutineTopLive(t *testing.T) {
	const n = 7
	stop := make(chan struct{})
	defer close(stop)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go parkedWorker(stop, &wg)
	}
	wg.Wait()
	time.Sleep(20 * time.Millisecond) // дать горутинам дойти до <-ch

	top, err := GoroutineTop(-1)
	if err != nil {
		t.Fatalf("GoroutineTop: %v", err)
	}
	var found *FuncCount
	for i := range top {
		if strings.HasSuffix(top[i].Func, ".parkedWorker") {
			found = &top[i]
		}
		if strings.Contains(top[i].Func, "+0x") {
			t.Errorf("смещение +0x... нужно отрезать: %q", top[i].Func)
		}
	}
	if found == nil || found.Count != n {
		t.Fatalf("ожидалось %d горутин в parkedWorker, топ: %v", n, top)
	}
	if top[0] != *found {
		t.Errorf("parkedWorker должен быть первым в топе: %v", top)
	}
	for i := 1; i < len(top); i++ {
		a, b := top[i-1], top[i]
		if a.Count < b.Count || a.Count == b.Count && a.Func > b.Func {
			t.Errorf("неверная сортировка: %v перед %v", a, b)
		}
	}
	two, err := GoroutineTop(2)
	if err != nil || len(two) > 2 || two[0] != *found {
		t.Errorf("GoroutineTop(2) = %v, %v", two, err)
	}
	if zero, err := GoroutineTop(0); err != nil || len(zero) != 0 {
		t.Errorf("GoroutineTop(0) = %v, %v", zero, err)
	}
}

var sink int

func busy(d time.Duration) {
	for start := time.Now(); time.Since(start) < d; {
		for i := 0; i < 10000; i++ {
			sink += i * i
		}
	}
}

func isGzip(b []byte) bool { return len(b) > 2 && b[0] == 0x1f && b[1] == 0x8b }

func TestCPUProfile(t *testing.T) {
	var buf bytes.Buffer
	calls := 0
	var nestedErr error
	var nestedCalled bool
	err := CPUProfile(&buf, func() {
		calls++
		// Второй профиль, пока идёт первый, запустить нельзя.
		nestedErr = CPUProfile(&bytes.Buffer{}, func() { nestedCalled = true })
		busy(50 * time.Millisecond)
	})
	if err != nil {
		t.Fatalf("CPUProfile: %v", err)
	}
	if calls != 1 {
		t.Errorf("work вызван %d раз", calls)
	}
	if nestedErr == nil || nestedCalled {
		t.Errorf("вложенный CPUProfile должен вернуть ошибку и не вызывать work (err=%v, called=%v)", nestedErr, nestedCalled)
	}
	if !isGzip(buf.Bytes()) {
		t.Errorf("CPU-профиль должен быть gzip-protobuf (начинаться с 1f 8b), получено %d байт", buf.Len())
	}
	// После завершения можно снова.
	if err := CPUProfile(&bytes.Buffer{}, func() {}); err != nil {
		t.Errorf("повторный CPUProfile после остановки: %v", err)
	}
}

func TestHeapProfile(t *testing.T) {
	var buf bytes.Buffer
	if err := HeapProfile(&buf); err != nil {
		t.Fatal(err)
	}
	if !isGzip(buf.Bytes()) {
		t.Errorf("heap-профиль должен быть gzip-protobuf, получено %d байт", buf.Len())
	}
}
