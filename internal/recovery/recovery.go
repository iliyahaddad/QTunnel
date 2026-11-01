package recovery

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qtunnel/qtunnel/internal/config"
	"github.com/qtunnel/qtunnel/internal/observability"
	"go.uber.org/zap"
)

// GracefulShutdownManager manages graceful shutdown of server/client
type GracefulShutdownManager struct {
	cfg         *config.ShutdownConfig
	logger      *zap.Logger
	metrics     *observability.Metrics
	
	// Shutdown state
	shutdownOnce sync.Once
	shutdownCh   chan struct{}
	forceCh      chan struct{}
	
	// Active connections tracking
	activeConns atomic.Int64
	activeStreams atomic.Int64
	
	// Drain state
	draining atomic.Bool
	drainDoneCh chan struct{}
	
	// Shutdown timing
	gracePeriod time.Duration
	forceAfter  time.Duration
}

// NewGracefulShutdownManager creates a new shutdown manager
func NewGracefulShutdownManager(cfg *config.ShutdownConfig, logger *zap.Logger, metrics *observability.Metrics) *GracefulShutdownManager {
	gracePeriod := 30 * time.Second
	forceAfter := 60 * time.Second
	
	if cfg != nil {
		if d, err := time.ParseDuration(cfg.GracePeriod); err == nil {
			gracePeriod = d
		}
		if d, err := time.ParseDuration(cfg.ForceAfter); err == nil {
			forceAfter = d
		}
	}
	
	m := &GracefulShutdownManager{
		cfg:         cfg,
		logger:      logger,
		metrics:     metrics,
		gracePeriod: gracePeriod,
		forceAfter:  forceAfter,
		shutdownCh:  make(chan struct{}),
		forceCh:     make(chan struct{}),
		drainDoneCh: make(chan struct{}),
	}
	
	// Start force shutdown timer
	go func() {
		time.Sleep(forceAfter)
		select {
		case m.forceCh <- struct{}{}:
		default:
		}
	}()
	
	return m
}

// IncrementConnections increments active connection count
func (m *GracefulShutdownManager) IncrementConnections() {
	m.activeConns.Add(1)
}

// DecrementConnections decrements active connection count
func (m *GracefulShutdownManager) DecrementConnections() {
	m.activeConns.Add(-1)
}

// IncrementStreams increments active stream count
func (m *GracefulShutdownManager) IncrementStreams() {
	m.activeStreams.Add(1)
}

// DecrementStreams decrements active stream count
func (m *GracefulShutdownManager) DecrementStreams() {
	m.activeStreams.Add(-1)
}

// StartShutdown initiates graceful shutdown
func (m *GracefulShutdownManager) StartShutdown(ctx context.Context) error {
	var err error
	m.shutdownOnce.Do(func() {
		m.logger.Info("Starting graceful shutdown")
		m.draining.Store(true)
		close(m.shutdownCh)
		
		// Wait for active connections to drain
		drainCtx, cancel := context.WithTimeout(ctx, m.gracePeriod)
		defer cancel()
		
		err = m.waitForDrain(drainCtx)
		if err != nil {
			m.logger.Warn("Graceful shutdown timed out, forcing", zap.Error(err))
		}
		
		close(m.drainDoneCh)
		m.logger.Info("Graceful shutdown complete")
	})
	return err
}

// waitForDrain waits for all connections to drain
func (m *GracefulShutdownManager) waitForDrain(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			conns := m.activeConns.Load()
			streams := m.activeStreams.Load()
			if conns == 0 && streams == 0 {
				return nil
			}
			m.logger.Debug("Waiting for drain", zap.Int64("connections", conns), zap.Int64("streams", streams))
		}
	}
}

// IsDraining returns whether shutdown is in progress
func (m *GracefulShutdownManager) IsDraining() bool {
	return m.draining.Load()
}

// ShutdownCh returns channel that closes on shutdown initiation
func (m *GracefulShutdownManager) ShutdownCh() <-chan struct{} {
	return m.shutdownCh
}

// ForceCh returns channel that closes when force shutdown is triggered
func (m *GracefulShutdownManager) ForceCh() <-chan struct{} {
	return m.forceCh
}

// DrainDoneCh returns channel that closes when drain is complete
func (m *GracefulShutdownManager) DrainDoneCh() <-chan struct{} {
	return m.drainDoneCh
}

// ConfigReloader handles configuration hot-reload
type ConfigReloader struct {
	cfgPath      string
	logger       *zap.Logger
	callbacks    []func(*config.Config) error
	lastModTime  time.Time
	mu           sync.RWMutex
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

// NewConfigReloader creates a new config reloader
func NewConfigReloader(cfgPath string, logger *zap.Logger) *ConfigReloader {
	return &ConfigReloader{
		cfgPath:  cfgPath,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

// AddCallback adds a callback for config reload
func (r *ConfigReloader) AddCallback(cb func(*config.Config) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callbacks = append(r.callbacks, cb)
}

// Start starts the config reloader
func (r *ConfigReloader) Start(interval time.Duration) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for {
			select {
			case <-r.stopCh:
				return
			case <-ticker.C:
				r.checkAndReload()
			}
		}
	}()
}

// Stop stops the config reloader
func (r *ConfigReloader) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}

