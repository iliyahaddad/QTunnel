package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qtunnel/qtunnel/internal/carrier"
	"github.com/qtunnel/qtunnel/internal/frame"
	"github.com/qtunnel/qtunnel/internal/observability"
	"github.com/qtunnel/qtunnel/internal/session"
	"go.uber.org/zap"
)

type TCPProxyServer struct {
	listenAddr string
	logger     *zap.Logger
	metrics    *observability.Metrics
	listener   net.Listener
	session    *session.Session
	sessionMu  sync.RWMutex
	
	stopOnce sync.Once
	stopped  atomic.Bool
}

func NewTCPProxyServer(listenAddr string, logger *zap.Logger, metrics *observability.Metrics) *TCPProxyServer {
	return &TCPProxyServer{
		listenAddr: listenAddr,
		logger:     logger,
		metrics:    metrics,
	}
}

func (s *TCPProxyServer) Start() error {
	ln, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return err
	}
	s.listener = ln
	s.logger.Info("TCP proxy server listening", zap.String("addr", s.listenAddr))
	
	go s.acceptLoop()
	return nil
}

func (s *TCPProxyServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.stopped.Load() {
				return
			}
			s.logger.Warn("Accept error", zap.Error(err))
			continue
		}
		
		go s.handleConnection(conn)
	}
}

func (s *TCPProxyServer) handleConnection(localConn net.Conn) {
	defer localConn.Close()
	
	s.sessionMu.RLock()
	sess := s.session
	s.sessionMu.RUnlock()
	
	if sess == nil || sess.State() != session.StateActive {
		s.logger.Warn("No active session, dropping connection")
		return
	}
	
	// Parse target address from the incoming connection
	// Supports: HTTP CONNECT, SOCKS5, or direct target from config
	targetAddr, err := s.parseTarget(localConn)
	if err != nil {
		s.logger.Warn("Failed to parse target", zap.Error(err))
		return
	}
	
	// Open stream to server
	ctx := context.Background()
	stream, err := sess.OpenStream(ctx)
	if err != nil {
		s.logger.Warn("Failed to open stream", zap.Error(err))
		return
	}
	defer stream.Close()
	
	if s.metrics != nil {
		s.metrics.RecordStreamOpened("outbound", true)
	}
	
	// Send CONNECT frame
	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		s.logger.Warn("Invalid target address", zap.String("target", targetAddr), zap.Error(err))
		return
	}
	port, _ := strconv.Atoi(portStr)
	
	connectFrame := frame.NewConnectFrame(host, uint16(port), 0)
	if err := connectFrame.EncodeTo(stream); err != nil {
		s.logger.Warn("Failed to send CONNECT frame", zap.Error(err))
		return
	}
	
	// Read CONNECT response
	respFrame, err := frame.DecodeFrom(stream)
	if err != nil {
		s.logger.Warn("Failed to read CONNECT response", zap.Error(err))
		return
	}
	
	switch respFrame.Type {
	case frame.FrameTypeConnectOK:
		s.logger.Debug("CONNECT successful", zap.String("target", targetAddr))
	case frame.FrameTypeConnectFail:
		code, msg, _ := frame.ParseConnectFailPayload(respFrame.Payload)
		s.logger.Warn("CONNECT failed", zap.Uint64("code", code), zap.String("msg", msg))
		return
	default:
		s.logger.Warn("Unexpected CONNECT response", zap.String("type", respFrame.Type.String()))
		return
	}
	
	// Start bidirectional piping
	Pipe(ctx, stream, localConn, s.metrics, "quic")
}

func (s *TCPProxyServer) SetSession(sess *session.Session) {
	s.sessionMu.Lock()
	s.session = sess
	s.sessionMu.Unlock()
}

// parseTarget parses the target address from the incoming connection
// Supports HTTP CONNECT and SOCKS5
func (s *TCPProxyServer) parseTarget(conn net.Conn) (string, error) {
	// Set a read deadline for target parsing
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	
	// Peek at first byte to determine protocol
	buf := make([]byte, 1)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", fmt.Errorf("empty request")
	}
	
	firstByte := buf[0]
	
	// SOCKS5 starts with 0x05
	if firstByte == 0x05 {
		return s.parseSOCKS5Target(conn, buf[:1])
	}
	
	// HTTP CONNECT starts with 'C' (CONNECT)
	if firstByte == 'C' {
		return s.parseHTTPConnectTarget(conn, buf[:1])
	}
	
	// Default: assume direct target (for testing)
	// Read the rest and try to parse as host:port
	rest := make([]byte, 1023)
	n, err = conn.Read(rest)
	if err != nil && err != io.EOF {
		return "", err
	}
	
	fullReq := append(buf, rest[:n]...)
	return s.parseDirectTarget(string(fullReq))
}

