package grpcframe

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestWriteMessage(t *testing.T) {
	cases := []struct {
		msg        string
		compressed bool
		want       []byte
	}{
		{"hi", false, []byte{0, 0, 0, 0, 2, 'h', 'i'}},
		{"", false, []byte{0, 0, 0, 0, 0}},
		{"z", true, []byte{1, 0, 0, 0, 1, 'z'}},
		{strings.Repeat("a", 300), false, append([]byte{0, 0, 0, 1, 0x2c}, strings.Repeat("a", 300)...)},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if err := WriteMessage(&buf, []byte(c.msg), c.compressed); err != nil {
			t.Fatalf("WriteMessage: %v", err)
		}
		if !bytes.Equal(buf.Bytes(), c.want) {
			t.Errorf("WriteMessage(%.10q, %v) = % x, ожидалось % x", c.msg, c.compressed, buf.Bytes()[:min(buf.Len(), 12)], c.want[:min(len(c.want), 12)])
		}
	}
}

type countingWriter struct {
	calls int
	bytes.Buffer
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.calls++
	return w.Buffer.Write(p)
}

func TestWriteMessageSingleWrite(t *testing.T) {
	var w countingWriter
	_ = WriteMessage(&w, []byte("hello"), false)
	if w.calls != 1 {
		t.Errorf("WriteMessage сделал %d вызовов Write, ожидался 1 (иначе фреймы из разных горутин перемешаются)", w.calls)
	}
}

func TestReadMessage(t *testing.T) {
	var buf bytes.Buffer
	_ = WriteMessage(&buf, []byte("first"), false)
	_ = WriteMessage(&buf, []byte{}, true)
	_ = WriteMessage(&buf, []byte("third"), false)

	for i, want := range []struct {
		msg        string
		compressed bool
	}{{"first", false}, {"", true}, {"third", false}} {
		msg, compressed, err := ReadMessage(&buf, 100)
		if err != nil || string(msg) != want.msg || compressed != want.compressed {
			t.Fatalf("сообщение %d: ReadMessage = %q, %v, %v; ожидалось %q, %v, nil", i, msg, compressed, err, want.msg, want.compressed)
		}
	}
	if _, _, err := ReadMessage(&buf, 100); err != io.EOF {
		t.Errorf("пустой поток: ошибка %v, ожидался io.EOF", err)
	}
}

// limitedSource отдаёт ровно заданные байты, затем io.EOF.
type limitedSource struct {
	data []byte
	read int
}

func (l *limitedSource) Read(p []byte) (int, error) {
	if l.read >= len(l.data) {
		return 0, io.EOF
	}
	n := copy(p, l.data[l.read:])
	l.read += n
	return n, nil
}

func TestReadMessageErrors(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		max  int
		want error
	}{
		{"оборванный заголовок", []byte{0, 0, 0}, 100, io.ErrUnexpectedEOF},
		{"оборванное тело", []byte{0, 0, 0, 0, 10, 'a', 'b', 'c'}, 100, io.ErrUnexpectedEOF},
		{"тело отсутствует", []byte{0, 0, 0, 0, 1}, 100, io.ErrUnexpectedEOF},
		{"слишком большое", []byte{0, 0, 0, 0, 11}, 10, ErrMessageTooLarge},
		{"4 ГБ из сети", []byte{0, 0xff, 0xff, 0xff, 0xff}, 1 << 20, ErrMessageTooLarge},
		{"плохой флаг", []byte{2, 0, 0, 0, 0}, 100, ErrInvalidFlag},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := &limitedSource{data: c.data}
			_, _, err := ReadMessage(src, c.max)
			if !errors.Is(err, c.want) {
				t.Errorf("ошибка %v, ожидалась %v", err, c.want)
			}
		})
	}
	// ровно на границе лимита — можно
	var buf bytes.Buffer
	_ = WriteMessage(&buf, []byte("0123456789"), false)
	if msg, _, err := ReadMessage(&buf, 10); err != nil || len(msg) != 10 {
		t.Errorf("сообщение ровно maxSize: %q, %v", msg, err)
	}
}
