package memsql_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gocourse/modules/04-data/memsql"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := memsql.NewDSN(t.Name())
	db, err := sql.Open("memsql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); memsql.Drop(dsn) })
	if err := db.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	return db
}

func mustExec(t *testing.T, db interface {
	Exec(string, ...any) (sql.Result, error)
}, q string, args ...any) sql.Result {
	t.Helper()
	res, err := db.Exec(q, args...)
	if err != nil {
		t.Fatalf("Exec(%q): %v", q, err)
	}
	return res
}

const usersDDL = `CREATE TABLE users (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	email VARCHAR(255) UNIQUE,
	age INTEGER,
	score REAL,
	active BOOLEAN
)`

func seedUsers(t *testing.T, db *sql.DB) {
	t.Helper()
	mustExec(t, db, usersDDL)
	mustExec(t, db, `INSERT INTO users (name, email, age, score, active) VALUES (?, ?, ?, ?, ?)`, "Анна", "anna@x.io", 30, 4.5, true)
	mustExec(t, db, `INSERT INTO users (name, email, age, score, active) VALUES (?, ?, ?, ?, ?)`, "Борис", nil, 25, 3.0, false)
	mustExec(t, db, `INSERT INTO users (name, email, age, score, active) VALUES (?, ?, ?, ?, ?), (?, ?, ?, ?, ?)`,
		"Вера", "vera@x.io", nil, 5, true,
		"Глеб", "gleb@x.io", 30, nil, nil)
}

