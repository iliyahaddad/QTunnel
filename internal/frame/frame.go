package frame

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

var (
	ErrInvalidFrame     = errors.New("invalid frame")
	ErrFrameTooLarge    = errors.New("frame too large")
	ErrUnknownFrameType = errors.New("unknown frame type")
	ErrVarintOverflow   = errors.New("varint overflow")
	ErrBufferTooSmall   = errors.New("buffer too small")
)

// FrameType represents the type of a frame
type FrameType uint64

const (
	FrameTypeReserved      FrameType = 0x00
	FrameTypeConnect       FrameType = 0x01
	FrameTypeConnectOK     FrameType = 0x02
	FrameTypeConnectFail   FrameType = 0x03
	FrameTypeData          FrameType = 0x04
	FrameTypeClose         FrameType = 0x05
	FrameTypePing          FrameType = 0x06
	FrameTypePong          FrameType = 0x07
	FrameTypeAuth          FrameType = 0x08
	FrameTypeAuthOK        FrameType = 0x09
	FrameTypeAuthFail      FrameType = 0x0A
	FrameTypeWindowUpdate  FrameType = 0x0B
	FrameTypeGoAway        FrameType = 0x0C
	// MASQUE frame types
	FrameTypeConnectUDP    FrameType = 0x10
	FrameTypeConnectIP     FrameType = 0x11
	FrameTypeCapsule       FrameType = 0x12
)

// FrameFlags represents frame flags
type FrameFlags uint64

const (
	FlagFIN     FrameFlags = 0x01 // Last frame on stream
	FlagACK     FrameFlags = 0x02 // Requires acknowledgment
	FlagPriority FrameFlags = 0x04 // High priority
)

// String returns the string representation of the FrameType
func (ft FrameType) String() string {
	switch ft {
	case FrameTypeReserved:
		return "RESERVED"
	case FrameTypeConnect:
		return "CONNECT"
	case FrameTypeConnectOK:
		return "CONNECT_OK"
	case FrameTypeConnectFail:
		return "CONNECT_FAIL"
	case FrameTypeData:
		return "DATA"
	case FrameTypeClose:
		return "CLOSE"
	case FrameTypePing:
		return "PING"
	case FrameTypePong:
		return "PONG"
	case FrameTypeAuth:
		return "AUTH"
	case FrameTypeAuthOK:
		return "AUTH_OK"
	case FrameTypeAuthFail:
		return "AUTH_FAIL"
	case FrameTypeWindowUpdate:
		return "WINDOW_UPDATE"
	case FrameTypeGoAway:
		return "GOAWAY"
	case FrameTypeConnectUDP:
		return "CONNECT_UDP"
	case FrameTypeConnectIP:
		return "CONNECT_IP"
	case FrameTypeCapsule:
		return "CAPSULE"
	default:
		return fmt.Sprintf("UNKNOWN(0x%X)", uint64(ft))
	}
}

// Frame represents a protocol frame
type Frame struct {
	Type   FrameType
	Flags  FrameFlags
	Length uint64
	Payload []byte
}

// MaxFrameSize is the maximum allowed frame size
const MaxFrameSize = 16 << 10 // 16KB

// Encode encodes the frame to a byte slice
func (f *Frame) Encode() ([]byte, error) {
	if f.Length > MaxFrameSize {
		return nil, ErrFrameTooLarge
	}
	if uint64(len(f.Payload)) != f.Length {
		return nil, errors.New("payload length mismatch")
	}

	// Calculate size needed
	typeBuf := make([]byte, 8)
	flagsBuf := make([]byte, 8)
	lenBuf := make([]byte, 8)

	typeN := putVarint(typeBuf, uint64(f.Type))
	flagsN := putVarint(flagsBuf, uint64(f.Flags))
	lenN := putVarint(lenBuf, f.Length)

	buf := make([]byte, typeN+flagsN+lenN+int(f.Length))
	off := 0
	copy(buf[off:], typeBuf[:typeN])
	off += typeN
	copy(buf[off:], flagsBuf[:flagsN])
	off += flagsN
	copy(buf[off:], lenBuf[:lenN])
	off += lenN
	copy(buf[off:], f.Payload)

	return buf, nil
}

// EncodeTo writes the frame to a writer
func (f *Frame) EncodeTo(w io.Writer) error {
	buf, err := f.Encode()
	if err != nil {
		return err
	}
	_, err = w.Write(buf)
	return err
}

