package http2

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/qtunnel/qtunnel/internal/carrier"
	"github.com/qtunnel/qtunnel/internal/config"
	"go.uber.org/zap"
)

// HTTP2Config holds parsed HTTP/2 configuration
type HTTP2Config struct {
	MaxIdleTimeout       time.Duration
	KeepAlive            time.Duration
	HandshakeTimeout     time.Duration
	MaxReceiveBufferSize int
	MaxConcurrentStreams int
	DisablePathMigration bool
}

// Carrier implements the carrier.Carrier interface for HTTP/2
type Carrier struct {
	http2Config *HTTP2Config
	logger      *zap.Logger
}

func NewCarrier(cfg *config.HTTP2Config, logger *zap.Logger) *Carrier {
	h2cfg := parseHTTP2Config(cfg)
	return &Carrier{
		http2Config: h2cfg,
		logger:      logger,
	}
}

func parseHTTP2Config(cfg *config.HTTP2Config) *HTTP2Config {
	if cfg == nil {
		cfg = &config.HTTP2Config{}
	}
	return &HTTP2Config{
		MaxIdleTimeout:       parseDuration(cfg.MaxIdleTimeout, 30*time.Second),
		KeepAlive:            parseDuration(cfg.KeepAlive, 10*time.Second),
		HandshakeTimeout:     parseDuration(cfg.HandshakeTimeout, 10*time.Second),
		MaxReceiveBufferSize: parseSize(cfg.MaxReceiveBufferSize, 10<<20),
		MaxConcurrentStreams: cfg.MaxConcurrentStreams,
		DisablePathMigration: cfg.DisablePathMigration,
	}
}

func (c *Carrier) Name() string {
	return "http2"
}

func (c *Carrier) Dial(ctx context.Context, addr, sni string, insecureSkipVerify bool) (carrier.Connection, error) {
	tlsConfig := &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: insecureSkipVerify,
		NextProtos:         []string{"h2"},
	}

	c.logger.Debug("Dialing HTTP/2", zap.String("addr", addr), zap.String("sni", sni))

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		c.logger.Error("HTTP/2 dial failed", zap.Error(err))
		return nil, err
	}

	// Create HTTP/2 client
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:    tlsConfig,
			ForceAttemptHTTP2:  true,
		},
	}

	return &Connection{
		conn:   conn,
		client: client,
		logger: c.logger,
	}, nil
}

func (c *Carrier) Listen(addr string, tlsConfig *tls.Config) (carrier.Listener, error) {
	if tlsConfig == nil {
		return nil, fmt.Errorf("tls config required")
	}
	tlsConfig.NextProtos = []string{"h2"}

	ln, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		return nil, err
	}

	c.logger.Info("HTTP/2 listener started", zap.String("addr", addr))

	return &Listener{
		listener: ln,
		logger:   c.logger,
	}, nil
}

// Connection wraps an HTTP/2 connection
type Connection struct {
	conn   *tls.Conn
	client *http.Client
	logger *zap.Logger
}

func (c *Connection) OpenStreamSync(ctx context.Context) (carrier.Stream, error) {
	// Use Extended CONNECT for HTTP/2
	req, err := http.NewRequestWithContext(ctx, http.MethodConnect, "https://placeholder/", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(":protocol", "connect-tcp")
	req.Header.Set(":authority", "placeholder")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CONNECT failed: %d", resp.StatusCode)
	}

	// The response body is the stream
	return &Stream{
		body:   resp.Body,
		ctx:    context.Background(),
		logger: c.logger,
	}, nil
}

func (c *Connection) AcceptStream(ctx context.Context) (carrier.Stream, error) {
	// For client connections, AcceptStream is not used
	return nil, fmt.Errorf("AcceptStream not implemented for client connection")
}

func (c *Connection) Close() error {
	return c.conn.Close()
}

func (c *Connection) Context() context.Context {
	return context.Background()
}

func (c *Connection) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *Connection) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// Listener wraps a TLS listener for HTTP/2
type Listener struct {
	listener net.Listener
	logger   *zap.Logger
}

func (l *Listener) Accept(ctx context.Context) (carrier.Connection, error) {
	conn, err := l.listener.Accept()
	if err != nil {
		return nil, err
	}
	l.logger.Debug("Accepted TLS connection", zap.String("remote", conn.RemoteAddr().String()))

	// For HTTP/2 server, we would need to handle h2c or TLS with h2
	// This is a simplified implementation
	return &Connection{
		conn: conn.(*tls.Conn),
		logger: l.logger,
	}, nil
}

func (l *Listener) Close() error {
	return l.listener.Close()
}

func (l *Listener) Addr() net.Addr {
	return l.listener.Addr()
}

// Stream wraps an HTTP/2 response body as a stream
type Stream struct {
	body   io.ReadCloser
	ctx    context.Context
	logger *zap.Logger
}

func (s *Stream) Read(p []byte) (int, error) {
	return s.body.Read(p)
}

func (s *Stream) Write(p []byte) (int, error) {
	// HTTP/2 response body is read-only for CONNECT
	return 0, fmt.Errorf("write not supported on response body")
}

func (s *Stream) Close() error {
	return s.body.Close()
}

func (s *Stream) ID() uint64 {
	return 0 // HTTP/2 streams don't have a simple ID like QUIC
}

func (s *Stream) Context() context.Context {
	return s.ctx
}

func (s *Stream) SetDeadline(t time.Time) error {
	return nil
}

func (s *Stream) SetReadDeadline(t time.Time) error {
	return nil
}

func (s *Stream) SetWriteDeadline(t time.Time) error {
	return nil
}

func (s *Stream) LocalAddr() string {
	return "http2"
}

func (s *Stream) RemoteAddr() string {
	return "http2"
}

func parseDuration(s string, defaultVal time.Duration) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}

func parseSize(s string, defaultVal int) int {
	return defaultVal
}