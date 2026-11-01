package chaos

import (
	"context"
	"math/rand"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ChaosConfig holds failure injection configuration
type ChaosConfig struct {
	// Latency injection
	LatencyMin time.Duration `yaml:"latency_min"`
	LatencyMax time.Duration `yaml:"latency_max"`
	LatencyPct float64       `yaml:"latency_pct"` // 0.0 to 1.0

	// Packet loss
	LossPct float64 `yaml:"loss_pct"` // 0.0 to 1.0

	// Packet reordering
	ReorderPct float64 `yaml:"reorder_pct"` // 0.0 to 1.0
	ReorderMax time.Duration `yaml:"reorder_max"`

	// Connection partition
	PartitionPct float64 `yaml:"partition_pct"` // 0.0 to 1.0

	// Bandwidth limit (bytes/sec)
	BandwidthLimit int64 `yaml:"bandwidth_limit"` // 0 = unlimited
}

// ChaosInjector injects network failures for testing
type ChaosInjector struct {
	cfg    *ChaosConfig
	logger *zap.Logger
	mu     sync.RWMutex
	enabled bool
}

// NewChaosInjector creates a new chaos injector
func NewChaosInjector(cfg *ChaosConfig, logger *zap.Logger) *ChaosInjector {
	return &ChaosInjector{
		cfg:    cfg,
		logger: logger,
	}
}

// Enable enables chaos injection
func (c *ChaosInjector) Enable() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enabled = true
	c.logger.Info("Chaos injection enabled", zap.Any("config", c.cfg))
}

// Disable disables chaos injection
func (c *ChaosInjector) Disable() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enabled = false
	c.logger.Info("Chaos injection disabled")
}

// IsEnabled returns whether chaos injection is enabled
func (c *ChaosInjector) IsEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.enabled
}

// MaybeInjectLatency injects random latency
func (c *ChaosInjector) MaybeInjectLatency() {
	c.mu.RLock()
	cfg := c.cfg
	enabled := c.enabled
	c.mu.RUnlock()

	if !enabled || cfg == nil || cfg.LatencyPct <= 0 {
		return
	}

	if rand.Float64() < cfg.LatencyPct {
		latency := cfg.LatencyMin + time.Duration(rand.Float64()*float64(cfg.LatencyMax-cfg.LatencyMin))
		c.logger.Debug("Injecting latency", zap.Duration("latency", latency))
		time.Sleep(latency)
	}
}

// MaybeDropPacket returns true if packet should be dropped
func (c *ChaosInjector) MaybeDropPacket() bool {
	c.mu.RLock()
	cfg := c.cfg
	enabled := c.enabled
	c.mu.RUnlock()

	if !enabled || cfg == nil || cfg.LossPct <= 0 {
		return false
	}

	return rand.Float64() < cfg.LossPct
}

// MaybeReorder returns delay for packet reordering
func (c *ChaosInjector) MaybeReorder() time.Duration {
	c.mu.RLock()
	cfg := c.cfg
	enabled := c.enabled
	c.mu.RUnlock()

	if !enabled || cfg == nil || cfg.ReorderPct <= 0 {
		return 0
	}

	if rand.Float64() < cfg.ReorderPct {
		delay := time.Duration(rand.Float64() * float64(cfg.ReorderMax))
		c.logger.Debug("Injecting reorder delay", zap.Duration("delay", delay))
		return delay
	}
	return 0
}

// MaybePartition returns true if connection should be partitioned
func (c *ChaosInjector) MaybePartition() bool {
	c.mu.RLock()
	cfg := c.cfg
	enabled := c.enabled
	c.mu.RUnlock()

	if !enabled || cfg == nil || cfg.PartitionPct <= 0 {
		return false
	}

	return rand.Float64() < cfg.PartitionPct
}

// BandwidthLimiter limits bandwidth
type BandwidthLimiter struct {
	limit     int64
	tokens    float64
	lastCheck time.Time
	mu        sync.Mutex
}

