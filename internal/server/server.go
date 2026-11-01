package server

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/qtunnel/qtunnel/internal/auth"
	"github.com/qtunnel/qtunnel/internal/carrier"
	"github.com/qtunnel/qtunnel/internal/config"
	"github.com/qtunnel/qtunnel/internal/frame"
	"github.com/qtunnel/qtunnel/internal/observability"
	"github.com/qtunnel/qtunnel/internal/proxy"
	"github.com/qtunnel/qtunnel/internal/session"
	"go.uber.org/zap"
)

// quicConnWrapper wraps quic.Connection to implement carrier.Connection
type quicConnWrapper struct {
	conn *quic.Conn
}

func (w *quicConnWrapper) OpenStreamSync(ctx context.Context) (carrier.Stream, error) {
	stream, err := w.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return &quicStreamWrapper{stream: stream}, nil
}

func (w *quicConnWrapper) AcceptStream(ctx context.Context) (carrier.Stream, error) {
	stream, err := w.conn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return &quicStreamWrapper{stream: stream}, nil
}

func (w *quicConnWrapper) Close() error {
	return w.conn.CloseWithError(0, "")
}

func (w *quicConnWrapper) Context() context.Context {
	return w.conn.Context()
}

func (w *quicConnWrapper) RemoteAddr() net.Addr {
	return w.conn.RemoteAddr()
}

func (w *quicConnWrapper) LocalAddr() net.Addr {
	return w.conn.LocalAddr()
}

// quicStreamWrapper wraps quic.Stream to implement carrier.Stream
type quicStreamWrapper struct {
	stream *quic.Stream
}

func (w *quicStreamWrapper) ID() uint64 {
	return uint64(w.stream.StreamID())
}

func (w *quicStreamWrapper) Read(p []byte) (int, error) {
	return w.stream.Read(p)
}

func (w *quicStreamWrapper) Write(p []byte) (int, error) {
	return w.stream.Write(p)
}

func (w *quicStreamWrapper) Close() error {
	return w.stream.Close()
}

func (w *quicStreamWrapper) Context() context.Context {
	return w.stream.Context()
}

func (w *quicStreamWrapper) SetDeadline(t time.Time) error {
	return w.stream.SetDeadline(t)
}

func (w *quicStreamWrapper) SetReadDeadline(t time.Time) error {
	return w.stream.SetReadDeadline(t)
}

func (w *quicStreamWrapper) SetWriteDeadline(t time.Time) error {
	return w.stream.SetWriteDeadline(t)
}

func (w *quicStreamWrapper) LocalAddr() string {
	return "quic-stream"
}

func (w *quicStreamWrapper) RemoteAddr() string {
	return "quic-stream"
}

type Server struct {
	cfg         *config.Config
	logger      *zap.Logger
	metrics     *observability.Metrics
	listener    *quic.Listener
	tokenMgr    *auth.TokenManager
	proxyServer *proxy.TCPProxyServer
	sessionMgr  *session.SessionManager

	stopOnce sync.Once
	stopped  atomic.Bool
}

func New(cfg *config.Config, logger *zap.Logger, metrics *observability.Metrics) *Server {
	limits, _ := cfg.GetParsedLimits()

	tokenMgr, _ := auth.NewTokenManager(cfg.Auth.Tokens, cfg.Auth.TokenFile, logger)

	sessionMgr := session.NewSessionManager(limits, logger, metrics)

	proxyServer := proxy.NewTCPProxyServer("", logger, metrics)

	return &Server{
		cfg:         cfg,
		logger:      logger,
		metrics:     metrics,
		tokenMgr:    tokenMgr,
		proxyServer: proxyServer,
		sessionMgr:  sessionMgr,
	}
}

