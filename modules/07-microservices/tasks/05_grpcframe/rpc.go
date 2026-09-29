//go:build !solution

package grpcframe

import (
	"context"
	"net"
)

// Server — мини-RPC сервер. Протокол фреймов описан в types.go.
type Server struct {
	// MaxRecvMsgSize — лимит размера запроса (0 → DefaultMaxRecvMsgSize).
	// Слишком большой запрос → трейлеры с ResourceExhausted (соединение живёт дальше).
	MaxRecvMsgSize int
	// TODO: поля (зарегистрированные методы под mutex)
}

// NewServer создаёт сервер без методов.
func NewServer() *Server {
	// TODO: реализуйте
	return &Server{}
}

// HandleUnary регистрирует unary-метод.
func (s *Server) HandleUnary(method string, h UnaryHandler) {
	// TODO: реализуйте
}

// HandleStream регистрирует server-streaming метод.
func (s *Server) HandleStream(method string, h StreamHandler) {
	// TODO: реализуйте
}

// Serve обслуживает conn, пока клиент его не закроет (тогда возвращает nil).
//
//   - HEADERS регистрирует поток (метод + таймаут), DATA с запросом запускает
//     обработчик в отдельной горутине с контекстом (WithTimeout, если таймаут > 0);
//   - ответ: DATA-фреймы, затем TRAILERS с кодом CodeOf(err) и текстом ошибки;
//   - неизвестный метод → Unimplemented, паника → Internal;
//   - CANCEL отменяет контекст обработчика;
//   - запись в conn из разных горутин — под mutex;
//   - при выходе: отменить все обработчики, закрыть conn, дождаться горутин.
func (s *Server) Serve(conn net.Conn) error {
	// TODO: реализуйте
	panic("TODO")
}

// Client — клиент мини-RPC; безопасен для конкурентного использования:
// много вызовов одновременно идут по одному соединению (мультиплексирование).
type Client struct {
	// TODO: поля
}

// NewClient создаёт клиента и запускает горутину, читающую фреймы из conn
// и раскладывающую их по потокам (stream id: 1, 3, 5, ...).
// Совет: читатель не должен блокироваться на медленном потребителе потока.
func NewClient(conn net.Conn) *Client {
	// TODO: реализуйте
	return &Client{}
}

// Close закрывает соединение; незавершённые и новые вызовы получают Unavailable.
func (c *Client) Close() error {
	// TODO: реализуйте
	return nil
}

// Call выполняет unary-вызов: HEADERS (таймаут = остаток до дедлайна ctx) + DATA,
// затем ждёт одно сообщение и трейлеры. Ошибки возвращаются как *Status.
// Отмена/дедлайн ctx → отправить CANCEL и вернуть Canceled/DeadlineExceeded.
func (c *Client) Call(ctx context.Context, method string, req []byte) ([]byte, error) {
	// TODO: реализуйте
	panic("TODO")
}

// ClientStream — клиентская сторона server-streaming вызова.
type ClientStream struct {
	// TODO: поля
}

// Stream открывает server-streaming вызов.
func (c *Client) Stream(ctx context.Context, method string, req []byte) (*ClientStream, error) {
	// TODO: реализуйте
	panic("TODO")
}

// Recv возвращает очередное сообщение; io.EOF — поток завершён со статусом OK;
// *Status — поток завершён с ошибкой.
func (s *ClientStream) Recv() ([]byte, error) {
	// TODO: реализуйте
	panic("TODO")
}
