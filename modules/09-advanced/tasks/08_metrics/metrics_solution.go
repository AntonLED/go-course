//go:build solution

package metrics

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	metricNameRe = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	labelNameRe  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// atomicFloat — float64 поверх atomic.Uint64 (CAS-цикл): быстрее мьютекса
// на горячем пути Inc/Observe.
type atomicFloat struct{ bits atomic.Uint64 }

func (f *atomicFloat) Load() float64   { return math.Float64frombits(f.bits.Load()) }
func (f *atomicFloat) Store(v float64) { f.bits.Store(math.Float64bits(v)) }
func (f *atomicFloat) Add(d float64) {
	for {
		old := f.bits.Load()
		if f.bits.CompareAndSwap(old, math.Float64bits(math.Float64frombits(old)+d)) {
			return
		}
	}
}

// collector — то, что Registry умеет печатать.
type collector interface {
	name() string
	write(w *bufio.Writer)
}

// family — общая часть всех типов метрик: имя, лейблы, серии.
type family[S any] struct {
	fname, help, typ string
	labels           []string
	newSeries        func() *S
	// writeSeries печатает строки одной серии (разные для counter/gauge/histogram).
	writeSeries func(w *bufio.Writer, f *family[S], e *seriesEntry[S])

	mu     sync.RWMutex
	series map[string]*seriesEntry[S] // ключ — значения лейблов через \xff
}

type seriesEntry[S any] struct {
	values []string
	s      *S
}

func (f *family[S]) name() string { return f.fname }

func (f *family[S]) get(values []string) *S {
	if len(values) != len(f.labels) {
		panic(fmt.Sprintf("metrics: %s: ожидалось %d значений лейблов, получено %d", f.fname, len(f.labels), len(values)))
	}
	key := strings.Join(values, "\xff")
	// Быстрый путь под RLock: серия обычно уже существует.
	f.mu.RLock()
	e, ok := f.series[key]
	f.mu.RUnlock()
	if ok {
		return e.s
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.series[key]; ok { // double-checked: могли создать, пока ждали Lock
		return e.s
	}
	e = &seriesEntry[S]{values: slices.Clone(values), s: f.newSeries()}
	f.series[key] = e
	return e.s
}

// sorted возвращает серии, отсортированные по значениям лейблов.
func (f *family[S]) sorted() []*seriesEntry[S] {
	f.mu.RLock()
	out := make([]*seriesEntry[S], 0, len(f.series))
	for _, e := range f.series {
		out = append(out, e)
	}
	f.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return slices.Compare(out[i].values, out[j].values) < 0 })
	return out
}

func (f *family[S]) header(w *bufio.Writer) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", f.fname, escapeHelp(f.help), f.fname, f.typ)
}

// labelString: {a="1",b="2"} (+ дополнительная пара, например le). Пусто → "".
func labelString(names, values []string, extraName, extraValue string) string {
	if len(names) == 0 && extraName == "" {
		return ""
	}
	var b strings.Builder
	b.WriteByte('{')
	for i, n := range names {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `%s="%s"`, n, escapeLabel(values[i]))
	}
	if extraName != "" {
		if len(names) > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `%s="%s"`, extraName, extraValue)
	}
	b.WriteByte('}')
	return b.String()
}

