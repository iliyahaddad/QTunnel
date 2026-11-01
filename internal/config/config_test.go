package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadClientConfig_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := filepath.ToSlash(filepath.Join(tmpDir, "server.crt"))
	keyFile := filepath.ToSlash(filepath.Join(tmpDir, "server.key"))
	
	// Generate self-signed cert for testing
	certDER, keyDER, err := generateSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate cert: %v", err)
	}
	
	if err := os.WriteFile(certFile, certDER, 0644); err != nil {
		t.Fatalf("Write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyDER, 0644); err != nil {
		t.Fatalf("Write key: %v", err)
	}
	
	configContent := `
role: "client"
server:
  addr: "example.com:4433"
  sni: "example.com"
auth:
  token: "dGVzdC10b2tlbi10aGF0LWlzLTMtMTEteHh4eHh4eHg="
transport:
  type: "quic"
proxy:
  mode: "tcp"
  listen: "127.0.0.1:8080"
`
	configFile := filepath.Join(tmpDir, "client.yaml")
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Write config: %v", err)
	}
	
	cfg, err := LoadClientConfig(configFile)
	if err != nil {
		t.Fatalf("LoadClientConfig failed: %v", err)
	}
	
	if cfg.Role != RoleClient {
		t.Errorf("Role = %s, want %s", cfg.Role, RoleClient)
	}
	if cfg.Server.Addr != "example.com:4433" {
		t.Errorf("Server.Addr = %s, want example.com:4433", cfg.Server.Addr)
	}
	if len(cfg.Auth.Tokens) != 1 {
		t.Errorf("Auth.Tokens length = %d, want 1", len(cfg.Auth.Tokens))
	}
}

func TestLoadClientConfig_MissingToken(t *testing.T) {
	tmpDir := t.TempDir()
	
	configContent := `
role: "client"
server:
  addr: "example.com:4433"
  sni: "example.com"
transport:
  type: "quic"
proxy:
  mode: "tcp"
  listen: "127.0.0.1:8080"
`
	configFile := filepath.Join(tmpDir, "client.yaml")
	os.WriteFile(configFile, []byte(configContent), 0644)
	
	_, err := LoadClientConfig(configFile)
	if err == nil {
		t.Fatal("Expected error for missing token")
	}
}

func TestLoadServerConfig_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := filepath.ToSlash(filepath.Join(tmpDir, "server.crt"))
	keyFile := filepath.ToSlash(filepath.Join(tmpDir, "server.key"))
	
	// Generate self-signed cert for testing
	certDER, keyDER, err := generateSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate cert: %v", err)
	}
	
	if err := os.WriteFile(certFile, certDER, 0644); err != nil {
		t.Fatalf("Write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyDER, 0644); err != nil {
		t.Fatalf("Write key: %v", err)
	}
	
	configContent := fmt.Sprintf(`
role: "server"
listen:
  addr: ":4433"
tls:
  cert_file: "%s"
  key_file: "%s"
auth:
  tokens:
    - "dGVzdC10b2tlbi10aGF0LWlzLTMtMTEteHh4eHh4eHg="
proxy:
  mode: "tcp"
  allowlist: ["*"]
  denylist: []
`, certFile, keyFile)
	
	configFile := filepath.Join(tmpDir, "server.yaml")
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Write config: %v", err)
	}
	
	cfg, err := LoadServerConfig(configFile)
	if err != nil {
		t.Fatalf("LoadServerConfig failed: %v", err)
	}
	
	if cfg.Role != RoleServer {
		t.Errorf("Role = %s, want %s", cfg.Role, RoleServer)
	}
	if cfg.Listen.Addr != ":4433" {
		t.Errorf("Listen.Addr = %s, want :4433", cfg.Listen.Addr)
	}
}

func TestValidateTokenFormat(t *testing.T) {
	validToken := "dGVzdC10b2tlbi10aGF0LWlzLTMtMTEteHh4eHh4eHg="
	if err := ValidateTokenFormat(validToken); err != nil {
		t.Errorf("ValidateTokenFormat failed for valid token: %v", err)
	}
	
	invalidToken := "not-base64!"
	if err := ValidateTokenFormat(invalidToken); err == nil {
		t.Error("ValidateTokenFormat should fail for invalid token")
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"1KB", 1024},
		{"1MB", 1024 * 1024},
		{"1GB", 1024 * 1024 * 1024},
		{"512", 512},
		{"", 0},
	}
	
	for _, tt := range tests {
		result, err := ParseSize(tt.input, 0)
		if err != nil {
			t.Errorf("ParseSize(%q) error: %v", tt.input, err)
			continue
		}
		if result != tt.expected {
			t.Errorf("ParseSize(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		defaultVal time.Duration
		expected time.Duration
	}{
		{"10s", 0, 10 * time.Second},
		{"5m", 0, 5 * time.Minute},
		{"1h", 0, 1 * time.Hour},
		{"", 30 * time.Second, 30 * time.Second},
		{"invalid", 10 * time.Second, 10 * time.Second},
	}
	
	for _, tt := range tests {
		result, err := ParseDuration(tt.input, tt.defaultVal)
		if err != nil {
			t.Errorf("ParseDuration(%q) error: %v", tt.input, err)
			continue
		}
		if result != tt.expected {
			t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func generateSelfSignedCert() ([]byte, []byte, error) {
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