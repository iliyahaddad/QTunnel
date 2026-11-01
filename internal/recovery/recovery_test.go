package recovery

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"github.com/qtunnel/qtunnel/internal/config"
	"github.com/qtunnel/qtunnel/internal/observability"
	"go.uber.org/zap"
)

func TestGracefulShutdownManager(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	metrics, _ := observability.NewMetrics(&observability.MetricsConfig{Enabled: false})
	
	cfg := &config.ShutdownConfig{
		GracePeriod: "1s",
		ForceAfter:  "5s",
	}
	
	mgr := NewGracefulShutdownManager(cfg, logger, metrics)
	
	// Test initial state
	if mgr.IsDraining() {
		t.Error("Should not be draining initially")
	}
	
	// Increment connections
	mgr.IncrementConnections()
	mgr.IncrementConnections()
	mgr.IncrementStreams()
	
	if mgr.activeConns.Load() != 2 {
		t.Errorf("Expected 2 connections, got %d", mgr.activeConns.Load())
	}
	
	// Start shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Decrement after starting shutdown
	go func() {
		time.Sleep(100 * time.Millisecond)
		mgr.DecrementConnections()
		mgr.DecrementConnections()
		mgr.DecrementStreams()
	}()
	
	err := mgr.StartShutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
	
	if !mgr.IsDraining() {
		t.Error("Should be draining after shutdown")
	}
}

func TestGracefulShutdownManager_Timeout(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	metrics, _ := observability.NewMetrics(&observability.MetricsConfig{Enabled: false})
	
	cfg := &config.ShutdownConfig{
		GracePeriod: "100ms",
		ForceAfter:  "500ms",
	}
	
	mgr := NewGracefulShutdownManager(cfg, logger, metrics)
	
	mgr.IncrementConnections()
	
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	
	err := mgr.StartShutdown(ctx)
	// Should timeout since connections don't drain
	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestConfigReloader(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	tmpFile := t.TempDir() + "/test.yaml"
	
	// Write initial config
	initialConfig := `
role: "server"
listen:
  addr: ":4433"
tls:
  cert_file: "server.crt"
  key_file: "server.key"
auth:
  tokens: ["token1"]
`
	if err := os.WriteFile(tmpFile, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Write initial config: %v", err)
	}
	
	reloader := NewConfigReloader(tmpFile, logger)
	
	callbackCalled := false
	reloader.AddCallback(func(cfg *config.Config) error {
		callbackCalled = true
		if cfg.Auth.Tokens[0] != "token2" {
			t.Errorf("Expected token2, got %s", cfg.Auth.Tokens[0])
		}
		return nil
	})
	
	reloader.Start(100 * time.Millisecond)
	defer reloader.Stop()
	
	// Update config file
	updatedConfig := `
role: "server"
listen:
  addr: ":4433"
tls:
  cert_file: "server.crt"
  key_file: "server.key"
auth:
  tokens: ["token2"]
`
	if err := os.WriteFile(tmpFile, []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("Write updated config: %v", err)
	}
	
	// Wait for reload
	time.Sleep(300 * time.Millisecond)
	
	if !callbackCalled {
		t.Error("Config reload callback not called")
	}
}

func TestCertificateReloader(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	// Generate test certificates
	certDER, keyDER, err := generateTestCert()
	if err != nil {
		t.Fatalf("Generate cert: %v", err)
	}
	
	certFile := t.TempDir() + "/server.crt"
	keyFile := t.TempDir() + "/server.key"
	
	if err := os.WriteFile(certFile, certDER, 0644); err != nil {
		t.Fatalf("Write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyDER, 0644); err != nil {
		t.Fatalf("Write key: %v", err)
	}
	
	reloader := NewCertificateReloader(certFile, keyFile, logger)
	reloader.Start(100 * time.Millisecond)
	defer reloader.Stop()
	
	// Wait for initial load
	time.Sleep(100 * time.Millisecond)
	
	cert, err := reloader.GetCertificate(nil)
	if err != nil {
		t.Fatalf("GetCertificate failed: %v", err)
	}
	
	if cert == nil {
		t.Error("Certificate should not be nil")
	}
	
	if len(cert.Certificate) == 0 {
		t.Error("Certificate should have at least one cert")
	}
}

func TestSessionResumptionManager(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	mgr := NewSessionResumptionManager(logger, 10, time.Minute)
	defer mgr.Stop()
	
	streamStates := map[uint64]*StreamState{
		1: {StreamID: 1, LocalAddr: "127.0.0.1:1234", RemoteAddr: "127.0.0.1:5678", SentSeq: 100, RecvSeq: 200, WindowSize: 65536, Closed: false},
	}
	
	mgr.StoreSession("session1", []byte("ticket1"), streamStates)
	
	sess, ok := mgr.GetSession("session1")
	if !ok {
		t.Fatal("Session not found")
	}
	
	if string(sess.Ticket) != "ticket1" {
		t.Errorf("Expected ticket1, got %s", string(sess.Ticket))
	}
	
	if len(sess.StreamStates) != 1 {
		t.Errorf("Expected 1 stream state, got %d", len(sess.StreamStates))
	}
	
	mgr.DeleteSession("session1")
	
	_, ok = mgr.GetSession("session1")
	if ok {
		t.Error("Session should be deleted")
	}
}

func TestSessionResumptionManager_Eviction(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	mgr := NewSessionResumptionManager(logger, 2, time.Minute)
	defer mgr.Stop()
	
	mgr.StoreSession("s1", []byte("t1"), nil)
	mgr.StoreSession("s2", []byte("t2"), nil)
	mgr.StoreSession("s3", []byte("t3"), nil) // Should evict s1
	
	_, ok1 := mgr.GetSession("s1")
	if ok1 {
		t.Error("s1 should be evicted")
	}
	
	_, ok2 := mgr.GetSession("s2")
	if !ok2 {
		t.Error("s2 should exist")
	}
	
	_, ok3 := mgr.GetSession("s3")
	if !ok3 {
		t.Error("s3 should exist")
	}
}

func TestSessionResumptionManager_TTL(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	mgr := NewSessionResumptionManager(logger, 10, 100*time.Millisecond)
	defer mgr.Stop()
	
	mgr.StoreSession("s1", []byte("t1"), nil)
	
	time.Sleep(200 * time.Millisecond)
	
	_, ok := mgr.GetSession("s1")
	if ok {
		t.Error("Session should be expired")
	}
}

func generateTestCert() ([]byte, []byte, error) {
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

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}

	return certDER, privDER, nil
}