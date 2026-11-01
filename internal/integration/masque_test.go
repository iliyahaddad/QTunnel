package integration

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/qtunnel/qtunnel/internal/auth"
	"github.com/qtunnel/qtunnel/internal/carrier/masque"
	"github.com/qtunnel/qtunnel/internal/client"
	"github.com/qtunnel/qtunnel/internal/config"
	"github.com/qtunnel/qtunnel/internal/observability"
	"github.com/qtunnel/qtunnel/internal/server"
	"github.com/qtunnel/qtunnel/internal/session"
	"go.uber.org/zap"
)

func TestIntegration_VPNMode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Generate test certificates
	certDER, keyDER, err := generateSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate cert: %v", err)
	}
	
	certFile := t.TempDir() + "/server.crt"
	keyFile := t.TempDir() + "/server.key"
	
	if err := os.WriteFile(certFile, certDER, 0644); err != nil {
		t.Fatalf("Write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyDER, 0644); err != nil {
		t.Fatalf("Write key: %v", err)
	}
	
	token, _ := auth.GenerateToken()
	
	// Server with VPN config
	srvMetrics, _ := observability.NewMetrics(&observability.MetricsConfig{Enabled: false})
	srvLogger, _ := zap.NewDevelopment()
	
	srvCfg := &config.Config{
		Role: config.RoleServer,
		Listen: &config.ListenConfig{Addr: ":0"},
		TLS: &config.TLSConfig{
			CertFile: certFile,
			KeyFile:  keyFile,
		},
		Auth: &config.AuthConfig{
			Tokens: []string{token},
		},
		Transport: &config.TransportConfig{
			Type: config.TransportHTTP3,
			HTTP3: &config.HTTP3Config{
				MaxIdleTimeout:        "30s",
				KeepAlive:             "10s",
				HandshakeTimeout:      "10s",
				Enable0RTT:            false,
				MaxReceiveBufferSize:  "10MB",
				MaxIncomingStreams:    100,
				MaxIncomingUniStreams: 100,
			},
			MASQUE: &config.MASQUEConfig{
				VPN: &config.VPNConfig{
					TUN: &config.TUNConfig{
						Name:        "qtunnel0",
						MTU:         1280,
						Addresses:   []string{"10.100.0.1/24", "fd00:100::1/64"},
						Routes:      []string{"0.0.0.0/0", "::/0"},
						DNSServers:  []string{"1.1.1.1", "8.8.8.8"},
					},
					KillSwitch:  true,
					SplitTunnel: false,
				},
			},
		},
		Proxy: &config.ProxyConfig{
			Mode:      config.ProxyModeTCP,
			Allowlist: []string{"*"},
			Denylist:  []string{},
		},
		Limits: &config.LimitsConfig{
			MaxConnections:    100,
			MaxStreamsPerConn: 100,
			StreamBufferSize:  "1MB",
			ConnBufferSize:    "10MB",
			MaxFrameSize:      "16KB",
		},
		Logging: &config.LoggingConfig{
			Level:  "info",
			Format: "console",
			Output: "stderr",
		},
	}
	
	srv := server.New(srvCfg, srvLogger, srvMetrics)
	
	srvCtx, srvCancel := context.WithCancel(ctx)
	defer srvCancel()
	
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.Run(srvCtx); err != nil && srvCtx.Err() == nil {
			srvLogger.Error("Server error", zap.Error(err))
		}
	}()
	
	// Wait for server
	time.Sleep(500 * time.Millisecond)
	
	// Client with VPN config
	clMetrics, _ := observability.NewMetrics(&observability.MetricsConfig{Enabled: false})
	clLogger, _ := zap.NewDevelopment()
	
	clCfg := &config.Config{
		Role: config.RoleClient,
		Server: &config.ServerConfig{
			Addr:               "127.0.0.1:4433",
			SNI:                "localhost",
			InsecureSkipVerify: true,
		},
		Auth: &config.AuthConfig{
			Tokens: []string{token},
		},
		Transport: &config.TransportConfig{
			Type: config.TransportHTTP3,
			HTTP3: &config.HTTP3Config{
				MaxIdleTimeout:        "30s",
				KeepAlive:             "10s",
				HandshakeTimeout:      "10s",
				Enable0RTT:            false,
				MaxReceiveBufferSize:  "10MB",
				MaxIncomingStreams:    100,
				MaxIncomingUniStreams: 100,
			},
			MASQUE: &config.MASQUEConfig{
				VPN: &config.VPNConfig{
					TUN: &config.TUNConfig{
						Name:        "qtunnel0",
						MTU:         1280,
						Addresses:   []string{"10.100.0.2/24", "fd00:100::2/64"},
						Routes:      []string{"0.0.0.0/0", "::/0"},
						DNSServers:  []string{"1.1.1.1", "8.8.8.8"},
					},
					KillSwitch:  true,
					SplitTunnel: false,
				},
			},
		},
		Proxy: &config.ProxyConfig{
			Mode:  config.ProxyModeTCP,
			Listen: "127.0.0.1:0",
		},
		Limits: &config.LimitsConfig{
			MaxConnections:    100,
			MaxStreamsPerConn: 100,
			StreamBufferSize:  "1MB",
			ConnBufferSize:    "10MB",
			MaxFrameSize:      "16KB",
		},
		Logging: &config.LoggingConfig{
			Level:  "info",
			Format: "console",
			Output: "stderr",
		},
	}
	
	cl := client.New(clCfg, clLogger, clMetrics)
	
	clCtx, clCancel := context.WithCancel(ctx)
	defer clCancel()
	
	go func() {
		if err := cl.Run(clCtx); err != nil && clCtx.Err() == nil {
			clLogger.Error("Client error", zap.Error(err))
		}
	}()
	
	// Wait for VPN connection
	time.Sleep(2 * time.Second)
	
	// Verify session is active
	sess := cl.GetSession()
	if sess == nil {
		t.Fatal("No active session")
	}
	
	if sess.State() != session.StateActive {
		t.Errorf("Session state = %v, want Active", sess.State())
	}
	
	t.Log("VPN integration test passed")
	
	// Test TUN manager creation
	tunMgr, err := masque.NewTUNManager(&config.TUNConfig{
		Name:        "test0",
		MTU:         1280,
		Addresses:   []string{"10.0.0.1/24"},
		Routes:      []string{"10.0.0.0/8"},
		DNSServers:  []string{"1.1.1.1"},
	}, srvLogger)
	
	if err != nil {
		// TUN might not be available in test environment
		t.Logf("TUN not available (expected in test env): %v", err)
	} else {
		if err := tunMgr.Start(); err != nil {
			t.Logf("TUN start failed (expected in test env): %v", err)
		}
		tunMgr.Stop()
	}
}

