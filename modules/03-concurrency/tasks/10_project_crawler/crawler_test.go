package crawler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func checkNoLeak(t *testing.T, base int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > base {
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<16)
			n := runtime.Stack(buf, true)
			t.Fatalf("утечка горутин: было %d, стало %d\n%s", base, runtime.NumGoroutine(), buf[:n])
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitInFetch ждёт, пока в стеках горутин наберётся не меньше n кадров
// CachedFetcher.Fetch (вместо time.Sleep «на всякий случай»). Пока общая
// загрузка заблокирована, вошедший в Fetch вызов обязан к ней присоединиться.
func waitInFetch(t *testing.T, n int) {
	t.Helper()
	buf := make([]byte, 1<<20)
	deadline := time.Now().Add(5 * time.Second)
	for {
		m := runtime.Stack(buf, true)
		if bytes.Count(buf[:m], []byte("(*CachedFetcher).Fetch(")) >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("за 5с в CachedFetcher.Fetch не вошли %d горутин", n)
		}
		time.Sleep(time.Millisecond)
	}
}

var errNotFound = errors.New("404")

// fakeFetcher — граф ссылок в памяти со статистикой вызовов.
type fakeFetcher struct {
	graph map[string][]string
	hook  func(ctx context.Context, url string) error // необязательный

	mu    sync.Mutex
	calls map[string]int

	active, maxActive atomic.Int64
}

func newFake(graph map[string][]string) *fakeFetcher {
	return &fakeFetcher{graph: graph, calls: map[string]int{}}
}

func (f *fakeFetcher) Fetch(ctx context.Context, url string) ([]string, error) {
	cur := f.active.Add(1)
	defer f.active.Add(-1)
	for {
		m := f.maxActive.Load()
		if cur <= m || f.maxActive.CompareAndSwap(m, cur) {
			break
		}
	}
	f.mu.Lock()
	f.calls[url]++
	f.mu.Unlock()
	if f.hook != nil {
		if err := f.hook(ctx, url); err != nil {
			return nil, err
		}
	}
	links, ok := f.graph[url]
	if !ok {
		return nil, fmt.Errorf("%s: %w", url, errNotFound)
	}
	return append([]string(nil), links...), nil
}

func (f *fakeFetcher) callCounts() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return maps.Clone(f.calls)
}

func testGraph() map[string][]string {
	return map[string][]string{
		"a": {"b", "c#frag", " d ", ""},
		"b": {"a", "c", "e"},
		"c": {"e", "f", "c"},
		"d": {"x-missing"},
		"e": {"g"},
		"f": {},
		"g": {"h", "a#top"},
		"h": {},
	}
}

func TestNormalize(t *testing.T) {
	tests := map[string]string{
		"a/b#top": "a/b", "  x  ": "x", "#only": "", "": "", "p?q=1#f#g": "p?q=1", "plain": "plain",
	}
	for in, want := range tests {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, ожидалось %q", in, got, want)
		}
	}
}

func TestCrawlDepths(t *testing.T) {
	tests := []struct {
		name      string
		depth     int
		wantPages map[string]int
		wantErrs  []string
	}{
		{"глубина 0", 0, map[string]int{"a": 0}, nil},
		{"отрицательная глубина", -3, map[string]int{"a": 0}, nil},
		{"глубина 1", 1, map[string]int{"a": 0, "b": 1, "c": 1, "d": 1}, nil},
		{"глубина 2", 2, map[string]int{"a": 0, "b": 1, "c": 1, "d": 1, "e": 2, "f": 2}, []string{"x-missing"}},
		{"весь граф", 10, map[string]int{"a": 0, "b": 1, "c": 1, "d": 1, "e": 2, "f": 2, "g": 3, "h": 4}, []string{"x-missing"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := runtime.NumGoroutine()
			f := newFake(testGraph())
			res, err := Crawl(context.Background(), f, "a", Options{MaxDepth: tc.depth, Workers: 3})
			if err != nil {
				t.Fatalf("Crawl вернул ошибку %v", err)
			}
			if !maps.Equal(res.Pages, tc.wantPages) {
				t.Errorf("Pages = %v, ожидалось %v", res.Pages, tc.wantPages)
			}
			if len(res.Errors) != len(tc.wantErrs) {
				t.Errorf("Errors = %v, ожидались ошибки для %v", res.Errors, tc.wantErrs)
			}
			for _, u := range tc.wantErrs {
				if !errors.Is(res.Errors[u], errNotFound) {
					t.Errorf("Errors[%q] = %v, ожидалась ошибка fetcher'а (404)", u, res.Errors[u])
				}
			}
			for u, n := range f.callCounts() {
				if n != 1 {
					t.Errorf("%q загружен %d раз — нужна дедупликация", u, n)
				}
			}
			checkNoLeak(t, base)
		})
	}
}

