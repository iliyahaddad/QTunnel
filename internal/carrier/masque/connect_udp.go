package masque

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/qtunnel/qtunnel/internal/carrier"
	"github.com/qtunnel/qtunnel/internal/config"
	"github.com/qtunnel/qtunnel/internal/frame"
	"go.uber.org/zap"
)

// CONNECTUDPConfig holds CONNECT-UDP specific configuration
type CONNECTUDPConfig struct {
	URITemplate     string `yaml:"uri_template"`
	MaxDatagramSize int    `yaml:"max_datagram_size"`
}

// CONNECTUDPClient implements CONNECT-UDP client
type CONNECTUDPClient struct {
	cfg         *CONNECTUDPConfig
	http3Client *http3.Client
	logger      *zap.Logger
	mu          sync.Mutex
	ctxMap      map[string]context.CancelFunc
}

// NewCONNECTUDPClient creates a new CONNECT-UDP client
func NewCONNECTUDPClient(cfg *CONNECTUDPConfig, http3Client *http3.Client, logger *zap.Logger) *CONNECTUDPClient {
	return &CONNECTUDPClient{
		cfg:         cfg,
		http3Client: http3Client,
		logger:      logger,
		ctxMap:      make(map[string]context.CancelFunc),
	}
}

// DialUDP establishes a CONNECT-UDP tunnel to the target
func (c *CONNECTUDPClient) DialUDP(ctx context.Context, targetHost string, targetPort uint16) (net.PacketConn, error) {
	// Expand URI template
	url, err := c.expandURITemplate(targetHost, fmt.Sprintf("%d", targetPort))
	if err != nil {
		return nil, fmt.Errorf("URI template expansion failed: %w", err)
	}

	// Create CONNECT-UDP request
	req, err := http.NewRequestWithContext(ctx, http.MethodConnect, url, nil)
	if err != nil {
		return nil, err
	}

	// Set Extended CONNECT headers per RFC 9220
	req.Header.Set(":protocol", "connect-udp")
	req.Header.Set(":authority", c.extractAuthority(url))
	req.Header.Set("capsule-protocol", "?1")

	// Send request
	resp, err := c.http3Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("CONNECT-UDP request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CONNECT-UDP failed: %d %s", resp.StatusCode, resp.Status)
	}

	// Create packet connection wrapper
	pconn := &UDPProxyPacketConn{
		body:   resp.Body,
		target: net.JoinHostPort(targetHost, fmt.Sprintf("%d", targetPort)),
		logger: logger,
	}

	return pconn, nil
}

func (c *CONNECTUDPClient) expandURITemplate(targetHost string, targetPort string) (string, error) {
	if c.cfg.URITemplate == "" {
		// Default template per RFC 9298 §2.6
		return fmt.Sprintf("https://%s/.well-known/masque/udp/%s/%s/", 
			"placeholder", targetHost, targetPort), nil
	}

	// Simple template expansion - replace {target_host} and {target_port}
	result := c.cfg.URITemplate
	result = strings.ReplaceAll(result, "{target_host}", targetHost)
	result = strings.ReplaceAll(result, "{target_port}", targetPort)
	
	return result, nil
}

func (c *CONNECTUDPClient) extractAuthority(url string) string {
	if strings.HasPrefix(url, "https://") {
		url = url[8:]
	}
	if idx := strings.Index(url, "/"); idx >= 0 {
		return url[:idx]
	}
	return url
}

// UDPProxyPacketConn wraps HTTP/3 response body as a packet connection
type UDPProxyPacketConn struct {
	body   interface {
		Read([]byte) (int, error)
		Close() error
	}
	target string
	logger *zap.Logger
}

func (p *UDPProxyPacketConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, err := p.body.Read(b)
	if err != nil {
		return 0, nil, err
	}
	// Parse CONNECT-UDP capsule format
	// For now, return raw data with target address
	addr, _ := net.ResolveUDPAddr("udp", p.target)
	return n, addr, nil
}

func (p *UDPProxyPacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	// For CONNECT-UDP, we'd write to the HTTP/3 request stream
	// This requires the request body which we don't have access to here
	// In a full implementation, we'd use the request stream
	return 0, fmt.Errorf("WriteTo not implemented for client")
}

func (p *UDPProxyPacketConn) Close() error {
	return p.body.Close()
}

func (p *UDPProxyPacketConn) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}

func (p *UDPProxyPacketConn) SetDeadline(t time.Time) error { return nil }
func (p *UDPProxyPacketConn) SetReadDeadline(t time.Time) error { return nil }
func (p *UDPProxyPacketConn) SetWriteDeadline(t time.Time) error { return nil }