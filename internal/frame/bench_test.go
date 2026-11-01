package frame

import (
	"testing"
)

func BenchmarkFrameEncode(b *testing.B) {
	frames := []*Frame{
		NewConnectFrame("example.com", 443, 0),
		NewConnectOKFrame(1, "10.0.0.1"),
		NewDataFrame(make([]byte, 1024), true),
		NewCloseFrame(0),
		NewPingFrame(1234567890, [8]byte{1, 2, 3, 4, 5, 6, 7, 8}),
		NewAuthFrame("test-token"),
		NewWindowUpdateFrame(65536),
		NewGoAwayFrame(0, 100, "graceful shutdown"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, f := range frames {
			_, _ = f.Encode()
		}
	}
}

func BenchmarkFrameDecode(b *testing.B) {
	encodedFrames := make([][]byte, 0, 8)
	frames := []*Frame{
		NewConnectFrame("example.com", 443, 0),
		NewConnectOKFrame(1, "10.0.0.1"),
		NewDataFrame(make([]byte, 1024), true),
		NewCloseFrame(0),
		NewPingFrame(1234567890, [8]byte{1, 2, 3, 4, 5, 6, 7, 8}),
		NewAuthFrame("test-token"),
		NewWindowUpdateFrame(65536),
		NewGoAwayFrame(0, 100, "graceful shutdown"),
	}

	for _, f := range frames {
		buf, _ := f.Encode()
		encodedFrames = append(encodedFrames, buf)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, buf := range encodedFrames {
			_, _ = Decode(buf)
		}
	}
}

func BenchmarkVarintEncode(b *testing.B) {
	values := []uint64{0, 63, 64, 16383, 16384, 1073741823, 1073741824, 4611686018427387903}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := make([]byte, 8)
		for _, v := range values {
			putVarint(buf, v)
		}
	}
}

func BenchmarkVarintDecode(b *testing.B) {
	bufs := make([][]byte, 0, 8)
	values := []uint64{0, 63, 64, 16383, 16384, 1073741823, 1073741824, 4611686018427387903}
	for _, v := range values {
		buf := make([]byte, 8)
		n := putVarint(buf, v)
		bufs = append(bufs, buf[:n])
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, buf := range bufs {
			_, _, _ = readVarint(buf)
		}
	}
}