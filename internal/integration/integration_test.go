package integration

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/qtunnel/qtunnel/internal/auth"
	"github.com/qtunnel/qtunnel/internal/client"
	"github.com/qtunnel/qtunnel/internal/config"
	"github.com/qtunnel/qtunnel/internal/observability"
	"github.com/qtunnel/qtunnel/internal/server"
	"github.com/qtunnel/qtunnel/internal/session"
	"go.uber.org/zap"
)

func generateSelfSignedCert() ([]byte, []byte, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
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

type testServer struct {
	srv      *server.Server
	token    string
	certFile string
	keyFile  string
	logger   *zap.Logger
	metrics  *observability.Metrics
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}

func newTestServer(t *testing.T) *testServer {
	logger, _ := zap.NewDevelopment()
	metrics, _ := observability.NewMetrics(&observability.MetricsConfig{Enabled: false})

	token, _ := auth.GenerateToken()

	certDER, keyDER, err := generateSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate cert: %v", err)
	}

	tempDir := t.TempDir()
	certFile := tempDir + "/server.crt"
	keyFile := tempDir + "/server.key"

	if err := os.WriteFile(certFile, certDER, 0644); err != nil {
		t.Fatalf("Write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyDER, 0644); err != nil {
		t.Fatalf("Write key: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	cfg := &config.Config{
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
			Type: config.TransportQUIC,
			QUIC: &config.QUICConfig{
				MaxIdleTimeout:           "30s",
				KeepAlive:                "10s",
				HandshakeTimeout:         "10s",
				Enable0RTT:               false,
				CongestionControl:        "cubic",
				InitialCongestionWindow:  10,
				MaxReceiveBufferSize:     "10MB",
				MaxIncomingStreams:       100,
				MaxIncomingUniStreams:    100,
			},
		},
		Proxy: &config.ProxyConfig{
			Mode:      config.ProxyModeTCP,
			Allowlist: []string{"*"},
			Denylist:  []string{},
		},
		Limits: &config.LimitsConfig{
			MaxConnections:    100,
			MaxStreamsPerConn: 10,
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

	srv := server.New(cfg, logger, metrics)

	return &testServer{
		srv:      srv,
		token:    token,
		certFile: certFile,
		keyFile:  keyFile,
		logger:   logger,
		metrics:  metrics,
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (ts *testServer) Start() (string, error) {
	ts.wg.Add(1)
	go func() {
		defer ts.wg.Done()
		if err := ts.srv.Run(ts.ctx); err != nil && ts.ctx.Err() == nil {
			ts.logger.Error("Server error", zap.Error(err))
		}
	}()

	time.Sleep(200 * time.Millisecond)
	return "127.0.0.1:4433", nil
}

func (ts *testServer) Stop() {
	ts.cancel()
	ts.wg.Wait()
}

type testClient struct {
	cl      *client.Client
	token   string
	logger  *zap.Logger
	metrics *observability.Metrics
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

func newTestClient(t *testing.T, serverAddr, token string) *testClient {
	logger, _ := zap.NewDevelopment()
	metrics, _ := observability.NewMetrics(&observability.MetricsConfig{Enabled: false})

	ctx, cancel := context.WithCancel(context.Background())

	cfg := &config.Config{
		Role: config.RoleClient,
		Server: &config.ServerConfig{
			Addr:               serverAddr,
			SNI:                "localhost",
			InsecureSkipVerify: true,
		},
		Auth: &config.AuthConfig{
			Tokens: []string{token},
		},
		Transport: &config.TransportConfig{
			Type: config.TransportQUIC,
			QUIC: &config.QUICConfig{
				MaxIdleTimeout:           "30s",
				KeepAlive:                "10s",
				HandshakeTimeout:         "10s",
				Enable0RTT:               false,
				CongestionControl:        "cubic",
				InitialCongestionWindow:  10,
				MaxReceiveBufferSize:     "10MB",
				MaxIncomingStreams:       100,
				MaxIncomingUniStreams:    100,
			},
		},
		Proxy: &config.ProxyConfig{
			Mode:  config.ProxyModeTCP,
			Listen: "127.0.0.1:0",
		},
		Limits: &config.LimitsConfig{
			MaxConnections:    100,
			MaxStreamsPerConn: 10,
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

	cl := client.New(cfg, logger, metrics)

	return &testClient{
		cl:      cl,
		token:   token,
		logger:  logger,
		metrics: metrics,
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (tc *testClient) Start() error {
	tc.wg.Add(1)
	go func() {
		defer tc.wg.Done()
		if err := tc.cl.Run(tc.ctx); err != nil && tc.ctx.Err() == nil {
			tc.logger.Error("Client error", zap.Error(err))
		}
	}()

	time.Sleep(500 * time.Millisecond)
	return nil
}

func (tc *testClient) Stop() {
	tc.cancel()
	tc.wg.Wait()
}

func TestIntegration_TCPProxy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ts := newTestServer(t)
	defer ts.Stop()

	serverAddr, err := ts.Start()
	if err != nil {
		t.Fatalf("Server start failed: %v", err)
	}
	t.Logf("Server listening on %s", serverAddr)

	tc := newTestClient(t, serverAddr, ts.token)
	defer tc.Stop()

	if err := tc.Start(); err != nil {
		t.Fatalf("Client start failed: %v", err)
	}
	t.Log("Client started")

	time.Sleep(1 * time.Second)

	sess := tc.cl.GetSession()
	if sess == nil {
		t.Fatal("No active session")
	}
	if sess.State() != session.StateActive {
		t.Errorf("Session state = %v, want Active", sess.State())
	}

	t.Log("Integration test passed")
}

func TestIntegration_ConcurrentConnections(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ts := newTestServer(t)
	defer ts.Stop()

	serverAddr, err := ts.Start()
	if err != nil {
		t.Fatalf("Server start failed: %v", err)
	}

	const numClients = 5
	clients := make([]*testClient, numClients)

	for i := 0; i < numClients; i++ {
		tc := newTestClient(t, serverAddr, ts.token)
		clients[i] = tc
		if err := tc.Start(); err != nil {
			t.Fatalf("Client %d start failed: %v", i, err)
		}
		defer tc.Stop()
	}

	time.Sleep(2 * time.Second)

	activeCount := 0
	for _, tc := range clients {
		sess := tc.cl.GetSession()
		if sess != nil && sess.State() == session.StateActive {
			activeCount++
		}
	}

	t.Logf("Active sessions: %d/%d", activeCount, numClients)
	if activeCount < numClients/2 {
		t.Errorf("Too few active sessions: %d/%d", activeCount, numClients)
	}
}

func TestIntegration_DataTransfer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ts := newTestServer(t)
	defer ts.Stop()

	serverAddr, err := ts.Start()
	if err != nil {
		t.Fatalf("Server start failed: %v", err)
	}

	tc := newTestClient(t, serverAddr, ts.token)
	defer tc.Stop()

	if err := tc.Start(); err != nil {
		t.Fatalf("Client start failed: %v", err)
	}

	time.Sleep(1 * time.Second)

	sess := tc.cl.GetSession()
	if sess == nil {
		t.Fatal("No active session")
	}

	stream, err := sess.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}
	defer stream.Close()

	testData := make([]byte, 1024*1024)
	rand.Read(testData)

	_, err = stream.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	buf := make([]byte, len(testData))
	n, err := io.ReadFull(stream, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		t.Fatalf("Read failed: %v", err)
	}

	t.Logf("Transferred %d bytes", n)
}