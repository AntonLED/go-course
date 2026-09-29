package memsql

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// --- лексер ---

type tokKind int

const (
	tEOF tokKind = iota
	tIdent
	tNumber
	tString
	tParam // ? или $N
	tSym
)

type token struct {
	kind tokKind
	s    string
	n    int // для $N — номер (1..), для ? — 0
	pos  int
}

func lex(q string) ([]token, error) {
	var toks []token
	rs := []rune(q)
	i := 0
	for i < len(rs) {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '-' && i+1 < len(rs) && rs[i+1] == '-': // комментарий до конца строки
			for i < len(rs) && rs[i] != '\n' {
				i++
			}
		case unicode.IsLetter(r) || r == '_':
			st := i
			for i < len(rs) && (unicode.IsLetter(rs[i]) || unicode.IsDigit(rs[i]) || rs[i] == '_') {
				i++
			}
			toks = append(toks, token{kind: tIdent, s: string(rs[st:i]), pos: st})
		case r == '"': // идентификатор в кавычках
			st := i
			i++
			for i < len(rs) && rs[i] != '"' {
				i++
			}
			if i >= len(rs) {
				return nil, fmt.Errorf("memsql: незакрытая кавычка в позиции %d", st)
			}
			toks = append(toks, token{kind: tIdent, s: string(rs[st+1 : i]), pos: st})
			i++
		case unicode.IsDigit(r) || (r == '.' && i+1 < len(rs) && unicode.IsDigit(rs[i+1])):
			st := i
			for i < len(rs) && (unicode.IsDigit(rs[i]) || rs[i] == '.' || rs[i] == 'e' || rs[i] == 'E' ||
				((rs[i] == '+' || rs[i] == '-') && (rs[i-1] == 'e' || rs[i-1] == 'E'))) {
				i++
			}
			toks = append(toks, token{kind: tNumber, s: string(rs[st:i]), pos: st})
		case r == '\'':
			st := i
			i++
			var sb strings.Builder
			for {
				if i >= len(rs) {
					return nil, fmt.Errorf("memsql: незакрытая строка в позиции %d", st)
				}
				if rs[i] == '\'' {
					if i+1 < len(rs) && rs[i+1] == '\'' { // '' — экранированная кавычка
						sb.WriteRune('\'')
						i += 2
						continue
					}
					i++
					break
				}
				sb.WriteRune(rs[i])
				i++
			}
			toks = append(toks, token{kind: tString, s: sb.String(), pos: st})
		case r == '?':
			toks = append(toks, token{kind: tParam, s: "?", pos: i})
			i++
		case r == '$':
			st := i
			i++
			for i < len(rs) && unicode.IsDigit(rs[i]) {
				i++
			}
			n, err := strconv.Atoi(string(rs[st+1 : i]))
			if err != nil || n < 1 {
				return nil, fmt.Errorf("memsql: неверный параметр в позиции %d", st)
			}
			toks = append(toks, token{kind: tParam, s: string(rs[st:i]), n: n, pos: st})
		default:
			two := ""
			if i+1 < len(rs) {
				two = string(rs[i : i+2])
			}
			switch two {
			case "<=", ">=", "!=", "<>":
				toks = append(toks, token{kind: tSym, s: two, pos: i})
				i += 2
				continue
			}
			if strings.ContainsRune("(),*=<>+-;", r) {
				toks = append(toks, token{kind: tSym, s: string(r), pos: i})
				i++
				continue
			}
			return nil, fmt.Errorf("memsql: неожиданный символ %q в позиции %d", r, i)
		}
	}
	toks = append(toks, token{kind: tEOF, pos: len(rs)})
	return toks, nil
}

// --- AST ---

type statement interface{ isStatement() }

// writer — операторы, изменяющие данные/схему.
type writer interface {
	statement
	apply(tables map[string]*table, args []driver.Value) (changes, driver.Result, error)
}

type operandKind int

const (
	opLit operandKind = iota
	opParam
	opCol
)

type operand struct {
	kind  operandKind
	val   driver.Value // opLit
	param int          // opParam: индекс аргумента (с 0)
	col   string       // opCol
}

type expr struct {
	left  operand
	op    byte // 0, '+', '-'
	right operand
}

type cond struct {
	col     string
	op      string // = != < <= > >= ISNULL NOTNULL
	operand expr
}

type orderItem struct {
	col  string
	desc bool
}

type createStmt struct {
	name        string
	ifNotExists bool
	cols        []column
}

type dropStmt struct {
	name     string
	ifExists bool
}

type insertStmt struct {
	table string
	cols  []string
	rows  [][]expr
}

type selectStmt struct {
	table  string
	cols   []string // nil — *
	count  bool
	where  []cond
	order  []orderItem
	limit  *expr
	offset *expr
}

