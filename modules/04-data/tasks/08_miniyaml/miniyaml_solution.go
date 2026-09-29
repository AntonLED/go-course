//go:build solution

// Package miniyaml — парсер небольшого подмножества YAML в map[string]any.
package miniyaml

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// line — значимая строка документа.
type line struct {
	num    int    // номер строки в исходнике (с 1)
	indent int    // число ведущих пробелов
	text   string // без отступа, комментария и хвостовых пробелов
}

type parser struct {
	lines []line
	i     int
}

func (p *parser) errf(l line, format string, a ...any) error {
	return fmt.Errorf("строка %d: %s", l.num, fmt.Sprintf(format, a...))
}

// Parse разбирает документ и возвращает корневое отображение.
func Parse(src string) (map[string]any, error) {
	p := &parser{}
	for n, raw := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		body := strings.TrimLeft(raw, " ")
		if strings.HasPrefix(body, "\t") {
			return nil, fmt.Errorf("строка %d: табуляция в отступе запрещена", n+1)
		}
		text := strings.TrimRight(stripComment(body), " \t")
		if text == "" || (text == "---" && len(p.lines) == 0) {
			continue
		}
		p.lines = append(p.lines, line{num: n + 1, indent: len(raw) - len(body), text: text})
	}
	if len(p.lines) == 0 {
		return map[string]any{}, nil
	}
	first := p.lines[0]
	if first.indent != 0 {
		return nil, p.errf(first, "документ должен начинаться без отступа")
	}
	if isListItem(first.text) {
		return nil, p.errf(first, "корень документа должен быть отображением")
	}
	m, err := p.parseMap(0)
	if err != nil {
		return nil, err
	}
	if p.i < len(p.lines) {
		return nil, p.errf(p.lines[p.i], "неожиданный отступ")
	}
	return m, nil
}

// stripComment убирает комментарий: '#' в начале или после пробела, вне кавычек.
func stripComment(s string) string {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t'):
			return s[:i]
		}
	}
	return s
}

func isListItem(t string) bool { return t == "-" || strings.HasPrefix(t, "- ") }

// splitKey делит "key: value" по первому ':' за которым пробел или конец
// строки, вне кавычек. ok=false — это не пара ключ-значение.
func splitKey(t string) (key, rest string, ok bool) {
	var quote byte
	for i := 0; i < len(t); i++ {
		c := t[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
		case (c == '"' || c == '\'') && i == 0:
			quote = c
		case c == ':' && (i+1 == len(t) || t[i+1] == ' '):
			key = strings.TrimSpace(t[:i])
			if len(key) >= 2 && (key[0] == '"' || key[0] == '\'') {
				k, err := parseQuoted(key)
				if err != nil {
					return "", "", false
				}
				key = k
			}
			return key, strings.TrimSpace(t[i+1:]), key != ""
		}
	}
	return "", "", false
}

// parseNode разбирает блок, начинающийся с текущей строки (её отступ = indent).
func (p *parser) parseNode(indent int) (any, error) {
	if isListItem(p.lines[p.i].text) {
		return p.parseList(indent)
	}
	return p.parseMap(indent)
}

