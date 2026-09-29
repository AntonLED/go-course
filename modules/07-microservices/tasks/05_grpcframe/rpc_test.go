package grpcframe

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// pair поднимает сервер и клиента на концах net.Pipe.
func pair(t *testing.T, srv *Server) *Client {
	t.Helper()
	cConn, sConn := net.Pipe()
	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.Serve(sConn) }()
	c := NewClient(cConn)
	t.Cleanup(func() {
		c.Close()
		select {
		case err := <-serveDone:
			if err != nil {
				t.Errorf("Serve вернул %v после закрытия клиента, ожидался nil", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("Serve не завершился после закрытия соединения клиентом")
		}
	})
	return c
}

func newTestServer() *Server {
	s := NewServer()
	s.HandleUnary("/test.Echo/Upper", func(ctx context.Context, req []byte) ([]byte, error) {
		return bytes.ToUpper(req), nil
	})
	s.HandleUnary("/test.Echo/NotFound", func(ctx context.Context, req []byte) ([]byte, error) {
		return nil, Errorf(NotFound, "user %s not found", req)
	})
	s.HandleUnary("/test.Echo/Plain", func(ctx context.Context, req []byte) ([]byte, error) {
		return nil, errors.New("plain failure")
	})
	s.HandleUnary("/test.Echo/Panic", func(ctx context.Context, req []byte) ([]byte, error) {
		panic("boom")
	})
	s.HandleUnary("/test.Echo/Empty", func(ctx context.Context, req []byte) ([]byte, error) {
		return []byte{}, nil
	})
	s.HandleStream("/test.Echo/Count", func(ctx context.Context, req []byte, send func([]byte) error) error {
		n, _ := strconv.Atoi(string(req))
		for i := 0; i < n; i++ {
			if err := send([]byte(strconv.Itoa(i))); err != nil {
				return err
			}
		}
		return nil
	})
	s.HandleStream("/test.Echo/Abort", func(ctx context.Context, req []byte, send func([]byte) error) error {
		_ = send([]byte("a"))
		_ = send([]byte("b"))
		return Errorf(Aborted, "stopped")
	})
	return s
}

func TestUnary(t *testing.T) {
	c := pair(t, newTestServer())
	ctx := context.Background()
	resp, err := c.Call(ctx, "/test.Echo/Upper", []byte("hello"))
	if err != nil || string(resp) != "HELLO" {
		t.Fatalf("Call(Upper) = %q, %v; ожидалось HELLO, nil", resp, err)
	}
	resp, err = c.Call(ctx, "/test.Echo/Empty", nil)
	if err != nil || len(resp) != 0 {
		t.Errorf("Call(Empty) = %q, %v; ожидалось пустое сообщение без ошибки", resp, err)
	}
}

func TestStatusCodes(t *testing.T) {
	c := pair(t, newTestServer())
	cases := []struct {
		method string
		code   Code
		msg    string
	}{
		{"/test.Echo/NotFound", NotFound, "user bob not found"},
		{"/test.Echo/Plain", Unknown, "plain failure"},
		{"/test.Echo/Panic", Internal, ""},
		{"/test.Echo/Missing", Unimplemented, ""},
	}
	for _, cs := range cases {
		_, err := c.Call(context.Background(), cs.method, []byte("bob"))
		var st *Status
		if !errors.As(err, &st) || st.Code != cs.code {
			t.Errorf("%s: ошибка %v, ожидался *Status с кодом %v", cs.method, err, cs.code)
			continue
		}
		if cs.msg != "" && st.Message != cs.msg {
			t.Errorf("%s: сообщение %q, ожидалось %q", cs.method, st.Message, cs.msg)
		}
	}
	// соединение живо после ошибок и паники
	if resp, err := c.Call(context.Background(), "/test.Echo/Upper", []byte("ok")); err != nil || string(resp) != "OK" {
		t.Errorf("после ошибок Call = %q, %v", resp, err)
	}
}

func TestServerStreaming(t *testing.T) {
	c := pair(t, newTestServer())
	st, err := c.Stream(context.Background(), "/test.Echo/Count", []byte("5"))
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var got []string
	for {
		msg, err := st.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		got = append(got, string(msg))
	}
	if strings.Join(got, ",") != "0,1,2,3,4" {
		t.Errorf("получено %v, ожидалось 0..4", got)
	}
	if _, err := st.Recv(); err != io.EOF {
		t.Errorf("повторный Recv после конца: %v, ожидался io.EOF", err)
	}

	st, _ = c.Stream(context.Background(), "/test.Echo/Abort", nil)
	m1, _ := st.Recv()
	m2, _ := st.Recv()
	_, err = st.Recv()
	if string(m1) != "a" || string(m2) != "b" || CodeOf(err) != Aborted {
		t.Errorf("Abort: %q, %q, %v; ожидалось a, b и Aborted", m1, m2, err)
	}
}

func TestManyMessagesSlowConsumer(t *testing.T) {
	// Поток из 2000 сообщений, который никто не читает, не должен блокировать другие вызовы.
	c := pair(t, newTestServer())
	st, err := c.Stream(context.Background(), "/test.Echo/Count", []byte("2000"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if resp, err := c.Call(ctx, "/test.Echo/Upper", []byte("x")); err != nil || string(resp) != "X" {
		t.Fatalf("вызов, пока соседний поток не вычитан: %q, %v (head-of-line blocking?)", resp, err)
	}
	n := 0
	for {
		if _, err := st.Recv(); err != nil {
			if err != io.EOF {
				t.Fatalf("Recv: %v", err)
			}
			break
		}
		n++
	}
	if n != 2000 {
		t.Errorf("получено %d сообщений, ожидалось 2000", n)
	}
}

func TestMultiplexing(t *testing.T) {
	s := newTestServer()
	release := make(chan struct{})
	s.HandleUnary("/test.Echo/Slow", func(ctx context.Context, req []byte) ([]byte, error) {
		select {
		case <-release:
			return []byte("slow"), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	s.HandleUnary("/test.Echo/Jitter", func(ctx context.Context, req []byte) ([]byte, error) {
		n, _ := strconv.Atoi(string(req))
		time.Sleep(time.Duration(n%4) * time.Millisecond)
		return []byte("r" + string(req)), nil
	})
	c := pair(t, s)

	slowDone := make(chan string, 1)
	go func() {
		resp, _ := c.Call(context.Background(), "/test.Echo/Slow", nil)
		slowDone <- string(resp)
	}()

	// Пока медленный вызов висит, 50 параллельных быстрых должны пройти и не перепутаться.
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := strconv.Itoa(i)
			resp, err := c.Call(context.Background(), "/test.Echo/Jitter", []byte(req))
			if err != nil || string(resp) != "r"+req {
				t.Errorf("Call(%s) = %q, %v; ожидалось %q", req, resp, err, "r"+req)
			}
		}(i)
	}
	wg.Wait()
	select {
	case <-slowDone:
		t.Fatal("медленный вызов завершился раньше времени")
	default:
	}
	close(release)
	select {
	case r := <-slowDone:
		if r != "slow" {
			t.Errorf("медленный вызов вернул %q", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("медленный вызов не завершился")
	}
}

func TestDeadlinePropagation(t *testing.T) {
	s := newTestServer()
	handlerDone := make(chan error, 1)
	hasDeadline := make(chan bool, 1)
	s.HandleUnary("/test.Echo/Block", func(ctx context.Context, req []byte) ([]byte, error) {
		_, ok := ctx.Deadline()
		hasDeadline <- ok
		<-ctx.Done()
		handlerDone <- ctx.Err()
		return nil, ctx.Err()
	})
	c := pair(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Call(ctx, "/test.Echo/Block", nil)
	if CodeOf(err) != DeadlineExceeded {
		t.Errorf("Call с таймаутом: %v, ожидался DeadlineExceeded", err)
	}
	if el := time.Since(start); el > time.Second {
		t.Errorf("Call ждал %v, хотя таймаут 50ms", el)
	}
	if ok := <-hasDeadline; !ok {
		t.Error("контекст обработчика должен иметь дедлайн, переданный клиентом")
	}
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("контекст обработчика не отменился после дедлайна клиента")
	}
}

func TestCancelPropagation(t *testing.T) {
	s := newTestServer()
	started := make(chan struct{})
	handlerDone := make(chan error, 1)
	s.HandleStream("/test.Echo/Forever", func(ctx context.Context, req []byte, send func([]byte) error) error {
		close(started)
		<-ctx.Done()
		handlerDone <- ctx.Err()
		return ctx.Err()
	})
	c := pair(t, s)

	ctx, cancel := context.WithCancel(context.Background()) // без дедлайна!
	st, err := c.Stream(ctx, "/test.Echo/Forever", nil)
	if err != nil {
		t.Fatal(err)
	}
	<-started
	errc := make(chan error, 1)
	go func() {
		_, err := st.Recv()
		errc <- err
	}()
	cancel()
	select {
	case err := <-errc:
		if CodeOf(err) != Canceled {
			t.Errorf("Recv после cancel: %v, ожидался Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Recv не вернулся после отмены контекста")
	}
	select {
	case err := <-handlerDone:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("ctx.Err() обработчика = %v, ожидался Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("сервер не отменил обработчик: клиент должен отправлять CANCEL")
	}
	// уже отменённый ctx — сразу ошибка
	if _, err := c.Call(ctx, "/test.Echo/Upper", nil); CodeOf(err) != Canceled {
		t.Errorf("Call с отменённым ctx: %v, ожидался Canceled", err)
	}
}

func TestMaxRecvMsgSize(t *testing.T) {
	s := newTestServer()
	s.MaxRecvMsgSize = 10
	c := pair(t, s)
	if _, err := c.Call(context.Background(), "/test.Echo/Upper", make([]byte, 20)); CodeOf(err) != ResourceExhausted {
		t.Errorf("запрос больше лимита: %v, ожидался ResourceExhausted", err)
	}
	if resp, err := c.Call(context.Background(), "/test.Echo/Upper", []byte("0123456789")); err != nil || string(resp) != "0123456789" {
		t.Errorf("запрос ровно в лимит: %q, %v", resp, err)
	}
}

func TestConnectionLoss(t *testing.T) {
	s := NewServer()
	started := make(chan struct{})
	s.HandleUnary("/test.Echo/Hang", func(ctx context.Context, req []byte) ([]byte, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	cConn, sConn := net.Pipe()
	go s.Serve(sConn)
	c := NewClient(cConn)
	defer c.Close()

	errc := make(chan error, 1)
	go func() {
		_, err := c.Call(context.Background(), "/test.Echo/Hang", nil)
		errc <- err
	}()
	<-started
	sConn.Close() // сервер «упал»
	select {
	case err := <-errc:
		if CodeOf(err) != Unavailable {
			t.Errorf("обрыв соединения: %v, ожидался Unavailable", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("вызов завис после обрыва соединения")
	}
	if _, err := c.Call(context.Background(), "/test.Echo/Hang", nil); CodeOf(err) != Unavailable {
		t.Errorf("вызов после обрыва: %v, ожидался Unavailable", err)
	}
}

// ---- проверка формата фреймов на «сыром» соединении

func rawFrame(stream uint32, kind byte, payload []byte) []byte {
	b := make([]byte, 9, 9+len(payload))
	binary.BigEndian.PutUint32(b[0:4], stream)
	b[4] = kind
	binary.BigEndian.PutUint32(b[5:9], uint32(len(payload)))
	return append(b, payload...)
}

func readRawFrame(t *testing.T, r io.Reader) (uint32, byte, []byte) {
	t.Helper()
	hdr := make([]byte, 9)
	if _, err := io.ReadFull(r, hdr); err != nil {
		t.Fatalf("чтение заголовка фрейма: %v", err)
	}
	p := make([]byte, binary.BigEndian.Uint32(hdr[5:9]))
	if _, err := io.ReadFull(r, p); err != nil {
		t.Fatalf("чтение payload: %v", err)
	}
	return binary.BigEndian.Uint32(hdr[0:4]), hdr[4], p
}

func TestServerWireFormat(t *testing.T) {
	cConn, sConn := net.Pipe()
	defer cConn.Close()
	go newTestServer().Serve(sConn)
	_ = cConn.SetDeadline(time.Now().Add(2 * time.Second))

	method := "/test.Echo/Upper"
	headers := append(make([]byte, 8), method...) // таймаут 0
	go func() {
		cConn.Write(rawFrame(7, FrameHeaders, headers))
		cConn.Write(rawFrame(7, FrameData, []byte{0, 0, 0, 0, 3, 'a', 'b', 'c'}))
	}()
	id, kind, p := readRawFrame(t, cConn)
	if id != 7 || kind != FrameData || !bytes.Equal(p, []byte{0, 0, 0, 0, 3, 'A', 'B', 'C'}) {
		t.Fatalf("первый фрейм: stream=%d kind=%d payload=% x; ожидался DATA stream=7 с 00 00 00 00 03 'ABC'", id, kind, p)
	}
	id, kind, p = readRawFrame(t, cConn)
	if id != 7 || kind != FrameTrailers || !bytes.Equal(p, []byte{0, 0, 0, 0}) {
		t.Fatalf("второй фрейм: stream=%d kind=%d payload=% x; ожидались TRAILERS stream=7 с кодом 0", id, kind, p)
	}
}

func TestClientWireFormat(t *testing.T) {
	cConn, sConn := net.Pipe()
	defer sConn.Close()
	c := NewClient(cConn)
	defer c.Close()
	_ = sConn.SetDeadline(time.Now().Add(2 * time.Second))

	type result struct {
		resp []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		resp, err := c.Call(ctx, "/svc/M", []byte("q"))
		done <- result{resp, err}
	}()

	id, kind, p := readRawFrame(t, sConn)
	if kind != FrameHeaders || id%2 != 1 {
		t.Fatalf("первый фрейм клиента: stream=%d kind=%d, ожидался HEADERS с нечётным id", id, kind)
	}
	timeout := time.Duration(binary.BigEndian.Uint64(p[:8]))
	if string(p[8:]) != "/svc/M" || timeout <= 0 || timeout > time.Second {
		t.Fatalf("HEADERS payload: метод %q, таймаут %v; ожидалось /svc/M и (0, 1s]", p[8:], timeout)
	}
	id2, kind, p := readRawFrame(t, sConn)
	if id2 != id || kind != FrameData || !bytes.Equal(p, []byte{0, 0, 0, 0, 1, 'q'}) {
		t.Fatalf("второй фрейм клиента: stream=%d kind=%d payload=% x", id2, kind, p)
	}
	sConn.Write(rawFrame(id, FrameTrailers, append([]byte{0, 0, 0, byte(PermissionDenied)}, "nope"...)))
	r := <-done
	var st *Status
	if !errors.As(r.err, &st) || st.Code != PermissionDenied || st.Message != "nope" {
		t.Errorf("Call вернул %v, ожидался Status{PermissionDenied, nope}", r.err)
	}
}

func TestCodeString(t *testing.T) {
	if fmt.Sprint(DeadlineExceeded) != "DeadlineExceeded" || CodeOf(nil) != OK {
		t.Error("types.go повреждён")
	}
}
