package config

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qtunnel/qtunnel/internal/observability"
	"gopkg.in/yaml.v3"
)

// Role types
const (
	RoleClient = "client"
	RoleServer = "server"
)

// Transport types
const (
	TransportQUIC  = "quic"
	TransportHTTP3 = "http3"
	TransportHTTP2 = "http2"
)

// Proxy modes
const (
	ProxyModeTCP   = "tcp"
	ProxyModeSOCKS5 = "socks5"
)

// Default values
const (
	DefaultMaxConnections       = 1000
	DefaultMaxStreamsPerConn    = 100
	DefaultStreamBufferSize     = 1 << 20      // 1MB
	DefaultConnBufferSize       = 10 << 20     // 10MB
	DefaultMaxFrameSize         = 16 << 10     // 16KB
	DefaultMaxIdleTimeout       = 30 * time.Second
	DefaultKeepAlive            = 10 * time.Second
	DefaultHandshakeTimeout     = 10 * time.Second
	DefaultConnectTimeout       = 10 * time.Second
	DefaultIdleTimeout          = 5 * time.Minute
	DefaultGracePeriod          = 30 * time.Second
	DefaultForceAfter           = 60 * time.Second
)

// Config is the root configuration
type Config struct {
	Role       string          `yaml:"role"`
	Server     *ServerConfig   `yaml:"server,omitempty"`
	Listen     *ListenConfig   `yaml:"listen,omitempty"`
	TLS        *TLSConfig      `yaml:"tls,omitempty"`
	Auth       *AuthConfig     `yaml:"auth,omitempty"`
	Transport  *TransportConfig `yaml:"transport,omitempty"`
	Proxy      *ProxyConfig    `yaml:"proxy,omitempty"`
	Limits     *LimitsConfig   `yaml:"limits,omitempty"`
	Logging    *LoggingConfig  `yaml:"logging,omitempty"`
	Observability *ObservabilityConfig `yaml:"observability,omitempty"`
	Shutdown   *ShutdownConfig `yaml:"shutdown,omitempty"`
}