// Decode decodes a frame from a byte slice
func Decode(data []byte) (*Frame, error) {
	if len(data) == 0 {
		return nil, ErrInvalidFrame
	}

	f := &Frame{}
	off := 0

	// Decode type
	typ, n, err := readVarint(data[off:])
	if err != nil {
		return nil, err
	}
	f.Type = FrameType(typ)
	off += n

	// Decode flags
	flags, n, err := readVarint(data[off:])
	if err != nil {
		return nil, err
	}
	f.Flags = FrameFlags(flags)
	off += n

	// Decode length
	length, n, err := readVarint(data[off:])
	if err != nil {
		return nil, err
	}
	f.Length = length
	off += n

	if f.Length > MaxFrameSize {
		return nil, ErrFrameTooLarge
	}

	if len(data) < off+int(f.Length) {
		return nil, ErrBufferTooSmall
	}

	f.Payload = make([]byte, f.Length)
	copy(f.Payload, data[off:off+int(f.Length)])

	return f, nil
}

// DecodeFrom reads and decodes a frame from a reader
func DecodeFrom(r io.Reader) (*Frame, error) {
	f := &Frame{}
	
	// Decode type
	typ, err := readVarintFrom(r)
	if err != nil {
		return nil, err
	}
	f.Type = FrameType(typ)
	
	// Decode flags
	flags, err := readVarintFrom(r)
	if err != nil {
		return nil, err
	}
	f.Flags = FrameFlags(flags)
	
	// Decode length
	length, err := readVarintFrom(r)
	if err != nil {
		return nil, err
	}
	f.Length = length
	
	if f.Length > MaxFrameSize {
		return nil, ErrFrameTooLarge
	}
	
	// Read payload
	f.Payload = make([]byte, f.Length)
	if f.Length > 0 {
		n, err := io.ReadFull(r, f.Payload)
		if err != nil {
			return nil, err
		}
		if uint64(n) != f.Length {
			return nil, ErrBufferTooSmall
		}
	}
	
	return f, nil
}

// readVarintFrom reads a varint from a reader
func readVarintFrom(r io.Reader) (uint64, error) {
	buf := make([]byte, 1)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	
	first := buf[0]
	prefix := first >> 6
	length := 1 << prefix
	
	if length == 1 {
		return uint64(first & 0x3F), nil
	}
	
	// Read remaining bytes
	extra := make([]byte, length-1)
	if _, err := io.ReadFull(r, extra); err != nil {
		return 0, err
	}
	
	var x uint64
	switch length {
	case 2:
		x = uint64(first&0x3F)<<8 | uint64(extra[0])
	case 4:
		x = uint64(first&0x3F)<<24 |
			uint64(extra[0])<<16 |
			uint64(extra[1])<<8 |
			uint64(extra[2])
	case 8:
		x = uint64(first&0x3F)<<56 |
			uint64(extra[0])<<48 |
			uint64(extra[1])<<40 |
			uint64(extra[2])<<32 |
			uint64(extra[3])<<24 |
			uint64(extra[4])<<16 |
			uint64(extra[5])<<8 |
			uint64(extra[6])
	}
	return x, nil
}

// putVarint encodes a varint to buf, returns bytes written
func putVarint(buf []byte, x uint64) int {
	if x < 64 {
		buf[0] = byte(x)
		return 1
	}
	if x < 16384 {
		buf[0] = 0x40 | byte(x>>8)
		buf[1] = byte(x)
		return 2
	}
	if x < 1073741824 {
		buf[0] = 0x80 | byte(x>>24)
		buf[1] = byte(x>>16)
		buf[2] = byte(x>>8)
		buf[3] = byte(x)
		return 4
	}
	buf[0] = 0xC0 | byte(x>>56)
	buf[1] = byte(x>>48)
	buf[2] = byte(x>>40)
	buf[3] = byte(x>>32)
	buf[4] = byte(x>>24)
	buf[5] = byte(x>>16)
	buf[6] = byte(x>>8)
	buf[7] = byte(x)
	return 8
}

