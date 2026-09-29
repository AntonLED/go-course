//go:build !solution

package di

import "context"

// Container — мини DI-контейнер в духе uber/dig: конструкторы регистрируются
// через Provide, граф разрешается лениво при Invoke, каждый тип — синглтон.
type Container struct {
	// TODO: провайдеры по reflect.Type, кэш построенных значений, мьютекс,
	// *Lifecycle.
}

// New создаёт пустой контейнер. *Lifecycle доступен как зависимость сразу.
func New() *Container {
	panic("TODO")
}

// Provide регистрирует конструктор вида
//
//	func(dep1 A, dep2 B, ...) T
//	func(dep1 A, dep2 B, ...) (T, error)
//
// Конструктор не вызывается сразу — только когда T понадобится.
func (c *Container) Provide(ctor any, opts ...Option) error {
	panic("TODO")
}

// Invoke разрешает аргументы fn из контейнера и вызывает её.
// fn может ничего не возвращать или возвращать error.
func (c *Container) Invoke(fn any) error {
	panic("TODO")
}

// Start вызывает OnStart зарегистрированных хуков в порядке добавления.
// При ошибке откатывает уже запущенные (OnStop в обратном порядке).
func (c *Container) Start(ctx context.Context) error {
	panic("TODO")
}

// Stop вызывает OnStop запущенных хуков в обратном порядке, собирая все ошибки.
func (c *Container) Stop(ctx context.Context) error {
	panic("TODO")
}