func TestCrawlEmptyStart(t *testing.T) {
	f := newFake(testGraph())
	res, err := Crawl(context.Background(), f, "  #x", Options{MaxDepth: 3})
	if err != nil || res == nil || len(res.Pages) != 0 || len(res.Errors) != 0 {
		t.Errorf("Crawl(пустой URL) = %+v, %v; ожидался пустой результат без ошибки", res, err)
	}
	if res != nil && (res.Pages == nil || res.Errors == nil) {
		t.Errorf("карты в Result должны быть не-nil")
	}
}

func starGraph(n int) map[string][]string {
	g := map[string][]string{"root": nil}
	for i := 0; i < n; i++ {
		u := fmt.Sprintf("p%d", i)
		g["root"] = append(g["root"], u)
		g[u] = nil
	}
	return g
}

func TestCrawlParallelAndLimited(t *testing.T) {
	const workers = 4
	f := newFake(starGraph(16))
	var timedOut atomic.Bool
	reached := make(chan struct{})
	var once sync.Once
	f.hook = func(_ context.Context, url string) error {
		if url == "root" {
			return nil
		}
		if f.active.Load() >= workers {
			once.Do(func() { close(reached) })
		}
		if !timedOut.Load() {
			select {
			case <-reached:
			case <-time.After(time.Second):
				timedOut.Store(true)
			}
		}
		return nil
	}
	base := runtime.NumGoroutine()
	res, err := Crawl(context.Background(), f, "root", Options{MaxDepth: 1, Workers: workers})
	if err != nil || len(res.Pages) != 17 {
		t.Fatalf("Crawl = %d страниц, err=%v; ожидалось 17 страниц", len(res.Pages), err)
	}
	if timedOut.Load() {
		t.Errorf("не набралось %d одновременных Fetch — обход не параллелен", workers)
	}
	if m := f.maxActive.Load(); m > workers {
		t.Errorf("одновременно выполнялось %d Fetch при Workers=%d", m, workers)
	}
	checkNoLeak(t, base)
}