var (
	labelEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	helpEscaper  = strings.NewReplacer(`\`, `\\`, "\n", `\n`)
)

func escapeLabel(s string) string { return labelEscaper.Replace(s) }
func escapeHelp(s string) string  { return helpEscaper.Replace(s) }

func formatFloat(v float64) string {
	switch {
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	case math.IsNaN(v):
		return "NaN"
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// ---------- Registry ----------

// Registry хранит метрики и отдаёт их в текстовом формате Prometheus.
type Registry struct {
	mu         sync.Mutex
	collectors map[string]collector
}

func NewRegistry() *Registry { return &Registry{collectors: make(map[string]collector)} }

func validate(name string, labels []string, reserved ...string) error {
	if !metricNameRe.MatchString(name) {
		return fmt.Errorf("%w: метрика %q", ErrInvalidName, name)
	}
	seen := map[string]bool{}
	for _, l := range labels {
		if !labelNameRe.MatchString(l) || strings.HasPrefix(l, "__") || slices.Contains(reserved, l) || seen[l] {
			return fmt.Errorf("%w: лейбл %q метрики %s", ErrInvalidName, l, name)
		}
		seen[l] = true
	}
	return nil
}

func register[S any](r *Registry, f *family[S]) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.collectors[f.fname]; ok {
		return fmt.Errorf("%w: %s", ErrDuplicate, f.fname)
	}
	r.collectors[f.fname] = f
	// Метрика без лейблов экспортируется сразу (со значением 0).
	if len(f.labels) == 0 {
		f.get(nil)
	}
	return nil
}

func newFamily[S any](name, help, typ string, labels []string, newSeries func() *S,
	writeSeries func(*bufio.Writer, *family[S], *seriesEntry[S])) *family[S] {
	return &family[S]{
		fname: name, help: help, typ: typ,
		labels:      slices.Clone(labels),
		newSeries:   newSeries,
		writeSeries: writeSeries,
		series:      make(map[string]*seriesEntry[S]),
	}
}

func (f *family[S]) write(w *bufio.Writer) {
	f.header(w)
	for _, e := range f.sorted() {
		f.writeSeries(w, f, e)
	}
}

// WriteText пишет все метрики, отсортированные по имени.
func (r *Registry) WriteText(w io.Writer) error {
	r.mu.Lock()
	cs := make([]collector, 0, len(r.collectors))
	for _, c := range r.collectors {
		cs = append(cs, c)
	}
	r.mu.Unlock()
	sort.Slice(cs, func(i, j int) bool { return cs[i].name() < cs[j].name() })

	bw := bufio.NewWriter(w)
	for _, c := range cs {
		c.write(bw)
	}
	return bw.Flush()
}

// Handler — http.Handler для /metrics.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", ContentType)
		_ = r.WriteText(w)
	})
}

// ---------- Counter ----------

type Counter struct{ f *family[CounterSeries] }
type CounterSeries struct{ v atomicFloat }

func (r *Registry) NewCounter(name, help string, labelNames ...string) (*Counter, error) {
	if err := validate(name, labelNames); err != nil {
		return nil, err
	}
	f := newFamily(name, help, "counter", labelNames, func() *CounterSeries { return &CounterSeries{} },
		func(w *bufio.Writer, f *family[CounterSeries], e *seriesEntry[CounterSeries]) {
			writeSample(w, f.fname, labelString(f.labels, e.values, "", ""), e.s.v.Load())
		})
	if err := register(r, f); err != nil {
		return nil, err
	}
	return &Counter{f: f}, nil
}

func writeSample(w *bufio.Writer, name, labels string, v float64) {
	fmt.Fprintf(w, "%s%s %s\n", name, labels, formatFloat(v))
}

func (c *Counter) WithLabelValues(values ...string) *CounterSeries { return c.f.get(values) }
func (s *CounterSeries) Inc()                                      { s.v.Add(1) }
func (s *CounterSeries) Add(v float64) {
	if v < 0 {
		panic("metrics: счётчик не может уменьшаться")
	}
	s.v.Add(v)
}

// ---------- Gauge ----------

type Gauge struct{ f *family[GaugeSeries] }
type GaugeSeries struct{ v atomicFloat }

func (r *Registry) NewGauge(name, help string, labelNames ...string) (*Gauge, error) {
	if err := validate(name, labelNames); err != nil {
		return nil, err
	}
	f := newFamily(name, help, "gauge", labelNames, func() *GaugeSeries { return &GaugeSeries{} },
		func(w *bufio.Writer, f *family[GaugeSeries], e *seriesEntry[GaugeSeries]) {
			writeSample(w, f.fname, labelString(f.labels, e.values, "", ""), e.s.v.Load())
		})
	if err := register(r, f); err != nil {
		return nil, err
	}
	return &Gauge{f: f}, nil
}

func (g *Gauge) WithLabelValues(values ...string) *GaugeSeries { return g.f.get(values) }
func (s *GaugeSeries) Set(v float64)                           { s.v.Store(v) }
func (s *GaugeSeries) Add(v float64)                           { s.v.Add(v) }
func (s *GaugeSeries) Inc()                                    { s.v.Add(1) }
func (s *GaugeSeries) Dec()                                    { s.v.Add(-1) }

// ---------- Histogram ----------

type Histogram struct{ f *family[HistogramSeries] }

type HistogramSeries struct {
	upper  []float64       // верхние границы (без +Inf)
	counts []atomic.Uint64 // НЕкумулятивные счётчики; последний — для +Inf
	sum    atomicFloat
}

func (r *Registry) NewHistogram(name, help string, buckets []float64, labelNames ...string) (*Histogram, error) {
	if err := validate(name, labelNames, "le"); err != nil {
		return nil, err
	}
	if buckets == nil {
		buckets = DefBuckets
	}
	buckets = slices.Clone(buckets)
	if n := len(buckets); n > 0 && math.IsInf(buckets[n-1], 1) {
		buckets = buckets[:n-1] // +Inf добавляется всегда сам
	}
	for i := range buckets {
		if math.IsNaN(buckets[i]) || (i > 0 && buckets[i] <= buckets[i-1]) {
			return nil, fmt.Errorf("%w: %v", ErrBuckets, buckets)
		}
	}
	f := newFamily(name, help, "histogram", labelNames, func() *HistogramSeries {
		return &HistogramSeries{upper: buckets, counts: make([]atomic.Uint64, len(buckets)+1)}
	}, writeHistogram)
	if err := register(r, f); err != nil {
		return nil, err
	}
	return &Histogram{f: f}, nil
}

func (h *Histogram) WithLabelValues(values ...string) *HistogramSeries { return h.f.get(values) }

// Observe: бакет le — «значение <= le», ищем первый такой бинпоиском.
func (s *HistogramSeries) Observe(v float64) {
	i := sort.SearchFloat64s(s.upper, v) // первый индекс с upper[i] >= v
	s.counts[i].Add(1)
	s.sum.Add(v)
}

func writeHistogram(w *bufio.Writer, f *family[HistogramSeries], e *seriesEntry[HistogramSeries]) {
	s := e.s
	var cum uint64
	for i := range s.counts {
		cum += s.counts[i].Load()
		le := "+Inf"
		if i < len(s.upper) {
			le = formatFloat(s.upper[i])
		}
		fmt.Fprintf(w, "%s_bucket%s %d\n", f.fname, labelString(f.labels, e.values, "le", le), cum)
	}
	ls := labelString(f.labels, e.values, "", "")
	writeSample(w, f.fname+"_sum", ls, s.sum.Load())
	// count = сумма бакетов: так _count и +Inf-бакет всегда согласованы,
	// даже если Observe идёт параллельно с экспортом.
	fmt.Fprintf(w, "%s_count%s %d\n", f.fname, ls, cum)
}