func (s *Server) Run(ctx context.Context) error {
	s.logger.Info("Starting server", zap.String("listen", s.cfg.Listen.Addr))

	// Prepare TLS config
	tlsConfig, err := s.cfg.GetTLSConfig(true)
	if err != nil {
		return err
	}

	// Prepare QUIC config
	quicConfig := s.createQUICConfig()

	// Start listener
	ln, err := quic.ListenAddr(s.cfg.Listen.Addr, tlsConfig, quicConfig)
	if err != nil {
		return err
	}
	s.listener = ln

	if s.metrics != nil {
		s.metrics.SetActiveConnections("quic", 0)
	}

	// Accept connections
	go s.acceptLoop(ctx)

	// Wait for context cancellation
	<-ctx.Done()
	s.logger.Info("Server context cancelled")
	return nil
}

func (s *Server) createQUICConfig() *quic.Config {
	quicCfg := s.cfg.Transport.QUIC
	if quicCfg == nil {
		quicCfg = &config.QUICConfig{}
	}

	qcfg := &quic.Config{
		MaxIdleTimeout:            parseDuration(quicCfg.MaxIdleTimeout, 30*time.Second),
		KeepAlivePeriod:           parseDuration(quicCfg.KeepAlive, 10*time.Second),
		HandshakeIdleTimeout:      parseDuration(quicCfg.HandshakeTimeout, 10*time.Second),
		MaxIncomingStreams:        int64(quicCfg.MaxIncomingStreams),
		MaxIncomingUniStreams:     int64(quicCfg.MaxIncomingUniStreams),
	}

	return qcfg
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn, err := s.listener.Accept(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				s.logger.Warn("Accept error", zap.Error(err))
				continue
			}

			go s.handleConnection(ctx, conn)
		}
	}
}

func (s *Server) handleConnection(ctx context.Context, conn *quic.Conn) {
	s.logger.Debug("New connection", zap.String("remote", conn.RemoteAddr().String()))
	
	if s.metrics != nil {
		s.metrics.RecordConnectionOpened("quic", true)
	}

	// Create session
	limits, _ := s.cfg.GetParsedLimits()
	sess := session.NewSession(conn.RemoteAddr().String(), &quicConnWrapper{conn: conn}, limits, s.logger, s.metrics)
	s.sessionMgr.AddSession(sess.ID(), &quicConnWrapper{conn: conn})

	// Handle streams
	go s.handleStreams(ctx, sess)

	// Wait for session to end
	<-sess.Done()

	s.sessionMgr.RemoveSession(sess.ID())

	if s.metrics != nil {
		s.metrics.RecordConnectionClosed("quic", "session_ended")
	}
}

func (s *Server) handleStreams(ctx context.Context, sess *session.Session) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-sess.Done():
			return
		default:
			stream, err := sess.AcceptStream(ctx)
			if err != nil {
				if ctx.Err() != nil || sess.State() != session.StateActive {
					return
				}
				s.logger.Warn("Accept stream error", zap.Error(err))
				continue
			}
			
			go s.handleStream(ctx, sess, stream)
		}
	}
}

func (s *Server) handleStream(ctx context.Context, sess *session.Session, strm carrier.Stream) {
	// Read first frame to determine type
	firstFrame, err := frame.DecodeFrom(strm)
	if err != nil {
		s.logger.Warn("Failed to decode first frame", zap.Error(err))
		strm.Close()
		return
	}

	switch firstFrame.Type {
	case frame.FrameTypeAuth:
		s.handleAuth(ctx, sess, strm, firstFrame)
	case frame.FrameTypeConnect:
		s.handleConnect(ctx, sess, strm, firstFrame)
	case frame.FrameTypeConnectUDP:
		s.handleConnectUDP(ctx, sess, strm, firstFrame)
	case frame.FrameTypeConnectIP:
		s.handleConnectIP(ctx, sess, strm, firstFrame)
	case frame.FrameTypeCapsule:
		s.handleCapsule(ctx, sess, strm, firstFrame)
	default:
		s.logger.Warn("Unexpected first frame type", zap.String("type", firstFrame.Type.String()))
		strm.Close()
	}
}

