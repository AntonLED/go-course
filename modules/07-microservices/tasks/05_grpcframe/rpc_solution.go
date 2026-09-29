//go:build solution

package grpcframe

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// encodeMessage упаковывает сообщение в payload DATA-фрейма.
func encodeMessage(msg []byte) []byte {
	var b bytes.Buffer
	_ = WriteMessage(&b, msg, false) // запись в bytes.Buffer не падает
	return b.Bytes()
}

func encodeTrailers(err error) []byte {
	code := CodeOf(err)
	var msg string
	var st *Status
	switch {
	case err == nil:
	case errors.As(err, &st):
		msg = st.Message
	default:
		msg = err.Error()
	}
	p := make([]byte, 4, 4+len(msg))
	binary.BigEndian.PutUint32(p, uint32(code))
	return append(p, msg...)
}

// ------------------------------------------------------------------ сервер

// Server — мини-RPC сервер. Один Server может обслуживать много соединений.
type Server struct {
	// MaxRecvMsgSize — лимит размера запроса (0 → DefaultMaxRecvMsgSize).
	MaxRecvMsgSize int

	mu      sync.RWMutex
	unary   map[string]UnaryHandler
	streams map[string]StreamHandler
}

// NewServer создаёт сервер без методов.
func NewServer() *Server {
	return &Server{unary: map[string]UnaryHandler{}, streams: map[string]StreamHandler{}}
}

// HandleUnary регистрирует unary-метод (например, "/inventory.Inventory/Get").
func (s *Server) HandleUnary(method string, h UnaryHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unary[method] = h
}

// HandleStream регистрирует server-streaming метод.
func (s *Server) HandleStream(method string, h StreamHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streams[method] = h
}

type serverStream struct {
	method  string
	timeout time.Duration
	started bool
	cancel  context.CancelFunc
}

// Serve обслуживает соединение, пока клиент его не закроет. Каждый поток
// обрабатывается в своей горутине; запись во conn сериализуется mutex-ом.
// Перед возвратом отменяет все активные обработчики, закрывает conn и
// дожидается их завершения.
func (s *Server) Serve(conn net.Conn) error {
	var (
		wmu     sync.Mutex
		smu     sync.Mutex
		wg      sync.WaitGroup
		streams = map[uint32]*serverStream{}
	)
	baseCtx, cancelAll := context.WithCancel(context.Background())
	defer func() {
		cancelAll()
		conn.Close()
		wg.Wait()
	}()
	write := func(f frame) error {
		wmu.Lock()
		defer wmu.Unlock()
		return writeFrame(conn, f)
	}

	for {
		f, err := readFrame(conn)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		smu.Lock()
		switch f.kind {
		case FrameHeaders:
			if len(f.payload) < 8 {
				smu.Unlock()
				return fmt.Errorf("grpcframe: malformed HEADERS on stream %d", f.stream)
			}
			streams[f.stream] = &serverStream{
				timeout: time.Duration(binary.BigEndian.Uint64(f.payload[:8])),
				method:  string(f.payload[8:]),
			}
		case FrameData:
			st := streams[f.stream]
			if st == nil || st.started {
				break // DATA без HEADERS или повторный — игнорируем
			}
			st.started = true
			var ctx context.Context
			if st.timeout > 0 {
				ctx, st.cancel = context.WithTimeout(baseCtx, st.timeout)
			} else {
				ctx, st.cancel = context.WithCancel(baseCtx)
			}
			id, cancel, method, payload := f.stream, st.cancel, st.method, f.payload
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer cancel()
				err := s.dispatch(ctx, id, method, payload, write)
				_ = write(frame{stream: id, kind: FrameTrailers, payload: encodeTrailers(err)})
				smu.Lock()
				delete(streams, id)
				smu.Unlock()
			}()
		case FrameCancel:
			if st := streams[f.stream]; st != nil {
				if st.cancel != nil {
					st.cancel()
				}
				delete(streams, f.stream)
			}
		}
		smu.Unlock()
	}
}

