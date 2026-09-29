package tracing

import (
	"encoding/hex"
	"errors"
	"time"
)

// TraceparentHeader — HTTP-заголовок W3C Trace Context.
const TraceparentHeader = "traceparent"

// ErrInvalidTraceparent — строка не соответствует W3C Trace Context.
var ErrInvalidTraceparent = errors.New("tracing: некорректный traceparent")

// TraceID — 16 байт, идентификатор всей трассы.
type TraceID [16]byte

// SpanID — 8 байт, идентификатор спана.
type SpanID [8]byte

func (t TraceID) String() string { return hex.EncodeToString(t[:]) }
func (s SpanID) String() string  { return hex.EncodeToString(s[:]) }

// IsValid: идентификатор из одних нулей недопустим.
func (t TraceID) IsValid() bool { return t != TraceID{} }
func (s SpanID) IsValid() bool  { return s != SpanID{} }

// SpanContext — то, что передаётся между процессами.
type SpanContext struct {
	TraceID TraceID
	SpanID  SpanID
	Sampled bool // бит 0x01 в trace-flags
}

// IsValid: оба идентификатора ненулевые.
func (sc SpanContext) IsValid() bool { return sc.TraceID.IsValid() && sc.SpanID.IsValid() }

// SpanData — снимок завершённого спана.
type SpanData struct {
	Name     string
	Context  SpanContext
	ParentID SpanID // нулевой у корневого спана
	Remote   bool   // родитель пришёл из другого процесса (через traceparent)
	Attrs    map[string]string
	Start    time.Time
	End      time.Time
}
