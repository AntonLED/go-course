package weather

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Weather — текущая погода.
type Weather struct {
	City      string
	TempC     float64
	Condition string // "clear", "clouds", "rain", "snow", ...
	UpdatedAt time.Time
}

var (
	ErrEmptyCity    = errors.New("weather: empty city")
	ErrCityNotFound = errors.New("weather: city not found")
	ErrUnauthorized = errors.New("weather: unauthorized")
	ErrRateLimited  = errors.New("weather: rate limited")
)

// RateLimitError — ответ 429. errors.Is(err, ErrRateLimited) == true.
type RateLimitError struct {
	RetryAfter time.Duration // 0, если сервер не сообщил
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("weather: rate limited, retry after %v", e.RetryAfter)
}

func (e *RateLimitError) Is(target error) bool { return target == ErrRateLimited }

// APIError — любой другой неуспешный ответ.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("weather: api error %d: %s", e.Status, e.Message)
}

// Client — клиент погодного API.
type Client struct {
	BaseURL string       // например, "https://api.weather.example"
	APIKey  string       // заголовок X-API-Key
	HTTP    *http.Client // nil → клиент с таймаутом 10s
}

// Provider — то, что нужно потребителю (Umbrella) от источника погоды.
// Интерфейс объявлен у потребителя и содержит ровно один метод — его легко подменить фейком.
type Provider interface {
	Current(ctx context.Context, city string) (Weather, error)
}
