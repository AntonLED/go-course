//go:build !solution

package weather

import (
	"context"
	"errors"
)

// Current запрашивает GET {BaseURL}/v1/current?city=<city>&units=metric (подробности — в README).
func (c *Client) Current(ctx context.Context, city string) (Weather, error) {
	// TODO
	return Weather{}, errors.New("TODO")
}

// Umbrella отвечает, нужен ли зонт: да, если Condition — "rain", "drizzle" или "thunderstorm".
func Umbrella(ctx context.Context, p Provider, city string) (bool, error) {
	// TODO
	return false, errors.New("TODO")
}