// readVarint decodes a varint from data, returns value, bytes read, error
func readVarint(data []byte) (uint64, int, error) {
	if len(data) == 0 {
		return 0, 0, ErrBufferTooSmall
	}

	first := data[0]
	prefix := first >> 6
	length := 1 << prefix

	if len(data) < length {
		return 0, 0, ErrBufferTooSmall
	}

	var x uint64
	switch length {
	case 1:
		x = uint64(first & 0x3F)
	case 2:
		x = uint64(first&0x3F)<<8 | uint64(data[1])
	case 4:
		x = uint64(first&0x3F)<<24 |
			uint64(data[1])<<16 |
			uint64(data[2])<<8 |
			uint64(data[3])
	case 8:
		x = uint64(first&0x3F)<<56 |
			uint64(data[1])<<48 |
			uint64(data[2])<<40 |
			uint64(data[3])<<32 |
			uint64(data[4])<<24 |
			uint64(data[5])<<16 |
			uint64(data[6])<<8 |
			uint64(data[7])
	}
	return x, length, nil
}

// Frame constructors

// NewConnectFrame creates a CONNECT frame
func NewConnectFrame(addr string, port uint16, protocol uint8) *Frame {
	payload := make([]byte, 0, len(addr)+3)
	payload = append(payload, byte(len(addr)))
	payload = append(payload, addr...)
	payload = append(payload, byte(port>>8), byte(port))
	payload = append(payload, protocol)

	return &Frame{
		Type:    FrameTypeConnect,
		Flags:   0,
		Length:  uint64(len(payload)),
		Payload: payload,
	}
}

// NewConnectOKFrame creates a CONNECT_OK frame
func NewConnectOKFrame(streamID uint64, assignedAddr string) *Frame {
	payload := make([]byte, 0, 8+1+len(assignedAddr))
	payload = append(payload, byte(streamID>>56), byte(streamID>>48), byte(streamID>>40), byte(streamID>>32),
		byte(streamID>>24), byte(streamID>>16), byte(streamID>>8), byte(streamID))
	if assignedAddr != "" {
		payload = append(payload, byte(len(assignedAddr)))
		payload = append(payload, assignedAddr...)
	} else {
		payload = append(payload, 0)
	}

	return &Frame{
		Type:    FrameTypeConnectOK,
		Flags:   0,
		Length:  uint64(len(payload)),
		Payload: payload,
	}
}

// NewConnectFailFrame creates a CONNECT_FAIL frame
func NewConnectFailFrame(code uint64, msg string) *Frame {
	payload := make([]byte, 0, 8+1+len(msg))
	payload = append(payload, byte(code>>56), byte(code>>48), byte(code>>40), byte(code>>32),
		byte(code>>24), byte(code>>16), byte(code>>8), byte(code))
	payload = append(payload, byte(len(msg)))
	payload = append(payload, msg...)

	return &Frame{
		Type:    FrameTypeConnectFail,
		Flags:   0,
		Length:  uint64(len(payload)),
		Payload: payload,
	}
}

// NewDataFrame creates a DATA frame
func NewDataFrame(data []byte, fin bool) *Frame {
	flags := FrameFlags(0)
	if fin {
		flags |= FlagFIN
	}
	return &Frame{
		Type:    FrameTypeData,
		Flags:   flags,
		Length:  uint64(len(data)),
		Payload: data,
	}
}

// NewCloseFrame creates a CLOSE frame
func NewCloseFrame(code uint64) *Frame {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint64(payload, code)
	return &Frame{
		Type:    FrameTypeClose,
		Flags:   0,
		Length:  8,
		Payload: payload,
	}
}

// NewPingFrame creates a PING frame
func NewPingFrame(timestamp uint64, token [8]byte) *Frame {
	payload := make([]byte, 16)
	binary.BigEndian.PutUint64(payload[:8], timestamp)
	copy(payload[8:], token[:])
	return &Frame{
		Type:    FrameTypePing,
		Flags:   FlagACK,
		Length:  16,
		Payload: payload,
	}
}

// NewPongFrame creates a PONG frame
func NewPongFrame(timestamp uint64, token [8]byte) *Frame {
	payload := make([]byte, 16)
	binary.BigEndian.PutUint64(payload[:8], timestamp)
	copy(payload[8:], token[:])
	return &Frame{
		Type:    FrameTypePong,
		Flags:   0,
		Length:  16,
		Payload: payload,
	}
}

// NewAuthFrame creates an AUTH frame
func NewAuthFrame(token string) *Frame {
	payload := []byte(token)
	return &Frame{
		Type:    FrameTypeAuth,
		Flags:   0,
		Length:  uint64(len(payload)),
		Payload: payload,
	}
}

// NewAuthOKFrame creates an AUTH_OK frame
func NewAuthOKFrame() *Frame {
	return &Frame{
		Type:    FrameTypeAuthOK,
		Flags:   0,
		Length:  0,
		Payload: nil,
	}
}

