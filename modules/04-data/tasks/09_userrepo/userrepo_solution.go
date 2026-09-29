//go:build solution

package userrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"gocourse/modules/04-data/memsql"
)

func validate(u *User) error {
	switch {
	case strings.TrimSpace(u.Name) == "":
		return fmt.Errorf("%w: пустое имя", ErrInvalid)
	case !strings.Contains(u.Email, "@"):
		return fmt.Errorf("%w: email %q", ErrInvalid, u.Email)
	case u.Age != nil && *u.Age < 0:
		return fmt.Errorf("%w: возраст %d", ErrInvalid, *u.Age)
	}
	return nil
}

// mapWriteErr переводит ошибку драйвера в доменную.
// В PostgreSQL: var pgErr *pgconn.PgError; errors.As(err, &pgErr) && pgErr.Code == "23505".
func mapWriteErr(err error) error {
	if errors.Is(err, memsql.ErrUniqueViolation) {
		return fmt.Errorf("%w: %v", ErrDuplicateEmail, err)
	}
	return err
}

// Create вставляет пользователя и записывает присвоенный ID в u.ID.
func (r *Repo) Create(ctx context.Context, u *User) error {
	if err := validate(u); err != nil {
		return err
	}
	// Только плейсхолдеры: значения никогда не склеиваются в текст запроса.
	// sql.NullString и *int драйвер получает уже как string/nil и int64/nil
	// (driver.Valuer и стандартный конвертер database/sql).
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO users (name, email, phone, age) VALUES (?, ?, ?, ?)`,
		u.Name, u.Email, u.Phone, u.Age)
	if err != nil {
		return mapWriteErr(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	u.ID = id
	return nil
}

const userCols = `id, name, email, phone, age`

// scanner — общий интерфейс *sql.Row и *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

func scanUser(s scanner) (User, error) {
	var u User
	// &u.Age имеет тип **int: database/sql сам выставит nil для NULL
	// или выделит новый int.
	err := s.Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.Age)
	return u, err
}

// Get возвращает пользователя по ID или ErrNotFound.
func (r *Repo) Get(ctx context.Context, id int64) (User, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("%w: пользователь %d", ErrNotFound, id)
	}
	return u, err
}

// List возвращает пользователей по возрастанию ID.
func (r *Repo) List(ctx context.Context, limit, offset int) ([]User, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		return nil, fmt.Errorf("%w: offset %d", ErrInvalid, offset)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+userCols+` FROM users ORDER BY id LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	// Без Close соединение не вернётся в пул при досрочном выходе.
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	// Next() == false бывает и из-за ошибки посреди выборки.
	return users, rows.Err()
}

// Update обновляет все поля пользователя u.ID.
func (r *Repo) Update(ctx context.Context, u User) error {
	if err := validate(&u); err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET name = ?, email = ?, phone = ?, age = ? WHERE id = ?`,
		u.Name, u.Email, u.Phone, u.Age, u.ID)
	if err != nil {
		return mapWriteErr(err)
	}
	return affectedOne(res, "пользователь", u.ID)
}

// Delete удаляет пользователя.
func (r *Repo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return affectedOne(res, "пользователь", id)
}

func affectedOne(res sql.Result, what string, id int64) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %s %d", ErrNotFound, what, id)
	}
	return nil
}

// OpenAccount открывает счёт пользователю.
func (r *Repo) OpenAccount(ctx context.Context, userID, initial int64) (int64, error) {
	if initial < 0 {
		return 0, fmt.Errorf("%w: начальный баланс %d", ErrInvalid, initial)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id = ?`, userID).Scan(&n); err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, fmt.Errorf("%w: пользователь %d", ErrNotFound, userID)
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO accounts (user_id, balance) VALUES (?, ?)`, userID, initial)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// Balance возвращает баланс счёта.
func (r *Repo) Balance(ctx context.Context, accountID int64) (int64, error) {
	return balance(ctx, r.db, accountID)
}

// querier — общее у *sql.DB и *sql.Tx: одна функция работает и там, и там.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func balance(ctx context.Context, q querier, id int64) (int64, error) {
	var b int64
	err := q.QueryRowContext(ctx, `SELECT balance FROM accounts WHERE id = ?`, id).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: счёт %d", ErrNotFound, id)
	}
	return b, err
}

// Transfer переводит amount со счёта from на счёт to в одной транзакции.
func (r *Repo) Transfer(ctx context.Context, from, to, amount int64) error {
	if amount <= 0 {
		return fmt.Errorf("%w: сумма %d", ErrInvalid, amount)
	}
	if from == to {
		return fmt.Errorf("%w: перевод самому себе", ErrInvalid)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// Любой return до Commit откатит транзакцию. После успешного Commit
	// Rollback вернёт sql.ErrTxDone, который мы игнорируем.
	defer tx.Rollback()

	// В PostgreSQL здесь был бы SELECT ... FOR UPDATE (или атомарный
	// UPDATE ... SET balance = balance - $1 WHERE id = $2 AND balance >= $1),
	// иначе два параллельных перевода могут оба увидеть «старый» баланс.
	// memsql сериализует транзакции целиком.
	bal, err := balance(ctx, tx, from)
	if err != nil {
		return err
	}
	if _, err := balance(ctx, tx, to); err != nil {
		return err
	}
	if bal < amount {
		return fmt.Errorf("%w: на счёте %d есть %d, нужно %d", ErrInsufficientFunds, from, bal, amount)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET balance = balance - ? WHERE id = ?`, amount, from); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET balance = balance + ? WHERE id = ?`, amount, to); err != nil {
		return err
	}
	return tx.Commit()
}
