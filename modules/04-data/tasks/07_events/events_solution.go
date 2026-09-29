//go:build solution

package events

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"time"
)

// MarshalJSON: Duration(90*time.Second) → "1m30s".
// Получатель — значение, чтобы работало и для Duration, и для *Duration.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalJSON: "1m30s" или число секунд.
// Получатель — указатель: метод меняет значение.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch x := v.(type) {
	case nil:
		// Соглашение encoding/json: null для Unmarshaler — no-op.
		return nil
	case string:
		dd, err := time.ParseDuration(x)
		if err != nil {
			return fmt.Errorf("events: duration: %w", err)
		}
		*d = Duration(dd)
	case float64: // числа в any всегда декодируются как float64
		if math.IsNaN(x) || math.Abs(x) > float64(math.MaxInt64)/float64(time.Second) {
			return fmt.Errorf("events: duration вне диапазона: %v", x)
		}
		*d = Duration(math.Round(x * float64(time.Second)))
	default:
		return fmt.Errorf("events: duration должна быть строкой или числом, получено %s", b)
	}
	return nil
}

// Marshal кодирует событие в «плоский» JSON с полем "type" первым.
//
// Поля встроенной (anonymous) структуры json поднимает на верхний уровень
// объекта, порядок — порядок объявления, поэтому "type" первым. Встраивать
// нужно конкретный тип: встроенный интерфейс (struct{Type string; Event})
// json кодирует как обычное поле "Event":{...}, без «поднятия».
func Marshal(e Event) ([]byte, error) {
	switch v := e.(type) {
	case Login:
		return json.Marshal(struct {
			Type string `json:"type"`
			*Login
		}{v.Kind(), &v})
	case Purchase:
		return json.Marshal(struct {
			Type string `json:"type"`
			*Purchase
		}{v.Kind(), &v})
	case Session:
		return json.Marshal(struct {
			Type string `json:"type"`
			*Session
		}{v.Kind(), &v})
	case nil:
		return nil, errors.New("events: nil-событие")
	}
	return nil, fmt.Errorf("%w: %T", ErrUnknownType, e)
}

// Unmarshal разбирает событие по полю "type".
func Unmarshal(data []byte) (Event, error) {
	// 1. Узнаём тип: декодируем только поле type, остальное игнорируется.
	var head struct {
		Type *string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, err
	}
	if head.Type == nil {
		return nil, ErrNoType
	}
	// 2. Декодируем в обёртку {Type; *Concrete}: с DisallowUnknownFields
	//    поле "type" иначе считалось бы неизвестным.
	decode := func(target any) error {
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		return dec.Decode(target)
	}
	switch *head.Type {
	case "login":
		var w struct {
			Type string `json:"type"`
			*Login
		}
		w.Login = new(Login)
		if err := decode(&w); err != nil {
			return nil, err
		}
		return *w.Login, nil
	case "purchase":
		var w struct {
			Type string `json:"type"`
			*Purchase
		}
		w.Purchase = new(Purchase)
		if err := decode(&w); err != nil {
			return nil, err
		}
		return *w.Purchase, nil
	case "session":
		var w struct {
			Type string `json:"type"`
			*Session
		}
		w.Session = new(Session)
		if err := decode(&w); err != nil {
			return nil, err
		}
		return *w.Session, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownType, *head.Type)
}

// Stream разбирает JSON-массив событий из r потоково.
func Stream(r io.Reader, fn func(Event) error) error {
	dec := json.NewDecoder(r)
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return fmt.Errorf("events: ожидался массив, получено %v", tok)
	}
	for i := 0; dec.More(); i++ {
		// RawMessage — сырые байты одного элемента; дальше — Unmarshal
		// с диспетчеризацией по type. В памяти всегда только один элемент.
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return fmt.Errorf("events: элемент %d: %w", i, err)
		}
		e, err := Unmarshal(raw)
		if err != nil {
			return fmt.Errorf("events: элемент %d: %w", i, err)
		}
		if err := fn(e); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil { // закрывающая ']'
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("events: лишние данные после массива")
	}
	return nil
}