// NewAuthFailFrame creates an AUTH_FAIL frame
func NewAuthFailFrame(msg string) *Frame {
	payload := []byte(msg)
	return &Frame{
		Type:    FrameTypeAuthFail,
		Flags:   0,
		Length:  uint64(len(payload)),
		Payload: payload,
	}
}

// NewWindowUpdateFrame creates a WINDOW_UPDATE frame
func NewWindowUpdateFrame(credit uint64) *Frame {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint64(payload, credit)
	return &Frame{
		Type:    FrameTypeWindowUpdate,
		Flags:   0,
		Length:  8,
		Payload: payload,
	}
}

// NewGoAwayFrame creates a GOAWAY frame
func NewGoAwayFrame(code uint64, lastStreamID uint64, debug string) *Frame {
	payload := make([]byte, 0, 16+1+len(debug))
	payload = append(payload,
		byte(code>>56), byte(code>>48), byte(code>>40), byte(code>>32),
		byte(code>>24), byte(code>>16), byte(code>>8), byte(code),
		byte(lastStreamID>>56), byte(lastStreamID>>48), byte(lastStreamID>>40), byte(lastStreamID>>32),
		byte(lastStreamID>>24), byte(lastStreamID>>16), byte(lastStreamID>>8), byte(lastStreamID))
	payload = append(payload, byte(len(debug)))
	payload = append(payload, debug...)

	return &Frame{
		Type:    FrameTypeGoAway,
		Flags:   0,
		Length:  uint64(len(payload)),
		Payload: payload,
	}
}

// ParseConnectPayload parses CONNECT frame payload
func ParseConnectPayload(payload []byte) (addr string, port uint16, protocol uint8, err error) {
	if len(payload) < 2 {
		return "", 0, 0, errors.New("connect payload too short")
	}
	addrLen := int(payload[0])
	if len(payload) < 1+addrLen+2 {
		return "", 0, 0, errors.New("connect payload truncated")
	}
	addr = string(payload[1 : 1+addrLen])
	port = binary.BigEndian.Uint16(payload[1+addrLen : 1+addrLen+2])
	protocol = 0
	if len(payload) > 1+addrLen+2 {
		protocol = payload[1+addrLen+2]
	}
	return addr, port, protocol, nil
}

// ParseConnectOKPayload parses CONNECT_OK frame payload
func ParseConnectOKPayload(payload []byte) (streamID uint64, assignedAddr string, err error) {
	if len(payload) < 8 {
		return 0, "", errors.New("connect_ok payload too short")
	}
	streamID = binary.BigEndian.Uint64(payload[:8])
	if len(payload) > 8 {
		addrLen := int(payload[8])
		if len(payload) >= 9+addrLen {
			assignedAddr = string(payload[9 : 9+addrLen])
		}
	}
	return streamID, assignedAddr, nil
}

// ParseConnectFailPayload parses CONNECT_FAIL frame payload
func ParseConnectFailPayload(payload []byte) (code uint64, msg string, err error) {
	if len(payload) < 8 {
		return 0, "", errors.New("connect_fail payload too short")
	}
	code = binary.BigEndian.Uint64(payload[:8])
	if len(payload) > 8 {
		msgLen := int(payload[8])
		if len(payload) >= 9+msgLen {
			msg = string(payload[9 : 9+msgLen])
		}
	}
	return code, msg, nil
}

// ParseAuthFailPayload parses AUTH_FAIL frame payload
func ParseAuthFailPayload(payload []byte) (msg string, err error) {
	if len(payload) == 0 {
		return "", errors.New("auth_fail payload too short")
	}
	return string(payload), nil
}

// ParsePingPayload parses PING/PONG frame payload
func ParsePingPayload(payload []byte) (timestamp uint64, token [8]byte, err error) {
	if len(payload) < 16 {
		return 0, [8]byte{}, errors.New("ping payload too short")
	}
	timestamp = binary.BigEndian.Uint64(payload[:8])
	copy(token[:], payload[8:16])
	return timestamp, token, nil
}

// NewConnectUDPFrame creates a CONNECT-UDP frame (MASQUE)
func NewConnectUDPFrame(targetHost string, targetPort uint16) *Frame {
	payload := make([]byte, 0, len(targetHost)+3)
	payload = append(payload, byte(len(targetHost)))
	payload = append(payload, targetHost...)
	payload = append(payload, byte(targetPort>>8), byte(targetPort))

	return &Frame{
		Type:    FrameTypeConnectUDP,
		Flags:   0,
		Length:  uint64(len(payload)),
		Payload: payload,
	}
}

