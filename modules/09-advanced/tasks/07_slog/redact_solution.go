//go:build solution

package redact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

// groupOrAttrs — одна операция WithGroup (group != "") или WithAttrs.
// Храним их списком и применяем при Handle: так проще всего корректно
// выбросить группы, в которые в итоге не попало ни одного атрибута.
type groupOrAttrs struct {
	group string
	attrs []slog.Attr
}

// Handler — slog.Handler, пишущий JSON-строки и маскирующий секреты.
type Handler struct {
	w      io.Writer
	mu     *sync.Mutex // общий для всех производных хендлеров: они пишут в один w
	level  slog.Leveler
	redact map[string]bool
	goas   []groupOrAttrs
}

var _ slog.Handler = (*Handler)(nil)

// NewHandler создаёт Handler. opts может быть nil.
func NewHandler(w io.Writer, opts *Options) *Handler {
	if opts == nil {
		opts = &Options{}
	}
	keys := opts.RedactKeys
	if keys == nil {
		keys = DefaultRedactKeys
	}
	h := &Handler{w: w, mu: &sync.Mutex{}, level: opts.Level, redact: make(map[string]bool, len(keys))}
	if h.level == nil {
		h.level = slog.LevelInfo
	}
	for _, k := range keys {
		h.redact[strings.ToLower(k)] = true
	}
	return h
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// with возвращает копию хендлера с добавленной операцией. Срез goas
// копируется: иначе два потомка одного хендлера делили бы backing array
// и затирали бы друг другу элементы при append.
func (h *Handler) with(g groupOrAttrs) *Handler {
	h2 := *h
	h2.goas = make([]groupOrAttrs, len(h.goas)+1)
	copy(h2.goas, h.goas)
	h2.goas[len(h.goas)] = g
	return &h2
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return h.with(groupOrAttrs{attrs: attrs})
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return h.with(groupOrAttrs{group: name})
}

// node — JSON-объект, собираемый в памяти перед сериализацией.
type node struct {
	keys []string
	vals []any // []byte (готовый JSON) или *node
}

func (n *node) add(key string, v any) {
	n.keys = append(n.keys, key)
	n.vals = append(n.vals, v)
}

// empty: в объекте нет ни одного значения (с учётом вложенных пустых групп).
func (n *node) empty() bool {
	for _, v := range n.vals {
		if c, ok := v.(*node); !ok || !c.empty() {
			return false
		}
	}
	return true
}

func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	root := &node{}
	if !r.Time.IsZero() {
		root.add(slog.TimeKey, jsonString(r.Time.Format(time.RFC3339Nano)))
	}
	root.add(slog.LevelKey, jsonString(r.Level.String()))
	root.add(slog.MessageKey, jsonString(r.Message))

	cur := root
	for _, g := range h.goas {
		if g.group != "" {
			child := &node{}
			cur.add(g.group, child)
			cur = child
			continue
		}
		for _, a := range g.attrs {
			h.addAttr(cur, a)
		}
	}
	r.Attrs(func(a slog.Attr) bool {
		h.addAttr(cur, a)
		return true
	})

	buf := make([]byte, 0, 256)
	buf = writeNode(buf, root)
	buf = append(buf, '\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(buf) // одна запись на строку — строки не перемешиваются
	return err
}

func (h *Handler) addAttr(n *node, a slog.Attr) {
	a.Value = a.Value.Resolve() // LogValuer → конечное значение
	if a.Equal(slog.Attr{}) {
		return // пустой атрибут игнорируется
	}
	if a.Key != "" && h.redact[strings.ToLower(a.Key)] {
		n.add(a.Key, jsonString(Redacted))
		return
	}
	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		if len(attrs) == 0 {
			return // пустая группа не выводится
		}
		target := n
		if a.Key != "" { // группа с пустым ключом «встраивается» в текущий объект
			target = &node{}
			n.add(a.Key, target)
		}
		for _, ga := range attrs {
			h.addAttr(target, ga)
		}
		return
	}
	n.add(a.Key, encodeValue(a.Value))
}

func encodeValue(v slog.Value) []byte {
	switch v.Kind() {
	case slog.KindString:
		return jsonString(v.String())
	case slog.KindInt64:
		return strconv.AppendInt(nil, v.Int64(), 10)
	case slog.KindUint64:
		return strconv.AppendUint(nil, v.Uint64(), 10)
	case slog.KindFloat64:
		f := v.Float64()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return jsonString(strconv.FormatFloat(f, 'g', -1, 64)) // в JSON нет NaN/Inf
		}
		return strconv.AppendFloat(nil, f, 'g', -1, 64)
	case slog.KindBool:
		return strconv.AppendBool(nil, v.Bool())
	case slog.KindDuration:
		return jsonString(v.Duration().String())
	case slog.KindTime:
		return jsonString(v.Time().Format(time.RFC3339Nano))
	}
	// KindAny
	x := v.Any()
	if err, ok := x.(error); ok {
		return jsonString(err.Error())
	}
	if b, err := json.Marshal(x); err == nil {
		return b
	}
	return jsonString(fmt.Sprint(x))
}

func jsonString(s string) []byte {
	b, _ := json.Marshal(s) // Marshal строки не возвращает ошибок
	return b
}

func writeNode(buf []byte, n *node) []byte {
	buf = append(buf, '{')
	first := true
	for i, k := range n.keys {
		v := n.vals[i]
		if c, ok := v.(*node); ok && c.empty() {
			continue // группа без атрибутов (например, WithGroup без данных)
		}
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = append(buf, jsonString(k)...)
		buf = append(buf, ':')
		switch v := v.(type) {
		case []byte:
			buf = append(buf, v...)
		case *node:
			buf = writeNode(buf, v)
		}
	}
	return append(buf, '}')
}
