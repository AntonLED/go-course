package rpcauth

// Этот файл повторяет в миниатюре API google.golang.org/grpc (metadata, peer,
// status/codes, интерсепторы, credentials.PerRPCCredentials), чтобы задачу
// можно было решать на стандартной библиотеке.

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
)

// ---- metadata

// MD — метаданные вызова (аналог metadata.MD). Ключи — в нижнем регистре.
type MD map[string][]string

// Get возвращает значения по ключу (регистр ключа не важен).
func (md MD) Get(key string) []string { return md[strings.ToLower(key)] }

type (
	incomingKey struct{}
	outgoingKey struct{}
	peerKey     struct{}
)

// NewIncomingContext — метаданные входящего вызова (их кладёт транспорт сервера).
func NewIncomingContext(ctx context.Context, md MD) context.Context {
	return context.WithValue(ctx, incomingKey{}, md)
}

// FromIncomingContext — метаданные входящего вызова.
func FromIncomingContext(ctx context.Context) (MD, bool) {
	md, ok := ctx.Value(incomingKey{}).(MD)
	return md, ok
}

// NewOutgoingContext — метаданные исходящего вызова (их отправит клиент).
func NewOutgoingContext(ctx context.Context, md MD) context.Context {
	return context.WithValue(ctx, outgoingKey{}, md)
}

// FromOutgoingContext — метаданные исходящего вызова.
func FromOutgoingContext(ctx context.Context) (MD, bool) {
	md, ok := ctx.Value(outgoingKey{}).(MD)
	return md, ok
}

// ---- peer

// Peer — информация о клиенте (аналог peer.Peer + credentials.TLSInfo).
type Peer struct {
	Addr net.Addr
	TLS  *tls.ConnectionState // nil — соединение без TLS
}

// NewPeerContext кладёт Peer в контекст (это делает транспорт сервера).
func NewPeerContext(ctx context.Context, p *Peer) context.Context {
	return context.WithValue(ctx, peerKey{}, p)
}

// PeerFromContext возвращает Peer.
func PeerFromContext(ctx context.Context) (*Peer, bool) {
	p, ok := ctx.Value(peerKey{}).(*Peer)
	return p, ok && p != nil
}

// ---- status

// Code — код статуса gRPC.
type Code uint32

const (
	OK               Code = 0
	PermissionDenied Code = 7
	Internal         Code = 13
	Unauthenticated  Code = 16
)

// Status — ошибка со статус-кодом.
type Status struct {
	Code    Code
	Message string
}

func (s *Status) Error() string {
	return fmt.Sprintf("rpc error: code = %d desc = %s", s.Code, s.Message)
}

// Errorf создаёт *Status.
func Errorf(c Code, format string, a ...any) error {
	return &Status{Code: c, Message: fmt.Sprintf(format, a...)}
}

// CodeOf возвращает код ошибки: nil → OK, *Status → его код, прочее → 2 (Unknown).
func CodeOf(err error) Code {
	var s *Status
	switch {
	case err == nil:
		return OK
	case errors.As(err, &s):
		return s.Code
	default:
		return 2
	}
}

// ---- интерсепторы

// UnaryServerInfo — информация о вызове.
type UnaryServerInfo struct {
	FullMethod string // "/package.Service/Method"
}

// UnaryHandler — обработчик вызова.
type UnaryHandler func(ctx context.Context, req any) (any, error)

// UnaryServerInterceptor — серверный интерсептор (сигнатура как в grpc-go).
type UnaryServerInterceptor func(ctx context.Context, req any, info *UnaryServerInfo, handler UnaryHandler) (any, error)

// ---- credentials

// PerRPCCredentials — учётные данные, прикрепляемые к каждому вызову
// (аналог credentials.PerRPCCredentials).
type PerRPCCredentials interface {
	GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error)
	RequireTransportSecurity() bool
}

// ErrInsecureTransport — попытка отправить секрет по незащищённому каналу.
var ErrInsecureTransport = errors.New("rpcauth: credentials require transport security")

// Principal — аутентифицированный пользователь/клиент из токена.
type Principal struct {
	Subject string
	Scopes  []string
}