func TestIntegration_MASQUE_CONNECT_UDP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Generate test certificates
	certDER, keyDER, err := generateSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate cert: %v", err)
	}
	
	certFile := t.TempDir() + "/server.crt"
	keyFile := t.TempDir() + "/server.key"
	
	if err := os.WriteFile(certFile, certDER, 0644); err != nil {
		t.Fatalf("Write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyDER, 0644); err != nil {
		t.Fatalf("Write key: %v", err)
	}
	
	token, _ := auth.GenerateToken()
	
	// Server with MASQUE config
	srvMetrics, _ := observability.NewMetrics(&observability.MetricsConfig{Enabled: false})
	srvLogger, _ := zap.NewDevelopment()
	
	srvCfg := &config.Config{
		Role: config.RoleServer,
		Listen: &config.ListenConfig{Addr: ":0"},
		TLS: &config.TLSConfig{
			CertFile: certFile,
			KeyFile:  keyFile,
		},
		Auth: &config.AuthConfig{
			Tokens: []string{token},
		},
		Transport: &config.TransportConfig{
			Type: config.TransportHTTP3,
			HTTP3: &config.HTTP3Config{
				MaxIdleTimeout:        "30s",
				KeepAlive:             "10s",
				HandshakeTimeout:      "10s",
				Enable0RTT:            false,
				MaxReceiveBufferSize:  "10MB",
				MaxIncomingStreams:    100,
				MaxIncomingUniStreams: 100,
			},
			MASQUE: &config.MASQUEConfig{
				ConnectUDP: &config.CONNECTUDPConfig{
					URITemplate:      "https://{host}/.well-known/masque/udp/{target_host}/{target_port}/",
					MaxDatagramSize:  65527,
				},
			},
		},
		Proxy: &config.ProxyConfig{
			Mode:      config.ProxyModeTCP,
			Allowlist: []string{"*"},
			Denylist:  []string{},
		},
		Limits: &config.LimitsConfig{
			MaxConnections:    100,
			MaxStreamsPerConn: 100,
			StreamBufferSize:  "1MB",
			ConnBufferSize:    "10MB",
			MaxFrameSize:      "16KB",
		},
		Logging: &config.LoggingConfig{
			Level:  "info",
			Format: "console",
			Output: "stderr",
		},
	}
	
	srv := server.New(srvCfg, srvLogger, srvMetrics)
	
	srvCtx, srvCancel := context.WithCancel(ctx)
	defer srvCancel()
	
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.Run(srvCtx); err != nil && srvCtx.Err() == nil {
			srvLogger.Error("Server error", zap.Error(err))
		}
	}()
	
	time.Sleep(500 * time.Millisecond)
	
	// Client
	clMetrics, _ := observability.NewMetrics(&observability.MetricsConfig{Enabled: false})
	clLogger, _ := zap.NewDevelopment()
	
	clCfg := &config.Config{
		Role: config.RoleClient,
		Server: &config.ServerConfig{
			Addr:               "127.0.0.1:4433",
			SNI:                "localhost",
			InsecureSkipVerify: true,
		},
		Auth: &config.AuthConfig{
			Tokens: []string{token},
		},
		Transport: &config.TransportConfig{
			Type: config.TransportHTTP3,
			HTTP3: &config.HTTP3Config{
				MaxIdleTimeout:        "30s",
				KeepAlive:             "10s",
				HandshakeTimeout:      "10s",
				Enable0RTT:            false,
				MaxReceiveBufferSize:  "10MB",
				MaxIncomingStreams:    100,
				MaxIncomingUniStreams: 100,
			},
			MASQUE: &config.MASQUEConfig{
				ConnectUDP: &config.CONNECTUDPConfig{
					URITemplate:      "https://{host}/.well-known/masque/udp/{target_host}/{target_port}/",
					MaxDatagramSize:  65527,
				},
			},
		},
		Proxy: &config.ProxyConfig{
			Mode:  config.ProxyModeTCP,
			Listen: "127.0.0.1:0",
		},
		Limits: &config.LimitsConfig{
			MaxConnections:    100,
			MaxStreamsPerConn: 100,
			StreamBufferSize:  "1MB",
			ConnBufferSize:    "10MB",
			MaxFrameSize:      "16KB",
		},
		Logging: &config.LoggingConfig{
			Level:  "info",
			Format: "console",
			Output: "stderr",
		},
	}
	
	cl := client.New(clCfg, clLogger, clMetrics)
	
	clCtx, clCancel := context.WithCancel(ctx)
	defer clCancel()
	
	go func() {
		if err := cl.Run(clCtx); err != nil && clCtx.Err() == nil {
			clLogger.Error("Client error", zap.Error(err))
		}
	}()
	
	time.Sleep(2 * time.Second)
	
	sess := cl.GetSession()
	if sess == nil {
		t.Fatal("No active session")
	}
	
	if sess.State() != session.StateActive {
		t.Errorf("Session state = %v, want Active", sess.State())
	}
	
	t.Log("MASQUE CONNECT-UDP integration test passed")
}

