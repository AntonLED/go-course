//go:build solution

package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var defaultHTTP = &http.Client{Timeout: 10 * time.Second}

type currentDTO struct {
	City      string    `json:"city"`
	TempC     float64   `json:"temp_c"`
	Condition string    `json:"condition"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Current запрашивает текущую погоду.
func (c *Client) Current(ctx context.Context, city string) (Weather, error) {
	city = strings.TrimSpace(city)
	if city == "" {
		return Weather{}, ErrEmptyCity
	}
	q := url.Values{"city": {city}, "units": {"metric"}}
	u := strings.TrimRight(c.BaseURL, "/") + "/v1/current?" + q.Encode() // экранирование — через url.Values

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Weather{}, fmt.Errorf("weather: build request: %w", err)
	}
	req.Header.Set("X-API-Key", c.APIKey)
	req.Header.Set("Accept", "application/json")

	hc := c.HTTP
	if hc == nil {
		hc = defaultHTTP
	}
	resp, err := hc.Do(req)
	if err != nil {
		return Weather{}, fmt.Errorf("weather: %w", err) // *url.Error → errors.Is(ctx.Err()) работает
	}
	defer func() {
		// Дочитываем остаток, чтобы соединение вернулось в пул.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
	}()
	body := io.LimitReader(resp.Body, 1<<20)

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return Weather{}, ErrCityNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		return Weather{}, ErrUnauthorized
	case http.StatusTooManyRequests:
		e := &RateLimitError{}
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
			e.RetryAfter = time.Duration(s) * time.Second
		}
		return Weather{}, e
	default:
		return Weather{}, apiError(resp.StatusCode, body)
	}

	var dto currentDTO
	if err := json.NewDecoder(body).Decode(&dto); err != nil {
		return Weather{}, fmt.Errorf("weather: decode: %w", err)
	}
	return Weather{City: dto.City, TempC: dto.TempC, Condition: dto.Condition, UpdatedAt: dto.UpdatedAt}, nil
}

func apiError(status int, body io.Reader) error {
	b, _ := io.ReadAll(io.LimitReader(body, 4<<10))
	var e struct {
		Error string `json:"error"`
	}
	msg := strings.TrimSpace(string(b))
	if json.Unmarshal(b, &e) == nil && e.Error != "" {
		msg = e.Error
	}
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return &APIError{Status: status, Message: msg}
}

// Umbrella — потребитель, зависящий только от интерфейса Provider.
func Umbrella(ctx context.Context, p Provider, city string) (bool, error) {
	w, err := p.Current(ctx, city)
	if err != nil {
		return false, fmt.Errorf("umbrella for %q: %w", city, err)
	}
	switch w.Condition {
	case "rain", "drizzle", "thunderstorm":
		return true, nil
	}
	return false, nil
}
