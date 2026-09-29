package memsql

import (
	"database/sql/driver"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

type colType int

const (
	typInt colType = iota
	typText
	typReal
	typBool
	typTime
)

func (t colType) String() string {
	return [...]string{"INTEGER", "TEXT", "REAL", "BOOLEAN", "TIMESTAMP"}[t]
}

type column struct {
	name                string
	typ                 colType
	pk, notNull, unique bool
}

// table неизменяема после публикации: любая запись создаёт новую *table
// (copy-on-write). Строки ([]driver.Value) тоже никогда не меняются на месте.
type table struct {
	name string
	cols []column
	idx  map[string]int
	rows [][]driver.Value
}

func (t *table) clone() *table {
	c := *t
	c.rows = slices.Clone(t.rows)
	return &c
}

func (t *table) colIndex(name string) (int, error) {
	i, ok := t.idx[name]
	if !ok {
		return 0, fmt.Errorf("memsql: в таблице %q нет колонки %q", t.name, name)
	}
	return i, nil
}

// autoPK возвращает индекс колонки INTEGER PRIMARY KEY или -1.
func (t *table) autoPK() int {
	for i, c := range t.cols {
		if c.pk && c.typ == typInt {
			return i
		}
	}
	return -1
}

func getTable(tables map[string]*table, name string) (*table, error) {
	t, ok := tables[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNoTable, name)
	}
	return t, nil
}

// --- значения ---

// coerce приводит значение к типу колонки.
func coerce(v driver.Value, c column) (driver.Value, error) {
	if v == nil {
		return nil, nil
	}
	bad := func() (driver.Value, error) {
		return nil, fmt.Errorf("memsql: колонка %q имеет тип %s, нельзя сохранить %T(%v)", c.name, c.typ, v, v)
	}
	switch c.typ {
	case typInt:
		switch x := v.(type) {
		case int64:
			return x, nil
		case float64:
			// Только целые значения из диапазона int64 (иначе int64(x) не определён).
			if x == math.Trunc(x) && x >= math.MinInt64 && x < math.MaxInt64 {
				return int64(x), nil
			}
		}
	case typReal:
		switch x := v.(type) {
		case float64:
			return x, nil
		case int64:
			return float64(x), nil
		}
	case typText:
		switch x := v.(type) {
		case string:
			return x, nil
		case []byte:
			return string(x), nil
		}
	case typBool:
		if x, ok := v.(bool); ok {
			return x, nil
		}
	case typTime:
		switch x := v.(type) {
		case time.Time:
			return x, nil
		case string:
			if tm, err := time.Parse(time.RFC3339Nano, x); err == nil {
				return tm, nil
			}
		}
	}
	return bad()
}

// compare сравнивает два не-NULL значения.
func compare(a, b driver.Value) (int, error) {
	switch x := a.(type) {
	case int64:
		switch y := b.(type) {
		case int64:
			return cmpOrdered(x, y), nil
		case float64:
			return cmpOrdered(float64(x), y), nil
		}
	case float64:
		switch y := b.(type) {
		case int64:
			return cmpOrdered(x, float64(y)), nil
		case float64:
			return cmpOrdered(x, y), nil
		}
	case string:
		switch y := b.(type) {
		case string:
			return strings.Compare(x, y), nil
		case []byte:
			return strings.Compare(x, string(y)), nil
		}
	case bool:
		if y, ok := b.(bool); ok {
			return cmpOrdered(b2i(x), b2i(y)), nil
		}
	case time.Time:
		if y, ok := b.(time.Time); ok {
			return x.Compare(y), nil
		}
	}
	return 0, fmt.Errorf("memsql: нельзя сравнить %T и %T", a, b)
}

