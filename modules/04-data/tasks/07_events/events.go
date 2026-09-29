//go:build !solution

package events

import "io"

// MarshalJSON: Duration(90*time.Second) → "1m30s".
func (d Duration) MarshalJSON() ([]byte, error) {
	// TODO
	panic("TODO")
}

// UnmarshalJSON: "1m30s" или число секунд (90, 1.5). Иное — ошибка.
func (d *Duration) UnmarshalJSON(b []byte) error {
	// TODO
	panic("TODO")
}

// Marshal кодирует событие в «плоский» JSON с полем "type" первым:
//
//	{"type":"login","user":"ann","at":"...","ip":"..."}
//
// Подсказка: встраивание в анонимную структуру поднимает поля наверх.
func Marshal(e Event) ([]byte, error) {
	// TODO
	panic("TODO")
}

// Unmarshal разбирает событие по полю "type" и возвращает значение
// конкретного типа (Login, Purchase или Session — не указатель).
// Нет type → ErrNoType, неизвестный type → ErrUnknownType (errors.Is).
// Неизвестные поля — ошибка (DisallowUnknownFields). Значение "type" — не
// «неизвестное поле»!
func Unmarshal(data []byte) (Event, error) {
	// TODO
	panic("TODO")
}

// Stream разбирает JSON-массив событий из r потоково (json.Decoder.Token/More/
// Decode), вызывая fn для каждого. Если fn вернула ошибку — прекратить чтение
// и вернуть её. Корень — не массив или мусор после ']' → ошибка.
func Stream(r io.Reader, fn func(Event) error) error {
	// TODO
	panic("TODO")
}
