// Package userrepo — репозиторий пользователей и счетов поверх database/sql.
//
// В тестах используется учебный драйвер memsql; с PostgreSQL (pgx/stdlib)
// код был бы тем же, кроме плейсхолдеров ($1 вместо ?) и получения ID
// (INSERT ... RETURNING id вместо LastInsertId).
package userrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Ошибки уровня предметной области. Наружу из репозитория не должны
// «протекать» sql.ErrNoRows и ошибки конкретного драйвера.
var (
	ErrNotFound          = errors.New("userrepo: не найдено")
	ErrDuplicateEmail    = errors.New("userrepo: email уже занят")
	ErrInvalid           = errors.New("userrepo: некорректные данные")
	ErrInsufficientFunds = errors.New("userrepo: недостаточно средств")
)

// User — пользователь. Phone и Age могут отсутствовать (NULL в БД):
// показаны оба способа — sql.NullString и указатель.
type User struct {
	ID    int64
	Name  string         // NOT NULL, непустое
	Email string         // NOT NULL UNIQUE, должен содержать '@'
	Phone sql.NullString // NULL ↔ Valid == false
	Age   *int           // NULL ↔ nil; если задан — >= 0
}

// Schema — миграции (memsql выполняет один оператор за Exec).
var Schema = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id    INTEGER PRIMARY KEY,
		name  TEXT NOT NULL,
		email TEXT NOT NULL UNIQUE,
		phone TEXT,
		age   INTEGER
	)`,
	`CREATE TABLE IF NOT EXISTS accounts (
		id      INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		balance INTEGER NOT NULL
	)`,
}

// Repo — репозиторий. *sql.DB — это пул соединений, безопасный для
// конкурентного использования; Repo хранит его, а не отдельное соединение.
type Repo struct {
	db *sql.DB
}

// New создаёт репозиторий.
func New(db *sql.DB) *Repo { return &Repo{db: db} }

// Migrate применяет Schema.
func (r *Repo) Migrate(ctx context.Context) error {
	for _, q := range Schema {
		if _, err := r.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}
