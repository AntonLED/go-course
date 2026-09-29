//go:build !solution

// Package protowire — ручная реализация wire-формата Protocol Buffers.
package protowire

// AppendVarint дописывает v в формате base-128 varint: по 7 бит, младшие
// группы первыми, старший бит байта = 1, если за ним есть ещё байты.
// 300 → ac 02.
func AppendVarint(b []byte, v uint64) []byte {
	// TODO: реализуйте
	return b
}

// SizeVarint возвращает длину varint-кодирования v (1..10 байт).
func SizeVarint(v uint64) int {
	// TODO: реализуйте
	return 0
}

// ConsumeVarint читает varint из начала b: (значение, число прочитанных байт, ошибка).
// Данные кончились → ErrTruncated; больше 10 байт или 10-й байт > 1 → ErrOverflow.
func ConsumeVarint(b []byte) (uint64, int, error) {
	// TODO: реализуйте
	return 0, 0, ErrTruncated
}

// EncodeZigZag: 0→0, -1→1, 1→2, -2→3, ...
func EncodeZigZag(v int64) uint64 {
	// TODO: реализуйте
	return 0
}

// DecodeZigZag — обратное к EncodeZigZag.
func DecodeZigZag(v uint64) int64 {
	// TODO: реализуйте
	return 0
}

// AppendTag дописывает ключ поля: varint((num << 3) | typ).
func AppendTag(b []byte, num Number, typ Type) []byte {
	// TODO: реализуйте
	return b
}

// ConsumeTag читает ключ поля. Номер поля вне 1..MaxValidNumber → ErrInvalidField.
func ConsumeTag(b []byte) (Number, Type, int, error) {
	// TODO: реализуйте
	return 0, 0, 0, ErrTruncated
}

// AppendBytes дописывает length-delimited значение: varint(len(v)) + v.
func AppendBytes(b, v []byte) []byte {
	// TODO: реализуйте
	return b
}

// ConsumeBytes читает length-delimited значение (результат ссылается на b).
// Длина больше остатка данных → ErrTruncated.
func ConsumeBytes(b []byte) ([]byte, int, error) {
	// TODO: реализуйте
	return nil, 0, ErrTruncated
}

// ConsumeFieldValue возвращает длину значения с wire type typ в начале b
// (для пропуска неизвестных полей). Группы → ErrUnsupported, типы 6 и 7 → ErrWireType.
func ConsumeFieldValue(typ Type, b []byte) (int, error) {
	// TODO: реализуйте
	return 0, ErrWireType
}

// Marshal кодирует User так же, как сгенерированный protoc-gen-go код:
// поля по возрастанию номеров, нулевые скаляры не пишутся, sint32 — zigzag.
func (u *User) Marshal() []byte {
	// TODO: реализуйте
	return nil
}

// Unmarshal декодирует b в u (предыдущее содержимое сбрасывается).
//   - неизвестные поля пропускаются;
//   - для скаляров побеждает последнее вхождение, repeated — накапливаются;
//   - известное поле с неверным wire type → ErrWireType;
//   - string с невалидным UTF-8 → ErrInvalidUTF8;
//   - sint32 декодируется из младших 32 бит varint.
func (u *User) Unmarshal(b []byte) error {
	// TODO: реализуйте
	return ErrTruncated
}
