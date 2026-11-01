package frame

import (
	"testing"
)

func TestFrameEncodeDecode(t *testing.T) {
	tests := []struct {
		name   string
		frame  *Frame
	}{
		{
			name: "CONNECT frame",
			frame: NewConnectFrame("example.com", 443, 0),
		},
		{
			name: "CONNECT_OK frame",
			frame: NewConnectOKFrame(1, "10.0.0.1"),
		},
		{
			name: "CONNECT_FAIL frame",
			frame: NewConnectFailFrame(ErrorCodeDenied, "access denied"),
		},
		{
			name: "DATA frame",
			frame: NewDataFrame([]byte("hello world"), true),
		},
		{
			name: "CLOSE frame",
			frame: NewCloseFrame(0),
		},
		{
			name: "PING frame",
			frame: NewPingFrame(1234567890, [8]byte{1, 2, 3, 4, 5, 6, 7, 8}),
		},
		{
			name: "PONG frame",
			frame: NewPongFrame(1234567890, [8]byte{1, 2, 3, 4, 5, 6, 7, 8}),
		},
		{
			name: "AUTH frame",
			frame: NewAuthFrame("test-token"),
		},
		{
			name: "AUTH_OK frame",
			frame: NewAuthOKFrame(),
		},
		{
			name: "AUTH_FAIL frame",
			frame: NewAuthFailFrame("invalid credentials"),
		},
		{
			name: "WINDOW_UPDATE frame",
			frame: NewWindowUpdateFrame(65536),
		},
		{
			name: "GOAWAY frame",
			frame: NewGoAwayFrame(0, 100, "graceful shutdown"),
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := tt.frame.Encode()
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}
			
			decoded, err := Decode(encoded)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}
			
			if decoded.Type != tt.frame.Type {
				t.Errorf("Type mismatch: got %v, want %v", decoded.Type, tt.frame.Type)
			}
			if decoded.Flags != tt.frame.Flags {
				t.Errorf("Flags mismatch: got %v, want %v", decoded.Flags, tt.frame.Flags)
			}
			if decoded.Length != tt.frame.Length {
				t.Errorf("Length mismatch: got %d, want %d", decoded.Length, tt.frame.Length)
			}
			// Payload comparison
			for i := range tt.frame.Payload {
				if decoded.Payload[i] != tt.frame.Payload[i] {
					t.Errorf("Payload mismatch at index %d: got %d, want %d", i, decoded.Payload[i], tt.frame.Payload[i])
					break
				}
			}
		})
	}
}

func TestParseConnectPayload(t *testing.T) {
	frame := NewConnectFrame("example.com", 443, 0)
	addr, port, protocol, err := ParseConnectPayload(frame.Payload)
	if err != nil {
		t.Fatalf("ParseConnectPayload failed: %v", err)
	}
	
	if addr != "example.com" {
		t.Errorf("addr = %s, want example.com", addr)
	}
	if port != 443 {
		t.Errorf("port = %d, want 443", port)
	}
	if protocol != 0 {
		t.Errorf("protocol = %d, want 0", protocol)
	}
}

func TestParseConnectOKPayload(t *testing.T) {
	frame := NewConnectOKFrame(42, "10.0.0.1")
	streamID, assignedAddr, err := ParseConnectOKPayload(frame.Payload)
	if err != nil {
		t.Fatalf("ParseConnectOKPayload failed: %v", err)
	}
	
	if streamID != 42 {
		t.Errorf("streamID = %d, want 42", streamID)
	}
	if assignedAddr != "10.0.0.1" {
		t.Errorf("assignedAddr = %s, want 10.0.0.1", assignedAddr)
	}
}

func TestParseConnectFailPayload(t *testing.T) {
	frame := NewConnectFailFrame(ErrorCodeDenied, "access denied")
	code, msg, err := ParseConnectFailPayload(frame.Payload)
	if err != nil {
		t.Fatalf("ParseConnectFailPayload failed: %v", err)
	}
	
	if code != ErrorCodeDenied {
		t.Errorf("code = %d, want %d", code, ErrorCodeDenied)
	}
	if msg != "access denied" {
		t.Errorf("msg = %s, want 'access denied'", msg)
	}
}

func TestParsePingPayload(t *testing.T) {
	token := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	frame := NewPingFrame(1234567890, token)
	timestamp, parsedToken, err := ParsePingPayload(frame.Payload)
	if err != nil {
		t.Fatalf("ParsePingPayload failed: %v", err)
	}
	
	if timestamp != 1234567890 {
		t.Errorf("timestamp = %d, want 1234567890", timestamp)
	}
	if parsedToken != token {
		t.Errorf("token = %v, want %v", parsedToken, token)
	}
}

func TestVarintEncoding(t *testing.T) {
	tests := []struct {
		value    uint64
		expected []byte
	}{
		{0, []byte{0x00}},
		{63, []byte{0x3F}},
		{64, []byte{0x40, 0x40}},
		{16383, []byte{0x7F, 0xFF}},
		{16384, []byte{0x80, 0x00, 0x40, 0x00}},
		{1073741823, []byte{0xBF, 0xFF, 0xFF, 0xFF}},
		{1073741824, []byte{0xC0, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00}},
	}
	
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			buf := make([]byte, 8)
			n := putVarint(buf, tt.value)
			if n != len(tt.expected) {
				t.Errorf("putVarint length = %d, want %d", n, len(tt.expected))
			}
			for i := range tt.expected {
				if buf[i] != tt.expected[i] {
					t.Errorf("putVarint[%d] = 0x%02x, want 0x%02x", i, buf[i], tt.expected[i])
				}
			}
			
			// Test decode
			val, n2, err := readVarint(buf[:n])
			if err != nil {
				t.Errorf("readVarint error: %v", err)
			}
			if n2 != n {
				t.Errorf("readVarint consumed %d bytes, want %d", n2, n)
			}
			if val != tt.value {
				t.Errorf("readVarint value = %d, want %d", val, tt.value)
			}
		})
	}
}

func TestFrameTooLarge(t *testing.T) {
	frame := &Frame{
		Type:   FrameTypeData,
		Flags:  0,
		Length: MaxFrameSize + 1,
		Payload: make([]byte, MaxFrameSize+1),
	}
	
	_, err := frame.Encode()
	if err != ErrFrameTooLarge {
		t.Errorf("Expected ErrFrameTooLarge, got %v", err)
	}
}

func TestDecodeBufferTooSmall(t *testing.T) {
	// Incomplete frame
	data := []byte{0x01, 0x00} // Type=1, Flags=0, missing length and payload
	_, err := Decode(data)
	if err != ErrBufferTooSmall {
		t.Errorf("Expected ErrBufferTooSmall, got %v", err)
	}
}