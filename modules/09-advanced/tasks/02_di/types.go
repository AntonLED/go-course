package di

import (
	"context"
	"errors"
	"sync"
)

// Сентинел-ошибки; конкретные ошибки оборачивают их (%w) и добавляют контекст.
var (
	ErrNotFunc      = errors.New("di: ожидается функция")
	ErrBadSignature = errors.New("di: неподдерживаемая сигнатура")
	ErrDuplicate    = errors.New("di: тип уже зарегистрирован")
	ErrBadOption    = errors.New("di: некорректная опция")
	ErrMissing      = errors.New("di: нет провайдера")
	ErrCycle        = errors.New("di: циклическая зависимость")
)

// Hook — пара функций жизненного цикла (как fx.Hook). Любая может быть nil.
type Hook struct {
	OnStart func(context.Context) error
	OnStop  func(context.Context) error
}

// Lifecycle доступен любому конструктору как зависимость *Lifecycle:
//
//	func NewServer(lc *di.Lifecycle, cfg Config) *Server {
//	    s := &Server{...}
//	    lc.Append(di.Hook{OnStart: s.Start, OnStop: s.Shutdown})
//	    return s
//	}
type Lifecycle struct {
	mu    sync.Mutex
	hooks []Hook
}

// Append регистрирует хук. Порядок регистрации = порядок OnStart.
func (l *Lifecycle) Append(h Hook) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hooks = append(l.hooks, h)
}

// snapshot возвращает копию списка хуков.
func (l *Lifecycle) snapshot() []Hook {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Hook(nil), l.hooks...)
}

// Option — функциональная опция для Provide.
type Option func(*provideOptions)

type provideOptions struct {
	as []any // указатели на интерфейсы: new(Repo), new(io.Writer)
}

// As дополнительно регистрирует результат конструктора под типом интерфейса.
// Аргумент — указатель на интерфейс: di.As(new(UserRepo)).
func As(ifacePtr any) Option {
	return func(o *provideOptions) { o.as = append(o.as, ifacePtr) }
}
