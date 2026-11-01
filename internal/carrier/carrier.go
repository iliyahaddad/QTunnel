package carrier

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"time"
)

// Stream represents a bidirectional stream
type Stream interface {
	io.Reader
	io.Writer
	io.Closer
	
	// ID returns the stream ID
	ID() uint64
	
	// Context returns the stream context
	Context() context.Context
	
	// SetDeadline sets the deadline for read/write operations
	SetDeadline(t time.Time) error
	
	// SetReadDeadline sets the deadline for read operations
	SetReadDeadline(t time.Time) error
	
	// SetWriteDeadline sets the deadline for write operations
	SetWriteDeadline(t time.Time) error
	
	// LocalAddr returns the local address
	LocalAddr() string
	
	// RemoteAddr returns the remote address
	RemoteAddr() string
}

// Connection represents a transport connection
type Connection interface {
	// OpenStreamSync opens a new bidirectional stream
	OpenStreamSync(ctx context.Context) (Stream, error)
	
	// AcceptStream accepts an incoming stream
	AcceptStream(ctx context.Context) (Stream, error)
	
	// Close closes the connection
	Close() error
	
	// Context returns the connection context
	Context() context.Context
	
	// RemoteAddr returns the remote address
	RemoteAddr() net.Addr
	
	// LocalAddr returns the local address
	LocalAddr() net.Addr
}

// Listener represents a transport listener
type Listener interface {
	// Accept accepts a new connection
	Accept(ctx context.Context) (Connection, error)
	
	// Close closes the listener
	Close() error
	
	// Addr returns the listener address
	Addr() net.Addr
}

// Carrier represents a transport carrier
type Carrier interface {
	// Name returns the carrier name
	Name() string
	
	// Dial dials a new connection
	Dial(ctx context.Context, addr, sni string, insecureSkipVerify bool) (Connection, error)
	
	// Listen creates a new listener
	Listen(addr string, tlsConfig *tls.Config) (Listener, error)
}

// StreamWrapper wraps a quic.Stream to implement carrier.Stream
type StreamWrapper struct {
	stream interface {
		io.Reader
		io.Writer
		io.Closer
		StreamID() int64
		Context() context.Context
		SetDeadline(t time.Time) error
		SetReadDeadline(t time.Time) error
		SetWriteDeadline(t time.Time) error
	}
}

func (s *StreamWrapper) Read(p []byte) (int, error) {
	return s.stream.Read(p)
}

func (s *StreamWrapper) Write(p []byte) (int, error) {
	return s.stream.Write(p)
}

func (s *StreamWrapper) Close() error {
	return s.stream.Close()
}

func (s *StreamWrapper) ID() uint64 {
	return uint64(s.stream.StreamID())
}

func (s *StreamWrapper) Context() context.Context {
	return s.stream.Context()
}

func (s *StreamWrapper) SetDeadline(t time.Time) error {
	return s.stream.SetDeadline(t)
}

func (s *StreamWrapper) SetReadDeadline(t time.Time) error {
	return s.stream.SetReadDeadline(t)
}

func (s *StreamWrapper) SetWriteDeadline(t time.Time) error {
	return s.stream.SetWriteDeadline(t)
}

func (s *StreamWrapper) LocalAddr() string {
	return "quic-stream"
}

func (s *StreamWrapper) RemoteAddr() string {
	return "quic-stream"
}

// ConnectionWrapper wraps a quic.Connection to implement carrier.Connection
type ConnectionWrapper struct {
	conn interface {
		OpenStreamSync(ctx context.Context) (interface{}, error)
		AcceptStream(ctx context.Context) (interface{}, error)
		Close() error
		Context() context.Context
		RemoteAddr() net.Addr
		LocalAddr() net.Addr
	}
}

func (c *ConnectionWrapper) OpenStreamSync(ctx context.Context) (Stream, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return &StreamWrapper{stream: stream.(interface {
		io.Reader
		io.Writer
		io.Closer
		StreamID() int64
		Context() context.Context
		SetDeadline(t time.Time) error
		SetReadDeadline(t time.Time) error
		SetWriteDeadline(t time.Time) error
	})}, nil
}

func (c *ConnectionWrapper) AcceptStream(ctx context.Context) (Stream, error) {
	stream, err := c.conn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return &StreamWrapper{stream: stream.(interface {
		io.Reader
		io.Writer
		io.Closer
		StreamID() int64
		Context() context.Context
		SetDeadline(t time.Time) error
		SetReadDeadline(t time.Time) error
		SetWriteDeadline(t time.Time) error
	})}, nil
}

func (c *ConnectionWrapper) Close() error {
	return c.conn.Close()
}

func (c *ConnectionWrapper) Context() context.Context {
	return c.conn.Context()
}

func (c *ConnectionWrapper) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *ConnectionWrapper) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}