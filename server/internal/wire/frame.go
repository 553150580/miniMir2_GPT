// 本文件实现共享二进制外层帧的版本、消息号、请求序号、会话代次、错误码和负载边界。

package wire

import (
	"encoding/binary"
	"errors"
	"google.golang.org/protobuf/proto"
)

// MaxFrame 限制单个完整应用帧的字节数，包含外层头和 Protobuf 负载。
const MaxFrame = 65536

// Frame 保存 HTTP RPC 与 WebSocket 共用的应用外层元数据。
type Frame struct {
	ID      uint16 // 共享协议清单分配的消息号。
	Request uint32 // 由请求方提供的序号，供回包与原请求关联。
	Binding uint64 // 当前角色的连接代次，入场前为零。
	Error   uint32 // 协议错误码，为零时省略错误段。
	Payload []byte // 与消息号对应的 Protobuf 数据。
}

// Encode 按版本、标记、消息号、请求序号及连接代次写入外层头，再拼接业务字节。
// f 提供待序列化的应用帧字段。
func Encode(f Frame) []byte {
	// b 先保存版本、标记和消息号，随后追加变长字段与负载。
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

// Decode 先校验帧长和头标记，再逐段提取有界变长整数，将余下字节交给消息解码器。
// b 持有未消费的帧字节，每读完一段就移到剩余内容。
func Decode(b []byte) (Frame, error) {
	// f 积累已成功解析的外层字段，供调用者判断和分发。
	var f Frame
	if len(b) < 6 || len(b) > MaxFrame || b[0] != 1 || b[1]&^byte(1) != 0 {
		return f, errors.New("invalid frame")
	}
	// flags 保存外层标记，最低位表示还有错误码字段。
	flags := b[1]
	f.ID = binary.BigEndian.Uint16(b[2:4])
	b = b[4:]
	// v 接收正在读取的请求序号、连接代次或错误码，n 表示当前变长字段占用的字节数，非正值说明没有成功解码。
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

// Message 将业务消息编码为负载并携带指定请求号和连接代次。
// id 选择负载所属的共享消息号，request 关联客户端请求与服务端的对应响应，binding 携带连接上的会话代次，用于防止旧连接命令被接受。
// m 提供交给 Protobuf 序列化的业务消息。
func Message(id uint16, request uint32, binding uint64, m proto.Message) Frame {
	// b 保存业务消息序列化后的负载字节。
	b, _ := proto.Marshal(m)
	return Frame{ID: id, Request: request, Binding: binding, Payload: b}
}