// NewBandwidthLimiter creates a new bandwidth limiter
func NewBandwidthLimiter(limit int64) *BandwidthLimiter {
	return &BandwidthLimiter{
		limit:     limit,
		tokens:    float64(limit),
		lastCheck: time.Now(),
	}
}

// Wait waits until enough tokens are available
func (b *BandwidthLimiter) Wait(n int) {
	if b.limit <= 0 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastCheck).Seconds()
	b.tokens += elapsed * float64(b.limit)
	if b.tokens > float64(b.limit) {
		b.tokens = float64(b.limit)
	}
	b.lastCheck = now

	for b.tokens < float64(n) {
		needed := float64(n) - b.tokens
		waitTime := time.Duration(needed/float64(b.limit)*1e9) * time.Nanosecond
		b.mu.Unlock()
		time.Sleep(waitTime)
		b.mu.Lock()
		
		now = time.Now()
		elapsed = now.Sub(b.lastCheck).Seconds()
		b.tokens += elapsed * float64(b.limit)
		if b.tokens > float64(b.limit) {
			b.tokens = float64(b.limit)
		}
		b.lastCheck = now
	}
	
	b.tokens -= float64(n)
}

// ChaosListener wraps a listener with chaos injection
type ChaosListener struct {
	net.Listener
	injector *ChaosInjector
	limiter  *BandwidthLimiter
}

func (c *ChaosListener) Accept() (net.Conn, error) {
	conn, err := c.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &ChaosConn{Conn: conn, injector: c.injector, limiter: c.limiter}, nil
}

type chaosListener struct {
	net.Listener
	injector *ChaosInjector
	limiter  *BandwidthLimiter
}

// WrapListener wraps a listener with chaos injection
func WrapListener(l net.Listener, injector *ChaosInjector, limiter *BandwidthLimiter) net.Listener {
	return &chaosListener{Listener: l, injector: injector, limiter: limiter}
}

func (c *chaosListener) Accept() (net.Conn, error) {
	conn, err := c.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &ChaosConn{Conn: conn, injector: c.injector, limiter: c.limiter}, nil
}

// ChaosConn wraps a connection with chaos injection
type ChaosConn struct {
	net.Conn
	injector *ChaosInjector
	limiter  *BandwidthLimiter
}

func (c *ChaosConn) Read(b []byte) (int, error) {
	if c.injector.MaybeDropPacket() {
		return 0, nil // Simulate packet loss by returning 0 bytes
	}
	
	c.injector.MaybeInjectLatency()
	
	n, err := c.Conn.Read(b)
	
	if c.limiter != nil && n > 0 {
		c.limiter.Wait(n)
	}
	
	if delay := c.injector.MaybeReorder(); delay > 0 {
		time.Sleep(delay)
	}
	
	return n, err
}

func (c *ChaosConn) Write(b []byte) (int, error) {
	if c.injector.MaybeDropPacket() {
		return len(b), nil // Simulate packet loss by pretending to write
	}
	
	c.injector.MaybeInjectLatency()
	
	n, err := c.Conn.Write(b)
	
	if c.limiter != nil && n > 0 {
		c.limiter.Wait(n)
	}
	
	if delay := c.injector.MaybeReorder(); delay > 0 {
		time.Sleep(delay)
	}
	
	return n, err
}

// WithContext adds context support
func (c *ChaosConn) WithContext(ctx context.Context) *ChaosConnWithContext {
	return &ChaosConnWithContext{ChaosConn: c, ctx: ctx}
}

type ChaosConnWithContext struct {
	*ChaosConn
	ctx context.Context
}

func (c *ChaosConnWithContext) Read(b []byte) (int, error) {
	select {
	case <-c.ctx.Done():
		return 0, c.ctx.Err()
	default:
		return c.ChaosConn.Read(b)
	}
}

func (c *ChaosConnWithContext) Write(b []byte) (int, error) {
	select {
	case <-c.ctx.Done():
		return 0, c.ctx.Err()
	default:
		return c.ChaosConn.Write(b)
	}
}