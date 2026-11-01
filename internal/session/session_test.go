package session

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/qtunnel/qtunnel/internal/carrier"
	"github.com/qtunnel/qtunnel/internal/config"
	"go.uber.org/zap"
)

type mockConnection struct {
	streams []*mockStream
	closed  bool
	ctx     context.Context
	cancel  context.CancelFunc
}

func newMockConnection() *mockConnection {
	ctx, cancel := context.WithCancel(context.Background())
	return &mockConnection{
		ctx:    ctx,
		cancel: cancel,
	}
}

func (m *mockConnection) OpenStreamSync(ctx context.Context) (carrier.Stream, error) {
	s := newMockStream()
	m.streams = append(m.streams, s)
	return s, nil
}

func (m *mockConnection) AcceptStream(ctx context.Context) (carrier.Stream, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		// Simulate no incoming streams for testing
		return nil, context.DeadlineExceeded
	}
}

func (m *mockConnection) Close() error {
	m.closed = true
	m.cancel()
	return nil
}

func (m *mockConnection) Context() context.Context {
	return m.ctx
}

func (m *mockConnection) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
}

func (m *mockConnection) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54321}
}

type mockStream struct {
	id       uint64
	data     []byte
	readPos  int
	closed   bool
	writeClosed bool
	ctx      context.Context
	cancel   context.CancelFunc
}

func newMockStream() *mockStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &mockStream{
		id: 1,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *mockStream) ID() uint64 {
	return s.id
}

func (s *mockStream) Read(p []byte) (int, error) {
	if s.readPos >= len(s.data) {
		if s.closed {
			return 0, io.EOF
		}
		return 0, nil // Would block
	}
	n := copy(p, s.data[s.readPos:])
	s.readPos += n
	return n, nil
}

func (s *mockStream) Write(p []byte) (int, error) {
	s.data = append(s.data, p...)
	return len(p), nil
}

func (s *mockStream) Close() error {
	s.closed = true
	s.cancel()
	return nil
}

func (s *mockStream) Context() context.Context {
	return s.ctx
}

func (s *mockStream) SetDeadline(t time.Time) error { return nil }
func (s *mockStream) SetReadDeadline(t time.Time) error { return nil }
func (s *mockStream) SetWriteDeadline(t time.Time) error { return nil }
func (s *mockStream) LocalAddr() string { return "local" }
func (s *mockStream) RemoteAddr() string { return "remote" }

func TestSessionOpenStream(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	limits := &config.ParsedLimits{
		MaxConnections:    100,
		MaxStreamsPerConn: 10,
		StreamBufferSize:  1024,
		ConnBufferSize:    10240,
		MaxFrameSize:      16384,
	}
	
	conn := newMockConnection()
	sess := NewSession("test-session", conn, limits, logger, nil)
	sess.SetState(StateActive)
	
	stream, err := sess.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}
	
	if stream.ID() == 0 {
		t.Error("Stream ID should not be 0")
	}
	
	if sess.StreamCount() != 1 {
		t.Errorf("StreamCount = %d, want 1", sess.StreamCount())
	}
	
	stream.Close()
	
	if sess.StreamCount() != 0 {
		t.Errorf("StreamCount after close = %d, want 0", sess.StreamCount())
	}
}

func TestSessionStreamLimit(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	limits := &config.ParsedLimits{
		MaxConnections:    100,
		MaxStreamsPerConn: 2,
		StreamBufferSize:  1024,
		ConnBufferSize:    10240,
		MaxFrameSize:      16384,
	}
	
	conn := newMockConnection()
	sess := NewSession("test-session", conn, limits, logger, nil)
	sess.SetState(StateActive)
	
	// Open first stream
	stream1, err := sess.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream 1 failed: %v", err)
	}
	
	// Open second stream
	stream2, err := sess.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream 2 failed: %v", err)
	}
	
	// Third should fail
	_, err = sess.OpenStream(context.Background())
	if err != ErrTooManyStreams {
		t.Errorf("Expected ErrTooManyStreams, got %v", err)
	}
	
	stream1.Close()
	stream2.Close()
}

func TestSessionClose(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	limits := &config.ParsedLimits{
		MaxConnections:    100,
		MaxStreamsPerConn: 10,
		StreamBufferSize:  1024,
		ConnBufferSize:    10240,
		MaxFrameSize:      16384,
	}
	
	conn := newMockConnection()
	sess := NewSession("test-session", conn, limits, logger, nil)
	sess.SetState(StateActive)
	
	// Open a stream
	stream, _ := sess.OpenStream(context.Background())
	
	// Close session
	err := sess.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	
	if sess.State() != StateClosed {
		t.Errorf("State = %v, want Closed", sess.State())
	}
	
	// Stream should be closed
	select {
	case <-stream.Context().Done():
		// OK
	case <-time.After(100 * time.Millisecond):
		t.Error("Stream context not cancelled")
	}
	
	// Double close should be idempotent
	err = sess.Close()
	if err != nil {
		t.Errorf("Double close failed: %v", err)
	}
}

func TestSessionManager(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	limits := &config.ParsedLimits{
		MaxConnections:    2,
		MaxStreamsPerConn: 10,
		StreamBufferSize:  1024,
		ConnBufferSize:    10240,
		MaxFrameSize:      16384,
	}
	
	mgr := NewSessionManager(limits, logger, nil)
	
	conn1 := newMockConnection()
	sess1 := mgr.AddSession("session1", conn1)
	if sess1 == nil {
		t.Fatal("AddSession returned nil")
	}
	
	conn2 := newMockConnection()
	sess2 := mgr.AddSession("session2", conn2)
	if sess2 == nil {
		t.Fatal("AddSession returned nil")
	}
	
	// Third should fail (limit is 2)
	conn3 := newMockConnection()
	sess3 := mgr.AddSession("session3", conn3)
	if sess3 != nil {
		t.Error("Expected nil for third session due to limit")
	}
	
	if mgr.SessionCount() != 2 {
		t.Errorf("SessionCount = %d, want 2", mgr.SessionCount())
	}
	
	// Get session
	got, ok := mgr.GetSession("session1")
	if !ok || got != sess1 {
		t.Error("GetSession failed")
	}
	
	// Remove session
	mgr.RemoveSession("session1")
	if mgr.SessionCount() != 1 {
		t.Errorf("SessionCount after remove = %d, want 1", mgr.SessionCount())
	}
	
	// Close all
	mgr.CloseAll()
	if mgr.SessionCount() != 0 {
		t.Errorf("SessionCount after CloseAll = %d, want 0", mgr.SessionCount())
	}
}

func TestFrameRoundTrip(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	limits := &config.ParsedLimits{
		MaxConnections:    100,
		MaxStreamsPerConn: 10,
		StreamBufferSize:  1024,
		ConnBufferSize:    10240,
		MaxFrameSize:      16384,
	}
	
	conn := newMockConnection()
	sess := NewSession("test-session", conn, limits, logger, nil)
	sess.SetState(StateActive)
	
	stream, err := sess.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}
	
	// Write raw data
	data := []byte("hello world")
	
	if _, err := stream.Write(data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	
	// Read back
	buf := make([]byte, len(data))
	n, err := stream.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	
	if n != len(data) {
		t.Errorf("Read %d bytes, want %d", n, len(data))
	}
	
	for i := range data {
		if buf[i] != data[i] {
			t.Errorf("Data mismatch at %d: got %d, want %d", i, buf[i], data[i])
		}
	}
}