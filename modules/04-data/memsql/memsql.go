// Package memsql — учебный in-memory драйвер для database/sql.
//
// Драйвер регистрируется под именем "memsql" в init, поэтому достаточно
// пустого импорта:
//
//	import _ "gocourse/modules/04-data/memsql"
//
//	db, err := sql.Open("memsql", "mydb")
//
// Один и тот же DSN внутри процесса — одна и та же база (все соединения пула
// видят общие данные), разные DSN — разные независимые базы. Для тестов
// удобно брать уникальное имя через NewDSN.
//
// Поддерживаемое подмножество SQL (ключевые слова — без учёта регистра,
// идентификаторы приводятся к нижнему регистру):
//
//	CREATE TABLE [IF NOT EXISTS] t (col TYPE [PRIMARY KEY] [NOT NULL] [UNIQUE], ...)
//	DROP TABLE [IF EXISTS] t
//	INSERT INTO t [(a, b)] VALUES (?, ?)[, (?, ?)...]
//	SELECT a, b | * | COUNT(*) FROM t [WHERE cond [AND cond ...]]
//	       [ORDER BY col [ASC|DESC], ...] [LIMIT n] [OFFSET n]
//	UPDATE t SET a = expr, b = expr [WHERE ...]
//	DELETE FROM t [WHERE ...]
//
// cond: `col op expr` (op: = != <> < <= > >=) или `col IS [NOT] NULL`.
// expr: `?`, `$N`, литерал (42, 1.5, 'text', TRUE, FALSE, NULL), имя колонки
// или `операнд +|- операнд` (например, `balance - ?`).
//
// Типы колонок: INTEGER/INT/BIGINT (int64), TEXT/VARCHAR(n) (string),
// REAL/FLOAT/DOUBLE (float64), BOOLEAN/BOOL (bool), TIMESTAMP/DATETIME (time.Time).
// Колонка INTEGER PRIMARY KEY при вставке NULL (или без значения) получает
// max+1 — как rowid в SQLite; это значение возвращает Result.LastInsertId.
//
// Транзакции: при Begin транзакция получает снимок таблиц (таблицы
// неизменяемы, запись идёт по принципу copy-on-write, поэтому снимок дешёвый).
// Изменения видны только внутри транзакции до Commit; Rollback их выбрасывает.
// Транзакции одной базы сериализуются (вторая BeginTx ждёт окончания первой
// или отмены контекста). Если таблицу, изменённую в транзакции, параллельно
// поменял автокоммит-запрос, Commit вернёт ErrSerialization.
package memsql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"maps"
	"sync"
	"sync/atomic"
)

func init() {
	sql.Register("memsql", &Driver{})
}

// Ошибки ограничений. Проверяйте через errors.Is: в реальном драйвере
// (pgx) аналогом будет *pgconn.PgError с Code "23505" (unique_violation).
var (
	ErrUniqueViolation  = errors.New("memsql: нарушено ограничение уникальности")
	ErrNotNullViolation = errors.New("memsql: нарушено ограничение NOT NULL")
	ErrSerialization    = errors.New("memsql: конфликт сериализации транзакции")
	ErrNoTable          = errors.New("memsql: таблица не существует")
)

// Driver реализует driver.Driver.
type Driver struct{}

var (
	regMu  sync.Mutex
	dbs    = map[string]*database{}
	dsnSeq atomic.Uint64
)

// Open открывает новое соединение с базой dsn (создаёт базу при первом обращении).
func (Driver) Open(dsn string) (driver.Conn, error) {
	regMu.Lock()
	defer regMu.Unlock()
	db, ok := dbs[dsn]
	if !ok {
		db = &database{
			tables:   map[string]*table{},
			versions: map[string]uint64{},
			txSem:    make(chan struct{}, 1),
		}
		dbs[dsn] = db
	}
	return &conn{db: db}, nil
}

// NewDSN возвращает уникальное имя базы вида "prefix-N". Удобно в тестах,
// чтобы -count=3 и параллельные тесты не делили данные.
func NewDSN(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, dsnSeq.Add(1))
}

// Drop забывает базу dsn. Уже открытые соединения продолжают работать со
// старыми данными, новые получат пустую базу.
func Drop(dsn string) {
	regMu.Lock()
	delete(dbs, dsn)
	regMu.Unlock()
}

type database struct {
	mu       sync.RWMutex
	tables   map[string]*table // опубликованные (неизменяемые) таблицы
	versions map[string]uint64 // счётчик изменений по имени таблицы
	txSem    chan struct{}     // сериализация транзакций
}

// changes — результат пишущей операции: новые версии таблиц (nil — удалить).
type changes map[string]*table

// --- соединение ---

type conn struct {
	db     *database
	tx     *tx
	closed bool
}

var (
	_ driver.Conn               = (*conn)(nil)
	_ driver.ConnBeginTx        = (*conn)(nil)
	_ driver.ConnPrepareContext = (*conn)(nil)
	_ driver.ExecerContext      = (*conn)(nil)
	_ driver.QueryerContext     = (*conn)(nil)
	_ driver.Pinger             = (*conn)(nil)
)