func cmpOrdered[T int64 | float64 | int](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (o operand) eval(t *table, row []driver.Value, args []driver.Value) (driver.Value, error) {
	switch o.kind {
	case opLit:
		return o.val, nil
	case opParam:
		if o.param >= len(args) {
			return nil, fmt.Errorf("memsql: нет аргумента для параметра %d", o.param+1)
		}
		v := args[o.param]
		if b, ok := v.([]byte); ok {
			v = string(b)
		}
		return v, nil
	default:
		if t == nil || row == nil {
			return nil, fmt.Errorf("memsql: колонка %q здесь недопустима", o.col)
		}
		i, err := t.colIndex(o.col)
		if err != nil {
			return nil, err
		}
		return row[i], nil
	}
}

func (e expr) eval(t *table, row []driver.Value, args []driver.Value) (driver.Value, error) {
	l, err := e.left.eval(t, row, args)
	if err != nil || e.op == 0 {
		return l, err
	}
	r, err := e.right.eval(t, row, args)
	if err != nil {
		return nil, err
	}
	if l == nil || r == nil {
		return nil, nil // NULL + x = NULL
	}
	sign := int64(1)
	if e.op == '-' {
		sign = -1
	}
	switch x := l.(type) {
	case int64:
		switch y := r.(type) {
		case int64:
			return x + sign*y, nil
		case float64:
			return float64(x) + float64(sign)*y, nil
		}
	case float64:
		switch y := r.(type) {
		case int64:
			return x + float64(sign*y), nil
		case float64:
			return x + float64(sign)*y, nil
		}
	}
	return nil, fmt.Errorf("memsql: арифметика %c недопустима для %T и %T", e.op, l, r)
}

// match проверяет, удовлетворяет ли строка всем условиям WHERE.
func match(t *table, row []driver.Value, where []cond, args []driver.Value) (bool, error) {
	for _, c := range where {
		i, err := t.colIndex(c.col)
		if err != nil {
			return false, err
		}
		v := row[i]
		switch c.op {
		case "ISNULL":
			if v != nil {
				return false, nil
			}
			continue
		case "NOTNULL":
			if v == nil {
				return false, nil
			}
			continue
		}
		rhs, err := c.operand.eval(t, row, args)
		if err != nil {
			return false, err
		}
		if v == nil || rhs == nil {
			return false, nil // сравнение с NULL — не TRUE (трёхзначная логика SQL)
		}
		if t.cols[i].typ == typTime {
			if rhs, err = coerce(rhs, t.cols[i]); err != nil {
				return false, err
			}
		}
		cmp, err := compare(v, rhs)
		if err != nil {
			return false, fmt.Errorf("%w (колонка %q)", err, c.col)
		}
		ok := false
		switch c.op {
		case "=":
			ok = cmp == 0
		case "!=":
			ok = cmp != 0
		case "<":
			ok = cmp < 0
		case "<=":
			ok = cmp <= 0
		case ">":
			ok = cmp > 0
		case ">=":
			ok = cmp >= 0
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// validate проверяет NOT NULL и уникальность во всей таблице.
func validate(t *table) error {
	for ci, c := range t.cols {
		if c.notNull {
			for _, r := range t.rows {
				if r[ci] == nil {
					return fmt.Errorf("%w: %s.%s", ErrNotNullViolation, t.name, c.name)
				}
			}
		}
		if c.unique {
			seen := make(map[any]struct{}, len(t.rows))
			for _, r := range t.rows {
				v := r[ci]
				if v == nil {
					continue
				}
				if tm, ok := v.(time.Time); ok {
					v = tm.UnixNano()
				}
				if _, dup := seen[v]; dup {
					return fmt.Errorf("%w: %s.%s = %v", ErrUniqueViolation, t.name, c.name, r[ci])
				}
				seen[v] = struct{}{}
			}
		}
	}
	return nil
}

// --- выполнение операторов ---

func (s *createStmt) apply(tables map[string]*table, _ []driver.Value) (changes, driver.Result, error) {
	if _, ok := tables[s.name]; ok {
		if s.ifNotExists {
			return nil, result{}, nil
		}
		return nil, nil, fmt.Errorf("memsql: таблица %q уже существует", s.name)
	}
	t := &table{name: s.name, cols: s.cols, idx: map[string]int{}}
	for i, c := range s.cols {
		t.idx[c.name] = i
	}
	return changes{s.name: t}, result{}, nil
}

func (s *dropStmt) apply(tables map[string]*table, _ []driver.Value) (changes, driver.Result, error) {
	if _, ok := tables[s.name]; !ok {
		if s.ifExists {
			return nil, result{}, nil
		}
		return nil, nil, fmt.Errorf("%w: %q", ErrNoTable, s.name)
	}
	return changes{s.name: nil}, result{}, nil
}

func (s *insertStmt) apply(tables map[string]*table, args []driver.Value) (changes, driver.Result, error) {
	old, err := getTable(tables, s.table)
	if err != nil {
		return nil, nil, err
	}
	// Порядок колонок в VALUES -> индексы в строке таблицы.
	targets := make([]int, 0, len(old.cols))
	if s.cols == nil {
		for i := range old.cols {
			targets = append(targets, i)
		}
	} else {
		seen := map[int]bool{}
		for _, name := range s.cols {
			i, err := old.colIndex(name)
			if err != nil {
				return nil, nil, err
			}
			if seen[i] {
				return nil, nil, fmt.Errorf("memsql: колонка %q указана дважды", name)
			}
			seen[i] = true
			targets = append(targets, i)
		}
	}
	t := old.clone()
	pk := t.autoPK()
	var nextID int64 = 1
	if pk >= 0 {
		for _, r := range t.rows {
			if id, ok := r[pk].(int64); ok && id >= nextID {
				nextID = id + 1
			}
		}
	}
	res := result{hasID: pk >= 0}
	for _, vals := range s.rows {
		if len(vals) != len(targets) {
			return nil, nil, fmt.Errorf("memsql: INSERT: в таблице %q %d колонок, передано %d значений", t.name, len(targets), len(vals))
		}
		row := make([]driver.Value, len(t.cols))
		for j, e := range vals {
			v, err := e.eval(nil, nil, args)
			if err != nil {
				return nil, nil, err
			}
			ci := targets[j]
			if row[ci], err = coerce(v, t.cols[ci]); err != nil {
				return nil, nil, err
			}
		}
		if pk >= 0 {
			if row[pk] == nil {
				row[pk] = nextID
			}
			id := row[pk].(int64)
			nextID = max(nextID, id+1)
			res.lastID = id
		}
		t.rows = append(t.rows, row)
		res.affected++
	}
	if err := validate(t); err != nil {
		return nil, nil, err
	}
	return changes{t.name: t}, res, nil
}

func (s *updateStmt) apply(tables map[string]*table, args []driver.Value) (changes, driver.Result, error) {
	old, err := getTable(tables, s.table)
	if err != nil {
		return nil, nil, err
	}
	idx := make([]int, len(s.sets))
	for i, it := range s.sets {
		if idx[i], err = old.colIndex(it.col); err != nil {
			return nil, nil, err
		}
	}
	t := old.clone()
	var n int64
	for ri, row := range t.rows {
		ok, err := match(t, row, s.where, args)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			continue
		}
		nr := slices.Clone(row)
		for i, it := range s.sets {
			v, err := it.e.eval(t, row, args) // выражения видят старые значения строки
			if err != nil {
				return nil, nil, err
			}
			if nr[idx[i]], err = coerce(v, t.cols[idx[i]]); err != nil {
				return nil, nil, err
			}
		}
		t.rows[ri] = nr
		n++
	}
	if n == 0 {
		return nil, result{}, nil
	}
	if err := validate(t); err != nil {
		return nil, nil, err
	}
	return changes{t.name: t}, result{affected: n}, nil
}

func (s *deleteStmt) apply(tables map[string]*table, args []driver.Value) (changes, driver.Result, error) {
	old, err := getTable(tables, s.table)
	if err != nil {
		return nil, nil, err
	}
	t := old.clone()
	t.rows = t.rows[:0:0]
	var n int64
	for _, row := range old.rows {
		ok, err := match(old, row, s.where, args)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			n++
			continue
		}
		t.rows = append(t.rows, row)
	}
	if n == 0 {
		return nil, result{}, nil
	}
	return changes{t.name: t}, result{affected: n}, nil
}

func (s *selectStmt) run(tables map[string]*table, args []driver.Value) (*rows, error) {
	t, err := getTable(tables, s.table)
	if err != nil {
		return nil, err
	}
	var proj []int
	var names []string
	switch {
	case s.count:
		names = []string{"count(*)"}
	case s.cols == nil:
		for i, c := range t.cols {
			proj = append(proj, i)
			names = append(names, c.name)
		}
	default:
		for _, name := range s.cols {
			i, err := t.colIndex(name)
			if err != nil {
				return nil, err
			}
			proj = append(proj, i)
			names = append(names, name)
		}
	}
	var matched [][]driver.Value
	for _, row := range t.rows {
		ok, err := match(t, row, s.where, args)
		if err != nil {
			return nil, err
		}
		if ok {
			matched = append(matched, row)
		}
	}
	if s.count {
		return &rows{cols: names, data: [][]driver.Value{{int64(len(matched))}}}, nil
	}
	if len(s.order) > 0 {
		oidx := make([]int, len(s.order))
		for i, o := range s.order {
			if oidx[i], err = t.colIndex(o.col); err != nil {
				return nil, err
			}
		}
		slices.SortStableFunc(matched, func(a, b []driver.Value) int {
			for i, o := range s.order {
				va, vb := a[oidx[i]], b[oidx[i]]
				var c int
				switch {
				case va == nil && vb == nil:
					c = 0
				case va == nil: // NULL меньше всего (как в SQLite)
					c = -1
				case vb == nil:
					c = 1
				default:
					c, _ = compare(va, vb) // в одной колонке типы совпадают
				}
				if o.desc {
					c = -c
				}
				if c != 0 {
					return c
				}
			}
			return 0
		})
	}
	num := func(e *expr, what string) (int, error) {
		v, err := e.eval(nil, nil, args)
		if err != nil {
			return 0, err
		}
		n, ok := v.(int64)
		if !ok || n < 0 {
			return 0, fmt.Errorf("memsql: %s должен быть неотрицательным целым, получено %v", what, v)
		}
		return int(n), nil
	}
	if s.offset != nil {
		off, err := num(s.offset, "OFFSET")
		if err != nil {
			return nil, err
		}
		matched = matched[min(off, len(matched)):]
	}
	if s.limit != nil {
		lim, err := num(s.limit, "LIMIT")
		if err != nil {
			return nil, err
		}
		matched = matched[:min(lim, len(matched))]
	}
	out := make([][]driver.Value, len(matched))
	for i, row := range matched {
		r := make([]driver.Value, len(proj))
		for j, ci := range proj {
			r[j] = row[ci]
		}
		out[i] = r
	}
	return &rows{cols: names, data: out}, nil
}
