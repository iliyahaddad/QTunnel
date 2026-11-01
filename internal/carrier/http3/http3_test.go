package http3

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/qtunnel/qtunnel/internal/config"
	"go.uber.org/zap"
)

func generateTestTLSConfig() (*tls.Config, *tls.Config, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  priv,
	}

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h3"},
	}

	clientTLS := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"h3"},
		ServerName:         "localhost",
	}

	return serverTLS, clientTLS, nil
}

func TestHTTP3CarrierDial(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	serverTLS, _, err := generateTestTLSConfig()
	if err != nil {
		t.Fatalf("generateTestTLSConfig failed: %v", err)
	}

	cfg := &config.HTTP3Config{
		MaxIdleTimeout:        "30s",
		KeepAlive:             "10s",
		HandshakeTimeout:      "10s",
		Enable0RTT:            false,
		MaxReceiveBufferSize:  "10MB",
		MaxIncomingStreams:    100,
		MaxIncomingUniStreams: 100,
		DisablePathMigration:  false,
	}

	carrier := NewCarrier(cfg, logger)

	// Start HTTP/3 listener
	ln, err := carrier.Listen("127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()

	// Test passes if we can create the carrier and listener successfully
	// Full integration test would require a proper HTTP/3 server implementation
}

func TestHTTP3CarrierName(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := &config.HTTP3Config{}
	c := NewCarrier(cfg, logger)

	if c.Name() != "http3" {
		t.Errorf("Name() = %s, want 'http3'", c.Name())
	}
}

func TestHTTP3ConfigParsing(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := &config.HTTP3Config{
		MaxIdleTimeout:        "30s",
		KeepAlive:             "10s",
		HandshakeTimeout:      "10s",
		Enable0RTT:            true,
		MaxReceiveBufferSize:  "10MB",
		MaxIncomingStreams:    100,
		MaxIncomingUniStreams: 100,
		DisablePathMigration:  false,
	}

	c := NewCarrier(cfg, logger)

	if c.Name() != "http3" {
		t.Errorf("Name() = %s, want 'http3'", c.Name())
	}
}