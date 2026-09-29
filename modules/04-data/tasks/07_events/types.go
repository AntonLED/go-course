// Package events — полиморфные JSON-события с дискриминатором "type",
// собственный маршалинг Duration и потоковый разбор большого массива.
package events

import (
	"errors"
	"time"
)

// Ошибки разбора.
var (
	ErrUnknownType = errors.New("events: неизвестный тип события")
	ErrNoType      = errors.New("events: нет поля type")
)

// Duration — time.Duration, которая в JSON выглядит как строка "1m30s".
// При чтении принимает и строку ("90s"), и число — количество секунд (1.5 → 1.5s).
type Duration time.Duration

// Event — общее для всех событий.
type Event interface {
	Kind() string // значение поля "type"
}

// Login — {"type":"login","user":"ann","at":"2024-05-01T10:00:00Z","ip":"10.0.0.1"}
type Login struct {
	User string    `json:"user"`
	At   time.Time `json:"at"`
	IP   string    `json:"ip,omitempty"`
}

// Purchase — {"type":"purchase","user":"bob","amount_cents":1999,"items":["book"]}
type Purchase struct {
	User   string   `json:"user"`
	Amount int64    `json:"amount_cents"`
	Items  []string `json:"items,omitempty"`
}

// Session — {"type":"session","user":"ann","length":"1h2m"}
type Session struct {
	User   string   `json:"user"`
	Length Duration `json:"length"`
}

func (Login) Kind() string    { return "login" }
func (Purchase) Kind() string { return "purchase" }
func (Session) Kind() string  { return "session" }
