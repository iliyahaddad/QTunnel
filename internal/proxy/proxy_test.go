package proxy

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/qtunnel/qtunnel/internal/observability"
	"go.uber.org/zap"
)

func TestSOCKS5Handshake(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	metrics, _ := observability.NewMetrics(&observability.MetricsConfig{Enabled: false})

	server := NewSOCKS5ProxyServer("127.0.0.1:0", logger, metrics)
	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer server.Stop()

	// Connect to SOCKS5 proxy
	conn, err := net.Dial("tcp", server.listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Send SOCKS5 handshake: VER=5, NMETHODS=1, METHOD=0 (no auth)
	handshake := []byte{0x05, 0x01, 0x00}
	if _, err := conn.Write(handshake); err != nil {
		t.Fatalf("Write handshake failed: %v", err)
	}

	// Read response: VER=5, METHOD=0
	resp := make([]byte, 2)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(conn, resp); err != nil {
		t.Fatalf("Read response failed: %v", err)
	}

	if resp[0] != 0x05 || resp[1] != 0x00 {
		t.Errorf("Unexpected response: %v", resp)
	}
}

func TestSOCKS5ConnectIPv4(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	metrics, _ := observability.NewMetrics(&observability.MetricsConfig{Enabled: false})

	server := NewSOCKS5ProxyServer("127.0.0.1:0", logger, metrics)
	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer server.Stop()

	conn, err := net.Dial("tcp", server.listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Handshake
	handshake := []byte{0x05, 0x01, 0x00}
	if _, err := conn.Write(handshake); err != nil {
		t.Fatalf("Write handshake failed: %v", err)
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		t.Fatalf("Read handshake response failed: %v", err)
	}

	if resp[0] != 0x05 || resp[1] != 0x00 {
		t.Errorf("Unexpected response: %v", resp)
	}

	// CONNECT request to 1.2.3.4:80
	connectReq := []byte{
		0x05, 0x01, 0x00, 0x01, // VER=5, CMD=CONNECT, RSV=0, ATYP=IPv4
		0x01, 0x02, 0x03, 0x04, // DST.ADDR
		0x00, 0x50, // DST.PORT = 80
	}
	if _, err := conn.Write(connectReq); err != nil {
		t.Fatalf("Write connect request failed: %v", err)
	}

	// Read response (10 bytes)
	connResp := make([]byte, 10)
	if _, err := io.ReadFull(conn, connResp); err != nil {
		t.Fatalf("Read connect response failed: %v", err)
	}

	if connResp[0] != 0x05 || connResp[1] != 0x00 {
		t.Errorf("Connect failed: %v", connResp)
	}
}

func TestSOCKS5ConnectDomain(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	metrics, _ := observability.NewMetrics(&observability.MetricsConfig{Enabled: false})

	server := NewSOCKS5ProxyServer("127.0.0.1:0", logger, metrics)
	if err := server.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer server.Stop()

	conn, err := net.Dial("tcp", server.listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Handshake
	handshake := []byte{0x05, 0x01, 0x00}
	if _, err := conn.Write(handshake); err != nil {
		t.Fatalf("Write handshake failed: %v", err)
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		t.Fatalf("Read handshake response failed: %v", err)
	}

	if resp[0] != 0x05 || resp[1] != 0x00 {
		t.Errorf("Handshake failed: %v", resp)
	}

	// CONNECT request to example.com:443
	domain := "example.com"
	connectReq := make([]byte, 0, 7+len(domain))
	connectReq = append(connectReq, 0x05, 0x01, 0x00, 0x03) // VER=5, CMD=CONNECT, RSV=0, ATYP=Domain
	connectReq = append(connectReq, byte(len(domain)))
	connectReq = append(connectReq, domain...)
	connectReq = append(connectReq, 0x01, 0xbb) // Port 443

	if _, err := conn.Write(connectReq); err != nil {
		t.Fatalf("Write connect request failed: %v", err)
	}

	connResp := make([]byte, 10)
	if _, err := io.ReadFull(conn, connResp); err != nil {
		t.Fatalf("Read connect response failed: %v", err)
	}

	if connResp[0] != 0x05 || connResp[1] != 0x00 {
		t.Errorf("Connect failed: %v", connResp)
	}
}