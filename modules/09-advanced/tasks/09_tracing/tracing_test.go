package tracing

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseTraceparent(t *testing.T) {
	const valid = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	sc, err := ParseTraceparent(valid)
	if err != nil {
		t.Fatalf("ParseTraceparent(%q): %v", valid, err)
	}
	if sc.TraceID.String() != "4bf92f3577b34da6a3ce929d0e0e4736" || sc.SpanID.String() != "00f067aa0ba902b7" || !sc.Sampled {
		t.Errorf("разобрано неверно: %+v", sc)
	}
	if got := sc.Traceparent(); got != valid {
		t.Errorf("Traceparent() = %q, ожидалось %q", got, valid)
	}

	okCases := []struct {
		in      string
		sampled bool
	}{
		{"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00", false},
		{"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-03", true}, // прочие биты флагов игнорируются
		{"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-future", true},
		{"cc-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", true},
	}
	for _, tc := range okCases {
		sc, err := ParseTraceparent(tc.in)
		if err != nil || sc.Sampled != tc.sampled {
			t.Errorf("ParseTraceparent(%q) = %+v, %v; ожидалось sampled=%v", tc.in, sc, err, tc.sampled)
		}
	}

	bad := []string{
		"",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7",      // нет флагов
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-x", // хвост у версии 00
		"ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",   // версия ff
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01",   // нулевой trace-id
		"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01",   // нулевой span-id
		"00-4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7-01",   // заглавные
		"00-4bf92f3577b34da6a3ce929d0e0e473g-00f067aa0ba902b7-01",   // не hex
		"00_4bf92f3577b34da6a3ce929d0e0e4736_00f067aa0ba902b7_01",   // разделители
		"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01x",  // будущая версия: после флагов не '-'
		"0-04bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",   // сдвиг
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-0z",   // флаги не hex
	}
	for _, in := range bad {
		if _, err := ParseTraceparent(in); !errors.Is(err, ErrInvalidTraceparent) {
			t.Errorf("ParseTraceparent(%q) = %v, ожидалось ErrInvalidTraceparent", in, err)
		}
	}
}

func TestSpanHierarchy(t *testing.T) {
	tr := NewTracer(nil)
	ctx, root := tr.Start(context.Background(), "root")
	if SpanFromContext(ctx) != root {
		t.Fatal("SpanFromContext должен возвращать только что созданный спан")
	}
	if SpanFromContext(context.Background()) != nil {
		t.Fatal("в пустом контексте спана нет")
	}
	cctx, child := tr.Start(ctx, "child")
	_, grand := tr.Start(cctx, "grand")
	rc, cc, gc := root.Context(), child.Context(), grand.Context()
	if !rc.IsValid() || !rc.Sampled {
		t.Errorf("корень должен быть валидным и sampled: %+v", rc)
	}
	if cc.TraceID != rc.TraceID || gc.TraceID != rc.TraceID {
		t.Error("дочерние спаны должны наследовать TraceID")
	}
	if cc.SpanID == rc.SpanID || gc.SpanID == cc.SpanID {
		t.Error("у каждого спана свой SpanID")
	}
	grand.End()
	child.End()
	child.End() // повторный End — no-op
	root.End()
	fin := tr.Finished()
	if len(fin) != 3 {
		t.Fatalf("завершено %d спанов, ожидалось 3", len(fin))
	}
	if fin[0].Name != "grand" || fin[0].ParentID != cc.SpanID || fin[1].ParentID != rc.SpanID || fin[2].ParentID.IsValid() {
		t.Errorf("неверные связи родитель-потомок: %+v", fin)
	}
	if fin[2].End.Before(fin[2].Start) {
		t.Error("End раньше Start")
	}
	_, other := tr.Start(context.Background(), "other")
	if other.Context().TraceID == rc.TraceID {
		t.Error("спан без родителя должен начинать новую трассу")
	}
}

func TestAttrsAndIsolation(t *testing.T) {
	tr := NewTracer(nil)
	_, s := tr.Start(context.Background(), "s")
	s.SetAttr("k", "v")
	s.End()
	s.SetAttr("late", "x") // после End игнорируется
	fin := tr.Finished()
	if fin[0].Attrs["k"] != "v" || fin[0].Attrs["late"] != "" {
		t.Errorf("Attrs = %v", fin[0].Attrs)
	}
	fin[0].Attrs["k"] = "changed"
	if tr.Finished()[0].Attrs["k"] != "v" {
		t.Error("Finished должен возвращать копию данных")
	}
}

// zeroThenReader: сначала отдаёт n нулевых байт, потом 0x11.
type zeroThenReader struct{ zeros int }

func (r *zeroThenReader) Read(p []byte) (int, error) {
	for i := range p {
		if r.zeros > 0 {
			p[i] = 0
			r.zeros--
		} else {
			p[i] = 0x11
		}
	}
	return len(p), nil
}

func TestZeroIDRegenerated(t *testing.T) {
	tr := NewTracer(&zeroThenReader{zeros: 16})
	_, s := tr.Start(context.Background(), "s")
	if !s.Context().IsValid() {
		t.Fatalf("нулевой ID должен перегенерироваться: %+v", s.Context())
	}
	if s.Context().TraceID.String() != strings.Repeat("11", 16) {
		t.Errorf("TraceID = %s", s.Context().TraceID)
	}
}

