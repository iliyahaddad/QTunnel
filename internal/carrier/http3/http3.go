package http3

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/qtunnel/qtunnel/internal/carrier"
	"github.com/qtunnel/qtunnel/internal/config"
	"go.uber.org/zap"
)

// HTTP3Config holds parsed HTTP/3 configuration
type HTTP3Config struct {
	MaxIdleTimeout        time.Duration
	KeepAlive             time.Duration
	HandshakeTimeout      time.Duration
	MaxReceiveBufferSize  int
	MaxIncomingStreams    int
	MaxIncomingUniStreams int
}

// Carrier implements the carrier.Carrier interface for HTTP/3
type Carrier struct {
	http3Config *HTTP3Config
	logger      *zap.Logger
}

func NewCarrier(cfg *config.HTTP3Config, logger *zap.Logger) *Carrier {
	h3cfg := parseHTTP3Config(cfg)
	return &Carrier{
		http3Config: h3cfg,
		logger:      logger,
	}
}

func parseHTTP3Config(cfg *config.HTTP3Config) *HTTP3Config {
	if cfg == nil {
		cfg = &config.HTTP3Config{}
	}
	return &HTTP3Config{
		MaxIdleTimeout:        parseDuration(cfg.MaxIdleTimeout, 30*time.Second),
		KeepAlive:             parseDuration(cfg.KeepAlive, 10*time.Second),
		HandshakeTimeout:      parseDuration(cfg.HandshakeTimeout, 10*time.Second),
		MaxReceiveBufferSize:  parseSize(cfg.MaxReceiveBufferSize, 10<<20),
		MaxIncomingStreams:    cfg.MaxIncomingStreams,
		MaxIncomingUniStreams: cfg.MaxIncomingUniStreams,
	}
}

func (c *Carrier) Name() string {
	return "http3"
}

func (c *Carrier) Dial(ctx context.Context, addr, sni string, insecureSkipVerify bool) (carrier.Connection, error) {
	tlsConfig := &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: insecureSkipVerify,
		NextProtos:         []string{"h3"},
	}

	quicConfig := &quic.Config{
		MaxIdleTimeout:        c.http3Config.MaxIdleTimeout,
		KeepAlivePeriod:       c.http3Config.KeepAlive,
		HandshakeIdleTimeout:  c.http3Config.HandshakeTimeout,
		MaxIncomingStreams:    int64(c.http3Config.MaxIncomingStreams),
		MaxIncomingUniStreams: int64(c.http3Config.MaxIncomingUniStreams),
	}

	c.logger.Debug("Dialing HTTP/3", zap.String("addr", addr), zap.String("sni", sni))

	conn, err := quic.DialAddr(ctx, addr, tlsConfig, quicConfig)
	if err != nil {
		c.logger.Error("HTTP/3 dial failed", zap.Error(err))
		return nil, err
	}

	return &Connection{
		conn:   conn,
		logger: c.logger,
	}, nil
}

func (c *Carrier) Listen(addr string, tlsConfig *tls.Config) (carrier.Listener, error) {
	if tlsConfig == nil {
		return nil, fmt.Errorf("tls config required")
	}
	tlsConfig.NextProtos = []string{"h3"}

	quicConfig := &quic.Config{
		MaxIdleTimeout:        c.http3Config.MaxIdleTimeout,
		KeepAlivePeriod:       c.http3Config.KeepAlive,
		HandshakeIdleTimeout:  c.http3Config.HandshakeTimeout,
		MaxIncomingStreams:    int64(c.http3Config.MaxIncomingStreams),
		MaxIncomingUniStreams: int64(c.http3Config.MaxIncomingUniStreams),
	}

	ln, err := quic.ListenAddr(addr, tlsConfig, quicConfig)
	if err != nil {
		return nil, err
	}

	c.logger.Info("HTTP/3 listener started", zap.String("addr", addr))

	return &Listener{
		listener: ln,
		logger:   c.logger,
	}, nil
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

// Connection wraps a quic.Conn
type Connection struct {
	conn   *quic.Conn
	logger *zap.Logger
}

func (c *Connection) OpenStreamSync(ctx context.Context) (carrier.Stream, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return &Stream{stream: stream}, nil
}

func (c *Connection) AcceptStream(ctx context.Context) (carrier.Stream, error) {
	return nil, fmt.Errorf("AcceptStream not implemented for client connection")
}

func (c *Connection) Close() error {
	return c.conn.CloseWithError(0, "")
}

func (c *Connection) Context() context.Context {
	return c.conn.Context()
}

func (c *Connection) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *Connection) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// Listener wraps a quic.Listener
type Listener struct {
	listener *quic.Listener
	logger   *zap.Logger
}

func (l *Listener) Accept(ctx context.Context) (carrier.Connection, error) {
	conn, err := l.listener.Accept(ctx)
	if err != nil {
		return nil, err
	}
	l.logger.Debug("Accepted QUIC connection", zap.String("remote", conn.RemoteAddr().String()))
	return &Connection{
		conn:   conn,
		logger: l.logger,
	}, nil
}

func (l *Listener) Close() error {
	return l.listener.Close()
}

func (l *Listener) Addr() net.Addr {
	return l.listener.Addr()
}

// Stream wraps a quic stream
type Stream struct {
	stream *quic.Stream
}

func (s *Stream) Read(p []byte) (int, error) {
	return s.stream.Read(p)
}

func (s *Stream) Write(p []byte) (int, error) {
	return s.stream.Write(p)
}

func (s *Stream) Close() error {
	return s.stream.Close()
}

func (s *Stream) ID() uint64 {
	return uint64(s.stream.StreamID())
}

func (s *Stream) Context() context.Context {
	return s.stream.Context()
}

func (s *Stream) SetDeadline(t time.Time) error {
	return s.stream.SetDeadline(t)
}

func (s *Stream) SetReadDeadline(t time.Time) error {
	return s.stream.SetReadDeadline(t)
}

func (s *Stream) SetWriteDeadline(t time.Time) error {
	return s.stream.SetWriteDeadline(t)
}

func (s *Stream) LocalAddr() string {
	return "http3"
}

func (s *Stream) RemoteAddr() string {
	return "http3"
}