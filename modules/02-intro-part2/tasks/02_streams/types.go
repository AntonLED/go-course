package streams

import "io"

// CountingWriter оборачивает io.Writer и считает, сколько байт было
// фактически записано во вложенный writer (n из его Write, даже если
// вместе с n вернулась ошибка).
type CountingWriter struct {
	w io.Writer
	n int64
}
