// Package tail — последние n строк из io.ReadSeeker без чтения всего файла.
package tail

import "errors"

// ErrNegative возвращается при n < 0.
var ErrNegative = errors.New("tail: n не может быть отрицательным")

// ChunkSize — размер блока, которым Tail читает поток с конца.
const ChunkSize = 4096
