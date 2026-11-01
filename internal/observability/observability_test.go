package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

func TestNewMetrics_Disabled(t *testing.T) {
	cfg := &MetricsConfig{Enabled: false}
	m, err := NewMetrics(cfg)
	if err != nil {
		t.Fatalf("NewMetrics failed: %v", err)
	}
	if m == nil {
		t.Fatal("Expected non-nil metrics")
	}
	// Should not have server
	if m.server != nil {
		t.Error("Server should be nil when disabled")
	}
}

func TestNewMetrics_Enabled(t *testing.T) {
	cfg := &MetricsConfig{
		Enabled: true,
		Listen:  "127.0.0.1:0",
		Path:    "/metrics",
	}
	m, err := NewMetrics(cfg)
	if err != nil {
		t.Fatalf("NewMetrics failed: %v", err)
	}
	if m.server == nil {
		t.Error("Server should not be nil when enabled")
	}
	defer m.Close()

	// Test recording metrics
	m.RecordConnectionOpened("quic", true)
	m.RecordConnectionClosed("quic", "normal")
	m.RecordStreamOpened("outbound", true)
	m.RecordStreamClosed("outbound", "completed")
	m.RecordBytesSent("quic", 1024)
	m.RecordBytesReceived("quic", 512)
	m.RecordError("test", "test_error")
	m.SetActiveConnections("quic", 5)
	m.SetActiveStreams("quic", 10)
	m.ObserveConnectionDuration("quic", 100*time.Millisecond)
	m.ObserveStreamDuration("outbound", 50*time.Millisecond)
	m.ObserveHandshakeDuration("quic", true, 25*time.Millisecond)

	// Verify metrics are recorded by gathering
	reg := m.Registry()
	metricFamilies, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	found := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "qtunnel_connections_opened_total" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected qtunnel_connections_opened_total metric")
	}
}

func TestNewLogger(t *testing.T) {
	cfg := &LoggerConfig{
		Level:  "debug",
		Format: "json",
		Output: "stderr",
		Caller: true,
	}
	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	if logger == nil {
		t.Fatal("Expected non-nil logger")
	}
	defer logger.Sync()

	logger.Info("test message", zap.String("key", "value"))
}

func TestHealthServer(t *testing.T) {
	checkCalled := false
	check := func() error {
		checkCalled = true
		return nil
	}

	cfg := &HealthConfig{
		Enabled: true,
		Listen:  "127.0.0.1:0",
		Path:    "/health",
	}

	hs, err := NewHealthServer(cfg, check)
	if err != nil {
		t.Fatalf("NewHealthServer failed: %v", err)
	}
	defer hs.Close()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Test health endpoint
	req := httptest.NewRequest("GET", "http://"+cfg.Listen+cfg.Path, nil)
	w := httptest.NewRecorder()
	hs.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Health check returned %d, want 200", w.Code)
	}
	if !checkCalled {
		t.Error("Health check function not called")
	}
}

func TestHealthServer_Unhealthy(t *testing.T) {
	check := func() error {
		return errTest
	}

	cfg := &HealthConfig{
		Enabled: true,
		Listen:  "127.0.0.1:0",
		Path:    "/health",
	}

	hs, err := NewHealthServer(cfg, check)
	if err != nil {
		t.Fatalf("NewHealthServer failed: %v", err)
	}
	defer hs.Close()

	time.Sleep(100 * time.Millisecond)

	req := httptest.NewRequest("GET", "http://"+cfg.Listen+cfg.Path, nil)
	w := httptest.NewRecorder()
	hs.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Health check returned %d, want 503", w.Code)
	}
}

var errTest = &testError{}

type testError struct{}

func (e *testError) Error() string {
	return "test error"
}

func TestMetrics_Registry(t *testing.T) {
	cfg := &MetricsConfig{Enabled: true, Listen: "127.0.0.1:0"}
	m, err := NewMetrics(cfg)
	if err != nil {
		t.Fatalf("NewMetrics failed: %v", err)
	}
	defer m.Close()

	reg := m.Registry()
	if reg == nil {
		t.Fatal("Registry should not be nil")
	}

	// Register a custom metric
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_counter",
		Help: "Test counter",
	})
	reg.MustRegister(counter)
	counter.Inc()

	// Gather and verify
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	found := false
	for _, mf := range families {
		if mf.GetName() == "test_counter" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Custom metric not found in registry")
	}
}