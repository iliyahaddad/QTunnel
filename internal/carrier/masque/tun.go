package masque

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/qtunnel/qtunnel/internal/config"
	"go.uber.org/zap"
)

// TUNConfig holds TUN device configuration
type TUNConfig struct {
	Name        string   `yaml:"name"`
	MTU         int      `yaml:"mtu"`
	Addresses   []string `yaml:"addresses"`    // CIDR notation
	Routes      []string `yaml:"routes"`       // CIDR notation
	DNSServers  []string `yaml:"dns_servers"`  // DNS server IPs
}

// TUNDevice represents a platform-abstracted TUN device
type TUNDevice interface {
	Name() string
	MTU() int
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
	
	// Platform-specific setup
	Configure(addresses []string, routes []string, dnsServers []string) error
}

// TUNManager manages TUN device lifecycle
type TUNManager struct {
	cfg      *TUNConfig
	device   TUNDevice
	logger   *zap.Logger
	mu       sync.Mutex
	running  bool
	readWG   sync.WaitGroup
	packetCh chan []byte
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewTUNManager creates a new TUN manager
func NewTUNManager(cfg *TUNConfig, logger *zap.Logger) (*TUNManager, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	mgr := &TUNManager{
		cfg:      cfg,
		logger:   logger,
		ctx:      ctx,
		cancel:   cancel,
		packetCh: make(chan []byte, 1024),
	}
	
	// Create platform-specific TUN device
	device, err := createTUNDevice(cfg.Name, cfg.MTU)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create TUN device: %w", err)
	}
	
	mgr.device = device
	return mgr, nil
}

// Start starts the TUN device
func (m *TUNManager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.running {
		return nil
	}
	
	// Configure device
	if err := m.device.Configure(m.cfg.Addresses, m.cfg.Routes, m.cfg.DNSServers); err != nil {
		return fmt.Errorf("failed to configure TUN device: %w", err)
	}
	
	m.running = true
	
	// Start read loop
	m.readWG.Add(1)
	go m.readLoop()
	
	m.logger.Info("TUN device started", zap.String("name", m.device.Name()), zap.Int("mtu", m.device.MTU()))
	return nil
}

// Stop stops the TUN device
func (m *TUNManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if !m.running {
		return nil
	}
	
	m.cancel()
	m.readWG.Wait()
	
	if err := m.device.Close(); err != nil {
		m.logger.Error("Error closing TUN device", zap.Error(err))
	}
	
	m.running = false
	m.logger.Info("TUN device stopped")
	return nil
}

// ReadPacket reads a packet from the TUN device
func (m *TUNManager) ReadPacket() ([]byte, error) {
	select {
	case pkt := <-m.packetCh:
		return pkt, nil
	case <-m.ctx.Done():
		return nil, m.ctx.Err()
	}
}

// WritePacket writes a packet to the TUN device
func (m *TUNManager) WritePacket(pkt []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if !m.running {
		return fmt.Errorf("TUN device not running")
	}
	
	_, err := m.device.Write(pkt)
	return err
}

// readLoop reads packets from the TUN device
func (m *TUNManager) readLoop() {
	defer m.readWG.Done()
	
	buf := make([]byte, m.device.MTU()+100)
	
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
			n, err := m.device.Read(buf)
			if err != nil {
				select {
				case <-m.ctx.Done():
					return
				default:
					m.logger.Error("TUN read error", zap.Error(err))
					time.Sleep(100 * time.Millisecond)
				}
				continue
			}
			
			if n > 0 {
				pkt := make([]byte, n)
				copy(pkt, buf[:n])
				
				select {
				case m.packetCh <- pkt:
				case <-m.ctx.Done():
					return
				default:
					// Channel full, drop packet
					m.logger.Warn("TUN packet channel full, dropping packet")
				}
			}
		}
	}
}

// DeviceName returns the TUN device name
func (m *TUNManager) DeviceName() string {
	if m.device != nil {
		return m.device.Name()
	}
	return m.cfg.Name
}

