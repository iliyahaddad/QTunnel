package masque

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
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

// CONNECTIPConfig holds CONNECT-IP specific configuration
type CONNECTIPConfig struct {
	URITemplate string `yaml:"uri_template"`
}

// Capsule types per RFC 9484
const (
	CapsuleTypeAddressAssign    = 0x01
	CapsuleTypeAddressRequest   = 0x02
	CapsuleTypeRouteAdvertisement = 0x03
)

// CONNECTIPClient implements CONNECT-IP client
type CONNECTIPClient struct {
	cfg         *CONNECTIPConfig
	http3Client *http3.Client
	logger      *zap.Logger
	
	// Assigned addresses
	addressesMu sync.RWMutex
	addresses   []IPAddress
	
	// Advertised routes
	routesMu sync.RWMutex
	routes   []RouteAdvertisement
}

// IPAddress represents an assigned IP address
type IPAddress struct {
	RequestID   uint64
	IPVersion   uint8  // 4 or 6
	IPAddress   net.IP
	PrefixLen   uint8
}

// RouteAdvertisement represents a route advertisement
type RouteAdvertisement struct {
	IPVersion   uint8  // 4 or 6
	StartIP     net.IP
	EndIP       net.IP
	IPProtocol  uint8  // 0 = all
}

// NewCONNECTIPClient creates a new CONNECT-IP client
func NewCONNECTIPClient(cfg *CONNECTIPConfig, http3Client *http3.Client, logger *zap.Logger) *CONNECTIPClient {
	return &CONNECTIPClient{
		cfg:         cfg,
		http3Client: http3Client,
		logger:      logger,
		addresses:   make([]IPAddress, 0),
		routes:      make([]RouteAdvertisement, 0),
	}
}

// DialIP establishes a CONNECT-IP tunnel
func (c *CONNECTIPClient) DialIP(ctx context.Context, target, ipProto string) (net.PacketConn, error) {
	url, err := c.expandURITemplate(target, ipProto)
	if err != nil {
		return nil, fmt.Errorf("URI template expansion failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodConnect, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set(":protocol", "connect-ip")
	req.Header.Set(":authority", c.extractAuthority(url))
	req.Header.Set("capsule-protocol", "?1")

	resp, err := c.http3Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("CONNECT-IP request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CONNECT-IP failed: %d %s", resp.StatusCode, resp.Status)
	}

	// Create packet connection
	pconn := &IPProxyPacketConn{
		body:   resp.Body,
		client: c,
		logger: logger,
	}

	// Start capsule processing in background
	go c.processCapsules(resp.Body)

	return pconn, nil
}

func (c *CONNECTIPClient) expandURITemplate(target, ipProto string) (string, error) {
	if c.cfg.URITemplate == "" {
		return fmt.Sprintf("https://%s/.well-known/masque/ip/%s/%s/", 
			"placeholder", target, ipProto), nil
	}

	result := c.cfg.URITemplate
	result = strings.ReplaceAll(result, "{target}", target)
	result = strings.ReplaceAll(result, "{ipproto}", ipProto)
	
	return result, nil
}

func (c *CONNECTIPClient) extractAuthority(url string) string {
	if strings.HasPrefix(url, "https://") {
		url = url[8:]
	}
	if idx := strings.Index(url, "/"); idx >= 0 {
		return url[:idx]
	}
	return url
}

// processCapsules reads and processes capsules from the response body
func (c *CONNECTIPClient) processCapsules(body io.Reader) {
	for {
		// Read capsule header (type, length)
		typeBuf := make([]byte, 8)
		_, err := io.ReadFull(body, typeBuf[:1])
		if err != nil {
			return
		}
		
		// Simplified - full implementation would parse varint
		// For now, just log and continue
		c.logger.Debug("Received capsule")
	}
}

// SendAddressRequest sends an ADDRESS_REQUEST capsule
func (c *CONNECTIPClient) SendAddressRequest(requestID uint64, ipVersion uint8, ip net.IP, prefixLen uint8) error {
	// Build ADDRESS_REQUEST capsule
	capsule := make([]byte, 0)
	
	// Type
	capsule = append(capsule, encodeVarint(CapsuleTypeAddressRequest)...)
	
	// Length placeholder
	lengthPos := len(capsule)
	capsule = append(capsule, 0, 0, 0, 0) // placeholder
	
	// Request ID
	capsule = append(capsule, encodeVarint(requestID)...)
	
	// IP Version
	capsule = append(capsule, ipVersion)
	
	// IP Address
	if ipVersion == 4 {
		capsule = append(capsule, ip.To4()...)
	} else {
		capsule = append(capsule, ip.To16()...)
	}
	
	// Prefix Length
	capsule = append(capsule, prefixLen)
	
	// Update length
	length := len(capsule) - lengthPos - 4
	binary.BigEndian.PutUint32(capsule[lengthPos:], uint32(length))
	
	// Send capsule (would write to request stream)
	return nil
}

// GetAssignedAddresses returns currently assigned addresses
func (c *CONNECTIPClient) GetAssignedAddresses() []IPAddress {
	c.addressesMu.RLock()
	defer c.addressesMu.RUnlock()
	
	result := make([]IPAddress, len(c.addresses))
	copy(result, c.addresses)
	return result
}

// GetRoutes returns currently advertised routes
func (c *CONNECTIPClient) GetRoutes() []RouteAdvertisement {
	c.routesMu.RLock()
	defer c.routesMu.RUnlock()
	
	result := make([]RouteAdvertisement, len(c.routes))
	copy(result, c.routes)
	return result
}

// IPProxyPacketConn wraps HTTP/3 response body for IP packets
type IPProxyPacketConn struct {
	body   interface {
		Read([]byte) (int, error)
		Close() error
	}
	client *CONNECTIPClient
	logger *zap.Logger
}

func (p *IPProxyPacketConn) ReadFrom(b []byte) (int, net.Addr, error) {
	// Read IP packet from capsule
	// In a full implementation, this would parse the capsule protocol
	n, err := p.body.Read(b)
	if err != nil {
		return 0, nil, err
	}
	
	// Return with a dummy address
	addr, _ := net.ResolveIPAddr("ip", "0.0.0.0")
	return n, addr, nil
}

func (p *IPProxyPacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	// Write IP packet as capsule
	// Would write to request stream
	return 0, fmt.Errorf("WriteTo not implemented")
}

func (p *IPProxyPacketConn) Close() error {
	return p.body.Close()
}

func (p *IPProxyPacketConn) LocalAddr() net.Addr {
	return &net.IPAddr{IP: net.IPv4(127, 0, 0, 1)}
}

func (p *IPProxyPacketConn) SetDeadline(t time.Time) error { return nil }
func (p *IPProxyPacketConn) SetReadDeadline(t time.Time) error { return nil }
func (p *IPProxyPacketConn) SetWriteDeadline(t time.Time) error { return nil }

// Capsule encoding helpers
func encodeVarint(x uint64) []byte {
	if x < 64 {
		return []byte{byte(x)}
	}
	if x < 16384 {
		return []byte{0x40 | byte(x>>8), byte(x)}
	}
	if x < 1073741824 {
		return []byte{0x80 | byte(x>>24), byte(x>>16), byte(x>>8), byte(x)}
	}
	return []byte{0xC0 | byte(x>>56), byte(x>>48), byte(x>>40), byte(x>>32),
		byte(x>>24), byte(x>>16), byte(x>>8), byte(x)}
}