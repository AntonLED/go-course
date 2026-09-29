package weather

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeAPI — фейковый погодный сервер: реальный HTTP на localhost, поведение по городу.
func fakeAPI(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/current", func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		if r.Header.Get("X-API-Key") != "k3y" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Accept") != "application/json" {
			http.Error(w, "need Accept", http.StatusNotAcceptable)
			return
		}
		if r.URL.Query().Get("units") != "metric" {
			http.Error(w, `{"error":"units required"}`, http.StatusBadRequest)
			return
		}
		switch city := r.URL.Query().Get("city"); city {
		case "Москва":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"city":"Москва","temp_c":-3.5,"condition":"snow","updated_at":"2024-01-15T09:00:00Z","humidity":80}`)
		case "London":
			fmt.Fprint(w, `{"city":"London","temp_c":11,"condition":"drizzle","updated_at":"2024-01-15T09:00:00Z"}`)
		case "New York & Co":
			fmt.Fprint(w, `{"city":"New York & Co","temp_c":5,"condition":"clear","updated_at":"2024-01-15T09:00:00Z"}`)
		case "Atlantis":
			w.WriteHeader(http.StatusNotFound)
		case "Busy":
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
		case "Busy2":
			w.WriteHeader(http.StatusTooManyRequests)
		case "Forbidden":
			w.WriteHeader(http.StatusForbidden)
		case "Broken":
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"db is down"}`)
		case "BrokenText":
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprint(w, "  upstream exploded\n")
		case "Garbage":
			fmt.Fprint(w, `{"city": "Garb`)
		case "Slow":
			select {
			case <-r.Context().Done():
			case <-time.After(3 * time.Second):
			}
		default:
			t.Errorf("неожиданный город %q (экранирование query?)", city)
			w.WriteHeader(http.StatusTeapot)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCurrentOK(t *testing.T) {
	srv := fakeAPI(t, nil)
	c := &Client{BaseURL: srv.URL + "/", APIKey: "k3y", HTTP: srv.Client()}
	w, err := c.Current(context.Background(), "  Москва ")
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	want := Weather{City: "Москва", TempC: -3.5, Condition: "snow", UpdatedAt: time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)}
	if w.City != want.City || w.TempC != want.TempC || w.Condition != want.Condition || !w.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("получено %+v, ожидалось %+v", w, want)
	}
	if w, err := c.Current(context.Background(), "New York & Co"); err != nil || w.Condition != "clear" {
		t.Errorf("город со спецсимволами: %+v %v (url.QueryEscape / url.Values)", w, err)
	}
	c.HTTP = nil // клиент по умолчанию тоже должен работать
	if _, err := c.Current(context.Background(), "London"); err != nil {
		t.Errorf("HTTP == nil: %v", err)
	}
}

func TestCurrentErrors(t *testing.T) {
	var hits atomic.Int32
	srv := fakeAPI(t, &hits)
	c := &Client{BaseURL: srv.URL, APIKey: "k3y", HTTP: srv.Client()}
	ctx := context.Background()

	if _, err := c.Current(ctx, "   "); !errors.Is(err, ErrEmptyCity) {
		t.Errorf("пустой город: %v", err)
	}
	if hits.Load() != 0 {
		t.Error("для пустого города не должно быть HTTP-запроса")
	}
	tests := []struct {
		city string
		want error
	}{
		{"Atlantis", ErrCityNotFound},
		{"Forbidden", ErrUnauthorized},
		{"Busy", ErrRateLimited},
	}
	for _, tt := range tests {
		if _, err := c.Current(ctx, tt.city); !errors.Is(err, tt.want) {
			t.Errorf("%s: ошибка %v, ожидалась %v", tt.city, err, tt.want)
		}
	}
	bad := &Client{BaseURL: srv.URL, APIKey: "wrong", HTTP: srv.Client()}
	if _, err := bad.Current(ctx, "Москва"); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("неверный ключ: %v", err)
	}

	_, err := c.Current(ctx, "Busy")
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.RetryAfter != 30*time.Second {
		t.Errorf("429 с Retry-After: 30 → RateLimitError{30s}, получено %v", err)
	}
	_, err = c.Current(ctx, "Busy2")
	if !errors.As(err, &rl) || rl.RetryAfter != 0 {
		t.Errorf("429 без Retry-After → RetryAfter=0, получено %v", err)
	}

	var ae *APIError
	_, err = c.Current(ctx, "Broken")
	if !errors.As(err, &ae) || ae.Status != 500 || ae.Message != "db is down" {
		t.Errorf("500 с JSON-ошибкой: %#v", err)
	}
	_, err = c.Current(ctx, "BrokenText")
	if !errors.As(err, &ae) || ae.Status != 502 || ae.Message != "upstream exploded" {
		t.Errorf("502 с текстом: %#v", err)
	}
	if _, err = c.Current(ctx, "Garbage"); err == nil {
		t.Error("битый JSON должен давать ошибку")
	}
}