func (r *ConfigReloader) checkAndReload() {
	info, err := os.Stat(r.cfgPath)
	if err != nil {
		r.logger.Warn("Failed to stat config file", zap.Error(err))
		return
	}
	
	if info.ModTime().After(r.lastModTime) {
		r.lastModTime = info.ModTime()
		r.logger.Info("Config file changed, reloading")
		
		cfg, err := config.LoadServerConfig(r.cfgPath)
		if err != nil {
			r.logger.Error("Failed to reload config", zap.Error(err))
			return
		}
		
		r.mu.RLock()
		callbacks := make([]func(*config.Config) error, len(r.callbacks))
		copy(callbacks, r.callbacks)
		r.mu.RUnlock()
		
		for _, cb := range callbacks {
			if err := cb(cfg); err != nil {
				r.logger.Error("Config reload callback failed", zap.Error(err))
			}
		}
		
		r.logger.Info("Config reloaded successfully")
	}
}

// CertificateReloader handles TLS certificate hot-reload
type CertificateReloader struct {
	certFile     string
	keyFile      string
	logger       *zap.Logger
	currentCert  *tls.Certificate
	mu           sync.RWMutex
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

// NewCertificateReloader creates a new certificate reloader
func NewCertificateReloader(certFile, keyFile string, logger *zap.Logger) *CertificateReloader {
	return &CertificateReloader{
		certFile: certFile,
		keyFile:  keyFile,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

// GetCertificate returns current certificate for TLS config
func (r *CertificateReloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	if r.currentCert == nil {
		return nil, fmt.Errorf("no certificate loaded")
	}
	return r.currentCert, nil
}

// Start starts the certificate reloader
func (r *CertificateReloader) Start(interval time.Duration) {
	// Initial load
	if err := r.reload(); err != nil {
		r.logger.Error("Initial certificate load failed", zap.Error(err))
	}
	
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for {
			select {
			case <-r.stopCh:
				return
			case <-ticker.C:
				r.reload()
			}
		}
	}()
}

// Stop stops the certificate reloader
func (r *CertificateReloader) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}

func (r *CertificateReloader) reload() error {
	certPEM, err := os.ReadFile(r.certFile)
	if err != nil {
		return fmt.Errorf("read cert: %w", err)
	}
	
	keyPEM, err := os.ReadFile(r.keyFile)
	if err != nil {
		return fmt.Errorf("read key: %w", err)
	}
	
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("parse keypair: %w", err)
	}
	
	// Verify certificate is valid
	if len(cert.Certificate) > 0 {
		if x509Cert, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
			if time.Now().After(x509Cert.NotAfter) {
				r.logger.Warn("Certificate is expired", zap.Time("not_after", x509Cert.NotAfter))
			}
		}
	}
	
	r.mu.Lock()
	r.currentCert = &cert
	r.mu.Unlock()
	
	r.logger.Info("Certificate reloaded", zap.String("cert", r.certFile))
	return nil
}

// SessionResumptionManager manages session resumption state
type SessionResumptionManager struct {
	logger       *zap.Logger
	sessions     map[string]*ResumableSession
	mu           sync.RWMutex
	maxSessions  int
	ttl          time.Duration
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

// ResumableSession holds resumable session state
type ResumableSession struct {
	ID           string
	Ticket       []byte
	CreatedAt    time.Time
	LastActivity time.Time
	StreamStates map[uint64]*StreamState
}

// StreamState holds stream state for resumption
type StreamState struct {
	StreamID     uint64
	LocalAddr    string
	RemoteAddr   string
	SentSeq      uint64
	RecvSeq      uint64
	WindowSize   int
	Closed       bool
}

// NewSessionResumptionManager creates a new session resumption manager
func NewSessionResumptionManager(logger *zap.Logger, maxSessions int, ttl time.Duration) *SessionResumptionManager {
	m := &SessionResumptionManager{
		logger:      logger,
		sessions:    make(map[string]*ResumableSession),
		maxSessions: maxSessions,
		ttl:         ttl,
		stopCh:      make(chan struct{}),
	}
	
	m.wg.Add(1)
	go m.cleanupLoop()
	
	return m
}

// StoreSession stores a session for resumption
func (m *SessionResumptionManager) StoreSession(id string, ticket []byte, streamStates map[uint64]*StreamState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Evict oldest if at capacity
	if len(m.sessions) >= m.maxSessions {
		var oldestID string
		var oldestTime time.Time
		for id, sess := range m.sessions {
			if oldestID == "" || sess.LastActivity.Before(oldestTime) {
				oldestID = id
				oldestTime = sess.LastActivity
			}
		}
		if oldestID != "" {
			delete(m.sessions, oldestID)
		}
	}
	
	m.sessions[id] = &ResumableSession{
		ID:           id,
		Ticket:       ticket,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		StreamStates: streamStates,
	}
}

// GetSession retrieves a session for resumption
func (m *SessionResumptionManager) GetSession(id string) (*ResumableSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	sess, ok := m.sessions[id]
	if ok {
		sess.LastActivity = time.Now()
	}
	return sess, ok
}

// DeleteSession deletes a session
func (m *SessionResumptionManager) DeleteSession(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
}

// cleanupLoop periodically cleans up expired sessions
func (m *SessionResumptionManager) cleanupLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.mu.Lock()
			now := time.Now()
			for id, sess := range m.sessions {
				if now.Sub(sess.LastActivity) > m.ttl {
					delete(m.sessions, id)
				}
			}
			m.mu.Unlock()
		}
	}
}

// Stop stops the session resumption manager
func (m *SessionResumptionManager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}