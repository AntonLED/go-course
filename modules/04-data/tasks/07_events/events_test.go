package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDuration(t *testing.T) {
	b, err := json.Marshal(Duration(90 * time.Second))
	if err != nil || string(b) != `"1m30s"` {
		t.Errorf("Marshal(90s) = %s, %v; ожидалось \"1m30s\"", b, err)
	}
	// Через указатель и внутри структуры — тоже должна работать.
	d := Duration(time.Millisecond)
	b, _ = json.Marshal(struct{ D *Duration }{&d})
	if string(b) != `{"D":"1ms"}` {
		t.Errorf("Marshal(&d) = %s", b)
	}
	b, _ = json.Marshal(struct{ D Duration }{Duration(2 * time.Hour)})
	if string(b) != `{"D":"2h0m0s"}` {
		t.Errorf("Marshal(поле-значение) = %s (метод MarshalJSON должен быть на значении)", b)
	}

	for in, want := range map[string]time.Duration{
		`"1m30s"`: 90 * time.Second,
		`"-5ms"`:  -5 * time.Millisecond,
		`90`:      90 * time.Second,
		`1.5`:     1500 * time.Millisecond,
		`0`:       0,
	} {
		var d Duration
		if err := json.Unmarshal([]byte(in), &d); err != nil || time.Duration(d) != want {
			t.Errorf("Unmarshal(%s) = %v, %v; ожидалось %v", in, time.Duration(d), err, want)
		}
	}
	for _, in := range []string{`"soon"`, `true`, `[1]`, `{}`, `"5"`, `1e300`} {
		var d Duration
		if err := json.Unmarshal([]byte(in), &d); err == nil {
			t.Errorf("Unmarshal(%s): ожидалась ошибка", in)
		}
	}
}

var at = time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)

func TestMarshal(t *testing.T) {
	tests := []struct {
		e    Event
		want string
	}{
		{Login{User: "ann", At: at, IP: "10.0.0.1"}, `{"type":"login","user":"ann","at":"2024-05-01T10:00:00Z","ip":"10.0.0.1"}`},
		{Login{User: "ann", At: at}, `{"type":"login","user":"ann","at":"2024-05-01T10:00:00Z"}`},
		{Purchase{User: "bob", Amount: 1999, Items: []string{"book", "pen"}}, `{"type":"purchase","user":"bob","amount_cents":1999,"items":["book","pen"]}`},
		{Purchase{User: "bob"}, `{"type":"purchase","user":"bob","amount_cents":0}`},
		{Session{User: "ann", Length: Duration(62 * time.Minute)}, `{"type":"session","user":"ann","length":"1h2m0s"}`},
	}
	for _, tt := range tests {
		got, err := Marshal(tt.e)
		if err != nil {
			t.Fatalf("Marshal(%+v): %v", tt.e, err)
		}
		if string(got) != tt.want {
			t.Errorf("Marshal(%+v) =\n %s\nожидалось\n %s", tt.e, got, tt.want)
		}
	}
	if _, err := Marshal(nil); err == nil {
		t.Error("Marshal(nil): ожидалась ошибка")
	}
}

