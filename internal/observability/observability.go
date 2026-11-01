package observability

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
	Path    string `yaml:"path"`
}

// HealthConfig holds health check configuration
type HealthConfig struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
	Path    string `yaml:"path"`
}

// PprofConfig holds pprof configuration
type PprofConfig struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
}

// Metrics holds Prometheus metrics
type Metrics struct {
	ConnectionsOpened   *prometheus.CounterVec
	ConnectionsClosed   *prometheus.CounterVec
	StreamsOpened       *prometheus.CounterVec
	StreamsClosed       *prometheus.CounterVec
	BytesSent           *prometheus.CounterVec
	BytesReceived       *prometheus.CounterVec
	Errors              *prometheus.CounterVec
	ActiveConnections   *prometheus.GaugeVec
	ActiveStreams       *prometheus.GaugeVec
	ConnectionDuration  *prometheus.HistogramVec
	StreamDuration      *prometheus.HistogramVec
	HandshakeDuration   *prometheus.HistogramVec
	
	// Simple counters for internal use
	connectionsOpened   prometheus.Counter
	connectionsClosed   prometheus.Counter
	streamsOpened       prometheus.Counter
	streamsClosed       prometheus.Counter
	
	registry *prometheus.Registry
	server   *http.Server
}

var (
	// Default metrics instance
	defaultMetrics *Metrics
)

// NewMetrics creates a new metrics instance
func NewMetrics(cfg *MetricsConfig) (*Metrics, error) {
	if cfg == nil || !cfg.Enabled {
		return &Metrics{registry: prometheus.NewRegistry()}, nil
	}

	reg := prometheus.NewRegistry()

	m := &Metrics{
		ConnectionsOpened: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "qtunnel_connections_opened_total",
				Help: "Total number of connections opened",
			},
			[]string{"transport", "result"},
		),
		ConnectionsClosed: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "qtunnel_connections_closed_total",
				Help: "Total number of connections closed",
			},
			[]string{"transport", "reason"},
		),
		StreamsOpened: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "qtunnel_streams_opened_total",
				Help: "Total number of streams opened",
			},
			[]string{"direction", "result"},
		),
		StreamsClosed: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "qtunnel_streams_closed_total",
				Help: "Total number of streams closed",
			},
			[]string{"direction", "reason"},
		),
		BytesSent: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "qtunnel_bytes_sent_total",
				Help: "Total bytes sent",
			},
			[]string{"transport"},
		),
		BytesReceived: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "qtunnel_bytes_received_total",
				Help: "Total bytes received",
			},
			[]string{"transport"},
		),
		Errors: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "qtunnel_errors_total",
				Help: "Total number of errors",
			},
			[]string{"component", "error_type"},
		),
		ActiveConnections: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "qtunnel_active_connections",
				Help: "Number of active connections",
			},
			[]string{"transport"},
		),
		ActiveStreams: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "qtunnel_active_streams",
				Help: "Number of active streams",
			},
			[]string{"transport"},
		),
		ConnectionDuration: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "qtunnel_connection_duration_seconds",
				Help:    "Connection duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"transport"},
		),
		StreamDuration: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "qtunnel_stream_duration_seconds",
				Help:    "Stream duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"direction"},
		),
		HandshakeDuration: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "qtunnel_handshake_duration_seconds",
				Help:    "Handshake duration in seconds",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"transport", "result"},
		),
		// Simple counters for internal use
		connectionsOpened: promauto.With(reg).NewCounter(
			prometheus.CounterOpts{
				Name: "qtunnel_connections_opened_simple_total",
				Help: "Total number of connections opened (simple)",
			},
		),
		connectionsClosed: promauto.With(reg).NewCounter(
			prometheus.CounterOpts{
				Name: "qtunnel_connections_closed_simple_total",
				Help: "Total number of connections closed (simple)",
			},
		),
		streamsOpened: promauto.With(reg).NewCounter(
			prometheus.CounterOpts{
				Name: "qtunnel_streams_opened_simple_total",
				Help: "Total number of streams opened (simple)",
			},
		),
		streamsClosed: promauto.With(reg).NewCounter(
			prometheus.CounterOpts{
				Name: "qtunnel_streams_closed_simple_total",
				Help: "Total number of streams closed (simple)",
			},
		),
		registry: reg,
	}

	// Start HTTP server for metrics
	if cfg.Listen != "" {
		mux := http.NewServeMux()
		path := cfg.Path
		if path == "" {
			path = "/metrics"
		}
		mux.Handle(path, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
		
		m.server = &http.Server{
			Addr:         cfg.Listen,
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		
		go func() {
			if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				// Log error but don't crash
			}
		}()
	}

	defaultMetrics = m
	return m, nil
}

// Close stops the metrics server
func (m *Metrics) Close() error {
	if m.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return m.server.Shutdown(ctx)
	}
	return nil
}

// Registry returns the Prometheus registry
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// RecordConnectionOpened records a connection opening
func (m *Metrics) RecordConnectionOpened(transport string, success bool) {
	if m.ConnectionsOpened != nil {
		result := "success"
		if !success {
			result = "failure"
		}
		m.ConnectionsOpened.WithLabelValues(transport, result).Inc()
	}
	if m.connectionsOpened != nil {
		m.connectionsOpened.Inc()
	}
}

// RecordConnectionClosed records a connection closing
func (m *Metrics) RecordConnectionClosed(transport, reason string) {
	if m.ConnectionsClosed != nil {
		m.ConnectionsClosed.WithLabelValues(transport, reason).Inc()
	}
	if m.connectionsClosed != nil {
		m.connectionsClosed.Inc()
	}
}

