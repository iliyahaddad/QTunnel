package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadClientConfig_EnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := filepath.ToSlash(filepath.Join(tmpDir, "server.crt"))
	keyFile := filepath.ToSlash(filepath.Join(tmpDir, "server.key"))
	
	// Generate self-signed cert for testing
	certDER, keyDER, err := generateTestCert()
	if err != nil {
		t.Fatalf("Generate cert: %v", err)
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

	// Set env override
	os.Setenv("QTTUNNEL_SERVER_ADDR", "override.example.com:5555")
	defer os.Unsetenv("QTTUNNEL_SERVER_ADDR")

	cfg, err := LoadClientConfig(configFile)
	if err != nil {
		t.Fatalf("LoadClientConfig failed: %v", err)
	}

	if cfg.Server.Addr != "override.example.com:5555" {
		t.Errorf("Server.Addr = %s, want override.example.com:5555", cfg.Server.Addr)
	}
}

func TestLoadClientConfig_EnvTokenOverride(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := filepath.ToSlash(filepath.Join(tmpDir, "server.crt"))
	keyFile := filepath.ToSlash(filepath.Join(tmpDir, "server.key"))
	
	// Generate test certificates
	certDER, keyDER, err := generateTestCert()
	if err != nil {
		t.Fatalf("Generate cert: %v", err)
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

	os.Setenv("QTTUNNEL_AUTH_TOKEN", "b3ZlcnJpZGUtZGVmYXVsdC10b2tlbg==")
	defer os.Unsetenv("QTTUNNEL_AUTH_TOKEN")

	cfg, err := LoadClientConfig(configFile)
	if err != nil {
		t.Fatalf("LoadClientConfig failed: %v", err)
	}

	if len(cfg.Auth.Tokens) != 1 {
		t.Errorf("Auth.Tokens length = %d, want 1", len(cfg.Auth.Tokens))
	}
	if cfg.Auth.Tokens[0] != "b3ZlcnJpZGUtZGVmYXVsdC10b2tlbg==" {
		t.Errorf("Auth.Tokens[0] = %s, want overridden token", cfg.Auth.Tokens[0])
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

func TestParseDuration_EdgeCases(t *testing.T) {
	tests := []struct {
		input     string
		defaultVal time.Duration
		expected  time.Duration
	}{
		{"10s", 0, 10 * time.Second},
		{"1m30s", 0, 90 * time.Second},
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

func TestParseSize_EdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"1KB", 1024},
		{"1MB", 1024 * 1024},
		{"1GB", 1024 * 1024 * 1024},
		{"512", 512},
		{"1kb", 1024},      // lowercase
		{"1mb", 1024*1024}, // lowercase
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

func TestIsValidHostname(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected bool
	}{
		{"valid domain", "example.com", true},
		{"valid subdomain", "api.example.com", true},
		{"wildcard", "*", true},
		{"empty", "", false},
		{"too long", string(make([]byte, 254)), false},
		{"invalid char", "exa_mple.com", false},
		{"starts with dash", "-example.com", false},
		{"ends with dash", "example-.com", false},
		{"label too long", "a" + string(make([]byte, 63)) + ".com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidHostname(tt.host)
			if result != tt.expected {
				t.Errorf("isValidHostname(%q) = %v, want %v", tt.host, result, tt.expected)
			}
		})
	}
}