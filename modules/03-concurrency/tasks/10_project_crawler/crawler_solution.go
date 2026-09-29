//go:build solution

// Package crawler — конкурентный обходчик ссылок (Задание 3).
package crawler

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"
)

// Normalize приводит URL к каноническому виду для дедупликации.
func Normalize(url string) string {
	url = strings.TrimSpace(url)
	if i := strings.IndexByte(url, '#'); i >= 0 {
		url = url[:i]
	}
	return url
}

// Crawl обходит граф ссылок в ширину, уровень за уровнем.
//
// Обход по уровням (level-synchronous BFS) даёт детерминированный результат:
// каждая страница получает минимальную глубину, независимо от того, какая
// горутина первой нашла ссылку. Внутри уровня страницы грузятся параллельно.
func Crawl(ctx context.Context, f Fetcher, start string, opt Options) (*Result, error) {
	maxDepth := max(opt.MaxDepth, 0)
	workers := max(opt.Workers, 1)

	res := &Result{Pages: map[string]int{}, Errors: map[string]error{}}
	start = Normalize(start)
	if start == "" {
		return res, nil
	}
	// seen читается и пишется только в этой горутине — мьютекс не нужен.
	seen := map[string]bool{start: true}
	level := []string{start}

	for depth := 0; len(level) > 0; depth++ {
		var (
			mu    sync.Mutex // защищает res и found
			found []string   // ссылки, найденные на этом уровне
			wg    sync.WaitGroup
			sem   = make(chan struct{}, workers) // семафор: ≤ workers Fetch одновременно
		)

	launch:
		for _, u := range level {
			// Приоритетная проверка отмены: select ниже выбирает случайно.
			if ctx.Err() != nil {
				break
			}
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				break launch
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()

				fctx, cancel := ctx, context.CancelFunc(func() {})
				if opt.PageTimeout > 0 {
					fctx, cancel = context.WithTimeout(ctx, opt.PageTimeout)
				}
				links, err := f.Fetch(fctx, u)
				cancel() // освобождаем таймер сразу, а не в конце обхода

				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					res.Errors[u] = err
					return
				}
				res.Pages[u] = depth
				if depth < maxDepth {
					found = append(found, links...)
				}
			}()
		}
		// Ждём всех запущенных — даже при отмене: иначе горутины утекут и
		// продолжат писать в res после возврата.
		wg.Wait()
		if err := ctx.Err(); err != nil {
			return res, err
		}

		// Дедупликация — в одной горутине. Сортировка делает порядок
		// обхода следующего уровня воспроизводимым.
		var next []string
		for _, l := range found {
			l = Normalize(l)
			if l == "" || seen[l] {
				continue
			}
			seen[l] = true
			next = append(next, l)
		}
		slices.Sort(next)
		level = next
	}
	return res, nil
}

type cacheEntry struct {
	links   []string
	expires time.Time
}

// flight — выполняющаяся загрузка одного URL.
type flight struct {
	done  chan struct{} // закрывается после записи links/err
	links []string
	err   error
}

// CachedFetcher — TTL-кэш + singleflight над Fetcher.
type CachedFetcher struct {
	Fetcher Fetcher
	TTL     time.Duration
	Now     func() time.Time

	mu       sync.Mutex
	cache    map[string]cacheEntry
	inflight map[string]*flight
}

func (c *CachedFetcher) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Fetch возвращает ссылки из кэша или загружает их (один раз на все
// одновременные запросы одного URL).
func (c *CachedFetcher) Fetch(ctx context.Context, url string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.cache == nil {
		c.cache = map[string]cacheEntry{}
		c.inflight = map[string]*flight{}
	}
	if e, ok := c.cache[url]; ok {
		if c.now().Before(e.expires) {
			c.mu.Unlock()
			// Копия: вызывающий не должен иметь возможности испортить кэш.
			return slices.Clone(e.links), nil
		}
		delete(c.cache, url) // протухла
	}
	fl, ok := c.inflight[url]
	if !ok {
		fl = &flight{done: make(chan struct{})}
		c.inflight[url] = fl
		// Общая загрузка не должна отменяться, если ушёл именно тот вызывающий,
		// который её начал: остальные ждут результат. WithoutCancel сохраняет
		// значения контекста, но отвязывает отмену.
		go c.load(context.WithoutCancel(ctx), url, fl)
	}
	c.mu.Unlock()

	select {
	case <-fl.done:
		return slices.Clone(fl.links), fl.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *CachedFetcher) load(ctx context.Context, url string, fl *flight) {
	links, err := c.Fetcher.Fetch(ctx, url)

	c.mu.Lock()
	delete(c.inflight, url)
	if err == nil && c.TTL > 0 { // ошибки не кэшируем
		c.cache[url] = cacheEntry{links: links, expires: c.now().Add(c.TTL)}
	}
	c.mu.Unlock()

	fl.links, fl.err = links, err
	close(fl.done) // happens-before для всех, кто ждёт <-fl.done
}
