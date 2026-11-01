package stress

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
	"sync/atomic"
	"testing"
	"time"

	"github.com/qtunnel/qtunnel/internal/auth"
	"github.com/qtunnel/qtunnel/internal/client"
	"github.com/qtunnel/qtunnel/internal/config"
	"github.com/qtunnel/qtunnel/internal/observability"
	"github.com/qtunnel/qtunnel/internal/proxy"
	"github.com/qtunnel/qtunnel/internal/server"
	"github.com/qtunnel/qtunnel/internal/session"
	"go.uber.org/zap"
)

// StressTestConfig holds stress test configuration
type StressTestConfig struct {
	NumClients        int
	ConnectionsPerClient int
	Duration          time.Duration
	MessageSize       int
	ConcurrentStreams int
}

// StressTestResult holds stress test results
type StressTestResult struct {
	TotalConnections    int64
	FailedConnections   int64
	TotalStreams        int64
	FailedStreams       int64
	TotalBytesSent      int64
	TotalBytesReceived  int64
	Errors              []error
	Duration            time.Duration
}

// StressTestRunner runs stress tests
type StressTestRunner struct {
	cfg    *StressTestConfig
	logger *zap.Logger
	result *StressTestResult
	
	ts     *testServer
	tc     []*testClient
}

// testServer wraps server for testing
type testServer struct {
	srv  *server.Server
	addr string
}

// testClient wraps client for testing
type testClient struct {
	cl    *client.Client
	sess  *session.Session
	proxy *proxy.TCPProxyServer
}

func NewStressTestRunner(cfg *StressTestConfig, logger *zap.Logger) *StressTestRunner {
	return &StressTestRunner{
		cfg:    cfg,
		logger: logger,
		result: &StressTestResult{},
		tc:     make([]*testClient, 0, cfg.NumClients),
	}
}

func (r *StressTestRunner) setup(t *testing.T) error {
	// Generate test certificates
	certDER, keyDER, err := generateSelfSignedCert()
	if err != nil {
		return err
	}
	
	certFile := t.TempDir() + "/server.crt"
	keyFile := t.TempDir() + "/server.key"
	
	if err := os.WriteFile(certFile, certDER, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(keyFile, keyDER, 0644); err != nil {
		return err
	}
	
	// Generate token
	token, _ := auth.GenerateToken()
	
	// Start server
	metrics, _ := observability.NewMetrics(&observability.MetricsConfig{Enabled: false})
	
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
			Type: config.TransportQUIC,
			QUIC: &config.QUICConfig{
				MaxIdleTimeout:        "30s",
				KeepAlive:             "10s",
				HandshakeTimeout:      "10s",
				Enable0RTT:            false,
				CongestionControl:     "cubic",
				InitialCongestionWindow: 10,
				MaxReceiveBufferSize:  "10MB",
				MaxIncomingStreams:    1000,
				MaxIncomingUniStreams: 1000,
			},
		},
		Proxy: &config.ProxyConfig{
			Mode:      config.ProxyModeTCP,
			Allowlist: []string{"*"},
			Denylist:  []string{},
		},
		Limits: &config.LimitsConfig{
			MaxConnections:    10000,
			MaxStreamsPerConn: 1000,
			StreamBufferSize:  "1MB",
			ConnBufferSize:    "10MB",
			MaxFrameSize:      "16KB",
		},
		Logging: &config.LoggingConfig{
			Level:  "warn",
			Format: "console",
			Output: "stderr",
		},
	}
	
	srv := server.New(srvCfg, r.logger, metrics)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start server in background
	go func() {
		if err := srv.Run(ctx); err != nil && ctx.Err() == nil {
			r.logger.Error("Server error", zap.Error(err))
		}
	}()
	
	// Wait for server to start
	time.Sleep(200 * time.Millisecond)
	
	// Get actual server address
	serverAddr := "127.0.0.1:4433"
	
	r.ts = &testServer{srv: srv, addr: serverAddr}
	
	// Start clients
	for i := 0; i < r.cfg.NumClients; i++ {
		clientCfg := &config.Config{
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
					MaxIdleTimeout:        "30s",
					KeepAlive:             "10s",
					HandshakeTimeout:      "10s",
					Enable0RTT:            false,
					CongestionControl:     "cubic",
					InitialCongestionWindow: 10,
					MaxReceiveBufferSize:  "10MB",
					MaxIncomingStreams:    1000,
					MaxIncomingUniStreams: 1000,
				},
			},
			Proxy: &config.ProxyConfig{
				Mode:  config.ProxyModeTCP,
				Listen: "127.0.0.1:0",
			},
			Limits: &config.LimitsConfig{
				MaxConnections:    1000,
				MaxStreamsPerConn: 1000,
				StreamBufferSize:  "1MB",
				ConnBufferSize:    "10MB",
				MaxFrameSize:      "16KB",
			},
			Logging: &config.LoggingConfig{
				Level:  "warn",
				Format: "console",
				Output: "stderr",
			},
		}
		
		cl := client.New(clientCfg, r.logger, metrics)
		r.tc = append(r.tc, &testClient{cl: cl})
		
		go func() {
			cl.Run(ctx)
		}()
	}
	
	// Wait for connections to establish
	time.Sleep(2 * time.Second)
	
	return nil
}