// NewConnectIPFrame creates a CONNECT-IP frame (MASQUE)
func NewConnectIPFrame(target string, ipProto uint8) *Frame {
	payload := make([]byte, 0, len(target)+2)
	payload = append(payload, byte(len(target)))
	payload = append(payload, target...)
	payload = append(payload, ipProto)

	return &Frame{
		Type:    FrameTypeConnectIP,
		Flags:   0,
		Length:  uint64(len(payload)),
		Payload: payload,
	}
}

// NewCapsuleFrame creates a CAPSULE frame (MASQUE)
func NewCapsuleFrame(capsuleType uint64, capsuleData []byte) *Frame {
	payload := make([]byte, 0, 8+len(capsuleData))
	payload = append(payload, encodeVarint(capsuleType)...)
	payload = append(payload, encodeVarint(uint64(len(capsuleData)))...)
	payload = append(payload, capsuleData...)

	return &Frame{
		Type:    FrameTypeCapsule,
		Flags:   0,
		Length:  uint64(len(payload)),
		Payload: payload,
	}
}

// encodeVarint encodes a uint64 as varint
func encodeVarint(x uint64) []byte {
	if x < 64 {
		return []byte{byte(x)}
	}
	if x < 16384 {
		return []byte{0x40 | byte(x>>8), byte(x)}
	}
	if x < 1073741824 {
		return []byte{0x80 | byte(x>>24), byte(x>>16), byte(x>>8), byte(x)}
	}
	return []byte{0xC0 | byte(x>>56), byte(x>>48), byte(x>>40), byte(x>>32),
		byte(x>>24), byte(x>>16), byte(x>>8), byte(x)}
}

// ParseConnectUDPPayload parses CONNECT-UDP frame payload
func ParseConnectUDPPayload(payload []byte) (targetHost string, targetPort uint16, err error) {
	if len(payload) < 2 {
		return "", 0, errors.New("connect_udp payload too short")
	}
	addrLen := int(payload[0])
	if len(payload) < 1+addrLen+2 {
		return "", 0, errors.New("connect_udp payload truncated")
	}
	targetHost = string(payload[1 : 1+addrLen])
	targetPort = binary.BigEndian.Uint16(payload[1+addrLen : 1+addrLen+2])
	return targetHost, targetPort, nil
}

// ParseConnectIPPayload parses CONNECT-IP frame payload
func ParseConnectIPPayload(payload []byte) (target string, ipProto uint8, err error) {
	if len(payload) < 2 {
		return "", 0, errors.New("connect_ip payload too short")
	}
	targetLen := int(payload[0])
	if len(payload) < 1+targetLen+1 {
		return "", 0, errors.New("connect_ip payload truncated")
	}
	target = string(payload[1 : 1+targetLen])
	ipProto = payload[1+targetLen]
	return target, ipProto, nil
}

// ParseCapsuleFrame parses a CAPSULE frame
func ParseCapsuleFrame(payload []byte) (capsuleType uint64, capsuleData []byte, err error) {
	if len(payload) < 1 {
		return 0, nil, errors.New("capsule payload too short")
	}
	
	capsuleType, n, err := readVarint(payload)
	if err != nil {
		return 0, nil, err
	}
	
	if len(payload) < n+1 {
		return 0, nil, errors.New("capsule payload truncated")
	}
	
	length, m, err := readVarint(payload[n:])
	if err != nil {
		return 0, nil, err
	}
	
	if len(payload) < n+m+int(length) {
		return 0, nil, errors.New("capsule payload truncated")
	}
	
	capsuleData = payload[n+m : n+m+int(length)]
	return capsuleType, capsuleData, nil
}

// Connection error codes
const (
	ErrorCodeNoError           = 0x00
	ErrorCodeInvalidDest       = 0x01
	ErrorCodeDenied            = 0x02
	ErrorCodeDNSFailed         = 0x03
	ErrorCodeConnectionRefused = 0x04
	ErrorCodeTimeout           = 0x05
	ErrorCodeResourceExhausted = 0x06
	ErrorCodeInternalError     = 0x07
	ErrorCodeProtocolError     = 0x08
	ErrorCodeFrameTooLarge     = 0x09
	ErrorCodeStreamStateError  = 0x0A
	ErrorCodeAuthFailed        = 0x0B
)