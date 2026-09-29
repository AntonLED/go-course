//go:build !solution

package wallet

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// ---------- Часть 1. Фейки ----------

// FakeClock — управляемые часы. Безопасны для конкурентного использования.
type FakeClock struct {
	// TODO
}

func NewFakeClock(t time.Time) *FakeClock    { return &FakeClock{} } // TODO
func (c *FakeClock) Now() time.Time          { return time.Time{} }  // TODO
func (c *FakeClock) Set(t time.Time)         {}                      // TODO
func (c *FakeClock) Advance(d time.Duration) {}                      // TODO

// MemStore — хранилище в памяти. Безопасно для конкурентного использования.
// Get возвращает ошибку, для которой errors.Is(err, ErrNotFound), если счёта нет.
// Если PutErr != nil, Put возвращает её и ничего не сохраняет.
type MemStore struct {
	PutErr error
	// TODO
}

func NewMemStore(accounts ...Account) *MemStore { return &MemStore{} } // TODO

func (s *MemStore) Get(ctx context.Context, id string) (Account, error) {
	return Account{}, errors.New("TODO")
}

func (s *MemStore) Put(ctx context.Context, a Account) error { return errors.New("TODO") }

// PutCount — сколько раз Put успешно сохранил счёт.
func (s *MemStore) PutCount() int { return 0 } // TODO

// SpyNotifier записывает каждый вызов Notify (даже если возвращает ошибку) и возвращает Err.
type SpyNotifier struct {
	Err error
	// TODO
}

func (n *SpyNotifier) Notify(ctx context.Context, msg Notification) error { return nil } // TODO

// Sent возвращает копию списка всех вызовов Notify по порядку.
func (n *SpyNotifier) Sent() []Notification { return nil } // TODO

// ---------- Часть 2. Тесты ----------

// CheckService проверяет реализацию Service по спецификации из service.go.
// newService получает зависимости (ваши фейки) и возвращает сервис.
// Проверка будет запущена на Wallet (ошибок быть не должно) и на мутантах (каждый должен быть пойман).
func CheckService(t testing.TB, newService func(Deps) Service) {
	t.Helper()
	// TODO
}

// ---------- Часть 3. Горячий путь ----------

// AppendStatement дописывает к dst строку выписки:
//
//	2024-03-05T10:00:00Z withdraw alice -12.34\n
//
// Время — в UTC, формат RFC 3339; сумма — копейки как рубли с двумя знаками после точки.
// Лимит: 0 аллокаций, если в dst хватает ёмкости.
func AppendStatement(dst []byte, e Entry) []byte {
	u := uint64(e.Amount)
	sign := ""
	if e.Amount < 0 {
		sign = "-"
		u = -u // беззнаковое отрицание корректно и для math.MinInt64
	}
	s := fmt.Sprintf("%s %s %s %s%d.%02d\n", e.Time.UTC().Format(time.RFC3339), e.Kind, e.AccountID, sign, u/100, u%100)
	return append(dst, s...)
}
