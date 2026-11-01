package chaos

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestChaosInjector(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &ChaosConfig{
		LatencyMin:   10 * time.Millisecond,
		LatencyMax:   50 * time.Millisecond,
		LatencyPct:   1.0,
		LossPct:      0.0,
		ReorderPct:   0.0,
		PartitionPct: 0.0,
	}
	
	injector := NewChaosInjector(cfg, logger)
	injector.Enable()
	
	start := time.Now()
	injector.MaybeInjectLatency()
	elapsed := time.Since(start)
	
	if elapsed < 10*time.Millisecond {
		t.Errorf("Expected at least 10ms latency, got %v", elapsed)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("Expected at most ~50ms latency, got %v", elapsed)
	}
}

func TestChaosInjector_Loss(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &ChaosConfig{
		LossPct: 1.0, // 100% loss
	}
	
	injector := NewChaosInjector(cfg, logger)
	injector.Enable()
	
	dropped := 0
	for i := 0; i < 100; i++ {
		if injector.MaybeDropPacket() {
			dropped++
		}
	}
	
	if dropped != 100 {
		t.Errorf("Expected 100 drops, got %d", dropped)
	}
}

func TestChaosInjector_Reorder(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &ChaosConfig{
		ReorderPct: 1.0,
		ReorderMax: 10 * time.Millisecond,
	}
	
	injector := NewChaosInjector(cfg, logger)
	injector.Enable()
	
	delayed := 0
	for i := 0; i < 100; i++ {
		if d := injector.MaybeReorder(); d > 0 {
			delayed++
		}
	}
	
	if delayed != 100 {
		t.Errorf("Expected 100 reorders, got %d", delayed)
	}
}

func TestBandwidthLimiter(t *testing.T) {
	limiter := NewBandwidthLimiter(1000) // 1000 bytes/sec
	
	// First wait should consume from initial bucket (1000 -> 500)
	start := time.Now()
	limiter.Wait(500)
	elapsed1 := time.Since(start)
	if elapsed1 > 10*time.Millisecond {
		t.Errorf("First wait should be immediate, got %v", elapsed1)
	}
	
	// Second wait should also be immediate (500 remaining)
	start = time.Now()
	limiter.Wait(500)
	elapsed2 := time.Since(start)
	if elapsed2 > 10*time.Millisecond {
		t.Errorf("Second wait should be immediate (500 remaining), got %v", elapsed2)
	}
	
	// Third wait should wait for tokens to replenish (bucket is empty)
	start = time.Now()
	limiter.Wait(500)
	elapsed3 := time.Since(start)
	
	// Should take ~0.5 second for 500 bytes at 1000 bytes/sec
	if elapsed3 < 250*time.Millisecond {
		t.Errorf("Expected ~0.5s wait for third call, got %v", elapsed3)
	}
}

func TestChaosInjector_Disabled(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &ChaosConfig{
		LatencyMin: 100 * time.Millisecond,
		LatencyPct: 1.0,
	}
	
	injector := NewChaosInjector(cfg, logger)
	// Not enabled
	
	start := time.Now()
	injector.MaybeInjectLatency()
	elapsed := time.Since(start)
	
	if elapsed > 10*time.Millisecond {
		t.Errorf("Expected no latency when disabled, got %v", elapsed)
	}
}

func TestChaosListener(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	cfg := &ChaosConfig{
		LatencyMin: 50 * time.Millisecond,
		LatencyMax: 100 * time.Millisecond,
		LatencyPct: 1.0,
	}
	
	injector := NewChaosInjector(cfg, logger)
	injector.Enable()
	
	// Test that chaos listener wraps correctly
	// Note: Full test would require actual network listener
	_ = injector
}