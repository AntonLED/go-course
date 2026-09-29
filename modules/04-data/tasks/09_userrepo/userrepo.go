//go:build !solution

package userrepo

import "context"

// Create вставляет пользователя и записывает присвоенный ID в u.ID.
// Невалидные данные → ErrInvalid, занятый email → ErrDuplicateEmail
// (подсказка: errors.Is(err, memsql.ErrUniqueViolation)).
func (r *Repo) Create(ctx context.Context, u *User) error {
	panic("TODO")
}

// Get возвращает пользователя по ID или ErrNotFound.
func (r *Repo) Get(ctx context.Context, id int64) (User, error) {
	panic("TODO")
}

// List возвращает пользователей по возрастанию ID. limit <= 0 → 100.
// Не забудьте rows.Close() и rows.Err().
func (r *Repo) List(ctx context.Context, limit, offset int) ([]User, error) {
	panic("TODO")
}

// Update обновляет все поля пользователя u.ID. Нет такого → ErrNotFound.
func (r *Repo) Update(ctx context.Context, u User) error {
	panic("TODO")
}

// Delete удаляет пользователя. Нет такого → ErrNotFound.
func (r *Repo) Delete(ctx context.Context, id int64) error {
	panic("TODO")
}

// OpenAccount открывает счёт пользователю с начальным балансом (>= 0)
// и возвращает ID счёта. Нет пользователя → ErrNotFound.
func (r *Repo) OpenAccount(ctx context.Context, userID, initial int64) (int64, error) {
	panic("TODO")
}

// Balance возвращает баланс счёта или ErrNotFound.
func (r *Repo) Balance(ctx context.Context, accountID int64) (int64, error) {
	panic("TODO")
}

// Transfer переводит amount (> 0) со счёта from на счёт to (from != to)
// в одной транзакции: при любой ошибке ничего не должно измениться.
// Ошибки: ErrInvalid, ErrNotFound, ErrInsufficientFunds.
//
// Шаблон:
//
//	tx, err := r.db.BeginTx(ctx, nil)
//	if err != nil { return err }
//	defer tx.Rollback() // после Commit — безвредный no-op (sql.ErrTxDone)
//	... все запросы — через tx, а НЕ через r.db ...
//	return tx.Commit()
func (r *Repo) Transfer(ctx context.Context, from, to, amount int64) error {
	panic("TODO")
}
