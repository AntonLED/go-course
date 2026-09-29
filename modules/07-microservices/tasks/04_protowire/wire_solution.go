//go:build solution

// Package protowire — ручная реализация wire-формата Protocol Buffers.
package protowire

import (
	"fmt"
	"unicode/utf8"
)

// AppendVarint дописывает v в формате base-128 varint (младшие 7 бит — первыми,
// старший бит байта = «дальше есть ещё байты»).
func AppendVarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

// SizeVarint возвращает длину varint-кодирования v (1..10 байт).
func SizeVarint(v uint64) int {
	n := 1
	for v >= 0x80 {
		v >>= 7
		n++
	}
	return n
}

// ConsumeVarint читает varint из начала b и возвращает значение и число байт.
func ConsumeVarint(b []byte) (uint64, int, error) {
	var v uint64
	for i := 0; i < 10; i++ {
		if i >= len(b) {
			return 0, 0, ErrTruncated
		}
		c := b[i]
		// 10-й байт может нести только 1 бит (биты 63).
		if i == 9 && c > 1 {
			return 0, 0, ErrOverflow
		}
		v |= uint64(c&0x7f) << (7 * i)
		if c < 0x80 {
			return v, i + 1, nil
		}
	}
	return 0, 0, ErrOverflow
}

// EncodeZigZag: 0→0, -1→1, 1→2, -2→3 ... — маленькие по модулю
// отрицательные числа становятся маленькими беззнаковыми.
func EncodeZigZag(v int64) uint64 {
	return uint64(v<<1) ^ uint64(v>>63) // v>>63 — арифметический сдвиг: 0 или все единицы
}

// DecodeZigZag — обратное преобразование.
func DecodeZigZag(v uint64) int64 {
	return int64(v>>1) ^ -int64(v&1)
}

// AppendTag дописывает ключ поля: varint((num << 3) | typ).
func AppendTag(b []byte, num Number, typ Type) []byte {
	return AppendVarint(b, uint64(num)<<3|uint64(typ&7))
}

// ConsumeTag читает ключ поля.
func ConsumeTag(b []byte) (Number, Type, int, error) {
	v, n, err := ConsumeVarint(b)
	if err != nil {
		return 0, 0, 0, err
	}
	num := v >> 3
	if num < 1 || num > uint64(MaxValidNumber) {
		return 0, 0, 0, ErrInvalidField
	}
	return Number(num), Type(v & 7), n, nil
}

// AppendBytes дописывает length-delimited значение: varint(len) + байты.
func AppendBytes(b, v []byte) []byte {
	b = AppendVarint(b, uint64(len(v)))
	return append(b, v...)
}

// ConsumeBytes читает length-delimited значение. Результат ссылается на b.
func ConsumeBytes(b []byte) ([]byte, int, error) {
	l, n, err := ConsumeVarint(b)
	if err != nil {
		return nil, 0, err
	}
	// Сравниваем в uint64: огромная длина не должна переполнить int.
	if l > uint64(len(b)-n) {
		return nil, 0, ErrTruncated
	}
	end := n + int(l)
	return b[n:end], end, nil
}

// ConsumeFieldValue возвращает длину значения с данным wire type — нужна,
// чтобы пропускать неизвестные поля.
func ConsumeFieldValue(typ Type, b []byte) (int, error) {
	switch typ {
	case VarintType:
		_, n, err := ConsumeVarint(b)
		return n, err
	case Fixed32Type:
		if len(b) < 4 {
			return 0, ErrTruncated
		}
		return 4, nil
	case Fixed64Type:
		if len(b) < 8 {
			return 0, ErrTruncated
		}
		return 8, nil
	case BytesType:
		_, n, err := ConsumeBytes(b)
		return n, err
	case StartGroupType, EndGroupType:
		return 0, ErrUnsupported
	default:
		return 0, ErrWireType
	}
}

// Marshal кодирует User. Поля пишутся по возрастанию номеров; скалярные
// поля со значением по умолчанию (0, "") в proto3 не пишутся.
func (u *User) Marshal() []byte {
	var b []byte
	if u.ID != 0 {
		b = AppendTag(b, 1, VarintType)
		b = AppendVarint(b, uint64(u.ID)) // отрицательный int64 → 10 байт
	}
	if u.Name != "" {
		b = AppendTag(b, 2, BytesType)
		b = AppendBytes(b, []byte(u.Name))
	}
	for _, t := range u.Tags { // элементы repeated пишутся всегда, даже ""
		b = AppendTag(b, 3, BytesType)
		b = AppendBytes(b, []byte(t))
	}
	if u.Score != 0 {
		b = AppendTag(b, 4, VarintType)
		// sint32: zigzag в 32 битах.
		s := u.Score
		b = AppendVarint(b, uint64(uint32(s<<1)^uint32(s>>31)))
	}
	return b
}

// Unmarshal декодирует b в u (предыдущее содержимое u сбрасывается).
// Неизвестные поля пропускаются; для скаляров побеждает последнее значение.
func (u *User) Unmarshal(b []byte) error {
	*u = User{}
	for len(b) > 0 {
		num, typ, n, err := ConsumeTag(b)
		if err != nil {
			return err
		}
		b = b[n:]
		switch num {
		case 1, 4:
			if typ != VarintType {
				return fmt.Errorf("field %d: %w", num, ErrWireType)
			}
			v, n, err := ConsumeVarint(b)
			if err != nil {
				return err
			}
			b = b[n:]
			if num == 1 {
				u.ID = int64(v)
			} else {
				// sint32 декодируется из младших 32 бит.
				u.Score = int32(DecodeZigZag(v & 0xFFFFFFFF))
			}
		case 2, 3:
			if typ != BytesType {
				return fmt.Errorf("field %d: %w", num, ErrWireType)
			}
			v, n, err := ConsumeBytes(b)
			if err != nil {
				return err
			}
			b = b[n:]
			if !utf8.Valid(v) {
				return fmt.Errorf("field %d: %w", num, ErrInvalidUTF8)
			}
			if num == 2 {
				u.Name = string(v) // string(...) копирует — не держим ссылку на буфер
			} else {
				u.Tags = append(u.Tags, string(v))
			}
		default:
			n, err := ConsumeFieldValue(typ, b)
			if err != nil {
				return fmt.Errorf("unknown field %d: %w", num, err)
			}
			b = b[n:]
		}
	}
	return nil
}
