package slugtests

import "unicode"

// MaxLen — максимальная длина слага в рунах.
const MaxLen = 32

// Slugify — эталонная реализация (её менять не нужно, её нужно тестировать).
//
// Правила:
//  1. Буквы (unicode.IsLetter) и цифры (unicode.IsDigit) сохраняются и переводятся
//     в нижний регистр (unicode.ToLower) — в том числе кириллица и другие алфавиты.
//  2. Апострофы ' и ’ удаляются бесследно: "Don't" → "dont".
//  3. Любая последовательность прочих символов (пробелы, пунктуация, '_', эмодзи…)
//     заменяется одним дефисом.
//  4. В начале и в конце дефисов нет.
//  5. Результат не длиннее MaxLen рун; если после обрезки на конце дефис — он удаляется.
func Slugify(s string) string {
	out := make([]rune, 0, len(s))
	pending := false
	for _, r := range s {
		switch {
		case r == '\'' || r == '’':
			// удаляем, разделителем не считаем
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if pending && len(out) > 0 {
				out = append(out, '-')
			}
			pending = false
			out = append(out, unicode.ToLower(r))
		default:
			pending = true
		}
	}
	if len(out) > MaxLen {
		out = out[:MaxLen]
	}
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	return string(out)
}