// dispatch декодирует запрос и вызывает обработчик. Возвращает итоговую ошибку потока.
func (s *Server) dispatch(ctx context.Context, id uint32, method string, payload []byte, write func(frame) error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = Errorf(Internal, "panic in handler: %v", p)
		}
	}()
	limit := s.MaxRecvMsgSize
	if limit <= 0 {
		limit = DefaultMaxRecvMsgSize
	}
	req, compressed, err := ReadMessage(bytes.NewReader(payload), limit)
	switch {
	case errors.Is(err, ErrMessageTooLarge):
		return Errorf(ResourceExhausted, "request larger than %d bytes", limit)
	case err != nil:
		return Errorf(Internal, "malformed request: %v", err)
	case compressed:
		return Errorf(Unimplemented, "compression is not supported")
	}

	send := func(msg []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return write(frame{stream: id, kind: FrameData, payload: encodeMessage(msg)})
	}

	s.mu.RLock()
	uh, isUnary := s.unary[method]
	sh, isStream := s.streams[method]
	s.mu.RUnlock()
	switch {
	case isUnary:
		resp, err := uh(ctx, req)
		if err != nil {
			return err
		}
		return send(resp)
	case isStream:
		return sh(ctx, req, send)
	default:
		return Errorf(Unimplemented, "unknown method %s", method)
	}
}

// ------------------------------------------------------------------ клиент

// clientStream — входящая очередь одного потока. Очередь не ограничена,
// чтобы читатель соединения никогда не блокировался на медленном потребителе
// (иначе один поток остановил бы все остальные — head-of-line blocking).
type clientStream struct {
	mu       sync.Mutex
	queue    [][]byte
	finished bool
	err      error // итог потока: nil — OK
	notify   chan struct{}
}

func newClientStream() *clientStream { return &clientStream{notify: make(chan struct{}, 1)} }

func (st *clientStream) wake() {
	select {
	case st.notify <- struct{}{}:
	default:
	}
}

func (st *clientStream) push(msg []byte) {
	st.mu.Lock()
	if !st.finished {
		st.queue = append(st.queue, msg)
	}
	st.mu.Unlock()
	st.wake()
}

func (st *clientStream) finish(err error) {
	st.mu.Lock()
	if !st.finished {
		st.finished, st.err = true, err
	}
	st.mu.Unlock()
	st.wake()
}

// Client — клиент мини-RPC; безопасен для конкурентного использования.
type Client struct {
	conn net.Conn
	wmu  sync.Mutex

	mu      sync.Mutex
	streams map[uint32]*clientStream
	nextID  uint32
	closed  bool
}

// NewClient создаёт клиента и запускает горутину-читателя соединения.
func NewClient(conn net.Conn) *Client {
	c := &Client{conn: conn, streams: map[uint32]*clientStream{}, nextID: 1}
	go c.readLoop()
	return c
}

// Close закрывает соединение; все незавершённые вызовы получат Unavailable.
func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) write(f frame) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return writeFrame(c.conn, f)
}

func (c *Client) readLoop() {
	for {
		f, err := readFrame(c.conn)
		if err != nil {
			c.failAll(Errorf(Unavailable, "connection closed: %v", err))
			return
		}
		c.mu.Lock()
		st := c.streams[f.stream]
		if f.kind == FrameTrailers {
			delete(c.streams, f.stream)
		}
		c.mu.Unlock()
		if st == nil {
			continue // поток уже отменён — фреймы опоздали
		}
		switch f.kind {
		case FrameData:
			msg, _, err := ReadMessage(bytes.NewReader(f.payload), MaxFramePayload)
			if err != nil {
				st.finish(Errorf(Internal, "malformed response: %v", err))
				continue
			}
			st.push(msg)
		case FrameTrailers:
			if len(f.payload) < 4 {
				st.finish(Errorf(Internal, "malformed trailers"))
				continue
			}
			code := Code(binary.BigEndian.Uint32(f.payload[:4]))
			if code == OK {
				st.finish(nil)
			} else {
				st.finish(&Status{Code: code, Message: string(f.payload[4:])})
			}
		}
	}
}