// RecordStreamOpened records a stream opening
func (m *Metrics) RecordStreamOpened(direction string, success bool) {
	if m.StreamsOpened != nil {
		result := "success"
		if !success {
			result = "failure"
		}
		m.StreamsOpened.WithLabelValues(direction, result).Inc()
	}
	if m.streamsOpened != nil {
		m.streamsOpened.Inc()
	}
}

// RecordStreamClosed records a stream closing
func (m *Metrics) RecordStreamClosed(direction, reason string) {
	if m.StreamsClosed != nil {
		m.StreamsClosed.WithLabelValues(direction, reason).Inc()
	}
	if m.streamsClosed != nil {
		m.streamsClosed.Inc()
	}
}

// RecordBytesSent records bytes sent
func (m *Metrics) RecordBytesSent(transport string, n int64) {
	if m.BytesSent != nil {
		m.BytesSent.WithLabelValues(transport).Add(float64(n))
	}
}

// RecordBytesReceived records bytes received
func (m *Metrics) RecordBytesReceived(transport string, n int64) {
	if m.BytesReceived != nil {
		m.BytesReceived.WithLabelValues(transport).Add(float64(n))
	}
}

// RecordError records an error
func (m *Metrics) RecordError(component, errorType string) {
	if m.Errors != nil {
		m.Errors.WithLabelValues(component, errorType).Inc()
	}
}

// SetActiveConnections sets the active connections gauge
func (m *Metrics) SetActiveConnections(transport string, n int) {
	if m.ActiveConnections != nil {
		m.ActiveConnections.WithLabelValues(transport).Set(float64(n))
	}
}

// SetActiveStreams sets the active streams gauge
func (m *Metrics) SetActiveStreams(transport string, n int) {
	if m.ActiveStreams != nil {
		m.ActiveStreams.WithLabelValues(transport).Set(float64(n))
	}
}

// ObserveConnectionDuration observes connection duration
func (m *Metrics) ObserveConnectionDuration(transport string, duration time.Duration) {
	if m.ConnectionDuration != nil {
		m.ConnectionDuration.WithLabelValues(transport).Observe(duration.Seconds())
	}
}

// ObserveStreamDuration observes stream duration
func (m *Metrics) ObserveStreamDuration(direction string, duration time.Duration) {
	if m.StreamDuration != nil {
		m.StreamDuration.WithLabelValues(direction).Observe(duration.Seconds())
	}
}

// ObserveHandshakeDuration observes handshake duration
func (m *Metrics) ObserveHandshakeDuration(transport string, success bool, duration time.Duration) {
	if m.HandshakeDuration != nil {
		result := "success"
		if !success {
			result = "failure"
		}
		m.HandshakeDuration.WithLabelValues(transport, result).Observe(duration.Seconds())
	}
}

// DefaultMetrics returns the default metrics instance
func DefaultMetrics() *Metrics {
	if defaultMetrics == nil {
		return &Metrics{registry: prometheus.NewRegistry()}
	}
	return defaultMetrics
}

// LoggerConfig holds logger configuration
type LoggerConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
	Caller bool   `yaml:"caller"`
}

// NewLogger creates a new zap logger
func NewLogger(cfg *LoggerConfig) (*zap.Logger, error) {
	if cfg == nil {
		cfg = &LoggerConfig{Level: "info", Format: "console", Output: "stderr"}
	}

	level := zapcore.InfoLevel
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level = zapcore.InfoLevel
	}

	var encoderConfig zapcore.EncoderConfig
	if cfg.Format == "json" {
		encoderConfig = zap.NewProductionEncoderConfig()
	} else {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
	}
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	var encoder zapcore.Encoder
	if cfg.Format == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	var output zapcore.WriteSyncer
	switch cfg.Output {
	case "stdout":
		output = zapcore.AddSync(zapcore.Lock(os.Stdout))
	case "stderr":
		output = zapcore.AddSync(zapcore.Lock(os.Stderr))
	default:
		file, _, err := zap.Open(cfg.Output)
		if err != nil {
			return nil, err
		}
		output = zapcore.AddSync(file)
	}

	core := zapcore.NewCore(encoder, output, level)
	opts := []zap.Option{zap.AddStacktrace(zapcore.ErrorLevel)}
	if cfg.Caller {
		opts = append(opts, zap.AddCaller())
	}

	return zap.New(core, opts...), nil
}

// HealthServer provides health check endpoints
type HealthServer struct {
	server *http.Server
	checks []HealthCheck
}

// HealthCheck is a function that returns health status
type HealthCheck func() error

// NewHealthServer creates a new health check server
func NewHealthServer(cfg *HealthConfig, checks ...HealthCheck) (*HealthServer, error) {
	if cfg == nil || !cfg.Enabled {
		return &HealthServer{}, nil
	}

	path := cfg.Path
	if path == "" {
		path = "/health"
	}

	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		for _, check := range checks {
			if err := check(); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("unhealthy: " + err.Error()))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("healthy"))
	})

	mux.HandleFunc(path+"/ready", func(w http.ResponseWriter, r *http.Request) {
		// Readiness check - same as health for now
		for _, check := range checks {
			if err := check(); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("not ready: " + err.Error()))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ready"))
	})

	s := &HealthServer{
		server: &http.Server{
			Addr:         cfg.Listen,
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		},
		checks: checks,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log error but don't crash
		}
	}()

	return s, nil
}

// Close stops the health server
func (h *HealthServer) Close() error {
	if h.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return h.server.Shutdown(ctx)
	}
	return nil
}