package quic

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/qtunnel/qtunnel/internal/carrier"
	"github.com/qtunnel/qtunnel/internal/config"
	"go.uber.org/zap"
)

// Dialer creates QUIC connections
type Dialer struct {
	quicConfig *quic.Config
	tlsConfig  *tls.Config
	logger     *zap.Logger
}

// QUICConfig holds parsed QUIC configuration
type QUICConfig struct {
	MaxIdleTimeout            time.Duration
	KeepAlive                 time.Duration
	HandshakeTimeout          time.Duration
	MaxReceiveBufferSize      int
	MaxIncomingStreams        int
	MaxIncomingUniStreams     int
}

// NewDialer creates a new QUIC dialer
func NewDialer(cfg *config.QUICConfig, logger *zap.Logger) *Dialer {
	qcfg := parseQUICConfig(cfg)

	quicConfig := &quic.Config{
		MaxIdleTimeout:            qcfg.MaxIdleTimeout,
		KeepAlivePeriod:           qcfg.KeepAlive,
		HandshakeIdleTimeout:      qcfg.HandshakeTimeout,
		MaxIncomingStreams:        int64(qcfg.MaxIncomingStreams),
		MaxIncomingUniStreams:     int64(qcfg.MaxIncomingUniStreams),
	}

	return &Dialer{
		quicConfig: quicConfig,
		logger:     logger,
	}
}

func parseQUICConfig(cfg *config.QUICConfig) *QUICConfig {
	if cfg == nil {
		cfg = &config.QUICConfig{}
	}
	return &QUICConfig{
		MaxIdleTimeout:            parseDuration(cfg.MaxIdleTimeout, 30*time.Second),
		KeepAlive:                 parseDuration(cfg.KeepAlive, 10*time.Second),
		HandshakeTimeout:          parseDuration(cfg.HandshakeTimeout, 10*time.Second),
		MaxReceiveBufferSize:      parseSize(cfg.MaxReceiveBufferSize, 10<<20),
		MaxIncomingStreams:        cfg.MaxIncomingStreams,
		MaxIncomingUniStreams:     cfg.MaxIncomingUniStreams,
	}
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

// SetTLSConfig sets the TLS configuration
func (d *Dialer) SetTLSConfig(tlsConfig *tls.Config) {
	d.tlsConfig = tlsConfig
}

// Dial dials a QUIC connection
func (d *Dialer) Dial(ctx context.Context, addr, sni string, insecureSkipVerify bool) (carrier.Connection, error) {
	if d.tlsConfig == nil {
		d.tlsConfig = &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: insecureSkipVerify,
			NextProtos:         []string{"qtunnel/1"},
		}
	} else {
		d.tlsConfig.ServerName = sni
		d.tlsConfig.InsecureSkipVerify = insecureSkipVerify
		d.tlsConfig.NextProtos = []string{"qtunnel/1"}
	}

	d.logger.Debug("Dialing QUIC", zap.String("addr", addr), zap.String("sni", sni))

	conn, err := quic.DialAddr(ctx, addr, d.tlsConfig, d.quicConfig)
	if err != nil {
		d.logger.Error("QUIC dial failed", zap.Error(err))
		return nil, err
	}

	d.logger.Info("QUIC connection established", zap.String("remote", conn.RemoteAddr().String()))

	return &Conn{conn: conn, logger: d.logger}, nil
}

// Listener wraps a QUIC listener
type Listener struct {
	listener *quic.Listener
	logger   *zap.Logger
}

// Listen creates a QUIC listener
func Listen(addr string, tlsConfig *tls.Config, quicConfig *quic.Config, logger *zap.Logger) (*Listener, error) {
	ln, err := quic.ListenAddr(addr, tlsConfig, quicConfig)
	if err != nil {
		return nil, err
	}
	logger.Info("QUIC listener started", zap.String("addr", addr))
	return &Listener{listener: ln, logger: logger}, nil
}

// Accept accepts a new connection
func (l *Listener) Accept(ctx context.Context) (carrier.Connection, error) {
	conn, err := l.listener.Accept(ctx)
	if err != nil {
		return nil, err
	}
	l.logger.Debug("Accepted QUIC connection", zap.String("remote", conn.RemoteAddr().String()))
	return &Conn{conn: conn, logger: l.logger}, nil
}