// ServerConfig holds client-side server connection settings
type ServerConfig struct {
	Addr               string `yaml:"addr"`
	SNI                string `yaml:"sni"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

// ListenConfig holds server listen settings
type ListenConfig struct {
	Addr        string `yaml:"addr"`
	ReusePort   bool   `yaml:"reuse_port"`
	TCPFastOpen bool   `yaml:"tcp_fast_open"`
}

// TLSConfig holds TLS settings
type TLSConfig struct {
	CertFile     string   `yaml:"cert_file"`
	KeyFile      string   `yaml:"key_file"`
	MinVersion   string   `yaml:"min_version"`
	CipherSuites []string `yaml:"cipher_suites"`
	ClientCAFile string   `yaml:"client_ca_file"`
	ClientAuth   string   `yaml:"client_auth"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

// AuthConfig holds authentication settings
type AuthConfig struct {
	Tokens    []string `yaml:"tokens"`
	Token     string   `yaml:"token"`
	TokenFile string   `yaml:"token_file"`
}

// TransportConfig holds transport settings
type TransportConfig struct {
	Type   string        `yaml:"type"`
	QUIC   *QUICConfig   `yaml:"quic,omitempty"`
	HTTP3  *HTTP3Config  `yaml:"http3,omitempty"`
	HTTP2  *HTTP2Config  `yaml:"http2,omitempty"`
	MASQUE *MASQUEConfig `yaml:"masque,omitempty"`
}

// QUICConfig holds QUIC-specific settings
type QUICConfig struct {
	MaxIdleTimeout            string `yaml:"max_idle_timeout"`
	KeepAlive                 string `yaml:"keep_alive"`
	HandshakeTimeout          string `yaml:"handshake_timeout"`
	Enable0RTT                bool   `yaml:"enable_0rtt"`
	CongestionControl         string `yaml:"congestion_control"`
	InitialCongestionWindow   int    `yaml:"initial_congestion_window"`
	MaxReceiveBufferSize      string `yaml:"max_receive_buffer_size"`
	MaxIncomingStreams        int    `yaml:"max_incoming_streams"`
	MaxIncomingUniStreams     int    `yaml:"max_incoming_uni_streams"`
	DisablePathMigration      bool   `yaml:"disable_path_migration"`
}

// HTTP3Config holds HTTP/3-specific settings
type HTTP3Config struct {
	MaxIdleTimeout        string `yaml:"max_idle_timeout"`
	KeepAlive             string `yaml:"keep_alive"`
	HandshakeTimeout      string `yaml:"handshake_timeout"`
	Enable0RTT            bool   `yaml:"enable_0rtt"`
	MaxReceiveBufferSize  string `yaml:"max_receive_buffer_size"`
	MaxIncomingStreams    int    `yaml:"max_incoming_streams"`
	MaxIncomingUniStreams int    `yaml:"max_incoming_uni_streams"`
	DisablePathMigration  bool   `yaml:"disable_path_migration"`
}

// HTTP2Config holds HTTP/2-specific settings
type HTTP2Config struct {
	MaxIdleTimeout       string `yaml:"max_idle_timeout"`
	KeepAlive            string `yaml:"keep_alive"`
	HandshakeTimeout     string `yaml:"handshake_timeout"`
	MaxReceiveBufferSize string `yaml:"max_receive_buffer_size"`
	MaxConcurrentStreams int    `yaml:"max_concurrent_streams"`
	DisablePathMigration bool   `yaml:"disable_path_migration"`
}

// MASQUEConfig holds MASQUE-specific settings
type MASQUEConfig struct {
	ConnectUDP *CONNECTUDPConfig `yaml:"connect_udp,omitempty"`
	ConnectIP  *CONNECTIPConfig  `yaml:"connect_ip,omitempty"`
	VPN        *VPNConfig        `yaml:"vpn,omitempty"`
}

// CONNECTUDPConfig holds CONNECT-UDP specific settings
type CONNECTUDPConfig struct {
	URITemplate      string `yaml:"uri_template"`
	MaxDatagramSize  int    `yaml:"max_datagram_size"`
}

// CONNECTIPConfig holds CONNECT-IP specific settings
type CONNECTIPConfig struct {
	URITemplate string `yaml:"uri_template"`
}

// VPNConfig holds VPN settings
type VPNConfig struct {
	TUN        *TUNConfig `yaml:"tun,omitempty"`
	KillSwitch bool       `yaml:"kill_switch"`
	SplitTunnel bool      `yaml:"split_tunnel"`
}

// TUNConfig holds TUN device settings
type TUNConfig struct {
	Name        string   `yaml:"name"`
	MTU         int      `yaml:"mtu"`
	Addresses   []string `yaml:"addresses"`
	Routes      []string `yaml:"routes"`
	DNSServers  []string `yaml:"dns_servers"`
}

// ProxyConfig holds proxy settings
type ProxyConfig struct {
	Mode     string      `yaml:"mode"`
	Listen   string      `yaml:"listen"`
	Allowlist []string   `yaml:"allowlist"`
	Denylist []string   `yaml:"denylist"`
	ConnectTimeout string `yaml:"connect_timeout"`
	IdleTimeout    string `yaml:"idle_timeout"`
	SOCKS5   *SOCKS5Config `yaml:"socks5,omitempty"`
}

// SOCKS5Config holds SOCKS5 proxy settings
type SOCKS5Config struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// LimitsConfig holds resource limits
type LimitsConfig struct {
	MaxConnections    int    `yaml:"max_connections"`
	MaxStreamsPerConn int    `yaml:"max_streams_per_conn"`
	StreamBufferSize  string `yaml:"stream_buffer_size"`
	ConnBufferSize    string `yaml:"conn_buffer_size"`
	MaxFrameSize      string `yaml:"max_frame_size"`
	RateLimit         int    `yaml:"rate_limit"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level   string `yaml:"level"`
	Format  string `yaml:"format"`
	Output  string `yaml:"output"`
	Caller  bool   `yaml:"caller"`
}

// ObservabilityConfig holds observability settings
type ObservabilityConfig struct {
	Metrics *observability.MetricsConfig `yaml:"metrics"`
	Health  *observability.HealthConfig  `yaml:"health"`
	Pprof   *observability.PprofConfig   `yaml:"pprof"`
}

// ShutdownConfig holds shutdown settings
type ShutdownConfig struct {
	GracePeriod string `yaml:"grace_period"`
	ForceAfter  string `yaml:"force_after"`
}

// ParseDuration parses a duration string with default
func ParseDuration(s string, defaultVal time.Duration) (time.Duration, error) {
	if s == "" {
		return defaultVal, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal, nil
	}
	return d, nil
}

// ParseSize parses a size string (e.g., "1MB", "16KB") with default
func ParseSize(s string, defaultVal int) (int, error) {
	if s == "" {
		return defaultVal, nil
	}
	s = strings.ToUpper(strings.TrimSpace(s))
	var mult int
	switch {
	case strings.HasSuffix(s, "KB"):
		mult = 1024
		s = strings.TrimSuffix(s, "KB")
	case strings.HasSuffix(s, "MB"):
		mult = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "GB"):
		mult = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GB")
	default:
		mult = 1
	}
	_, err := fmt.Sscanf(s, "%d", new(int))
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	// Simple parsing without fmt
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n * mult, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Role != RoleClient && c.Role != RoleServer {
		return fmt.Errorf("invalid role: %s (must be 'client' or 'server')", c.Role)
	}

	if c.Role == RoleClient {
		return c.validateClient()
	}
	return c.validateServer()
}

func (c *Config) validateClient() error {
	if c.Server == nil {
		return fmt.Errorf("server config required for client")
	}
	if c.Server.Addr == "" {
		return fmt.Errorf("server.addr required")
	}
	if c.Server.SNI == "" {
		// Extract host from addr for SNI
		host, _, err := net.SplitHostPort(c.Server.Addr)
		if err != nil {
			return fmt.Errorf("invalid server.addr: %w", err)
		}
		if ip := net.ParseIP(host); ip != nil {
			return fmt.Errorf("server.sni required when server.addr is an IP address")
		}
		c.Server.SNI = host
	}

	if c.Auth == nil || (len(c.Auth.Tokens) == 0 && c.Auth.Token == "" && c.Auth.TokenFile == "") {
		return fmt.Errorf("auth.token or auth.token_file required")
	}
	// Support both singular token and plural tokens for backward compatibility
	if c.Auth.Token != "" {
		// If Tokens already has values (from env override), replace with Token
		// Otherwise append Token to Tokens
		if len(c.Auth.Tokens) > 0 {
			c.Auth.Tokens = []string{c.Auth.Token}
		} else {
			c.Auth.Tokens = append(c.Auth.Tokens, c.Auth.Token)
		}
	}
	if len(c.Auth.Tokens) > 0 {
		for i, tok := range c.Auth.Tokens {
			if _, err := base64.StdEncoding.DecodeString(tok); err != nil {
				return fmt.Errorf("auth.tokens[%d]: invalid base64: %w", i, err)
			}
		}
	}

	if c.Transport == nil {
		c.Transport = &TransportConfig{Type: TransportQUIC}
	}
	if c.Transport.Type != TransportQUIC && c.Transport.Type != TransportHTTP3 && c.Transport.Type != TransportHTTP2 {
		return fmt.Errorf("invalid transport.type: %s", c.Transport.Type)
	}

	if c.Proxy == nil {
		c.Proxy = &ProxyConfig{Mode: ProxyModeTCP, Listen: "127.0.0.1:8080"}
	}
	if c.Proxy.Mode != ProxyModeTCP && c.Proxy.Mode != ProxyModeSOCKS5 {
		return fmt.Errorf("invalid proxy.mode: %s", c.Proxy.Mode)
	}
	if c.Proxy.Listen == "" {
		return fmt.Errorf("proxy.listen required")
	}

	return c.validateCommon()
}

func (c *Config) validateServer() error {
	if c.Listen == nil {
		return fmt.Errorf("listen config required for server")
	}
	if c.Listen.Addr == "" {
		return fmt.Errorf("listen.addr required")
	}

	if c.TLS == nil {
		return fmt.Errorf("tls config required for server")
	}
	if c.TLS.CertFile == "" || c.TLS.KeyFile == "" {
		return fmt.Errorf("tls.cert_file and tls.key_file required")
	}
	if _, err := os.Stat(c.TLS.CertFile); err != nil {
		return fmt.Errorf("tls.cert_file: %w", err)
	}
	if _, err := os.Stat(c.TLS.KeyFile); err != nil {
		return fmt.Errorf("tls.key_file: %w", err)
	}

	if c.Auth == nil || (len(c.Auth.Tokens) == 0 && c.Auth.Token == "" && c.Auth.TokenFile == "") {
		return fmt.Errorf("auth.tokens, auth.token or auth.token_file required")
	}
	if len(c.Auth.Tokens) > 0 {
		for i, tok := range c.Auth.Tokens {
			if _, err := base64.StdEncoding.DecodeString(tok); err != nil {
				return fmt.Errorf("auth.tokens[%d]: invalid base64: %w", i, err)
			}
		}
	}
	if c.Auth.Token != "" {
		if _, err := base64.StdEncoding.DecodeString(c.Auth.Token); err != nil {
			return fmt.Errorf("auth.token: invalid base64: %w", err)
		}
	}

	if c.Transport == nil {
		c.Transport = &TransportConfig{Type: TransportQUIC}
	}
	if c.Transport.Type != TransportQUIC && c.Transport.Type != TransportHTTP3 && c.Transport.Type != TransportHTTP2 {
		return fmt.Errorf("invalid transport.type: %s", c.Transport.Type)
	}

	if c.Proxy == nil {
		c.Proxy = &ProxyConfig{Mode: ProxyModeTCP}
	}

	return c.validateCommon()
}

func (c *Config) validateCommon() error {
	if c.Limits == nil {
		c.Limits = &LimitsConfig{}
	}
	if c.Limits.MaxConnections <= 0 {
		c.Limits.MaxConnections = DefaultMaxConnections
	}
	if c.Limits.MaxStreamsPerConn <= 0 {
		c.Limits.MaxStreamsPerConn = DefaultMaxStreamsPerConn
	}
	if c.Limits.StreamBufferSize == "" {
		c.Limits.StreamBufferSize = fmt.Sprintf("%dKB", DefaultStreamBufferSize/1024)
	}
	if c.Limits.ConnBufferSize == "" {
		c.Limits.ConnBufferSize = fmt.Sprintf("%dKB", DefaultConnBufferSize/1024)
	}
	if c.Limits.MaxFrameSize == "" {
		c.Limits.MaxFrameSize = fmt.Sprintf("%dKB", DefaultMaxFrameSize/1024)
	}

	if c.Logging == nil {
		c.Logging = &LoggingConfig{Level: "info", Format: "console", Output: "stderr"}
	}

	if c.Observability == nil {
		c.Observability = &ObservabilityConfig{}
	}

	if c.Shutdown == nil {
		c.Shutdown = &ShutdownConfig{}
	}

	// Validate denylist entries (CIDR or hostname)
	for _, entry := range c.Proxy.Denylist {
		if _, err := netip.ParsePrefix(entry); err != nil {
			// Not a CIDR, validate as hostname
			if !isValidHostname(entry) && entry != "*" {
				return fmt.Errorf("invalid denylist entry: %s", entry)
			}
		}
	}
	for _, entry := range c.Proxy.Allowlist {
		if _, err := netip.ParsePrefix(entry); err != nil {
			if !isValidHostname(entry) && entry != "*" {
				return fmt.Errorf("invalid allowlist entry: %s", entry)
			}
		}
	}

	return nil
}

func isValidHostname(h string) bool {
	if len(h) == 0 || len(h) > 253 {
		return false
	}
	// Allow wildcard
	if h == "*" {
		return true
	}
	// Simple validation
	for _, label := range strings.Split(h, ".") {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
	}
	return true
}

// LoadClientConfig loads and validates client configuration
func LoadClientConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	// Apply environment variable overrides
	cfg.applyEnvOverrides()
	// Set defaults
	cfg.setDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadServerConfig loads and validates server configuration
func LoadServerConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyEnvOverrides()
	cfg.setDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) applyEnvOverrides() {
	// QTTUNNEL_SERVER_ADDR
	if v := os.Getenv("QTTUNNEL_SERVER_ADDR"); v != "" && c.Server != nil {
		c.Server.Addr = v
	}
	if v := os.Getenv("QTTUNNEL_SERVER_SNI"); v != "" && c.Server != nil {
		c.Server.SNI = v
	}
	if v := os.Getenv("QTTUNNEL_AUTH_TOKEN"); v != "" {
		if c.Auth == nil {
			c.Auth = &AuthConfig{}
		}
		c.Auth.Tokens = []string{v}
		c.Auth.Token = "" // Clear token to avoid duplication in validation
	}
	if v := os.Getenv("QTTUNNEL_LISTEN_ADDR"); v != "" && c.Listen != nil {
		c.Listen.Addr = v
	}
	if v := os.Getenv("QTTUNNEL_TLS_CERT"); v != "" && c.TLS != nil {
		c.TLS.CertFile = v
	}
	if v := os.Getenv("QTTUNNEL_TLS_KEY"); v != "" && c.TLS != nil {
		c.TLS.KeyFile = v
	}
	if v := os.Getenv("QTTUNNEL_PROXY_LISTEN"); v != "" && c.Proxy != nil {
		c.Proxy.Listen = v
	}
	if v := os.Getenv("QTTUNNEL_LOG_LEVEL"); v != "" && c.Logging != nil {
		c.Logging.Level = v
	}
}

func (c *Config) setDefaults() {
	if c.Transport != nil && c.Transport.Type == "" {
		c.Transport.Type = TransportQUIC
	}
	if c.Transport != nil && c.Transport.QUIC != nil {
		q := c.Transport.QUIC
		if q.MaxIdleTimeout == "" {
			q.MaxIdleTimeout = DefaultMaxIdleTimeout.String()
		}
		if q.KeepAlive == "" {
			q.KeepAlive = DefaultKeepAlive.String()
		}
		if q.HandshakeTimeout == "" {
			q.HandshakeTimeout = DefaultHandshakeTimeout.String()
		}
		if q.CongestionControl == "" {
			q.CongestionControl = "cubic"
		}
		if q.InitialCongestionWindow <= 0 {
			q.InitialCongestionWindow = 10
		}
		if q.MaxReceiveBufferSize == "" {
			q.MaxReceiveBufferSize = fmt.Sprintf("%dKB", DefaultConnBufferSize/1024)
		}
		if q.MaxIncomingStreams <= 0 {
			q.MaxIncomingStreams = 100
		}
		if q.MaxIncomingUniStreams <= 0 {
			q.MaxIncomingUniStreams = 100
		}
	}
	if c.Transport != nil && c.Transport.HTTP2 != nil {
		h2 := c.Transport.HTTP2
		if h2.MaxIdleTimeout == "" {
			h2.MaxIdleTimeout = DefaultMaxIdleTimeout.String()
		}
		if h2.KeepAlive == "" {
			h2.KeepAlive = DefaultKeepAlive.String()
		}
		if h2.HandshakeTimeout == "" {
			h2.HandshakeTimeout = DefaultHandshakeTimeout.String()
		}
		if h2.MaxReceiveBufferSize == "" {
			h2.MaxReceiveBufferSize = fmt.Sprintf("%dKB", DefaultConnBufferSize/1024)
		}
		if h2.MaxConcurrentStreams <= 0 {
			h2.MaxConcurrentStreams = 100
		}
	}
	if c.Proxy != nil {
		if c.Proxy.Mode == "" {
			c.Proxy.Mode = ProxyModeTCP
		}
		if c.Proxy.ConnectTimeout == "" {
			c.Proxy.ConnectTimeout = DefaultConnectTimeout.String()
		}
		if c.Proxy.IdleTimeout == "" {
			c.Proxy.IdleTimeout = DefaultIdleTimeout.String()
		}
	}
	if c.Shutdown != nil {
		if c.Shutdown.GracePeriod == "" {
			c.Shutdown.GracePeriod = DefaultGracePeriod.String()
		}
		if c.Shutdown.ForceAfter == "" {
			c.Shutdown.ForceAfter = DefaultForceAfter.String()
		}
	}
}

// GetTLSConfig returns a tls.Config for client or server
func (c *Config) GetTLSConfig(isServer bool) (*tls.Config, error) {
	if isServer {
		if c.TLS == nil {
			return nil, fmt.Errorf("tls config missing")
		}
		cert, err := tls.LoadX509KeyPair(c.TLS.CertFile, c.TLS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load cert: %w", err)
		}
		cfg := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS13,
			NextProtos:   []string{"qtunnel/1"},
		}
		if c.TLS.ClientCAFile != "" {
			// Load CA for client cert verification
			// Implementation depends on use case
		}
		return cfg, nil
	}

	// Client TLS config
	cfg := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		ServerName:         c.Server.SNI,
		InsecureSkipVerify: c.Server.InsecureSkipVerify,
		NextProtos:         []string{"qtunnel/1"},
	}
	return cfg, nil
}

// GetParsedLimits returns parsed limit values
func (c *Config) GetParsedLimits() (*ParsedLimits, error) {
	streamBuf, err := ParseSize(c.Limits.StreamBufferSize, DefaultStreamBufferSize)
	if err != nil {
		return nil, err
	}
	connBuf, err := ParseSize(c.Limits.ConnBufferSize, DefaultConnBufferSize)
	if err != nil {
		return nil, err
	}
	maxFrame, err := ParseSize(c.Limits.MaxFrameSize, DefaultMaxFrameSize)
	if err != nil {
		return nil, err
	}
	return &ParsedLimits{
		MaxConnections:    c.Limits.MaxConnections,
		MaxStreamsPerConn: c.Limits.MaxStreamsPerConn,
		StreamBufferSize:  streamBuf,
		ConnBufferSize:    connBuf,
		MaxFrameSize:      maxFrame,
	}, nil
}

// ParsedLimits holds validated limit values
type ParsedLimits struct {
	MaxConnections    int
	MaxStreamsPerConn int
	StreamBufferSize  int
	ConnBufferSize    int
	MaxFrameSize      int
}

// ExampleConfigPath returns path to example config
func ExampleConfigPath(role string) string {
	dir := "configs"
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// Try parent directory
		dir = filepath.Join("..", "configs")
	}
	return filepath.Join(dir, fmt.Sprintf("%s.example.yaml", role))
}

// ValidateTokenFormat validates a token is valid base64
func ValidateTokenFormat(token string) error {
	if _, err := base64.StdEncoding.DecodeString(token); err != nil {
		return fmt.Errorf("invalid token format: %w", err)
	}
	return nil
}