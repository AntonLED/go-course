//go:build solution

package di

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

var (
	errorType     = reflect.TypeFor[error]()
	lifecycleType = reflect.TypeFor[*Lifecycle]()
)

// provider — зарегистрированный конструктор.
type provider struct {
	fn   reflect.Value
	out  reflect.Type // тип, который строит конструктор
	errs bool         // второй результат — error
}

// Container — мини DI-контейнер в духе uber/dig.
type Container struct {
	mu        sync.Mutex
	providers map[reflect.Type]*provider
	values    map[reflect.Type]reflect.Value // синглтоны: уже построенные значения
	lc        *Lifecycle
	started   int // сколько хуков успешно запущено
}

// New создаёт пустой контейнер. *Lifecycle доступен как зависимость сразу.
func New() *Container {
	lc := &Lifecycle{}
	return &Container{
		providers: make(map[reflect.Type]*provider),
		values:    map[reflect.Type]reflect.Value{lifecycleType: reflect.ValueOf(lc)},
		lc:        lc,
	}
}

// Provide регистрирует конструктор. Проверяет сигнатуру сразу (fail fast),
// а вызывает конструктор лениво.
func (c *Container) Provide(ctor any, opts ...Option) error {
	fv := reflect.ValueOf(ctor)
	if fv.Kind() != reflect.Func || fv.IsNil() {
		return fmt.Errorf("%w: Provide(%T)", ErrNotFunc, ctor)
	}
	ft := fv.Type()
	if ft.IsVariadic() {
		return fmt.Errorf("%w: variadic-конструктор %s", ErrBadSignature, ft)
	}
	p := &provider{fn: fv}
	switch ft.NumOut() {
	case 1:
	case 2:
		if ft.Out(1) != errorType {
			return fmt.Errorf("%w: второй результат %s должен быть error", ErrBadSignature, ft)
		}
		p.errs = true
	default:
		return fmt.Errorf("%w: %s должен возвращать T или (T, error)", ErrBadSignature, ft)
	}
	p.out = ft.Out(0)
	if p.out == errorType {
		return fmt.Errorf("%w: конструктор не может строить error", ErrBadSignature)
	}

	var o provideOptions
	for _, opt := range opts {
		opt(&o)
	}
	// Все типы, под которыми регистрируем провайдер.
	types := []reflect.Type{p.out}
	for _, a := range o.as {
		it := reflect.TypeOf(a)
		if it == nil || it.Kind() != reflect.Pointer || it.Elem().Kind() != reflect.Interface {
			return fmt.Errorf("%w: As ожидает указатель на интерфейс, получено %T", ErrBadOption, a)
		}
		it = it.Elem()
		if !p.out.Implements(it) {
			return fmt.Errorf("%w: %s не реализует %s", ErrBadOption, p.out, it)
		}
		types = append(types, it)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range types {
		if _, ok := c.providers[t]; ok || t == lifecycleType {
			return fmt.Errorf("%w: %s", ErrDuplicate, t)
		}
	}
	// Регистрируем атомарно: либо все типы, либо ни одного.
	for _, t := range types {
		c.providers[t] = p
	}
	return nil
}

// Invoke разрешает аргументы fn из контейнера и вызывает её.
func (c *Container) Invoke(fn any) error {
	fv := reflect.ValueOf(fn)
	if fv.Kind() != reflect.Func || fv.IsNil() {
		return fmt.Errorf("%w: Invoke(%T)", ErrNotFunc, fn)
	}
	ft := fv.Type()
	if ft.NumOut() > 1 || (ft.NumOut() == 1 && ft.Out(0) != errorType) || ft.IsVariadic() {
		return fmt.Errorf("%w: Invoke(%s): функция должна возвращать ничего или error", ErrBadSignature, ft)
	}

	args, err := c.resolveArgs(ft, "Invoke")
	if err != nil {
		return err
	}
	// fn вызываем уже без блокировки: она может, например, стартовать сервер.
	out := fv.Call(args)
	if len(out) == 1 && !out[0].IsNil() {
		return out[0].Interface().(error)
	}
	return nil
}

func (c *Container) resolveArgs(ft reflect.Type, who string) ([]reflect.Value, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	args := make([]reflect.Value, ft.NumIn())
	for i := range args {
		v, err := c.resolve(ft.In(i), []reflect.Type{}, who)
		if err != nil {
			return nil, err
		}
		args[i] = v
	}
	return args, nil
}

// resolve строит значение типа t (вызывается под c.mu). path — стек типов,
// которые сейчас строятся: если t уже в нём, это цикл.
func (c *Container) resolve(t reflect.Type, path []reflect.Type, who string) (reflect.Value, error) {
	if v, ok := c.values[t]; ok {
		return v, nil // синглтон уже построен
	}
	for i, p := range path {
		if p == t {
			return reflect.Value{}, fmt.Errorf("%w: %s", ErrCycle, formatPath(append(path[i:], t)))
		}
	}
	p, ok := c.providers[t]
	if !ok {
		return reflect.Value{}, fmt.Errorf("%w для %s (требуется в %s)", ErrMissing, t, who)
	}
	// Значение могли уже построить под другим типом (As): тот же провайдер.
	if v, ok := c.values[p.out]; ok {
		c.values[t] = v
		return v, nil
	}

	path = append(path, t)
	ft := p.fn.Type()
	args := make([]reflect.Value, ft.NumIn())
	for i := range args {
		v, err := c.resolve(ft.In(i), path, p.out.String())
		if err != nil {
			return reflect.Value{}, err
		}
		args[i] = v
	}
	out := p.fn.Call(args)
	if p.errs && !out[1].IsNil() {
		// Ошибку не кэшируем: следующий Invoke попробует снова.
		return reflect.Value{}, fmt.Errorf("di: конструктор %s: %w", p.out, out[1].Interface().(error))
	}
	v := out[0]
	c.values[p.out] = v
	c.values[t] = v
	return v, nil
}

func formatPath(ts []reflect.Type) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = t.String()
	}
	return strings.Join(parts, " -> ")
}

// Start вызывает OnStart в порядке регистрации; при ошибке откатывает запущенные.
func (c *Container) Start(ctx context.Context) error {
	hooks := c.lc.snapshot()
	c.mu.Lock()
	from := c.started
	c.mu.Unlock()
	for i := from; i < len(hooks); i++ {
		if h := hooks[i].OnStart; h != nil {
			if err := h(ctx); err != nil {
				// Откат: останавливаем всё, что успели запустить.
				c.mu.Lock()
				c.started = i
				c.mu.Unlock()
				return errors.Join(fmt.Errorf("di: OnStart #%d: %w", i, err), c.Stop(ctx))
			}
		}
	}
	c.mu.Lock()
	c.started = len(hooks)
	c.mu.Unlock()
	return nil
}

// Stop вызывает OnStop запущенных хуков в обратном порядке и собирает ошибки.
func (c *Container) Stop(ctx context.Context) error {
	hooks := c.lc.snapshot()
	c.mu.Lock()
	n := c.started
	c.started = 0
	c.mu.Unlock()
	var errs []error
	for i := n - 1; i >= 0; i-- {
		if h := hooks[i].OnStop; h != nil {
			if err := h(ctx); err != nil {
				errs = append(errs, fmt.Errorf("di: OnStop #%d: %w", i, err))
			}
		}
	}
	return errors.Join(errs...)
}