func (p *parser) parseMap(indent int) (map[string]any, error) {
	m := map[string]any{}
	for p.i < len(p.lines) && p.lines[p.i].indent == indent {
		l := p.lines[p.i]
		if isListItem(l.text) {
			return nil, p.errf(l, "ожидался ключ, а не элемент списка")
		}
		key, rest, ok := splitKey(l.text)
		if !ok {
			return nil, p.errf(l, "ожидалось \"ключ: значение\", получено %q", l.text)
		}
		if _, dup := m[key]; dup {
			return nil, p.errf(l, "повторяющийся ключ %q", key)
		}
		p.i++
		if rest != "" {
			v, err := parseValue(rest)
			if err != nil {
				return nil, p.errf(l, "%v", err)
			}
			m[key] = v
			if p.i < len(p.lines) && p.lines[p.i].indent > indent {
				return nil, p.errf(p.lines[p.i], "неожиданный отступ после скалярного значения")
			}
			continue
		}
		// "key:" без значения: дальше вложенный блок, список на том же
		// отступе (так тоже можно в YAML) или ничего (null).
		switch {
		case p.i < len(p.lines) && p.lines[p.i].indent > indent:
			v, err := p.parseNode(p.lines[p.i].indent)
			if err != nil {
				return nil, err
			}
			m[key] = v
		case p.i < len(p.lines) && p.lines[p.i].indent == indent && isListItem(p.lines[p.i].text):
			v, err := p.parseList(indent)
			if err != nil {
				return nil, err
			}
			m[key] = v
		default:
			m[key] = nil
		}
	}
	if p.i < len(p.lines) && p.lines[p.i].indent > indent {
		return nil, p.errf(p.lines[p.i], "неожиданный отступ")
	}
	return m, nil
}

func (p *parser) parseList(indent int) ([]any, error) {
	list := []any{}
	for p.i < len(p.lines) && p.lines[p.i].indent == indent && isListItem(p.lines[p.i].text) {
		l := p.lines[p.i]
		rest := strings.TrimLeft(strings.TrimPrefix(l.text, "-"), " ")
		switch {
		case rest == "":
			// "-" и вложенный блок на следующих строках.
			p.i++
			if p.i < len(p.lines) && p.lines[p.i].indent > indent {
				v, err := p.parseNode(p.lines[p.i].indent)
				if err != nil {
					return nil, err
				}
				list = append(list, v)
			} else {
				list = append(list, nil)
			}
		case isListItem(rest):
			return nil, p.errf(l, "вложенный список в одной строке не поддерживается")
		default:
			if _, _, isKey := splitKey(rest); isKey {
				// "- key: v" — начало отображения; его ключи стоят на отступе
				// первого ключа. Подменяем текущую строку и разбираем как map.
				col := indent + len(l.text) - len(rest)
				p.lines[p.i] = line{num: l.num, indent: col, text: rest}
				m, err := p.parseMap(col)
				if err != nil {
					return nil, err
				}
				list = append(list, m)
				continue
			}
			v, err := parseValue(rest)
			if err != nil {
				return nil, p.errf(l, "%v", err)
			}
			list = append(list, v)
			p.i++
		}
	}
	if p.i < len(p.lines) && p.lines[p.i].indent > indent {
		return nil, p.errf(p.lines[p.i], "неожиданный отступ")
	}
	return list, nil
}

// parseValue разбирает значение в строке: flow-список, {} или скаляр.
func parseValue(s string) (any, error) {
	switch s[0] {
	case '&', '*', '!':
		return nil, fmt.Errorf("якоря, ссылки и теги (%q) не поддерживаются", s)
	case '|', '>':
		return nil, fmt.Errorf("многострочные скаляры (%q) не поддерживаются", s)
	case '{':
		if len(s) >= 2 && s[len(s)-1] == '}' && strings.TrimSpace(s[1:len(s)-1]) == "" {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("flow-отображения не поддерживаются: %q", s)
	case '[':
		return parseFlowList(s)
	}
	return parseScalar(s)
}

func parseFlowList(s string) (any, error) {
	if !strings.HasSuffix(s, "]") {
		return nil, fmt.Errorf("незакрытый список %q", s)
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	out := []any{}
	if inner == "" {
		return out, nil
	}
	// Делим по запятым вне кавычек.
	var parts []string
	var quote byte
	start := 0
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '[' || c == '{':
			return nil, fmt.Errorf("вложенные flow-коллекции не поддерживаются: %q", s)
		case c == ',':
			parts = append(parts, inner[start:i])
			start = i + 1
		}
	}
	parts = append(parts, inner[start:])
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("пустой элемент в %q", s)
		}
		v, err := parseScalar(part)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// parseScalar приводит скаляр к типу по правилам YAML 1.2 core schema.
