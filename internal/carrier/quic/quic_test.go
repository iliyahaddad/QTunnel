package quic

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
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
		NextProtos:   []string{"qtunnel/1"},
	}

	clientTLS := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"qtunnel/1"},
		ServerName:         "localhost",
	}

	return serverTLS, clientTLS, nil
}

func TestQUICDialer(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	serverTLS, clientTLS, err := generateTestTLSConfig()
	if err != nil {
		t.Fatalf("generateTestTLSConfig failed: %v", err)
	}

	quicConfig := &quic.Config{
		MaxIdleTimeout:            30 * time.Second,
		KeepAlivePeriod:           10 * time.Second,
		HandshakeIdleTimeout:      10 * time.Second,
		MaxIncomingStreams:        100,
		MaxIncomingUniStreams:     100,
	}

	dialer := &Dialer{
		quicConfig: quicConfig,
		logger:     logger,
	}
	dialer.SetTLSConfig(clientTLS)

	// Start listener
	listener, err := quic.ListenAddr("127.0.0.1:0", serverTLS, quicConfig)
	if err != nil {
		t.Fatalf("ListenAddr failed: %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()

	// Accept connection in background
	var serverConn quic.Connection
	acceptDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept(context.Background())
		if err != nil {
			acceptDone <- err
			return
		}
		serverConn = conn
		acceptDone <- nil
	}()

	// Dial from client
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientConn, err := dialer.Dial(ctx, serverAddr, "localhost", true)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer clientConn.Close()

	// Wait for server accept
	if err := <-acceptDone; err != nil {
		t.Fatalf("Accept failed: %v", err)
	}
	defer serverConn.CloseWithError(0, "")

	// Test stream
	stream, err := clientConn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatalf("OpenStreamSync failed: %v", err)
	}
	defer stream.Close()

	// Accept on server
	serverStream, err := serverConn.AcceptStream(ctx)
	if err != nil {
		t.Fatalf("AcceptStream failed: %v", err)
	}
	defer serverStream.Close()

	// Test bidirectional communication
	testData := []byte("hello quic")
	if _, err := stream.Write(testData); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	buf := make([]byte, len(testData))
	n, err := serverStream.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Read %d bytes, want %d", n, len(testData))
	}
	for i := range testData {
		if buf[i] != testData[i] {
			t.Errorf("Data mismatch at %d", i)
		}
	}

	// Echo back
	if _, err := serverStream.Write(testData); err != nil {
		t.Fatalf("Server write failed: %v", err)
	}

	n, err = stream.Read(buf)
	if err != nil {
		t.Fatalf("Client read failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Read %d bytes, want %d", n, len(testData))
	}
	for i := range testData {
		if buf[i] != testData[i] {
			t.Errorf("Data mismatch at %d", i)
		}
	}
}

func TestQUICMultipleStreams(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	serverTLS, clientTLS, err := generateTestTLSConfig()
	if err != nil {
		t.Fatalf("generateTestTLSConfig failed: %v", err)
	}

	quicConfig := &quic.Config{
		MaxIdleTimeout:            30 * time.Second,
		KeepAlivePeriod:           10 * time.Second,
		HandshakeIdleTimeout:      10 * time.Second,
		MaxIncomingStreams:        100,
		MaxIncomingUniStreams:     100,
	}

	dialer := &Dialer{
		quicConfig: quicConfig,
		logger:     logger,
	}
	dialer.SetTLSConfig(clientTLS)

	listener, err := quic.ListenAddr("127.0.0.1:0", serverTLS, quicConfig)
	if err != nil {
		t.Fatalf("ListenAddr failed: %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()

	var serverConn quic.Connection
	acceptDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept(context.Background())
		if err != nil {
			acceptDone <- err
			return
		}
		serverConn = conn
		acceptDone <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientConn, err := dialer.Dial(ctx, serverAddr, "localhost", true)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer clientConn.Close()

	if err := <-acceptDone; err != nil {
		t.Fatalf("Accept failed: %v", err)
	}
	defer serverConn.CloseWithError(0, "")

	// Open multiple streams
	const numStreams = 10
	streams := make([]quic.Stream, numStreams)

	for i := 0; i < numStreams; i++ {
		stream, err := clientConn.OpenStreamSync(ctx)
		if err != nil {
			t.Fatalf("OpenStreamSync %d failed: %v", i, err)
		}
		streams[i] = stream
	}

	// Accept all streams on server
	for i := 0; i < numStreams; i++ {
		stream, err := serverConn.AcceptStream(ctx)
		if err != nil {
			t.Fatalf("AcceptStream %d failed: %v", i, err)
		}

		// Echo data
		go func(s quic.Stream) {
			buf := make([]byte, 1024)
			for {
				n, err := s.Read(buf)
				if err != nil {
					return
				}
				if _, err := s.Write(buf[:n]); err != nil {
					return
				}
			}
		}(stream)
	}

	// Send data on all streams
	for i, stream := range streams {
		data := []byte("stream data")
		if _, err := stream.Write(data); err != nil {
			t.Fatalf("Write stream %d failed: %v", i, err)
		}

		buf := make([]byte, len(data))
		n, err := stream.Read(buf)
		if err != nil {
			t.Fatalf("Read stream %d failed: %v", i, err)
		}
		if n != len(data) {
			t.Errorf("Stream %d: read %d bytes, want %d", i, n, len(data))
		}
		stream.Close()
	}
}

func TestQUICConnectionMigration(t *testing.T) {
	// This test would require multiple network interfaces
	// Skipping for now as it requires specific network setup
	t.Skip("Requires multi-homed host")
}