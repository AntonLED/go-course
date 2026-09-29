package protowire

import "errors"

// Number — номер поля protobuf (1 .. MaxValidNumber).
type Number int32

// MaxValidNumber — максимальный номер поля: 2^29 - 1.
const MaxValidNumber Number = 1<<29 - 1

// Type — wire type: младшие 3 бита тега.
type Type int8

const (
	VarintType     Type = 0 // int32, int64, uint32, uint64, sint32, sint64, bool, enum
	Fixed64Type    Type = 1 // fixed64, sfixed64, double
	BytesType      Type = 2 // string, bytes, вложенные сообщения, packed repeated
	StartGroupType Type = 3 // устаревшие группы (proto2)
	EndGroupType   Type = 4
	Fixed32Type    Type = 5 // fixed32, sfixed32, float
)

var (
	ErrTruncated    = errors.New("protowire: unexpected end of data")
	ErrOverflow     = errors.New("protowire: varint overflows 64 bits")
	ErrInvalidField = errors.New("protowire: invalid field number")
	ErrWireType     = errors.New("protowire: invalid or unexpected wire type")
	ErrUnsupported  = errors.New("protowire: groups are not supported")
	ErrInvalidUTF8  = errors.New("protowire: string field contains invalid UTF-8")
)

// User соответствует сообщению:
//
//	syntax = "proto3";
//	message User {
//	  int64  id   = 1;
//	  string name = 2;
//	  repeated string tags = 3;
//	  sint32 score = 4;
//	}
type User struct {
	ID    int64
	Name  string
	Tags  []string
	Score int32
}
