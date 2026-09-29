//go:build !solution

// Package grpcframe — gRPC-подобный фрейминг сообщений и мини-RPC
// с мультиплексированием потоков поверх одного net.Conn.
package grpcframe

import "io"

// WriteMessage пишет сообщение в формате gRPC Length-Prefixed-Message:
//
//	[1 байт: compressed-flag 0|1][4 байта: длина, big-endian][сообщение]
//
// Всё должно уходить одним вызовом w.Write (соберите буфер целиком).
func WriteMessage(w io.Writer, msg []byte, compressed bool) error {
	// TODO: реализуйте
	return nil
}

// ReadMessage читает одно сообщение.
//   - поток пуст (0 байт) → io.EOF;
//   - заголовок или тело оборваны → io.ErrUnexpectedEOF;
//   - флаг не 0/1 → ErrInvalidFlag;
//   - длина > maxSize → ErrMessageTooLarge (проверить ДО выделения памяти и чтения тела).
func ReadMessage(r io.Reader, maxSize int) (msg []byte, compressed bool, err error) {
	// TODO: реализуйте (io.ReadFull, encoding/binary.BigEndian)
	return nil, false, io.EOF
}