func (c *conn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

func (c *conn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	st, n, err := parse(query)
	if err != nil {
		return nil, err
	}
	return &stmt{c: c, st: st, numInput: n}, nil
}

func (c *conn) Close() error {
	if c.tx != nil {
		_ = c.tx.Rollback()
	}
	c.closed = true
	return nil
}

func (c *conn) Ping(ctx context.Context) error {
	if c.closed {
		return driver.ErrBadConn
	}
	return ctx.Err()
}

func (c *conn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *conn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if c.tx != nil {
		return nil, errors.New("memsql: транзакция уже открыта на этом соединении")
	}
	// Ждём своей очереди, уважая отмену контекста.
	select {
	case c.db.txSem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	c.db.mu.RLock()
	t := &tx{
		c:        c,
		tables:   maps.Clone(c.db.tables), // таблицы неизменяемы — копии указателей достаточно
		base:     maps.Clone(c.db.versions),
		dirty:    map[string]bool{},
		readOnly: opts.ReadOnly,
	}
	c.db.mu.RUnlock()
	c.tx = t
	return t, nil
}

func (c *conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	st, n, err := parse(query)
	if err != nil {
		return nil, err
	}
	vals, err := namedToValues(args, n)
	if err != nil {
		return nil, err
	}
	return c.exec(ctx, st, vals)
}

func (c *conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	st, n, err := parse(query)
	if err != nil {
		return nil, err
	}
	vals, err := namedToValues(args, n)
	if err != nil {
		return nil, err
	}
	return c.query(ctx, st, vals)
}

func namedToValues(args []driver.NamedValue, want int) ([]driver.Value, error) {
	if len(args) != want {
		return nil, fmt.Errorf("memsql: ожидалось %d аргументов, передано %d", want, len(args))
	}
	vals := make([]driver.Value, len(args))
	for i, a := range args {
		if a.Name != "" {
			return nil, fmt.Errorf("memsql: именованные параметры (%s) не поддерживаются", a.Name)
		}
		vals[i] = a.Value
	}
	return vals, nil
}

// exec выполняет пишущий (или DDL) оператор.
func (c *conn) exec(ctx context.Context, st statement, args []driver.Value) (driver.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.closed {
		return nil, driver.ErrBadConn
	}
	w, ok := st.(writer)
	if !ok {
		return nil, errors.New("memsql: для SELECT используйте Query")
	}
	if c.tx != nil {
		if c.tx.readOnly {
			return nil, errors.New("memsql: запись в READ ONLY транзакции")
		}
		ch, res, err := w.apply(c.tx.tables, args)
		if err != nil {
			return nil, err
		}
		for name, t := range ch {
			if t == nil {
				delete(c.tx.tables, name)
			} else {
				c.tx.tables[name] = t
			}
			c.tx.dirty[name] = true
		}
		return res, nil
	}
	db := c.db
	db.mu.Lock()
	defer db.mu.Unlock()
	ch, res, err := w.apply(db.tables, args)
	if err != nil {
		return nil, err
	}
	for name, t := range ch {
		if t == nil {
			delete(db.tables, name)
		} else {
			db.tables[name] = t
		}
		db.versions[name]++
	}
	return res, nil
}

func (c *conn) query(ctx context.Context, st statement, args []driver.Value) (driver.Rows, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.closed {
		return nil, driver.ErrBadConn
	}
	s, ok := st.(*selectStmt)
	if !ok {
		return nil, errors.New("memsql: Query поддерживает только SELECT, используйте Exec")
	}
	if c.tx != nil {
		return s.run(c.tx.tables, args)
	}
	c.db.mu.RLock()
	defer c.db.mu.RUnlock()
	return s.run(c.db.tables, args)
}

// --- транзакция ---

type tx struct {
	c        *conn
	tables   map[string]*table
	base     map[string]uint64
	dirty    map[string]bool
	readOnly bool
}

func (t *tx) finish() {
	t.c.tx = nil
	<-t.c.db.txSem
}

func (t *tx) Commit() error {
	if t.c.tx != t {
		return errors.New("memsql: транзакция уже завершена")
	}
	defer t.finish()
	db := t.c.db
	db.mu.Lock()
	defer db.mu.Unlock()
	for name := range t.dirty {
		if db.versions[name] != t.base[name] {
			return fmt.Errorf("%w: таблица %q изменена вне транзакции", ErrSerialization, name)
		}
	}
	for name := range t.dirty {
		if tb, ok := t.tables[name]; ok {
			db.tables[name] = tb
		} else {
			delete(db.tables, name)
		}
		db.versions[name]++
	}
	return nil
}

func (t *tx) Rollback() error {
	if t.c.tx != t {
		return errors.New("memsql: транзакция уже завершена")
	}
	t.finish()
	return nil
}

// --- подготовленный запрос ---

type stmt struct {
	c        *conn
	st       statement
	numInput int
}

var (
	_ driver.Stmt             = (*stmt)(nil)
	_ driver.StmtExecContext  = (*stmt)(nil)
	_ driver.StmtQueryContext = (*stmt)(nil)
)

func (s *stmt) Close() error  { return nil }
func (s *stmt) NumInput() int { return s.numInput }

func (s *stmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.c.exec(context.Background(), s.st, args)
}

func (s *stmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.c.query(context.Background(), s.st, args)
}

func (s *stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	vals, err := namedToValues(args, s.numInput)
	if err != nil {
		return nil, err
	}
	return s.c.exec(ctx, s.st, vals)
}

func (s *stmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	vals, err := namedToValues(args, s.numInput)
	if err != nil {
		return nil, err
	}
	return s.c.query(ctx, s.st, vals)
}

// --- результат и строки ---

type result struct {
	lastID   int64
	hasID    bool
	affected int64
}

func (r result) LastInsertId() (int64, error) {
	if !r.hasID {
		return 0, errors.New("memsql: LastInsertId доступен только для INSERT в таблицу с INTEGER PRIMARY KEY")
	}
	return r.lastID, nil
}

func (r result) RowsAffected() (int64, error) { return r.affected, nil }

type rows struct {
	cols []string
	data [][]driver.Value
	i    int
}

func (r *rows) Columns() []string { return r.cols }
func (r *rows) Close() error      { r.i = len(r.data); return nil }

func (r *rows) Next(dest []driver.Value) error {
	if r.i >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.i])
	r.i++
	return nil
}