// Close closes the listener
func (l *Listener) Close() error {
	return l.listener.Close()
}

// Addr returns the listener address
func (l *Listener) Addr() net.Addr {
	return l.listener.Addr()
}

// Conn wraps a quic.Connection
type Conn struct {
	conn   *quic.Conn
	logger *zap.Logger
}

// OpenStreamSync opens a new bidirectional stream
func (c *Conn) OpenStreamSync(ctx context.Context) (carrier.Stream, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return &Stream{stream: stream, logger: c.logger}, nil
}

// AcceptStream accepts an incoming stream
func (c *Conn) AcceptStream(ctx context.Context) (carrier.Stream, error) {
	stream, err := c.conn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return &Stream{stream: stream, logger: c.logger}, nil
}

// Close closes the connection
func (c *Conn) Close() error {
	return c.conn.CloseWithError(0, "")
}

// Context returns the connection context
func (c *Conn) Context() context.Context {
	return c.conn.Context()
}

// RemoteAddr returns the remote address
func (c *Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// LocalAddr returns the local address
func (c *Conn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// Stream wraps a quic.Stream
type Stream struct {
	stream *quic.Stream
	logger *zap.Logger
}

// Read reads from the stream
func (s *Stream) Read(p []byte) (int, error) {
	return s.stream.Read(p)
}

// Write writes to the stream
func (s *Stream) Write(p []byte) (int, error) {
	return s.stream.Write(p)
}

// Close closes the stream
func (s *Stream) Close() error {
	return s.stream.Close()
}

// ID returns the stream ID
func (s *Stream) ID() uint64 {
	return uint64(s.stream.StreamID())
}

// Context returns the stream context
func (s *Stream) Context() context.Context {
	return s.stream.Context()
}

// SetDeadline sets the deadline
func (s *Stream) SetDeadline(t time.Time) error {
	return s.stream.SetDeadline(t)
}

// SetReadDeadline sets the read deadline
func (s *Stream) SetReadDeadline(t time.Time) error {
	return s.stream.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline
func (s *Stream) SetWriteDeadline(t time.Time) error {
	return s.stream.SetWriteDeadline(t)
}

// LocalAddr returns the local address
func (s *Stream) LocalAddr() string {
	return "quic-stream"
}

// RemoteAddr returns the remote address
func (s *Stream) RemoteAddr() string {
	return "quic-stream"
}

// SendDatagram sends a QUIC datagram (for future use)
func (c *Conn) SendDatagram(data []byte) error {
	return c.conn.SendDatagram(data)
}

// ReceiveDatagram receives a QUIC datagram
func (c *Conn) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	return c.conn.ReceiveDatagram(ctx)
}

// ConnectionState returns the QUIC connection state
func (c *Conn) ConnectionState() quic.ConnectionState {
	return c.conn.ConnectionState()
}

// Carrier implements the carrier.Carrier interface for QUIC
type Carrier struct {
	dialer  *Dialer
	listener *Listener
	logger   *zap.Logger
}

func NewCarrier(cfg *config.QUICConfig, logger *zap.Logger) *Carrier {
	dialer := NewDialer(cfg, logger)
	return &Carrier{
		dialer:  dialer,
		logger:  logger,
	}
}

func (c *Carrier) Name() string {
	return "quic"
}

func (c *Carrier) Dial(ctx context.Context, addr, sni string, insecureSkipVerify bool) (carrier.Connection, error) {
	return c.dialer.Dial(ctx, addr, sni, insecureSkipVerify)
}

func (c *Carrier) Listen(addr string, tlsConfig *tls.Config) (carrier.Listener, error) {
	quicConfig := &quic.Config{
		MaxIdleTimeout:        30 * time.Second,
		KeepAlivePeriod:       10 * time.Second,
		HandshakeIdleTimeout:  10 * time.Second,
		MaxIncomingStreams:    100,
		MaxIncomingUniStreams: 100,
	}
	ln, err := quic.ListenAddr(addr, tlsConfig, quicConfig)
	if err != nil {
		return nil, err
	}
	return &Listener{listener: ln, logger: c.logger}, nil
}