func (s *Server) handleAuth(ctx context.Context, sess *session.Session, strm carrier.Stream, authFrame *frame.Frame) {
	token := string(authFrame.Payload)

	var resp *frame.Frame
	if s.tokenMgr.ValidateToken(token) {
		s.logger.Debug("Authentication successful", zap.String("remote", sess.Context().Value("remote").(string)))
		resp = frame.NewAuthOKFrame()
	} else {
		s.logger.Warn("Authentication failed", zap.String("remote", sess.Context().Value("remote").(string)))
		resp = frame.NewAuthFailFrame("invalid token")
	}

	if err := resp.EncodeTo(strm); err != nil {
		s.logger.Warn("Failed to send auth response", zap.Error(err))
	}

	if resp.Type == frame.FrameTypeAuthOK {
		sess.SetState(session.StateActive)
	} else {
		strm.Close()
	}
}

func (s *Server) handleConnect(ctx context.Context, sess *session.Session, strm carrier.Stream, connectFrame *frame.Frame) {
	// Check if session is authenticated
	if sess.State() != session.StateActive {
		resp := frame.NewConnectFailFrame(frame.ErrorCodeAuthFailed, "not authenticated")
		resp.EncodeTo(strm)
		strm.Close()
		return
	}

	// Parse CONNECT payload
	addr, port, _, err := frame.ParseConnectPayload(connectFrame.Payload)
	if err != nil {
		s.logger.Warn("Invalid CONNECT payload", zap.Error(err))
		resp := frame.NewConnectFailFrame(frame.ErrorCodeInvalidDest, "invalid destination")
		resp.EncodeTo(strm)
		strm.Close()
		return
	}

	// Check ACL
	if !s.checkACL(addr, port) {
		s.logger.Warn("Destination denied by ACL", zap.String("addr", addr), zap.Int("port", int(port)))
		resp := frame.NewConnectFailFrame(frame.ErrorCodeDenied, "destination denied")
		resp.EncodeTo(strm)
		strm.Close()
		return
	}

	// Dial target
	targetAddr := net.JoinHostPort(addr, strconv.Itoa(int(port)))
	targetConn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		s.logger.Warn("Failed to connect to target", zap.String("target", targetAddr), zap.Error(err))
		resp := frame.NewConnectFailFrame(frame.ErrorCodeConnectionRefused, err.Error())
		resp.EncodeTo(strm)
		strm.Close()
		return
	}

	s.logger.Debug("Connected to target", zap.String("target", targetAddr))

	// Send CONNECT_OK
	resp := frame.NewConnectOKFrame(strm.ID(), "")
	if err := resp.EncodeTo(strm); err != nil {
		s.logger.Warn("Failed to send CONNECT_OK", zap.Error(err))
		targetConn.Close()
		strm.Close()
		return
	}

	if s.metrics != nil {
		s.metrics.RecordStreamOpened("inbound", true)
	}

	// Start proxying
	go proxy.Pipe(ctx, strm, targetConn, s.metrics, "quic")
}

