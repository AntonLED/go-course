//go:build !solution

package retry

import (
	"errors"
	"net/http"
)

// Do выполняет req с повторами (правила — в README):
//   - повторяются только идемпотентные запросы (или с Idempotency-Key) с перечитываемым телом;
//   - повод для повтора: сетевая ошибка (но не отмена ctx) и коды 429, 500, 502, 503, 504;
//   - задержка: Retry-After (для 429/503) или BaseDelay·2^(n-1), не больше MaxDelay;
//   - тело неудачного ответа дочитать и закрыть;
//   - на последней попытке ретраибельный ответ вернуть как есть (resp, nil).
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	// TODO
	return nil, errors.New("TODO")
}

// RoundTrip клонирует запрос, добавляет t.Header (если заголовка ещё нет),
// вызывает Base и пишет в Logf строку:
//
//	"GET http://host/path -> 200 (12ms)"  или  "GET http://host/path -> error: ..."
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// TODO
	return nil, errors.New("TODO")
}