func parseScalar(s string) (any, error) {
	if s[0] == '"' || s[0] == '\'' {
		return parseQuoted(s)
	}
	switch s {
	case "null", "Null", "NULL", "~":
		return nil, nil
	case "true", "True", "TRUE":
		return true, nil
	case "false", "False", "FALSE":
		return false, nil
	case ".inf", ".Inf", ".INF", "+.inf", "+.Inf", "+.INF":
		return math.Inf(1), nil
	case "-.inf", "-.Inf", "-.INF":
		return math.Inf(-1), nil
	case ".nan", ".NaN", ".NAN":
		return math.NaN(), nil
	}
	// yes/no/on/off — в YAML 1.2 это строки («норвежская проблема» 1.1).
	if isInt(s) {
		return toInt(s, 10), nil
	}
	// 0o17 и 0x1F — восьмеричные и шестнадцатеричные целые core schema
	// (без знака; «старые» восьмеричные вида 0777 — это YAML 1.1, здесь — 777).
	if digits, ok := strings.CutPrefix(s, "0o"); ok && isDigits(digits, 8) {
		return toInt(digits, 8), nil
	}
	if digits, ok := strings.CutPrefix(s, "0x"); ok && isDigits(digits, 16) {
		return toInt(digits, 16), nil
	}
	if isFloat(s) {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f, nil
		}
	}
	return s, nil
}

// toInt переводит заведомо корректную запись целого в int, а если значение
// не влезает в int — в float64.
func toInt(s string, base int) any {
	if n, err := strconv.ParseInt(s, base, 0); err == nil {
		return int(n)
	}
	b, _ := new(big.Int).SetString(strings.TrimPrefix(s, "+"), base)
	f, _ := new(big.Float).SetInt(b).Float64()
	return f
}

// isDigits: s непуста и состоит только из цифр системы счисления base (8 или 16).
func isDigits(s string, base int) bool {
	if s == "" {
		return false
	}
	for _, c := range strings.ToLower(s) {
		switch {
		case c >= '0' && c <= '7':
		case (c == '8' || c == '9') && base == 16:
		case c >= 'a' && c <= 'f' && base == 16:
		default:
			return false
		}
	}
	return true
}

// trimSign убирает один ведущий знак.
func trimSign(s string) string {
	if s != "" && (s[0] == '+' || s[0] == '-') {
		return s[1:]
	}
	return s
}

func isInt(s string) bool {
	s = trimSign(s)
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// isFloat — только десятичная запись: [-+]?(\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][-+]?[0-9]+)?
// (strconv.ParseFloat сам по себе принимает лишнее: "Inf", "0x1p-2", "1_000").
func isFloat(s string) bool {
	s = trimSign(s)
	mant, exp, hasExp := strings.Cut(strings.ToLower(s), "e")
	intPart, frac, _ := strings.Cut(mant, ".")
	if intPart == "" && frac == "" {
		return false
	}
	for _, c := range intPart + frac {
		if c < '0' || c > '9' {
			return false
		}
	}
	if hasExp {
		exp = trimSign(exp)
		if exp == "" {
			return false
		}
		for _, c := range exp {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

func parseQuoted(s string) (string, error) {
	q := s[0]
	if len(s) < 2 || s[len(s)-1] != q {
		return "", fmt.Errorf("незакрытая кавычка в %q", s)
	}
	if q == '\'' {
		// В одинарных кавычках экранирования нет, кроме '' → '.
		inner := s[1 : len(s)-1]
		if strings.Contains(strings.ReplaceAll(inner, "''", ""), "'") {
			return "", fmt.Errorf("лишняя кавычка в %q", s)
		}
		return strings.ReplaceAll(inner, "''", "'"), nil
	}
	// Экранирование в двойных кавычках близко к Go: \n \t \" \\ \uXXXX.
	v, err := strconv.Unquote(s)
	if err != nil {
		return "", fmt.Errorf("неверная строка %s: %v", s, err)
	}
	return v, nil
}