func TestCurrentContext(t *testing.T) {
	srv := fakeAPI(t, nil)
	c := &Client{BaseURL: srv.URL, APIKey: "k3y", HTTP: srv.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Current(ctx, "Slow")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("ожидался context.DeadlineExceeded, получено %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("клиент не уважает контекст (NewRequestWithContext)")
	}
}

// --- тест без сети: RoundTripper-фейк проверяет, что тело всегда закрывается ---

type closeTracker struct {
	io.Reader
	closed *atomic.Int32
}

func (c closeTracker) Close() error { c.closed.Add(1); return nil }

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBodyAlwaysClosed(t *testing.T) {
	for _, code := range []int{200, 404, 401, 429, 500, 503} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			var closed atomic.Int32
			var gotURL string
			hc := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
				gotURL = r.URL.String()
				return &http.Response{
					StatusCode: code,
					Header:     http.Header{},
					Body:       closeTracker{strings.NewReader(`{"city":"X","temp_c":1,"condition":"rain","updated_at":"2024-01-01T00:00:00Z"}`), &closed},
					Request:    r,
				}, nil
			})}
			c := &Client{BaseURL: "http://weather.test", APIKey: "k", HTTP: hc}
			_, _ = c.Current(context.Background(), "Санкт-Петербург")
			if closed.Load() != 1 {
				t.Errorf("тело ответа %d закрыто %d раз, ожидался 1", code, closed.Load())
			}
			want := "http://weather.test/v1/current?city=%D0%A1%D0%B0%D0%BD%D0%BA%D1%82-%D0%9F%D0%B5%D1%82%D0%B5%D1%80%D0%B1%D1%83%D1%80%D0%B3&units=metric"
			if gotURL != want {
				t.Errorf("URL = %s\nожидался %s", gotURL, want)
			}
		})
	}
}

// --- ручной фейк Provider для потребителя ---

type fakeProvider struct {
	w     Weather
	err   error
	calls []string
}

func (f *fakeProvider) Current(_ context.Context, city string) (Weather, error) {
	f.calls = append(f.calls, city)
	return f.w, f.err
}

func TestUmbrella(t *testing.T) {
	tests := []struct {
		cond string
		want bool
	}{{"rain", true}, {"drizzle", true}, {"thunderstorm", true}, {"clear", false}, {"snow", false}, {"", false}}
	for _, tt := range tests {
		p := &fakeProvider{w: Weather{Condition: tt.cond}}
		got, err := Umbrella(context.Background(), p, "Казань")
		if err != nil || got != tt.want {
			t.Errorf("Umbrella при %q = %v, %v; ожидалось %v", tt.cond, got, err, tt.want)
		}
		if len(p.calls) != 1 || p.calls[0] != "Казань" {
			t.Errorf("Provider вызван с %v", p.calls)
		}
	}
	p := &fakeProvider{err: ErrCityNotFound}
	if _, err := Umbrella(context.Background(), p, "Atlantis"); !errors.Is(err, ErrCityNotFound) {
		t.Errorf("ошибка провайдера должна оборачиваться: %v", err)
	}
	// Реальный клиент — тоже Provider.
	var _ Provider = (*Client)(nil)
}
