package crawler

import (
	"context"
	"time"
)

// Fetcher загружает страницу и возвращает ссылки на ней.
// Реализация обязана уважать ctx (в тестах используются фейковые реализации).
type Fetcher interface {
	Fetch(ctx context.Context, url string) (links []string, err error)
}

// FetcherFunc позволяет использовать обычную функцию как Fetcher.
type FetcherFunc func(ctx context.Context, url string) ([]string, error)

// Fetch вызывает f(ctx, url).
func (f FetcherFunc) Fetch(ctx context.Context, url string) ([]string, error) { return f(ctx, url) }

// Options — настройки обхода.
type Options struct {
	MaxDepth    int           // максимальная глубина (стартовая страница — 0); < 0 трактуется как 0
	Workers     int           // максимум одновременных вызовов Fetch; <= 0 трактуется как 1
	PageTimeout time.Duration // таймаут на один Fetch; 0 — без таймаута
}

// Result — итог обхода.
type Result struct {
	Pages  map[string]int   // успешно загруженные страницы: URL → минимальная глубина
	Errors map[string]error // страницы, загрузка которых завершилась ошибкой
}