func TestUnmarshal(t *testing.T) {
	tests := []struct {
		in   string
		want Event
	}{
		{`{"type":"login","user":"ann","at":"2024-05-01T10:00:00Z"}`, Login{User: "ann", At: at}},
		{`{"user":"bob","amount_cents":5,"items":["x"],"type":"purchase"}`, Purchase{User: "bob", Amount: 5, Items: []string{"x"}}},
		{`{"type":"session","user":"c","length":"30s"}`, Session{User: "c", Length: Duration(30 * time.Second)}},
		{`{"type":"session","user":"c","length":45}`, Session{User: "c", Length: Duration(45 * time.Second)}},
	}
	for _, tt := range tests {
		got, err := Unmarshal([]byte(tt.in))
		if err != nil {
			t.Fatalf("Unmarshal(%s): %v", tt.in, err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Unmarshal(%s) = %#v, ожидалось %#v", tt.in, got, tt.want)
		}
	}
	// Круговой тест.
	for _, e := range []Event{Login{User: "u", At: at, IP: "::1"}, Purchase{User: "p", Amount: -1}, Session{User: "s", Length: Duration(time.Nanosecond)}} {
		b, _ := Marshal(e)
		back, err := Unmarshal(b)
		if err != nil || !reflect.DeepEqual(back, e) {
			t.Errorf("круг %#v → %s → %#v, %v", e, b, back, err)
		}
	}
}

func TestUnmarshalErrors(t *testing.T) {
	if _, err := Unmarshal([]byte(`{"user":"x"}`)); !errors.Is(err, ErrNoType) {
		t.Errorf("нет type: %v, ожидалось ErrNoType", err)
	}
	if _, err := Unmarshal([]byte(`{"type":"logout","user":"x"}`)); !errors.Is(err, ErrUnknownType) {
		t.Errorf("неизвестный type: %v, ожидалось ErrUnknownType", err)
	}
	for _, in := range []string{
		`{"type":"login","user":"x","password":"123"}`, // неизвестное поле
		`{"type":"purchase","amount_cents":"10"}`,      // строка вместо числа
		`{"type":"purchase","amount_cents":1.5}`,       // дробное в int64
		`{"type":"session","length":"долго"}`,
		`{"type":1}`,
		`[1,2]`,
		`{"type":"login"`,
	} {
		if _, err := Unmarshal([]byte(in)); err == nil {
			t.Errorf("Unmarshal(%s): ожидалась ошибка", in)
		}
	}
}

// countingReader считает прочитанные байты.
type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	if len(p) > 512 {
		p = p[:512]
	}
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func TestStream(t *testing.T) {
	in := `[
		{"type":"login","user":"a","at":"2024-05-01T10:00:00Z"},
		{"type":"purchase","user":"a","amount_cents":100},
		{"type":"session","user":"a","length":"1s"}
	]`
	var kinds []string
	err := Stream(strings.NewReader(in), func(e Event) error {
		kinds = append(kinds, e.Kind())
		return nil
	})
	if err != nil || fmt.Sprint(kinds) != "[login purchase session]" {
		t.Errorf("Stream: %v, %v", kinds, err)
	}

	n := 0
	if err := Stream(strings.NewReader(" [ ] "), func(Event) error { n++; return nil }); err != nil || n != 0 {
		t.Errorf("пустой массив: n=%d, err=%v", n, err)
	}
}

func TestStreamIsStreaming(t *testing.T) {
	// Генерируем 100 000 событий (~6 МБ) через io.Pipe — массив никогда не лежит в памяти целиком.
	pr, pw := io.Pipe()
	go func() {
		pw.Write([]byte("["))
		for i := range 100_000 {
			if i > 0 {
				pw.Write([]byte(","))
			}
			if _, err := fmt.Fprintf(pw, `{"type":"purchase","user":"u%d","amount_cents":%d}`, i, i); err != nil {
				return // читатель закрыл трубу
			}
		}
		pw.Write([]byte("]"))
		pw.Close()
	}()
	defer pr.Close()
	cr := &countingReader{r: pr}
	stop := errors.New("хватит")
	var sum int64
	seen := 0
	err := Stream(cr, func(e Event) error {
		p := e.(Purchase)
		sum += p.Amount
		seen++
		if seen == 3 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) {
		t.Fatalf("ошибка из fn должна вернуться как есть, получено %v", err)
	}
	if sum != 0+1+2 {
		t.Errorf("сумма первых трёх = %d, ожидалось 3", sum)
	}
	if cr.n > 64*1024 {
		t.Errorf("прочитано %d байт — Stream должен читать потоково, а не весь ввод", cr.n)
	}
}

func TestStreamErrors(t *testing.T) {
	for _, in := range []string{
		``,
		`{"type":"login"}`,
		`[{"type":"login","user":"a"},]`,
		`[{"type":"nope"}]`,
		`[{"type":"login","user":"a"}`,
		`[{"type":"login","user":"a"}] [`,
		`[1]`,
	} {
		if err := Stream(strings.NewReader(in), func(Event) error { return nil }); err == nil {
			t.Errorf("Stream(%q): ожидалась ошибка", in)
		}
	}
}