func TestRemoteParentAndInject(t *testing.T) {
	tr := NewTracer(nil)
	h := http.Header{}
	h.Set(TraceparentHeader, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00")
	ctx := Extract(context.Background(), h)

	out := http.Header{}
	Inject(ctx, out) // спана ещё нет — пробрасываем удалённый контекст как есть
	if out.Get(TraceparentHeader) != h.Get(TraceparentHeader) {
		t.Errorf("Inject без спана = %q", out.Get(TraceparentHeader))
	}

	ctx, s := tr.Start(ctx, "server")
	sc := s.Context()
	if sc.TraceID.String() != "4bf92f3577b34da6a3ce929d0e0e4736" || sc.Sampled {
		t.Errorf("спан должен продолжать удалённую трассу и наследовать sampled=false: %+v", sc)
	}
	Inject(ctx, out)
	if out.Get(TraceparentHeader) != sc.Traceparent() {
		t.Errorf("Inject = %q, ожидалось %q", out.Get(TraceparentHeader), sc.Traceparent())
	}
	s.End()
	d := tr.Finished()[0]
	if !d.Remote || d.ParentID.String() != "00f067aa0ba902b7" {
		t.Errorf("ожидался удалённый родитель 00f067aa0ba902b7: %+v", d)
	}

	bad := http.Header{}
	bad.Set(TraceparentHeader, "garbage")
	_, s2 := tr.Start(Extract(context.Background(), bad), "fresh")
	if s2.Context().TraceID == sc.TraceID || !s2.Context().Sampled {
		t.Error("невалидный traceparent игнорируется — новая трасса")
	}
	empty := http.Header{}
	Inject(context.Background(), empty)
	if len(empty) != 0 {
		t.Error("Inject без спана и удалённого контекста ничего не пишет")
	}
}

func TestMiddlewareTwoServices(t *testing.T) {
	tr := NewTracer(nil)

	// Сервис B.
	b := httptest.NewServer(Middleware(tr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, s := tr.Start(r.Context(), "b.db")
		time.Sleep(time.Millisecond)
		s.End()
		w.WriteHeader(http.StatusTeapot)
	})))
	defer b.Close()

	// Сервис A вызывает B, пробрасывая контекст.
	a := httptest.NewServer(Middleware(tr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, s := tr.Start(r.Context(), "a.call-b")
		defer s.End()
		req, _ := http.NewRequestWithContext(ctx, "GET", b.URL+"/inner", nil)
		Inject(ctx, req.Header)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		_, _ = w.Write([]byte("ok"))
	})))
	defer a.Close()

	req, _ := http.NewRequest("GET", a.URL+"/outer", nil)
	req.Header.Set(TraceparentHeader, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	spans := tr.Finished()
	if len(spans) != 4 {
		t.Fatalf("спанов %d, ожидалось 4: %+v", len(spans), spans)
	}
	byName := map[string]SpanData{}
	for _, s := range spans {
		if s.Context.TraceID.String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Errorf("спан %q из другой трассы", s.Name)
		}
		byName[s.Name] = s
	}
	if byName["GET /inner"].Attrs["http.status_code"] != "418" {
		t.Errorf("статус B = %q, ожидалось 418", byName["GET /inner"].Attrs["http.status_code"])
	}
	if byName["GET /outer"].Attrs["http.status_code"] != "200" || byName["GET /outer"].Attrs["http.method"] != "GET" {
		t.Errorf("атрибуты A: %v", byName["GET /outer"].Attrs)
	}
	want := "GET /outer\n  a.call-b\n    GET /inner\n      b.db\n"
	if got := Tree(spans); got != want {
		t.Errorf("Tree:\n%s\nожидалось:\n%s", got, want)
	}
}

func TestTree(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	id := func(b byte) SpanID { return SpanID{b} }
	sp := func(name string, self, parent byte, off int) SpanData {
		d := SpanData{Name: name, Start: t0.Add(time.Duration(off) * time.Millisecond)}
		d.Context.SpanID = id(self)
		if parent != 0 {
			d.ParentID = id(parent)
		}
		return d
	}
	spans := []SpanData{
		sp("c2", 4, 2, 5),
		sp("root", 1, 0, 0),
		sp("c1", 2, 1, 1),
		sp("b", 5, 1, 3),
		sp("a", 3, 1, 3), // тот же Start, что у b → по имени
		sp("orphan", 6, 99, 10),
	}
	want := "root\n  c1\n    c2\n  a\n  b\norphan\n"
	if got := Tree(spans); got != want {
		t.Errorf("Tree:\n%s\nожидалось:\n%s", got, want)
	}
	if Tree(nil) != "" {
		t.Error("Tree(nil) должен быть пустым")
	}
}

func TestConcurrentSpans(t *testing.T) {
	tr := NewTracer(nil)
	ctx, root := tr.Start(context.Background(), "root")
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, s := tr.Start(ctx, "worker")
			s.SetAttr("x", "y")
			s.End()
		}()
	}
	wg.Wait()
	root.End()
	if n := len(tr.Finished()); n != 51 {
		t.Errorf("спанов %d, ожидалось 51", n)
	}
	var buf bytes.Buffer
	buf.WriteString(Tree(tr.Finished()))
	if strings.Count(buf.String(), "  worker\n") != 50 {
		t.Errorf("все worker должны быть детьми root:\n%s", buf.String())
	}
}