func (r *StressTestRunner) Run(t *testing.T) *StressTestResult {
	if err := r.setup(t); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), r.cfg.Duration)
	defer cancel()
	
	var wg sync.WaitGroup
	startTime := time.Now()
	
	// Run concurrent connections
	for i := 0; i < r.cfg.ConnectionsPerClient; i++ {
		wg.Add(1)
		go func(clientIdx int) {
			defer wg.Done()
			r.runConnectionCycle(ctx, clientIdx)
		}(i)
	}
	
	// Wait for all connections or timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		r.result.Duration = time.Since(startTime)
	case <-ctx.Done():
		r.result.Duration = time.Since(startTime)
	}
	
	return r.result
}

func (r *StressTestRunner) runConnectionCycle(ctx context.Context, clientIdx int) {
	tc := r.tc[clientIdx%len(r.tc)]
	
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		
		atomic.AddInt64(&r.result.TotalConnections, 1)
		
		// Get session
		sess := tc.cl.GetSession()
		if sess == nil || sess.State() != session.StateActive {
			atomic.AddInt64(&r.result.FailedConnections, 1)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		
		// Open multiple streams
		for s := 0; s < r.cfg.ConcurrentStreams; s++ {
			atomic.AddInt64(&r.result.TotalStreams, 1)
			
			stream, err := sess.OpenStream(ctx)
			if err != nil {
				atomic.AddInt64(&r.result.FailedStreams, 1)
				continue
			}
			
			// Send data
			data := make([]byte, r.cfg.MessageSize)
			rand.Read(data)
			
			if _, err := stream.Write(data); err != nil {
				atomic.AddInt64(&r.result.FailedStreams, 1)
				stream.Close()
				continue
			}
			atomic.AddInt64(&r.result.TotalBytesSent, int64(len(data)))
			
			// Read echo
			buf := make([]byte, len(data))
			if _, err := io.ReadFull(stream, buf); err != nil {
				atomic.AddInt64(&r.result.FailedStreams, 1)
				stream.Close()
				continue
			}
			atomic.AddInt64(&r.result.TotalBytesReceived, int64(len(buf)))
			
			stream.Close()
		}
		
		// Small delay between cycles
		time.Sleep(10 * time.Millisecond)
	}
}

func (r *StressTestRunner) Stop() {
	if r.ts != nil {
		r.ts.srv.Shutdown(context.Background())
	}
	for _, tc := range r.tc {
		tc.cl.Shutdown(context.Background())
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