func TestCrawlCancel(t *testing.T) {
	f := newFake(starGraph(20))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	var once sync.Once
	f.hook = func(ctx context.Context, url string) error {
		if url == "root" {
			return nil
		}
		once.Do(func() { close(started) })
		<-ctx.Done() // «долгая» загрузка, уважающая контекст
		return ctx.Err()
	}
	go func() {
		<-started
		cancel()
	}()

	base := runtime.NumGoroutine()
	type out struct {
		res *Result
		err error
	}
	done := make(chan out, 1)
	go func() {
		res, err := Crawl(ctx, f, "root", Options{MaxDepth: 5, Workers: 3})
		done <- out{res, err}
	}()
	select {
	case o := <-done:
		if !errors.Is(o.err, context.Canceled) {
			t.Errorf("Crawl после отмены вернул err=%v, ожидалось context.Canceled", o.err)
		}
		if o.res == nil || o.res.Pages["root"] != 0 || len(o.res.Pages) != 1 {
			t.Errorf("ожидался частичный результат с root, получено %+v", o.res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Crawl не завершился после отмены контекста")
	}
	var total int
	for _, n := range f.callCounts() {
		total += n
	}
	// root + не больше Workers загрузок в полёте; +1 — допуск на редкую гонку, когда
	// отмена и освобождение слота семафора случаются между проверкой ctx.Err() и select.
	if total > 1+3+1 {
		t.Errorf("после отмены продолжились загрузки: всего %d вызовов Fetch при Workers=3", total)
	}
	checkNoLeak(t, base)
}

func TestCrawlPageTimeout(t *testing.T) {
	f := newFake(map[string][]string{"root": {"slow", "fast"}, "slow": {"never"}, "fast": {}})
	f.hook = func(ctx context.Context, url string) error {
		if url != "slow" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
			return errors.New("у Fetch нет таймаута")
		}
	}
	base := runtime.NumGoroutine()
	res, err := Crawl(context.Background(), f, "root", Options{MaxDepth: 3, Workers: 2, PageTimeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatalf("таймаут одной страницы не должен прерывать обход, err=%v", err)
	}
	if !errors.Is(res.Errors["slow"], context.DeadlineExceeded) {
		t.Errorf("Errors[slow] = %v, ожидалось context.DeadlineExceeded", res.Errors["slow"])
	}
	if _, ok := res.Pages["fast"]; !ok {
		t.Errorf("страница fast должна быть загружена: %v", res.Pages)
	}
	checkNoLeak(t, base)
}

// ---- CachedFetcher ----

type fakeClock struct{ now atomic.Int64 }

func (c *fakeClock) Now() time.Time          { return time.Unix(0, c.now.Load()) }
func (c *fakeClock) Advance(d time.Duration) { c.now.Add(int64(d)) }

func TestCachedFetcherTTL(t *testing.T) {
	f := newFake(map[string][]string{"u": {"x", "y"}})
	clock := &fakeClock{}
	c := &CachedFetcher{Fetcher: f, TTL: time.Minute, Now: clock.Now}
	ctx := context.Background()

	links, err := c.Fetch(ctx, "u")
	if err != nil || len(links) != 2 {
		t.Fatalf("Fetch = %v, %v", links, err)
	}
	links[0] = "испорчено" // мутация результата не должна портить кэш
	clock.Advance(59 * time.Second)
	links, _ = c.Fetch(ctx, "u")
	if f.callCounts()["u"] != 1 {
		t.Errorf("повторный Fetch до истечения TTL не должен ходить в Fetcher (вызовов: %d)", f.callCounts()["u"])
	}
	if links[0] != "x" {
		t.Errorf("кэш испорчен мутацией возвращённого среза: %v", links)
	}
	clock.Advance(2 * time.Second)
	c.Fetch(ctx, "u")
	if n := f.callCounts()["u"]; n != 2 {
		t.Errorf("после истечения TTL вызовов Fetcher = %d, ожидалось 2", n)
	}
}

func TestCachedFetcherNoCacheForErrorsAndZeroTTL(t *testing.T) {
	f := newFake(map[string][]string{"ok": {}})
	c := &CachedFetcher{Fetcher: f, TTL: time.Hour}
	for i := 0; i < 3; i++ {
		if _, err := c.Fetch(context.Background(), "missing"); !errors.Is(err, errNotFound) {
			t.Fatalf("Fetch(missing) = %v, ожидалась 404", err)
		}
	}
	if n := f.callCounts()["missing"]; n != 3 {
		t.Errorf("ошибки не должны кэшироваться: вызовов %d, ожидалось 3", n)
	}

	f2 := newFake(map[string][]string{"ok": {}})
	c2 := &CachedFetcher{Fetcher: f2} // TTL 0 — без кэша
	c2.Fetch(context.Background(), "ok")
	c2.Fetch(context.Background(), "ok")
	if n := f2.callCounts()["ok"]; n != 2 {
		t.Errorf("TTL=0: вызовов %d, ожидалось 2", n)
	}
}

func TestCachedFetcherSingleflight(t *testing.T) {
	base := runtime.NumGoroutine()
	f := newFake(map[string][]string{"hot": {"z"}})
	release := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	f.hook = func(context.Context, string) error {
		once.Do(func() { close(started) })
		<-release
		return nil
	}
	c := &CachedFetcher{Fetcher: f, TTL: time.Hour}
	const n = 50
	var ready, done sync.WaitGroup
	ready.Add(n)
	done.Add(n)
	var bad atomic.Int64
	for i := 0; i < n; i++ {
		go func() {
			defer done.Done()
			ready.Done()
			links, err := c.Fetch(context.Background(), "hot")
			if err != nil || len(links) != 1 || links[0] != "z" {
				bad.Add(1)
			}
		}()
	}
	ready.Wait()
	<-started
	waitInFetch(t, n) // все n вызовов внутри Fetch и ждут одну загрузку
	close(release)
	done.Wait()
	if b := bad.Load(); b > 0 {
		t.Errorf("%d горутин получили неверный результат", b)
	}
	if k := f.callCounts()["hot"]; k != 1 {
		t.Errorf("одновременные запросы одного URL: вызовов Fetcher %d, ожидался 1 (cache stampede)", k)
	}
	checkNoLeak(t, base)
}

// Отмена контекста одного ожидающего не отменяет общую загрузку для других.
func TestCachedFetcherWaiterCancel(t *testing.T) {
	base := runtime.NumGoroutine()
	f := newFake(map[string][]string{"u": {"v"}})
	release := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	f.hook = func(ctx context.Context, _ string) error {
		once.Do(func() { close(started) })
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c := &CachedFetcher{Fetcher: f, TTL: time.Hour}

	ctx1, cancel1 := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() {
		_, err := c.Fetch(ctx1, "u")
		first <- err
	}()
	<-started
	cancel1()
	select {
	case err := <-first:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("отменённый ожидающий получил %v, ожидалось context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Fetch не вернулся после отмены своего контекста")
	}

	second := make(chan []string, 1)
	go func() {
		links, _ := c.Fetch(context.Background(), "u")
		second <- links
	}()
	waitInFetch(t, 1) // второй вызов присоединился к ещё идущей загрузке
	close(release)
	select {
	case links := <-second:
		if len(links) != 1 || links[0] != "v" {
			t.Errorf("второй вызов получил %v, ожидалось [v]", links)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("второй Fetch не вернулся")
	}
	if k := f.callCounts()["u"]; k != 1 {
		t.Errorf("загрузка отменилась вместе с первым вызывающим: вызовов Fetcher %d, ожидался 1", k)
	}
	checkNoLeak(t, base)
}

func TestCrawlWithCache(t *testing.T) {
	f := newFake(testGraph())
	c := &CachedFetcher{Fetcher: f, TTL: time.Hour}
	for i := 0; i < 3; i++ {
		res, err := Crawl(context.Background(), c, "a", Options{MaxDepth: 10, Workers: 4})
		if err != nil || len(res.Pages) != 8 {
			t.Fatalf("проход %d: %d страниц, err=%v; ожидалось 8", i, len(res.Pages), err)
		}
	}
	for u, n := range f.callCounts() {
		if u != "x-missing" && n != 1 {
			t.Errorf("%q загружен %d раз за 3 обхода, ожидалось 1 (кэш)", u, n)
		}
	}
}
