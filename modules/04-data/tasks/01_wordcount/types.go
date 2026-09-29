// Package wordcount — потоковый подсчёт строк, слов, байт и рун (аналог wc).
package wordcount

// Counts — результат подсчёта, аналог утилиты wc.
type Counts struct {
	Lines      int // число строк (последняя строка без '\n' тоже считается)
	Words      int // число слов (разделители — unicode.IsSpace)
	Bytes      int // число байт
	Runes      int // число рун (невалидный байт UTF-8 = 1 руна)
	MaxLineLen int // длина самой длинной строки в рунах, без '\n' и завершающего '\r'
}
