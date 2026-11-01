package session

import (
	"context"
	"testing"

	"github.com/qtunnel/qtunnel/internal/config"
	"go.uber.org/zap"
)

func BenchmarkSessionOpenStream(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	limits := &config.ParsedLimits{
		MaxConnections:    1000,
		MaxStreamsPerConn: 100,
		StreamBufferSize:  1024,
		ConnBufferSize:    10240,
		MaxFrameSize:      16384,
	}

	conn := newMockConnection()
	sess := NewSession("bench-session", conn, limits, logger, nil)
	sess.SetState(StateActive)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream, err := sess.OpenStream(context.Background())
		if err != nil {
			b.Fatal(err)
		}
		stream.Close()
	}
}

func BenchmarkSessionStreamCount(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	limits := &config.ParsedLimits{
		MaxConnections:    1000,
		MaxStreamsPerConn: 100,
		StreamBufferSize:  1024,
		ConnBufferSize:    10240,
		MaxFrameSize:      16384,
	}

	conn := newMockConnection()
	sess := NewSession("bench-session", conn, limits, logger, nil)
	sess.SetState(StateActive)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sess.StreamCount()
	}
}

func BenchmarkSessionManagerAddRemove(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	limits := &config.ParsedLimits{
		MaxConnections:    1000,
		MaxStreamsPerConn: 100,
		StreamBufferSize:  1024,
		ConnBufferSize:    10240,
		MaxFrameSize:      16384,
	}

	mgr := NewSessionManager(limits, logger, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn := newMockConnection()
		sess := mgr.AddSession("test", conn)
		if sess != nil {
			mgr.RemoveSession("test")
		}
	}
}

func BenchmarkFlowControl(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	limits := &config.ParsedLimits{
		MaxConnections:    1000,
		MaxStreamsPerConn: 100,
		StreamBufferSize:  1024,
		ConnBufferSize:    10240,
		MaxFrameSize:      16384,
	}

	conn := newMockConnection()
	sess := NewSession("bench-session", conn, limits, logger, nil)
	sess.SetState(StateActive)

	stream, _ := sess.OpenStream(context.Background())
	defer stream.Close()

	data := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = stream.Write(data)
		_, _ = stream.Read(data)
	}
}