// handleConnectUDP handles CONNECT-UDP requests (MASQUE RFC 9298)
func (s *Server) handleConnectUDP(ctx context.Context, sess *session.Session, strm carrier.Stream, connectFrame *frame.Frame) {
	// Check if session is authenticated
	if sess.State() != session.StateActive {
		resp := frame.NewConnectFailFrame(frame.ErrorCodeAuthFailed, "not authenticated")
		resp.EncodeTo(strm)
		strm.Close()
		return
	}

	// Parse CONNECT-UDP payload
	targetHost, targetPort, err := frame.ParseConnectUDPPayload(connectFrame.Payload)
	if err != nil {
		s.logger.Warn("Invalid CONNECT-UDP payload", zap.Error(err))
		resp := frame.NewConnectFailFrame(frame.ErrorCodeInvalidDest, "invalid destination")
		resp.EncodeTo(strm)
		strm.Close()
		return
	}

	// Check ACL for UDP target
	if !s.checkACL(targetHost, targetPort) {
		s.logger.Warn("UDP destination denied by ACL", zap.String("addr", targetHost), zap.Int("port", int(targetPort)))
		resp := frame.NewConnectFailFrame(frame.ErrorCodeDenied, "destination denied")
		resp.EncodeTo(strm)
		strm.Close()
		return
	}

	// Create UDP connection to target
	targetAddr := net.JoinHostPort(targetHost, strconv.Itoa(int(targetPort)))
	udpConn, err := net.DialTimeout("udp", targetAddr, 10*time.Second)
	if err != nil {
		s.logger.Warn("Failed to connect to UDP target", zap.String("target", targetAddr), zap.Error(err))
		resp := frame.NewConnectFailFrame(frame.ErrorCodeConnectionRefused, err.Error())
		resp.EncodeTo(strm)
		strm.Close()
		return
	}

	s.logger.Debug("UDP connection established", zap.String("target", targetAddr))

	// Send CONNECT_OK
	resp := frame.NewConnectOKFrame(strm.ID(), "")
	if err := resp.EncodeTo(strm); err != nil {
		s.logger.Warn("Failed to send CONNECT_OK", zap.Error(err))
		udpConn.Close()
		strm.Close()
		return
	}

	if s.metrics != nil {
		s.metrics.RecordStreamOpened("inbound", true)
	}

	// Proxy UDP packets through the stream
	go s.proxyUDP(ctx, strm, udpConn)
}

// proxyUDP proxies UDP packets between the stream and UDP connection
func (s *Server) proxyUDP(ctx context.Context, strm carrier.Stream, udpConn net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Stream -> UDP
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Read capsule from stream
			f, err := frame.DecodeFrom(strm)
			if err != nil {
				return
			}

			if f.Type != frame.FrameTypeCapsule {
				continue
			}

			// Parse capsule - expect DATAGRAM capsule (type 0x00)
			capsuleType, capsuleData, err := frame.ParseCapsuleFrame(f.Payload)
			if err != nil || capsuleType != 0x00 {
				continue
			}

			// Write UDP packet
			if _, err := udpConn.Write(capsuleData); err != nil {
				return
			}
		}
	}()

	// UDP -> Stream
	go func() {
		defer wg.Done()
		buf := make([]byte, 65536)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, err := udpConn.Read(buf)
			if err != nil {
				return
			}

			// Wrap UDP packet in DATAGRAM capsule (type 0x00)
			capsuleFrame := frame.NewCapsuleFrame(0x00, buf[:n])
			if err := capsuleFrame.EncodeTo(strm); err != nil {
				return
			}
		}
	}()

	wg.Wait()
	strm.Close()
	udpConn.Close()
}

// handleConnectIP handles CONNECT-IP requests (MASQUE RFC 9484)
func (s *Server) handleConnectIP(ctx context.Context, sess *session.Session, strm carrier.Stream, connectFrame *frame.Frame) {
	// Check if session is authenticated
	if sess.State() != session.StateActive {
		resp := frame.NewConnectFailFrame(frame.ErrorCodeAuthFailed, "not authenticated")
		resp.EncodeTo(strm)
		strm.Close()
		return
	}

	// Parse CONNECT-IP payload
	target, ipProto, err := frame.ParseConnectIPPayload(connectFrame.Payload)
	if err != nil {
		s.logger.Warn("Invalid CONNECT-IP payload", zap.Error(err))
		resp := frame.NewConnectFailFrame(frame.ErrorCodeInvalidDest, "invalid destination")
		resp.EncodeTo(strm)
		strm.Close()
		return
	}

	s.logger.Debug("CONNECT-IP request", zap.String("target", target), zap.Uint8("ipproto", ipProto))

	// For CONNECT-IP, we send CONNECT_OK and then handle capsules
	// This is a simplified implementation - full implementation would:
	// 1. Assign IP addresses via ADDRESS_ASSIGN capsule
	// 2. Advertise routes via ROUTE_ADVERTISEMENT capsule
	// 3. Proxy IP packets through capsules

	// Send CONNECT_OK
	resp := frame.NewConnectOKFrame(strm.ID(), "")
	if err := resp.EncodeTo(strm); err != nil {
		s.logger.Warn("Failed to send CONNECT_OK", zap.Error(err))
		strm.Close()
		return
	}

	if s.metrics != nil {
		s.metrics.RecordStreamOpened("inbound", true)
	}

	// Handle IP capsules
	go s.proxyIP(ctx, strm)
}

