package wire

import (
	"encoding/binary"
	"errors"
	"google.golang.org/protobuf/proto"
)

const MaxFrame = 65536

type Frame struct {
	ID      uint16
	Request uint32
	Binding uint64
	Error   uint32
	Payload []byte
}

func Encode(f Frame) []byte {
	b := []byte{1, 0, byte(f.ID >> 8), byte(f.ID)}
	if f.Error != 0 {
		b[1] = 1
	}
	b = binary.AppendUvarint(b, uint64(f.Request))
	b = binary.AppendUvarint(b, f.Binding)
	if f.Error != 0 {
		b = binary.AppendUvarint(b, uint64(f.Error))
	}
	return append(b, f.Payload...)
}
func Decode(b []byte) (Frame, error) {
	var f Frame
	if len(b) < 6 || len(b) > MaxFrame || b[0] != 1 || b[1]&^byte(1) != 0 {
		return f, errors.New("invalid frame")
	}
	flags := b[1]
	f.ID = binary.BigEndian.Uint16(b[2:4])
	b = b[4:]
	v, n := binary.Uvarint(b)
	if n <= 0 || n > 5 || v > 1<<32-1 {
		return f, errors.New("invalid request sequence")
	}
	f.Request = uint32(v)
	b = b[n:]
	v, n = binary.Uvarint(b)
	if n <= 0 || n > 10 {
		return f, errors.New("invalid binding")
	}
	f.Binding = v
	b = b[n:]
	if flags&1 != 0 {
		v, n = binary.Uvarint(b)
		if n <= 0 || n > 5 || v > 1<<32-1 {
			return f, errors.New("invalid error")
		}
		f.Error = uint32(v)
		b = b[n:]
	}
	f.Payload = b
	return f, nil
}
func Message(id uint16, request uint32, binding uint64, m proto.Message) Frame {
	b, _ := proto.Marshal(m)
	return Frame{ID: id, Request: request, Binding: binding, Payload: b}
}