type setItem struct {
	col string
	e   expr
}

type updateStmt struct {
	table string
	sets  []setItem
	where []cond
}

type deleteStmt struct {
	table string
	where []cond
}

func (*createStmt) isStatement() {}
func (*dropStmt) isStatement()   {}
func (*insertStmt) isStatement() {}
func (*selectStmt) isStatement() {}
func (*updateStmt) isStatement() {}
func (*deleteStmt) isStatement() {}

// --- парсер ---

type parser struct {
	toks     []token
	pos      int
	nQ       int // сколько встретилось ?
	maxDol   int // максимальный $N
	sawDol   bool
	sawQ     bool
	original string
}

// parse разбирает запрос и возвращает AST и число параметров.
func parse(q string) (statement, int, error) {
	toks, err := lex(q)
	if err != nil {
		return nil, 0, err
	}
	p := &parser{toks: toks, original: q}
	st, err := p.statement()
	if err != nil {
		return nil, 0, err
	}
	p.acceptSym(";")
	if p.peek().kind != tEOF {
		return nil, 0, p.errf("лишний текст после конца оператора")
	}
	if p.sawQ && p.sawDol {
		return nil, 0, p.errf("нельзя смешивать плейсхолдеры ? и $N")
	}
	n := p.nQ
	if p.sawDol {
		n = p.maxDol
	}
	return st, n, nil
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token {
	t := p.toks[p.pos]
	if t.kind != tEOF {
		p.pos++
	}
	return t
}

func (p *parser) errf(format string, a ...any) error {
	t := p.peek()
	near := t.s
	if t.kind == tEOF {
		near = "конец запроса"
	}
	return fmt.Errorf("memsql: синтаксическая ошибка около %q (позиция %d): %s", near, t.pos, fmt.Sprintf(format, a...))
}

func (p *parser) isKW(kw string) bool {
	t := p.peek()
	return t.kind == tIdent && strings.EqualFold(t.s, kw)
}

func (p *parser) acceptKW(kw string) bool {
	if p.isKW(kw) {
		p.pos++
		return true
	}
	return false
}

func (p *parser) expectKW(kw string) error {
	if !p.acceptKW(kw) {
		return p.errf("ожидалось %s", kw)
	}
	return nil
}

func (p *parser) acceptSym(s string) bool {
	t := p.peek()
	if t.kind == tSym && t.s == s {
		p.pos++
		return true
	}
	return false
}

func (p *parser) expectSym(s string) error {
	if !p.acceptSym(s) {
		return p.errf("ожидалось %q", s)
	}
	return nil
}

var reserved = map[string]bool{
	"SELECT": true, "FROM": true, "WHERE": true, "AND": true, "ORDER": true, "BY": true,
	"LIMIT": true, "OFFSET": true, "INSERT": true, "INTO": true, "VALUES": true, "UPDATE": true,
	"SET": true, "DELETE": true, "CREATE": true, "TABLE": true, "DROP": true, "NULL": true,
	"IS": true, "NOT": true, "TRUE": true, "FALSE": true, "PRIMARY": true, "KEY": true,
	"UNIQUE": true, "ASC": true, "DESC": true, "OR": true,
}

func (p *parser) ident() (string, error) {
	t := p.peek()
	if t.kind != tIdent || reserved[strings.ToUpper(t.s)] {
		return "", p.errf("ожидался идентификатор")
	}
	p.pos++
	return strings.ToLower(t.s), nil
}

func (p *parser) statement() (statement, error) {
	switch {
	case p.acceptKW("CREATE"):
		return p.create()
	case p.acceptKW("DROP"):
		return p.drop()
	case p.acceptKW("INSERT"):
		return p.insert()
	case p.acceptKW("SELECT"):
		return p.selectStmt()
	case p.acceptKW("UPDATE"):
		return p.update()
	case p.acceptKW("DELETE"):
		return p.delete()
	}
	return nil, p.errf("неизвестный оператор (поддерживаются CREATE, DROP, INSERT, SELECT, UPDATE, DELETE)")
}

func (p *parser) create() (statement, error) {
	if err := p.expectKW("TABLE"); err != nil {
		return nil, err
	}
	s := &createStmt{}
	if p.acceptKW("IF") {
		if err := p.expectKW("NOT"); err != nil {
			return nil, err
		}
		if err := p.expectKW("EXISTS"); err != nil {
			return nil, err
		}
		s.ifNotExists = true
	}
	name, err := p.ident()
	if err != nil {
		return nil, err
	}
	s.name = name
	if err := p.expectSym("("); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for {
		var c column
		if c.name, err = p.ident(); err != nil {
			return nil, err
		}
		if seen[c.name] {
			return nil, p.errf("колонка %q объявлена дважды", c.name)
		}
		seen[c.name] = true
		if c.typ, err = p.colType(); err != nil {
			return nil, err
		}
		for {
			switch {
			case p.acceptKW("PRIMARY"):
				if err := p.expectKW("KEY"); err != nil {
					return nil, err
				}
				c.pk, c.notNull, c.unique = true, true, true
				continue
			case p.acceptKW("NOT"):
				if err := p.expectKW("NULL"); err != nil {
					return nil, err
				}
				c.notNull = true
				continue
			case p.acceptKW("UNIQUE"):
				c.unique = true
				continue
			case p.acceptKW("NULL"):
				continue
			}
			break
		}
		s.cols = append(s.cols, c)
		if p.acceptSym(",") {
			continue
		}
		if err := p.expectSym(")"); err != nil {
			return nil, err
		}
		break
	}
	pks := 0
	for _, c := range s.cols {
		if c.pk {
			pks++
		}
	}
	if pks > 1 {
		return nil, fmt.Errorf("memsql: составной PRIMARY KEY не поддерживается")
	}
	return s, nil
}

func (p *parser) colType() (colType, error) {
	t := p.peek()
	if t.kind != tIdent {
		return 0, p.errf("ожидался тип колонки")
	}
	p.pos++
	var ct colType
	switch strings.ToUpper(t.s) {
	case "INTEGER", "INT", "BIGINT", "SMALLINT":
		ct = typInt
	case "TEXT", "VARCHAR", "CHAR", "STRING":
		ct = typText
	case "REAL", "FLOAT", "DOUBLE", "NUMERIC":
		ct = typReal
		p.acceptKW("PRECISION")
	case "BOOLEAN", "BOOL":
		ct = typBool
	case "TIMESTAMP", "DATETIME", "TIMESTAMPTZ":
		ct = typTime
	default:
		p.pos--
		return 0, p.errf("неизвестный тип %s", t.s)
	}
	// VARCHAR(255), NUMERIC(10, 2) — размер игнорируем.
	if p.acceptSym("(") {
		for !p.acceptSym(")") {
			if p.peek().kind == tEOF {
				return 0, p.errf("ожидалось )")
			}
			p.next()
		}
	}
	return ct, nil
}

func (p *parser) drop() (statement, error) {
	if err := p.expectKW("TABLE"); err != nil {
		return nil, err
	}
	s := &dropStmt{}
	if p.acceptKW("IF") {
		if err := p.expectKW("EXISTS"); err != nil {
			return nil, err
		}
		s.ifExists = true
	}
	var err error
	s.name, err = p.ident()
	return s, err
}

func (p *parser) identList() ([]string, error) {
	var out []string
	for {
		id, err := p.ident()
		if err != nil {
			return nil, err
		}
		out = append(out, id)
		if !p.acceptSym(",") {
			return out, nil
		}
	}
}

func (p *parser) insert() (statement, error) {
	if err := p.expectKW("INTO"); err != nil {
		return nil, err
	}
	s := &insertStmt{}
	var err error
	if s.table, err = p.ident(); err != nil {
		return nil, err
	}
	if p.acceptSym("(") {
		if s.cols, err = p.identList(); err != nil {
			return nil, err
		}
		if err := p.expectSym(")"); err != nil {
			return nil, err
		}
	}
	if err := p.expectKW("VALUES"); err != nil {
		return nil, err
	}
	for {
		if err := p.expectSym("("); err != nil {
			return nil, err
		}
		var row []expr
		for {
			e, err := p.expr()
			if err != nil {
				return nil, err
			}
			row = append(row, e)
			if !p.acceptSym(",") {
				break
			}
		}
		if err := p.expectSym(")"); err != nil {
			return nil, err
		}
		if s.cols != nil && len(row) != len(s.cols) {
			return nil, fmt.Errorf("memsql: INSERT: %d колонок, но %d значений", len(s.cols), len(row))
		}
		s.rows = append(s.rows, row)
		if !p.acceptSym(",") {
			break
		}
	}
	return s, nil
}

func (p *parser) selectStmt() (statement, error) {
	s := &selectStmt{}
	var err error
	switch {
	case p.acceptSym("*"):
	case p.isKW("COUNT"):
		p.next()
		if err := p.expectSym("("); err != nil {
			return nil, err
		}
		if err := p.expectSym("*"); err != nil {
			return nil, err
		}
		if err := p.expectSym(")"); err != nil {
			return nil, err
		}
		s.count = true
	default:
		if s.cols, err = p.identList(); err != nil {
			return nil, err
		}
	}
	if err := p.expectKW("FROM"); err != nil {
		return nil, err
	}
	if s.table, err = p.ident(); err != nil {
		return nil, err
	}
	if s.where, err = p.where(); err != nil {
		return nil, err
	}
	if p.acceptKW("ORDER") {
		if err := p.expectKW("BY"); err != nil {
			return nil, err
		}
		for {
			var it orderItem
			if it.col, err = p.ident(); err != nil {
				return nil, err
			}
			if p.acceptKW("DESC") {
				it.desc = true
			} else {
				p.acceptKW("ASC")
			}
			s.order = append(s.order, it)
			if !p.acceptSym(",") {
				break
			}
		}
	}
	if p.acceptKW("LIMIT") {
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		s.limit = &e
	}
	if p.acceptKW("OFFSET") {
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		s.offset = &e
	}
	return s, nil
}

func (p *parser) update() (statement, error) {
	s := &updateStmt{}
	var err error
	if s.table, err = p.ident(); err != nil {
		return nil, err
	}
	if err := p.expectKW("SET"); err != nil {
		return nil, err
	}
	for {
		var it setItem
		if it.col, err = p.ident(); err != nil {
			return nil, err
		}
		if err := p.expectSym("="); err != nil {
			return nil, err
		}
		if it.e, err = p.expr(); err != nil {
			return nil, err
		}
		s.sets = append(s.sets, it)
		if !p.acceptSym(",") {
			break
		}
	}
	s.where, err = p.where()
	return s, err
}

func (p *parser) delete() (statement, error) {
	if err := p.expectKW("FROM"); err != nil {
		return nil, err
	}
	s := &deleteStmt{}
	var err error
	if s.table, err = p.ident(); err != nil {
		return nil, err
	}
	s.where, err = p.where()
	return s, err
}

func (p *parser) where() ([]cond, error) {
	if !p.acceptKW("WHERE") {
		return nil, nil
	}
	var out []cond
	for {
		var c cond
		var err error
		if c.col, err = p.ident(); err != nil {
			return nil, err
		}
		if p.acceptKW("IS") {
			if p.acceptKW("NOT") {
				c.op = "NOTNULL"
			} else {
				c.op = "ISNULL"
			}
			if err := p.expectKW("NULL"); err != nil {
				return nil, err
			}
		} else {
			t := p.peek()
			if t.kind != tSym || !strings.Contains(" = != <> < <= > >= ", " "+t.s+" ") {
				return nil, p.errf("ожидался оператор сравнения")
			}
			p.next()
			c.op = t.s
			if c.op == "<>" {
				c.op = "!="
			}
			if c.operand, err = p.expr(); err != nil {
				return nil, err
			}
		}
		out = append(out, c)
		if p.isKW("OR") {
			return nil, p.errf("OR не поддерживается")
		}
		if !p.acceptKW("AND") {
			return out, nil
		}
	}
}

func (p *parser) expr() (expr, error) {
	l, err := p.operand()
	if err != nil {
		return expr{}, err
	}
	e := expr{left: l}
	if p.acceptSym("+") {
		e.op = '+'
	} else if p.acceptSym("-") {
		e.op = '-'
	} else {
		return e, nil
	}
	if e.right, err = p.operand(); err != nil {
		return expr{}, err
	}
	return e, nil
}

func (p *parser) operand() (operand, error) {
	t := p.peek()
	neg := false
	if t.kind == tSym && t.s == "-" && p.toks[p.pos+1].kind == tNumber {
		neg = true
		p.next()
		t = p.peek()
	}
	switch t.kind {
	case tParam:
		p.next()
		if t.n > 0 {
			p.sawDol = true
			p.maxDol = max(p.maxDol, t.n)
			return operand{kind: opParam, param: t.n - 1}, nil
		}
		p.sawQ = true
		p.nQ++
		return operand{kind: opParam, param: p.nQ - 1}, nil
	case tNumber:
		p.next()
		s := t.s
		if neg {
			s = "-" + s
		}
		if strings.ContainsAny(s, ".eE") {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return operand{}, fmt.Errorf("memsql: неверное число %q", s)
			}
			return operand{kind: opLit, val: f}, nil
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return operand{}, fmt.Errorf("memsql: неверное число %q", s)
		}
		return operand{kind: opLit, val: n}, nil
	case tString:
		p.next()
		return operand{kind: opLit, val: t.s}, nil
	case tIdent:
		switch strings.ToUpper(t.s) {
		case "NULL":
			p.next()
			return operand{kind: opLit, val: nil}, nil
		case "TRUE":
			p.next()
			return operand{kind: opLit, val: true}, nil
		case "FALSE":
			p.next()
			return operand{kind: opLit, val: false}, nil
		}
		name, err := p.ident()
		if err != nil {
			return operand{}, err
		}
		return operand{kind: opCol, col: name}, nil
	}
	return operand{}, p.errf("ожидалось значение")
}