// proxyIP proxies IP packets through capsules
func (s *Server) proxyIP(ctx context.Context, strm carrier.Stream) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		f, err := frame.DecodeFrom(strm)
		if err != nil {
			return
		}

		if f.Type != frame.FrameTypeCapsule {
			continue
		}

		// Parse capsule
		capsuleType, _, err := frame.ParseCapsuleFrame(f.Payload)
		if err != nil {
			continue
		}

		// Handle different capsule types
		switch capsuleType {
		case 0x01: // ADDRESS_ASSIGN
			// Handle address assignment
		case 0x02: // ADDRESS_REQUEST
			// Handle address request
		case 0x03: // ROUTE_ADVERTISEMENT
			// Handle route advertisement
		case 0x00: // DATAGRAM (IP packet)
			// Forward IP packet (would need TUN device integration)
		}
	}
}

// handleCapsule handles incoming capsule frames
func (s *Server) handleCapsule(ctx context.Context, sess *session.Session, strm carrier.Stream, capsuleFrame *frame.Frame) {
	// Parse and handle capsule
	capsuleType, capsuleData, err := frame.ParseCapsuleFrame(capsuleFrame.Payload)
	if err != nil {
		s.logger.Warn("Failed to parse capsule", zap.Error(err))
		return
	}

	s.logger.Debug("Received capsule", zap.Uint64("type", capsuleType), zap.Int("len", len(capsuleData)))

	// Handle different capsule types
	switch capsuleType {
	case 0x01: // ADDRESS_ASSIGN
		// Handle address assignment
	case 0x02: // ADDRESS_REQUEST
		// Handle address request
	case 0x03: // ROUTE_ADVERTISEMENT
		// Handle route advertisement
	case 0x00: // DATAGRAM
		// Forward datagram
	default:
		s.logger.Debug("Unknown capsule type", zap.Uint64("type", capsuleType))
	}
}

func (s *Server) checkACL(addr string, port uint16) bool {
	// Check denylist first
	for _, entry := range s.cfg.Proxy.Denylist {
		if s.matchACL(entry, addr, port) {
			return false
		}
	}

	// Check allowlist
	if len(s.cfg.Proxy.Allowlist) == 0 {
		return true // No allowlist = allow all (after denylist)
	}

	for _, entry := range s.cfg.Proxy.Allowlist {
		if s.matchACL(entry, addr, port) {
			return true
		}
	}

	return false
}

func (s *Server) matchACL(entry, addr string, port uint16) bool {
	// Try CIDR match
	if prefix, err := netip.ParsePrefix(entry); err == nil {
		if ip := net.ParseIP(addr); ip != nil {
			// Convert net.IP to netip.Addr
			var netipAddr netip.Addr
			if ip4 := ip.To4(); ip4 != nil {
				netipAddr, _ = netip.AddrFromSlice(ip4)
			} else if ip16 := ip.To16(); ip16 != nil {
				netipAddr, _ = netip.AddrFromSlice(ip16)
			} else {
				return false
			}
			return prefix.Contains(netipAddr)
		}
		return false
	}

	// Hostname match
	if entry == "*" || entry == addr {
		return true
	}

	// Wildcard domain match (e.g., *.example.com)
	if len(entry) > 2 && entry[0] == '*' && entry[1] == '.' {
		suffix := entry[2:]
		return len(addr) > len(suffix) && addr[len(addr)-len(suffix):] == suffix
	}

	return false
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.stopOnce.Do(func() {
		s.stopped.Store(true)
	})

	if s.listener != nil {
		s.listener.Close()
	}

	s.sessionMgr.CloseAll()

	if s.tokenMgr != nil {
		s.tokenMgr.Close()
	}

	return nil
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