func TestCRUD(t *testing.T) {
	db := openDB(t)
	seedUsers(t, db)

	var name string
	var email sql.NullString
	var age sql.NullInt64
	var score sql.NullFloat64
	var active sql.NullBool
	err := db.QueryRow(`SELECT name, email, age, score, active FROM users WHERE id = ?`, 2).
		Scan(&name, &email, &age, &score, &active)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Борис" || email.Valid || !age.Valid || age.Int64 != 25 || score.Float64 != 3 || !active.Valid || active.Bool {
		t.Errorf("получено %q %v %v %v %v", name, email, age, score, active)
	}

	// REAL-колонка принимает int, INTEGER PRIMARY KEY автоинкрементируется.
	var id int64
	var sc float64
	if err := db.QueryRow(`SELECT id, score FROM users WHERE name = 'Вера'`).Scan(&id, &sc); err != nil {
		t.Fatal(err)
	}
	if id != 3 || sc != 5 {
		t.Errorf("Вера: id=%d score=%v, ожидалось 3 и 5", id, sc)
	}

	err = db.QueryRow(`SELECT name FROM users WHERE id = ?`, 100).Scan(&name)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("ожидалось sql.ErrNoRows, получено %v", err)
	}

	res := mustExec(t, db, `UPDATE users SET age = age + ?, score = 1.5 WHERE age = ?`, 1, 30)
	if n, _ := res.RowsAffected(); n != 2 {
		t.Errorf("UPDATE затронул %d строк, ожидалось 2", n)
	}
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE age = 31 AND score = ?`, 1.5).Scan(&cnt); err != nil || cnt != 2 {
		t.Errorf("COUNT = %d, %v; ожидалось 2", cnt, err)
	}

	res = mustExec(t, db, `DELETE FROM users WHERE email IS NULL`)
	if n, _ := res.RowsAffected(); n != 1 {
		t.Errorf("DELETE затронул %d строк, ожидалось 1", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&cnt); err != nil || cnt != 3 {
		t.Errorf("COUNT после DELETE = %d, %v; ожидалось 3", cnt, err)
	}
}

func TestSelectOrderLimit(t *testing.T) {
	db := openDB(t)
	seedUsers(t, db)
	tests := []struct {
		q    string
		args []any
		want string
	}{
		{`SELECT name FROM users ORDER BY name DESC`, nil, "Глеб,Вера,Борис,Анна"},
		{`SELECT name FROM users ORDER BY age, name`, nil, "Вера,Борис,Анна,Глеб"},        // NULL первым
		{`SELECT name FROM users ORDER BY age DESC, name DESC LIMIT 2`, nil, "Глеб,Анна"}, // NULL последним
		{`SELECT name FROM users ORDER BY id LIMIT ? OFFSET ?`, []any{2, 1}, "Борис,Вера"},
		{`SELECT name FROM users WHERE age >= ? AND age < 31 ORDER BY id`, []any{26}, "Анна,Глеб"},
		{`SELECT name FROM users WHERE age != 30 ORDER BY id`, nil, "Борис"}, // NULL не проходит !=
		{`SELECT name FROM users WHERE active = TRUE ORDER BY id`, nil, "Анна,Вера"},
		{`SELECT name FROM users WHERE score IS NOT NULL AND id <> $1 ORDER BY id`, []any{1}, "Борис,Вера"},
		{`select name from users where name = 'Анна'`, nil, "Анна"},
		{`SELECT name FROM users LIMIT 0`, nil, ""},
		{`SELECT name FROM users ORDER BY id OFFSET 10`, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.q, func(t *testing.T) {
			rows, err := db.Query(tt.q, tt.args...)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			got := ""
			for rows.Next() {
				var s string
				if err := rows.Scan(&s); err != nil {
					t.Fatal(err)
				}
				if got != "" {
					got += ","
				}
				got += s
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("получено %q, ожидалось %q", got, tt.want)
			}
		})
	}
}

func TestSelectStar(t *testing.T) {
	db := openDB(t)
	seedUsers(t, db)
	rows, err := db.Query(`SELECT * FROM users WHERE id = $1`, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	if fmt.Sprint(cols) != "[id name email age score active]" {
		t.Errorf("Columns() = %v", cols)
	}
	if !rows.Next() {
		t.Fatal("нет строк")
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(vals) != "[1 Анна anna@x.io 30 4.5 true]" {
		t.Errorf("строка = %v", vals)
	}
}

func TestConstraints(t *testing.T) {
	db := openDB(t)
	seedUsers(t, db)
	_, err := db.Exec(`INSERT INTO users (id, name) VALUES (?, ?)`, 1, "дубль")
	if !errors.Is(err, memsql.ErrUniqueViolation) {
		t.Errorf("дубль PK: ожидалось ErrUniqueViolation, получено %v", err)
	}
	_, err = db.Exec(`INSERT INTO users (name, email) VALUES (?, ?)`, "x", "anna@x.io")
	if !errors.Is(err, memsql.ErrUniqueViolation) {
		t.Errorf("дубль UNIQUE: ожидалось ErrUniqueViolation, получено %v", err)
	}
	_, err = db.Exec(`INSERT INTO users (email) VALUES (?)`, "new@x.io")
	if !errors.Is(err, memsql.ErrNotNullViolation) {
		t.Errorf("NOT NULL: ожидалось ErrNotNullViolation, получено %v", err)
	}
	_, err = db.Exec(`UPDATE users SET email = ? WHERE id = 2`, "vera@x.io")
	if !errors.Is(err, memsql.ErrUniqueViolation) {
		t.Errorf("UPDATE в дубль: ожидалось ErrUniqueViolation, получено %v", err)
	}
	// Несколько NULL в UNIQUE-колонке допустимы.
	mustExec(t, db, `INSERT INTO users (name) VALUES ('a'), ('b')`)

	// Ошибочная многострочная вставка не должна оставить частичных данных.
	_, err = db.Exec(`INSERT INTO users (name, email) VALUES ('ok', 'uniq@x.io'), ('bad', 'uniq@x.io')`)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM users WHERE email = 'uniq@x.io'`).Scan(&n)
	if n != 0 {
		t.Errorf("после неудачного INSERT осталось %d строк", n)
	}
}

func TestErrors(t *testing.T) {
	db := openDB(t)
	mustExec(t, db, `CREATE TABLE t (id INTEGER PRIMARY KEY, s TEXT, b BOOL)`)
	mustExec(t, db, `INSERT INTO t (s, b) VALUES ('x', FALSE)`)
	bad := []struct {
		q    string
		args []any
	}{
		{`SELEC * FROM t`, nil},
		{`SELECT * FROM nope`, nil},
		{`SELECT nope FROM t`, nil},
		{`SELECT * FROM t WHERE id = ?`, nil},         // не хватает аргумента
		{`SELECT * FROM t WHERE id = ?`, []any{1, 2}}, // лишний
		{`SELECT * FROM t WHERE s = 'x' OR s = 'y'`, nil},
		{`SELECT * FROM t WHERE id = ? AND s = $1`, []any{1}},
		{`INSERT INTO t (id, s) VALUES (?, ?)`, []any{"not-int", "x"}},
		{`INSERT INTO t (b) VALUES (?)`, []any{1}},
		{`INSERT INTO t (s) VALUES ('unterminated)`, nil},
		{`CREATE TABLE t (x INTEGER)`, nil},
		{`SELECT * FROM t WHERE s = 1`, nil},
		{`SELECT * FROM t LIMIT -1`, nil},
	}
	for _, tt := range bad {
		var err error
		if strings.HasPrefix(tt.q, "SELECT") {
			var rows *sql.Rows
			if rows, err = db.Query(tt.q, tt.args...); err == nil {
				rows.Close()
			}
		} else {
			_, err = db.Exec(tt.q, tt.args...)
		}
		if err == nil {
			t.Errorf("%q %v: ожидалась ошибка", tt.q, tt.args)
		}
	}
	_, err := db.Exec(`SELECT * FROM t WHERE s = 'x'`)
	if err == nil {
		t.Error("Exec(SELECT) должен вернуть ошибку")
	}
	mustExec(t, db, `CREATE TABLE IF NOT EXISTS t (x INTEGER)`)
	mustExec(t, db, `DROP TABLE t`)
	if _, err := db.Exec(`DROP TABLE t`); !errors.Is(err, memsql.ErrNoTable) {
		t.Errorf("DROP несуществующей: %v", err)
	}
	mustExec(t, db, `DROP TABLE IF EXISTS t;`)
}