// createTUNDevice creates a platform-specific TUN device
func createTUNDevice(name string, mtu int) (TUNDevice, error) {
	switch runtime.GOOS {
	case "linux":
		return newLinuxTUN(name, mtu)
	case "darwin":
		return newDarwinTUN(name, mtu)
	case "windows":
		return newWindowsTUN(name, mtu)
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// Platform-specific implementations

// linuxTUN implements TUNDevice for Linux
type linuxTUN struct {
	fd   int
	name string
	mtu  int
}

func newLinuxTUN(name string, mtu int) (TUNDevice, error) {
	// Open /dev/net/tun
	fd, err := syscall.Open("/dev/net/tun", syscall.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open /dev/net/tun: %w", err)
	}
	
	// Configure TUN device using ioctl
	// This is a simplified version - full implementation would use ioctl
	// to set IFF_TUN | IFF_NO_PI flags
	
	return &linuxTUN{fd: fd, name: name, mtu: mtu}, nil
}

func (t *linuxTUN) Name() string { return t.name }
func (t *linuxTUN) MTU() int { return t.mtu }
func (t *linuxTUN) Read(b []byte) (int, error) {
	return syscall.Read(t.fd, b)
}
func (t *linuxTUN) Write(b []byte) (int, error) {
	return syscall.Write(t.fd, b)
}
func (t *linuxTUN) Close() error {
	return syscall.Close(t.fd)
}
func (t *linuxTUN) Configure(addresses []string, routes []string, dnsServers []string) error {
	// On Linux, configuration is typically done via iproute2 commands
	// This would execute: ip addr add, ip route add, etc.
	return nil
}

// darwinTUN implements TUNDevice for macOS
type darwinTUN struct {
	fd   int
	name string
	mtu  int
}

func newDarwinTUN(name string, mtu int) (TUNDevice, error) {
	// On macOS, use utun interface
	// This is a simplified version
	return &darwinTUN{name: name, mtu: mtu}, nil
}

func (t *darwinTUN) Name() string { return t.name }
func (t *darwinTUN) MTU() int { return t.mtu }
func (t *darwinTUN) Read(b []byte) (int, error) {
	// Read from utun
	return 0, nil
}
func (t *darwinTUN) Write(b []byte) (int, error) {
	// Write to utun
	return 0, nil
}
func (t *darwinTUN) Close() error {
	return nil
}
func (t *darwinTUN) Configure(addresses []string, routes []string, dnsServers []string) error {
	// On macOS, use networksetup or ifconfig
	return nil
}

// windowsTUN implements TUNDevice for Windows (Wintun)
type windowsTUN struct {
	session interface{} // Wintun session
	name    string
	mtu     int
}

func newWindowsTUN(name string, mtu int) (TUNDevice, error) {
	// On Windows, use Wintun driver
	// This would require github.com/wintun/wintun-go
	return &windowsTUN{name: name, mtu: mtu}, nil
}

func (t *windowsTUN) Name() string { return t.name }
func (t *windowsTUN) MTU() int { return t.mtu }
func (t *windowsTUN) Read(b []byte) (int, error) {
	// Read from Wintun session
	return 0, nil
}
func (t *windowsTUN) Write(b []byte) (int, error) {
	// Write to Wintun session
	return 0, nil
}
func (t *windowsTUN) Close() error {
	return nil
}
func (t *windowsTUN) Configure(addresses []string, routes []string, dnsServers []string) error {
	// On Windows, use netsh or Wintun API
	return nil
}

// VPNConfig holds VPN configuration
type VPNConfig struct {
	TUN        TUNConfig `yaml:"tun"`
	KillSwitch bool      `yaml:"kill_switch"`
	SplitTunnel bool     `yaml:"split_tunnel"`
}

// VPNClient manages client-side VPN
type VPNClient struct {
	cfg       *VPNConfig
	tunMgr    *TUNManager
	connectIP *CONNECTIPClient
	logger    *zap.Logger
	mu        sync.Mutex
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewVPNClient creates a new VPN client
func NewVPNClient(cfg *VPNConfig, connectIP *CONNECTIPClient, logger *zap.Logger) (*VPNClient, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	tunMgr, err := NewTUNManager(&cfg.TUN, logger)
	if err != nil {
		cancel()
		return nil, err
	}
	
	return &VPNClient{
		cfg:       cfg,
		tunMgr:    tunMgr,
		connectIP: connectIP,
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
	}, nil
}

// Start starts the VPN client
func (v *VPNClient) Start() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	if v.running {
		return nil
	}
	
	// Start TUN manager
	if err := v.tunMgr.Start(); err != nil {
		return err
	}
	
	v.running = true
	
	// Start packet forwarding loops
	go v.forwardFromTUN()
	go v.forwardToTUN()
	
	v.logger.Info("VPN client started")
	return nil
}

// Stop stops the VPN client
func (v *VPNClient) Stop() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	if !v.running {
		return nil
	}
	
	v.cancel()
	
	if err := v.tunMgr.Stop(); err != nil {
		v.logger.Error("Error stopping TUN manager", zap.Error(err))
	}
	
	v.running = false
	v.logger.Info("VPN client stopped")
	return nil
}

// forwardFromTUN reads packets from TUN and sends to CONNECT-IP
func (v *VPNClient) forwardFromTUN() {
	for {
		select {
		case <-v.ctx.Done():
			return
		default:
			pkt, err := v.tunMgr.ReadPacket()
			if err != nil {
				v.logger.Error("TUN read error", zap.Error(err))
				continue
			}
			
			// Send packet via CONNECT-IP
			// This would write to the IPProxyPacketConn
			_ = pkt
		}
	}
}

// forwardToTUN reads packets from CONNECT-IP and writes to TUN
func (v *VPNClient) forwardToTUN() {
	for {
		select {
		case <-v.ctx.Done():
			return
		default:
			// Read from CONNECT-IP packet connection
			// Write to TUN
		}
	}
}

// VPNServer manages server-side VPN
type VPNServer struct {
	cfg     *VPNConfig
	logger  *zap.Logger
	clients map[string]*VPNClientSession
	mu      sync.Mutex
}

// VPNClientSession represents a connected VPN client
type VPNClientSession struct {
	id         string
	assignedIP net.IP
	routes     []RouteAdvertisement
}

// NewVPNServer creates a new VPN server
func NewVPNServer(cfg *VPNConfig, logger *zap.Logger) *VPNServer {
	return &VPNServer{
		cfg:     cfg,
		logger:  logger,
		clients: make(map[string]*VPNClientSession),
	}
}

// AssignAddress assigns an IP address to a client
func (s *VPNServer) AssignAddress(clientID string, ipVersion uint8, prefixLen uint8) (net.IP, error) {
	// In a real implementation, this would manage an IP pool
	// For now, return a dummy address
	if ipVersion == 4 {
		return net.IPv4(10, 100, 0, 1), nil
	}
	return net.IP{0xfd, 0x00, 0x10, 0x00, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, nil
}

// AdvertiseRoutes advertises routes to a client
func (s *VPNServer) AdvertiseRoutes(clientID string, routes []RouteAdvertisement) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if session, ok := s.clients[clientID]; ok {
		session.routes = routes
	}
	return nil
}