func (s *TCPProxyServer) parseSOCKS5Target(conn net.Conn, prefix []byte) (string, error) {
	// Read nmethods
	nmethodsBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, nmethodsBuf); err != nil {
		return "", err
	}
	nmethods := nmethodsBuf[0]
	
	// Skip methods
	methods := make([]byte, nmethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", err
	}
	
	// Send no-auth response
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return "", err
	}
	
	// Read request header (4 bytes: VER, CMD, RSV, ATYP)
	reqHeader := make([]byte, 4)
	if _, err := io.ReadFull(conn, reqHeader); err != nil {
		return "", err
	}
	
	if reqHeader[0] != 0x05 || reqHeader[1] != 0x01 { // VER=5, CMD=CONNECT
		return "", fmt.Errorf("unsupported SOCKS5 command")
	}
	
	atyp := reqHeader[3]
	var addr string
	
	switch atyp {
	case 0x01: // IPv4
		ip := make([]byte, 4)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return "", err
		}
		addr = net.IP(ip).String()
	case 0x03: // Domain name
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", err
		}
		domain := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", err
		}
		addr = string(domain)
	case 0x04: // IPv6
		ip := make([]byte, 16)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return "", err
		}
		addr = net.IP(ip).String()
	default:
		return "", fmt.Errorf("unsupported address type")
	}
	
	// Read port (2 bytes)
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", err
	}
	port := uint16(portBuf[0])<<8 | uint16(portBuf[1])
	
	// Send success response
	resp := []byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	conn.Write(resp)
	
	return net.JoinHostPort(addr, strconv.Itoa(int(port))), nil
}

func (s *TCPProxyServer) parseHTTPConnectTarget(conn net.Conn, prefix []byte) (string, error) {
	// Read until end of headers
	buf := make([]byte, 4096)
	copy(buf, prefix)
	
	n, err := conn.Read(buf[1:])
	if err != nil {
		return "", err
	}
	
	request := string(buf[:n+1])
	
	// Parse CONNECT request line: CONNECT host:port HTTP/1.1
	lines := strings.Split(request, "\r\n")
	if len(lines) == 0 {
		return "", fmt.Errorf("empty request")
	}
	
	parts := strings.Split(lines[0], " ")
	if len(parts) != 3 || parts[0] != "CONNECT" {
		return "", fmt.Errorf("invalid CONNECT request")
	}
	
	hostPort := parts[1]
	
	// Validate it's a valid host:port
	if _, _, err := net.SplitHostPort(hostPort); err != nil {
		return "", err
	}
	
	// Send 200 Connection Established
	resp := "HTTP/1.1 200 Connection Established\r\n\r\n"
	conn.Write([]byte(resp))
	
	return hostPort, nil
}

func (s *TCPProxyServer) parseDirectTarget(req string) (string, error) {
	// Try to parse as host:port
	if _, _, err := net.SplitHostPort(req); err == nil {
		return req, nil
	}
	
	// If no port, assume 443
	if strings.Contains(req, ":") {
		return "", fmt.Errorf("invalid address format")
	}
	
	return net.JoinHostPort(req, "443"), nil
}

func (s *TCPProxyServer) Stop() {
	s.stopOnce.Do(func() {
		s.stopped.Store(true)
		if s.listener != nil {
			s.listener.Close()
		}
	})
}

// Pipe copies data bidirectionally between a carrier stream and a net.Conn
func Pipe(ctx context.Context, stream carrier.Stream, conn net.Conn, metrics *observability.Metrics, transport string) {
	var wg sync.WaitGroup
	wg.Add(2)
	
	// Stream -> Conn
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			
			n, err := stream.Read(buf)
			if n > 0 {
				if _, err := conn.Write(buf[:n]); err != nil {
					return
				}
				if metrics != nil {
					metrics.RecordBytesSent(transport, int64(n))
				}
			}
			if err != nil {
				return
			}
		}
	}()
	
	// Conn -> Stream
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			
			n, err := conn.Read(buf)
			if n > 0 {
				if _, err := stream.Write(buf[:n]); err != nil {
					return
				}
				if metrics != nil {
					metrics.RecordBytesReceived(transport, int64(n))
				}
			}
			if err != nil {
				return
			}
		}
	}()
	
	wg.Wait()
	stream.Close()
	conn.Close()
	
	if metrics != nil {
		metrics.RecordStreamClosed("bidirectional", "completed")
	}
}