func TestLastInsertIDAndLiterals(t *testing.T) {
	db := openDB(t)
	mustExec(t, db, `CREATE TABLE t (id INTEGER PRIMARY KEY, s TEXT, f REAL)`)
	res := mustExec(t, db, `INSERT INTO t (id, s, f) VALUES (10, 'it''s', -2.5)`)
	if id, err := res.LastInsertId(); err != nil || id != 10 {
		t.Errorf("LastInsertId = %d, %v", id, err)
	}
	res = mustExec(t, db, `INSERT INTO t (s) VALUES (?)`, []byte("bytes"))
	if id, _ := res.LastInsertId(); id != 11 {
		t.Errorf("LastInsertId = %d, ожидалось 11", id)
	}
	var s string
	var f float64
	if err := db.QueryRow(`SELECT s, f FROM t WHERE id = 10`).Scan(&s, &f); err != nil || s != "it's" || f != -2.5 {
		t.Errorf("получено %q %v %v", s, f, err)
	}
	mustExec(t, db, `CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT)`)
	res = mustExec(t, db, `INSERT INTO kv VALUES ('a', 'b')`)
	if _, err := res.LastInsertId(); err == nil {
		t.Error("LastInsertId для TEXT PK должен вернуть ошибку")
	}
}

func TestTimestamp(t *testing.T) {
	db := openDB(t)
	mustExec(t, db, `CREATE TABLE ev (id INTEGER PRIMARY KEY, at TIMESTAMP)`)
	t0 := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	mustExec(t, db, `INSERT INTO ev (at) VALUES (?), (?)`, t0, t0.Add(time.Hour))
	var got time.Time
	if err := db.QueryRow(`SELECT at FROM ev WHERE at > ?`, t0).Scan(&got); err != nil || !got.Equal(t0.Add(time.Hour)) {
		t.Errorf("получено %v, %v", got, err)
	}
}

