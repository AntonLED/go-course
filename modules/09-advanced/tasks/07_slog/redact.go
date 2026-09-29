//go:build !solution

package redact

import (
	"context"
	"io"
	"log/slog"
)

// Handler — slog.Handler, пишущий JSON-строки и маскирующий секреты.
type Handler struct {
	// TODO: writer, общий *sync.Mutex, опции, накопленные WithGroup/WithAttrs.
}

var _ slog.Handler = (*Handler)(nil)

// NewHandler создаёт Handler. opts может быть nil.
func NewHandler(w io.Writer, opts *Options) *Handler {
	panic("TODO")
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	panic("TODO")
}

func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	panic("TODO")
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	panic("TODO")
}

func (h *Handler) WithGroup(name string) slog.Handler {
	panic("TODO")
}