func (c *Client) failAll(err error) {
	c.mu.Lock()
	c.closed = true
	streams := c.streams
	c.streams = map[uint32]*clientStream{}
	c.mu.Unlock()
	for _, st := range streams {
		st.finish(err)
	}
}

func ctxStatus(err error) error { return &Status{Code: CodeOf(err), Message: err.Error()} }

// open открывает поток: HEADERS (с оставшимся временем до дедлайна) + DATA.
func (c *Client) open(ctx context.Context, method string, req []byte) (uint32, *clientStream, error) {
	if err := ctx.Err(); err != nil {
		return 0, nil, ctxStatus(err)
	}
	var timeout time.Duration
	if dl, ok := ctx.Deadline(); ok {
		timeout = time.Until(dl)
		if timeout <= 0 {
			return 0, nil, ctxStatus(context.DeadlineExceeded)
		}
	}
	st := newClientStream()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return 0, nil, Errorf(Unavailable, "connection is closed")
	}
	id := c.nextID
	c.nextID += 2
	c.streams[id] = st
	c.mu.Unlock()

	hdr := make([]byte, 8, 8+len(method))
	binary.BigEndian.PutUint64(hdr, uint64(timeout))
	hdr = append(hdr, method...)
	c.wmu.Lock()
	err := writeFrame(c.conn, frame{stream: id, kind: FrameHeaders, payload: hdr})
	if err == nil {
		err = writeFrame(c.conn, frame{stream: id, kind: FrameData, payload: encodeMessage(req)})
	}
	c.wmu.Unlock()
	if err != nil {
		c.forget(id)
		return 0, nil, Errorf(Unavailable, "write: %v", err)
	}
	return id, st, nil
}

func (c *Client) forget(id uint32) {
	c.mu.Lock()
	delete(c.streams, id)
	c.mu.Unlock()
}

// recv ждёт очередное сообщение потока. io.EOF — поток завершён со статусом OK.
func (c *Client) recv(ctx context.Context, id uint32, st *clientStream) ([]byte, error) {
	for {
		st.mu.Lock()
		if len(st.queue) > 0 {
			msg := st.queue[0]
			st.queue = st.queue[1:]
			st.mu.Unlock()
			return msg, nil
		}
		if st.finished {
			err := st.err
			st.mu.Unlock()
			if err == nil {
				return nil, io.EOF
			}
			return nil, err
		}
		st.mu.Unlock()

		select {
		case <-st.notify:
		case <-ctx.Done():
			// Сообщаем серверу, что результат больше не нужен.
			c.forget(id)
			_ = c.write(frame{stream: id, kind: FrameCancel})
			st.finish(ctxStatus(ctx.Err()))
			return nil, ctxStatus(ctx.Err())
		}
	}
}

// Call выполняет unary-вызов.
func (c *Client) Call(ctx context.Context, method string, req []byte) ([]byte, error) {
	id, st, err := c.open(ctx, method, req)
	if err != nil {
		return nil, err
	}
	resp, err := c.recv(ctx, id, st)
	if err == io.EOF {
		return nil, Errorf(Internal, "no response message for unary call")
	}
	if err != nil {
		return nil, err
	}
	// Ждём трейлеры: статус мог оказаться ошибкой.
	if _, err := c.recv(ctx, id, st); err != io.EOF {
		if err == nil {
			return nil, Errorf(Internal, "unary call returned several messages")
		}
		return nil, err
	}
	return resp, nil
}

// ClientStream — клиентская сторона server-streaming вызова.
type ClientStream struct {
	c   *Client
	ctx context.Context
	id  uint32
	st  *clientStream
}

// Stream открывает server-streaming вызов.
func (c *Client) Stream(ctx context.Context, method string, req []byte) (*ClientStream, error) {
	id, st, err := c.open(ctx, method, req)
	if err != nil {
		return nil, err
	}
	return &ClientStream{c: c, ctx: ctx, id: id, st: st}, nil
}

// Recv возвращает очередное сообщение; io.EOF — сервер завершил поток с OK;
// *Status — с ошибкой.
func (s *ClientStream) Recv() ([]byte, error) {
	return s.c.recv(s.ctx, s.id, s.st)
}