func TestPrepared(t *testing.T) {
	db := openDB(t)
	mustExec(t, db, `CREATE TABLE t (id INTEGER PRIMARY KEY, v INTEGER)`)
	ins, err := db.Prepare(`INSERT INTO t (v) VALUES (?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer ins.Close()
	for i := range 5 {
		if _, err := ins.Exec(i * 10); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ins.Exec(); err == nil {
		t.Error("ожидалась ошибка числа аргументов")
	}
	var sum int64
	sel, err := db.Prepare(`SELECT v FROM t WHERE v > ?`)
	if err != nil {
		t.Fatal(err)
	}
	defer sel.Close()
	rows, err := sel.Query(0)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var v int64
		rows.Scan(&v)
		sum += v
	}
	rows.Close()
	if sum != 100 {
		t.Errorf("сумма = %d, ожидалось 100", sum)
	}
}

func TestSeparateDSN(t *testing.T) {
	a := openDB(t)
	b := openDB(t)
	mustExec(t, a, `CREATE TABLE t (x INTEGER)`)
	if _, err := b.Exec(`INSERT INTO t VALUES (1)`); !errors.Is(err, memsql.ErrNoTable) {
		t.Errorf("разные DSN должны быть разными базами, получено %v", err)
	}
	// Одинаковый DSN — одна база.
	dsn := memsql.NewDSN("shared")
	defer memsql.Drop(dsn)
	c1, _ := sql.Open("memsql", dsn)
	c2, _ := sql.Open("memsql", dsn)
	defer c1.Close()
	defer c2.Close()
	mustExec(t, c1, `CREATE TABLE t (x INTEGER)`)
	mustExec(t, c2, `INSERT INTO t VALUES (1)`)
}

func TestTxCommitRollback(t *testing.T) {
	db := openDB(t)
	mustExec(t, db, `CREATE TABLE acc (id INTEGER PRIMARY KEY, bal INTEGER NOT NULL)`)
	mustExec(t, db, `INSERT INTO acc (bal) VALUES (100), (0)`)
	count := func(q *sql.DB) int64 {
		var s int64
		if err := q.QueryRow(`SELECT bal FROM acc WHERE id = 2`).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, tx, `UPDATE acc SET bal = bal - ? WHERE id = ?`, 30, 1)
	mustExec(t, tx, `UPDATE acc SET bal = bal + ? WHERE id = ?`, 30, 2)
	var inTx int64
	tx.QueryRow(`SELECT bal FROM acc WHERE id = 2`).Scan(&inTx)
	if inTx != 30 {
		t.Errorf("внутри транзакции bal = %d, ожидалось 30", inTx)
	}
	if got := count(db); got != 0 {
		t.Errorf("до Commit снаружи видно bal = %d (грязное чтение)", got)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := count(db); got != 30 {
		t.Errorf("после Commit bal = %d, ожидалось 30", got)
	}
	if err := tx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
		t.Errorf("Rollback после Commit: %v, ожидалось sql.ErrTxDone", err)
	}

	tx, _ = db.Begin()
	mustExec(t, tx, `UPDATE acc SET bal = 999 WHERE id = 2`)
	mustExec(t, tx, `CREATE TABLE tmp (x INTEGER)`)
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got := count(db); got != 30 {
		t.Errorf("после Rollback bal = %d, ожидалось 30", got)
	}
	if _, err := db.Exec(`INSERT INTO tmp VALUES (1)`); err == nil {
		t.Error("таблица из откатанной транзакции не должна существовать")
	}

	// Ошибка ограничения внутри транзакции не ломает транзакцию целиком.
	tx, _ = db.Begin()
	if _, err := tx.Exec(`INSERT INTO acc (id, bal) VALUES (1, 5)`); !errors.Is(err, memsql.ErrUniqueViolation) {
		t.Errorf("ожидалось нарушение PK, получено %v", err)
	}
	tx.Rollback()

	// READ ONLY.
	tx, err = db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`DELETE FROM acc`); err == nil {
		t.Error("запись в READ ONLY транзакции должна падать")
	}
	tx.Rollback()
}

func TestTxSerializationConflict(t *testing.T) {
	db := openDB(t)
	mustExec(t, db, `CREATE TABLE t (x INTEGER)`)
	tx, _ := db.Begin()
	mustExec(t, tx, `INSERT INTO t VALUES (1)`)
	mustExec(t, db, `INSERT INTO t VALUES (2)`) // автокоммит мимо транзакции
	if err := tx.Commit(); !errors.Is(err, memsql.ErrSerialization) {
		t.Errorf("ожидалось ErrSerialization, получено %v", err)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM t`).Scan(&n)
	if n != 1 {
		t.Errorf("COUNT = %d, ожидалось 1", n)
	}
}

func TestContext(t *testing.T) {
	db := openDB(t)
	mustExec(t, db, `CREATE TABLE t (x INTEGER)`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := db.ExecContext(ctx, `INSERT INTO t VALUES (1)`); !errors.Is(err, context.Canceled) {
		t.Errorf("ExecContext с отменённым ctx: %v", err)
	}
	// Вторая транзакция ждёт первую и уважает таймаут.
	tx, _ := db.Begin()
	defer tx.Rollback()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel2()
	if _, err := db.BeginTx(ctx2, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("BeginTx при занятой базе: %v, ожидалось DeadlineExceeded", err)
	}
}

func TestConcurrent(t *testing.T) {
	db := openDB(t)
	mustExec(t, db, `CREATE TABLE c (id INTEGER PRIMARY KEY, n INTEGER)`)
	mustExec(t, db, `INSERT INTO c (id, n) VALUES (1, 0)`)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 10 {
				tx, err := db.Begin()
				if err != nil {
					t.Error(err)
					return
				}
				if _, err := tx.Exec(`UPDATE c SET n = n + 1 WHERE id = 1`); err != nil {
					t.Error(err)
				}
				if err := tx.Commit(); err != nil {
					t.Error(err)
				}
			}
		}()
		go func() {
			defer wg.Done()
			var n int
			for range 10 {
				if err := db.QueryRow(`SELECT n FROM c WHERE id = 1`).Scan(&n); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	var n int
	db.QueryRow(`SELECT n FROM c WHERE id = 1`).Scan(&n)
	if n != 200 {
		t.Errorf("n = %d, ожидалось 200 (транзакции должны сериализоваться)", n)
	}
}
