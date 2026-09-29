//go:build !solution

// Package crawler — конкурентный обходчик ссылок (Задание 3).
package crawler

import (
	"context"
	"time"
)

// Normalize приводит URL к каноническому виду для дедупликации:
// обрезает пробелы по краям и отбрасывает фрагмент ("#...").
// "a/b#top" → "a/b".
func Normalize(url string) string {
	// TODO
	panic("TODO")
}

// Crawl обходит граф ссылок в ширину, начиная со start, уровень за уровнем.
// См. README.md — там полная спецификация.
func Crawl(ctx context.Context, f Fetcher, start string, opt Options) (*Result, error) {
	// TODO
	panic("TODO")
}

// CachedFetcher — обёртка над Fetcher с TTL-кэшем успешных результатов и
// дедупликацией одновременных запросов одного URL (singleflight).
// Нулевое значение с заполненным полем Fetcher готово к работе.
type CachedFetcher struct {
	Fetcher Fetcher
	TTL     time.Duration    // <= 0 — не кэшировать (дедупликация in-flight остаётся)
	Now     func() time.Time // источник времени; nil — time.Now

	// TODO: приватные поля (мьютекс, кэш, in-flight вызовы)
}

// Fetch возвращает ссылки из кэша или загружает их через c.Fetcher.
func (c *CachedFetcher) Fetch(ctx context.Context, url string) ([]string, error) {
	// TODO
	panic("TODO")
}
