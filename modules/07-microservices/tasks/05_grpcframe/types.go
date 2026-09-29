package grpcframe

import (
	"context"
	"errors"
	"fmt"
)

// HeaderLen — длина префикса gRPC-сообщения: 1 байт флага сжатия +
// 4 байта длины (big-endian).
const HeaderLen = 5

// DefaultMaxRecvMsgSize — лимит размера входящего сообщения по умолчанию (как в grpc-go).
const DefaultMaxRecvMsgSize = 4 << 20

// MaxFramePayload — предельный размер payload одного транспортного фрейма.
const MaxFramePayload = 16 << 20

var (
	// ErrMessageTooLarge — длина из префикса превышает лимит.
	ErrMessageTooLarge = errors.New("grpcframe: message too large")
	// ErrInvalidFlag — байт флага сжатия не 0 и не 1.
	ErrInvalidFlag = errors.New("grpcframe: invalid compressed flag")
)

// Транспортный протокол мини-RPC (упрощённый HTTP/2).
// Каждый фрейм:
//
//	[4 байта stream id, BE][1 байт тип][4 байта длина payload, BE][payload]
//
// Клиент нумерует потоки нечётными числами: 1, 3, 5, ...
const (
	// FrameHeaders (клиент → сервер) открывает поток.
	// payload: [8 байт: таймаут в наносекундах, BE int64; 0 — без дедлайна][имя метода]
	FrameHeaders byte = 1
	// FrameData — сообщение. payload — gRPC-сообщение в формате WriteMessage
	// (5-байтный префикс + тело). Клиент шлёт ровно один DATA с запросом.
	FrameData byte = 2
	// FrameTrailers (сервер → клиент) закрывает поток.
	// payload: [4 байта: Code, BE][текст ошибки]
	FrameTrailers byte = 3
	// FrameCancel (клиент → сервер) — аналог RST_STREAM: отмена вызова. payload пустой.
	FrameCancel byte = 4
)

// Code — код статуса gRPC (google.golang.org/grpc/codes).
type Code uint32

const (
	OK                 Code = 0
	Canceled           Code = 1
	Unknown            Code = 2
	InvalidArgument    Code = 3
	DeadlineExceeded   Code = 4
	NotFound           Code = 5
	AlreadyExists      Code = 6
	PermissionDenied   Code = 7
	ResourceExhausted  Code = 8
	FailedPrecondition Code = 9
	Aborted            Code = 10
	OutOfRange         Code = 11
	Unimplemented      Code = 12
	Internal           Code = 13
	Unavailable        Code = 14
	DataLoss           Code = 15
	Unauthenticated    Code = 16
)

var codeNames = [...]string{"OK", "Canceled", "Unknown", "InvalidArgument", "DeadlineExceeded",
	"NotFound", "AlreadyExists", "PermissionDenied", "ResourceExhausted", "FailedPrecondition",
	"Aborted", "OutOfRange", "Unimplemented", "Internal", "Unavailable", "DataLoss", "Unauthenticated"}

func (c Code) String() string {
	if int(c) < len(codeNames) {
		return codeNames[c]
	}
	return fmt.Sprintf("Code(%d)", uint32(c))
}

// Status — ошибка со статус-кодом (аналог status.Status из grpc-go).
type Status struct {
	Code    Code
	Message string
}

func (s *Status) Error() string {
	return fmt.Sprintf("rpc error: code = %s desc = %s", s.Code, s.Message)
}

// Errorf создаёт *Status (аналог status.Errorf).
func Errorf(c Code, format string, a ...any) error {
	return &Status{Code: c, Message: fmt.Sprintf(format, a...)}
}

// CodeOf возвращает код ошибки (аналог status.Code):
// nil → OK; *Status → его код; ошибки контекста → DeadlineExceeded / Canceled;
// всё остальное → Unknown.
func CodeOf(err error) Code {
	var s *Status
	switch {
	case err == nil:
		return OK
	case errors.As(err, &s):
		return s.Code
	case errors.Is(err, context.DeadlineExceeded):
		return DeadlineExceeded
	case errors.Is(err, context.Canceled):
		return Canceled
	default:
		return Unknown
	}
}

// UnaryHandler обрабатывает unary-вызов.
type UnaryHandler func(ctx context.Context, req []byte) ([]byte, error)

// StreamHandler обрабатывает server-streaming вызов: send отправляет очередное
// сообщение клиенту (возвращает ошибку, если поток отменён).
type StreamHandler func(ctx context.Context, req []byte, send func(msg []byte) error) error