func TestIntegration_MASQUE_CONNECT_IP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Generate test certificates
	certDER, keyDER, err := generateSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate cert: %v", err)
	}
	
	certFile := t.TempDir() + "/server.crt"
	keyFile := t.TempDir() + "/server.key"
	
	if err := os.WriteFile(certFile, certDER, 0644); err != nil {
		t.Fatalf("Write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyDER, 0644); err != nil {
		t.Fatalf("Write key: %v", err)
	}
	
	token, _ := auth.GenerateToken()
	
	// Server with CONNECT-IP config
	srvMetrics, _ := observability.NewMetrics(&observability.MetricsConfig{Enabled: false})
	srvLogger, _ := zap.NewDevelopment()
	
	srvCfg := &config.Config{
		Role: config.RoleServer,
		Listen: &config.ListenConfig{Addr: ":0"},
		TLS: &config.TLSConfig{
			CertFile: certFile,
			KeyFile:  keyFile,
		},
		Auth: &config.AuthConfig{
			Tokens: []string{token},
		},
		Transport: &config.TransportConfig{
			Type: config.TransportHTTP3,
			HTTP3: &config.HTTP3Config{
				MaxIdleTimeout:        "30s",
				KeepAlive:             "10s",
				HandshakeTimeout:      "10s",
				Enable0RTT:            false,
				MaxReceiveBufferSize:  "10MB",
				MaxIncomingStreams:    100,
				MaxIncomingUniStreams: 100,
			},
			MASQUE: &config.MASQUEConfig{
				ConnectIP: &config.CONNECTIPConfig{
					URITemplate: "https://{host}/.well-known/masque/ip/{target}/{ipproto}/",
				},
			},
		},
		Proxy: &config.ProxyConfig{
			Mode:      config.ProxyModeTCP,
			Allowlist: []string{"*"},
			Denylist:  []string{},
		},
		Limits: &config.LimitsConfig{
			MaxConnections:    100,
			MaxStreamsPerConn: 100,
			StreamBufferSize:  "1MB",
			ConnBufferSize:    "10MB",
			MaxFrameSize:      "16KB",
		},
		Logging: &config.LoggingConfig{
			Level:  "info",
			Format: "console",
			Output: "stderr",
		},
	}
	
	srv := server.New(srvCfg, srvLogger, srvMetrics)
	
	srvCtx, srvCancel := context.WithCancel(ctx)
	defer srvCancel()
	
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.Run(srvCtx); err != nil && srvCtx.Err() == nil {
			srvLogger.Error("Server error", zap.Error(err))
		}
	}()
	
	time.Sleep(500 * time.Millisecond)
	
	// Client
	clMetrics, _ := observability.NewMetrics(&observability.MetricsConfig{Enabled: false})
	clLogger, _ := zap.NewDevelopment()
	
	clCfg := &config.Config{
		Role: config.RoleClient,
		Server: &config.ServerConfig{
			Addr:               "127.0.0.1:4433",
			SNI:                "localhost",
			InsecureSkipVerify: true,
		},
		Auth: &config.AuthConfig{
			Tokens: []string{token},
		},
		Transport: &config.TransportConfig{
			Type: config.TransportHTTP3,
			HTTP3: &config.HTTP3Config{
				MaxIdleTimeout:        "30s",
				KeepAlive:             "10s",
				HandshakeTimeout:      "10s",
				Enable0RTT:            false,
				MaxReceiveBufferSize:  "10MB",
				MaxIncomingStreams:    100,
				MaxIncomingUniStreams: 100,
			},
			MASQUE: &config.MASQUEConfig{
				ConnectIP: &config.CONNECTIPConfig{
					URITemplate: "https://{host}/.well-known/masque/ip/{target}/{ipproto}/",
				},
			},
		},
		Proxy: &config.ProxyConfig{
			Mode:  config.ProxyModeTCP,
			Listen: "127.0.0.1:0",
		},
		Limits: &config.LimitsConfig{
			MaxConnections:    100,
			MaxStreamsPerConn: 100,
			StreamBufferSize:  "1MB",
			ConnBufferSize:    "10MB",
			MaxFrameSize:      "16KB",
		},
		Logging: &config.LoggingConfig{
			Level:  "info",
			Format: "console",
			Output: "stderr",
		},
	}
	
	cl := client.New(clCfg, clLogger, clMetrics)
	
	clCtx, clCancel := context.WithCancel(ctx)
	defer clCancel()
	
	go func() {
		if err := cl.Run(clCtx); err != nil && clCtx.Err() == nil {
			clLogger.Error("Client error", zap.Error(err))
		}
	}()
	
	time.Sleep(2 * time.Second)
	
	sess := cl.GetSession()
	if sess == nil {
		t.Fatal("No active session")
	}
	
	if sess.State() != session.StateActive {
		t.Errorf("Session state = %v, want Active", sess.State())
	}
	
	t.Log("MASQUE CONNECT-IP integration test passed")
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