// SOCKS5ProxyServer implements a SOCKS5 proxy
type SOCKS5ProxyServer struct {
	listenAddr string
	logger     *zap.Logger
	metrics    *observability.Metrics
	listener   net.Listener
	session    *session.Session
	sessionMu  sync.RWMutex
	
	stopOnce sync.Once
	stopped  atomic.Bool
}

func NewSOCKS5ProxyServer(listenAddr string, logger *zap.Logger, metrics *observability.Metrics) *SOCKS5ProxyServer {
	return &SOCKS5ProxyServer{
		listenAddr: listenAddr,
		logger:     logger,
		metrics:    metrics,
	}
}

func (s *SOCKS5ProxyServer) Start() error {
	ln, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return err
	}
	s.listener = ln
	s.logger.Info("SOCKS5 proxy server listening", zap.String("addr", s.listenAddr))
	
	go s.acceptLoop()
	return nil
}

func (s *SOCKS5ProxyServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.stopped.Load() {
				return
			}
			s.logger.Warn("Accept error", zap.Error(err))
			continue
		}
		
		go s.handleSOCKS5Connection(conn)
	}
}

func (s *SOCKS5ProxyServer) handleSOCKS5Connection(conn net.Conn) {
	defer conn.Close()
	
	// SOCKS5 handshake
	if err := s.socks5Handshake(conn); err != nil {
		s.logger.Warn("SOCKS5 handshake failed", zap.Error(err))
		return
	}
	
	// Get target address
	targetAddr, err := s.socks5GetTarget(conn)
	if err != nil {
		s.logger.Warn("Failed to get target", zap.Error(err))
		return
	}
	
	s.sessionMu.RLock()
	sess := s.session
	s.sessionMu.RUnlock()
	
	if sess == nil || sess.State() != session.StateActive {
		s.logger.Warn("No active session")
		return
	}
	
	// Open stream
	ctx := context.Background()
	stream, err := sess.OpenStream(ctx)
	if err != nil {
		s.logger.Warn("Failed to open stream", zap.Error(err))
		return
	}
	defer stream.Close()
	
	// Send CONNECT frame
	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		s.logger.Warn("Invalid target address", zap.Error(err))
		return
	}
	_ = host
	_ = portStr
	
	// For now, just pipe
	Pipe(ctx, stream, conn, s.metrics, "quic")
}

func (s *SOCKS5ProxyServer) socks5Handshake(conn net.Conn) error {
	// Read version and nmethods
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return err
	}
	if buf[0] != 0x05 {
		return io.EOF
	}
	
	nmethods := buf[1]
	methods := make([]byte, nmethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}
	
	// We support no authentication (0x00)
	resp := []byte{0x05, 0x00}
	_, err := conn.Write(resp)
	return err
}

func (s *SOCKS5ProxyServer) socks5GetTarget(conn net.Conn) (string, error) {
	// Read request
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", err
	}
	if buf[0] != 0x05 || buf[1] != 0x01 { // CONNECT command
		return "", io.EOF
	}
	
	// Read address type
	atyp := buf[3]
	var addr string
	
	switch atyp {
	case 0x01: // IPv4
		ip := make([]byte, 4)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return "", err
		}
		addr = net.IP(ip).String()
	case 0x03: // Domain name
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", err
		}
		domain := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", err
		}
		addr = string(domain)
	case 0x04: // IPv6
		ip := make([]byte, 16)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return "", err
		}
		addr = net.IP(ip).String()
	default:
		return "", io.EOF
	}
	
	// Read port
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", err
	}
	port := uint16(portBuf[0])<<8 | uint16(portBuf[1])
	
	// Send success response
	resp := []byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	conn.Write(resp)
	
	return net.JoinHostPort(addr, strconv.Itoa(int(port))), nil
}

func (s *SOCKS5ProxyServer) SetSession(sess *session.Session) {
	s.sessionMu.Lock()
	s.session = sess
	s.sessionMu.Unlock()
}

func (s *SOCKS5ProxyServer) Stop() {
	s.stopOnce.Do(func() {
		s.stopped.Store(true)
		if s.listener != nil {
			s.listener.Close()
		}
	})
}