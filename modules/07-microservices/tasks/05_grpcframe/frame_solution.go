//go:build solution

// Package grpcframe — gRPC-подобный фрейминг сообщений и мини-RPC
// с мультиплексированием потоков поверх одного net.Conn.
package grpcframe

import (
	"encoding/binary"
	"errors"
	"io"
)

// WriteMessage пишет сообщение с 5-байтным префиксом одним вызовом Write —
// чтобы при записи из нескольких горутин (под mutex) фрейм не рвался.
func WriteMessage(w io.Writer, msg []byte, compressed bool) error {
	buf := make([]byte, HeaderLen+len(msg))
	if compressed {
		buf[0] = 1
	}
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(msg)))
	copy(buf[HeaderLen:], msg)
	_, err := w.Write(buf)
	return err
}

// ReadMessage читает одно сообщение.
func ReadMessage(r io.Reader, maxSize int) ([]byte, bool, error) {
	var hdr [HeaderLen]byte
	// io.ReadFull: 0 байт → io.EOF, часть → io.ErrUnexpectedEOF.
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, false, err
	}
	var compressed bool
	switch hdr[0] {
	case 0:
	case 1:
		compressed = true
	default:
		return nil, false, ErrInvalidFlag
	}
	n := binary.BigEndian.Uint32(hdr[1:])
	// Проверяем лимит ДО выделения памяти: иначе 4-байтная длина из сети
	// позволяет заставить нас аллоцировать 4 ГБ.
	if uint64(n) > uint64(maxSize) {
		return nil, false, ErrMessageTooLarge
	}
	msg := make([]byte, n)
	if _, err := io.ReadFull(r, msg); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, false, err
	}
	return msg, compressed, nil
}

// frame — транспортный фрейм.
type frame struct {
	stream  uint32
	kind    byte
	payload []byte
}

const frameHeaderLen = 9

func writeFrame(w io.Writer, f frame) error {
	buf := make([]byte, frameHeaderLen+len(f.payload))
	binary.BigEndian.PutUint32(buf[0:4], f.stream)
	buf[4] = f.kind
	binary.BigEndian.PutUint32(buf[5:9], uint32(len(f.payload)))
	copy(buf[frameHeaderLen:], f.payload)
	_, err := w.Write(buf)
	return err
}

var errFrameTooLarge = errors.New("grpcframe: frame too large")

func readFrame(r io.Reader) (frame, error) {
	var hdr [frameHeaderLen]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return frame{}, err
	}
	n := binary.BigEndian.Uint32(hdr[5:9])
	if n > MaxFramePayload {
		return frame{}, errFrameTooLarge
	}
	f := frame{stream: binary.BigEndian.Uint32(hdr[0:4]), kind: hdr[4], payload: make([]byte, n)}
	if _, err := io.ReadFull(r, f.payload); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return frame{}, err
	}
	return f, nil
}
