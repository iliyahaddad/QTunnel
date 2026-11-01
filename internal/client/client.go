package client

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qtunnel/qtunnel/internal/auth"
	"github.com/qtunnel/qtunnel/internal/carrier"
	"github.com/qtunnel/qtunnel/internal/carrier/http2"
	"github.com/qtunnel/qtunnel/internal/carrier/http3"
	"github.com/qtunnel/qtunnel/internal/carrier/quic"
	"github.com/qtunnel/qtunnel/internal/config"
	"github.com/qtunnel/qtunnel/internal/frame"
	"github.com/qtunnel/qtunnel/internal/observability"
	"github.com/qtunnel/qtunnel/internal/proxy"
	"github.com/qtunnel/qtunnel/internal/session"
	"go.uber.org/zap"
)

type Client struct {
	cfg         *config.Config
	logger      *zap.Logger
	metrics     *observability.Metrics
	carrier     carrier.Carrier
	tokenMgr    *auth.TokenManager
	proxyServer *proxy.TCPProxyServer
	sessionMgr  *session.SessionManager
	
	conn        carrier.Connection
	session     *session.Session
	connMu      sync.Mutex
	
	reconnectCh chan struct{}
	stopOnce    sync.Once
	stopped     atomic.Bool
}

func New(cfg *config.Config, logger *zap.Logger, metrics *observability.Metrics) *Client {
	limits, _ := cfg.GetParsedLimits()
	
	var cr carrier.Carrier
	switch cfg.Transport.Type {
	case config.TransportHTTP3:
		cr = http3.NewCarrier(cfg.Transport.HTTP3, logger)
	case config.TransportHTTP2:
		cr = http2.NewCarrier(cfg.Transport.HTTP2, logger)
	default:
		cr = quic.NewCarrier(cfg.Transport.QUIC, logger)
	}
	
	tokenMgr, _ := auth.NewTokenManager(cfg.Auth.Tokens, cfg.Auth.TokenFile, logger)
	
	sessionMgr := session.NewSessionManager(limits, logger, metrics)
	
	proxyServer := proxy.NewTCPProxyServer(cfg.Proxy.Listen, logger, metrics)
	
	return &Client{
		cfg:         cfg,
		logger:      logger,
		metrics:     metrics,
		carrier:     cr,
		tokenMgr:    tokenMgr,
		proxyServer: proxyServer,
		sessionMgr:  sessionMgr,
		reconnectCh: make(chan struct{}, 1),
	}
}

func (c *Client) Run(ctx context.Context) error {
	c.logger.Info("Starting client", zap.String("server", c.cfg.Server.Addr))
	
	// Start proxy server
	if err := c.proxyServer.Start(); err != nil {
		return err
	}
	
	// Start connection manager
	go c.connectionManager(ctx)
	
	// Trigger initial connection
	c.scheduleReconnect()
	
	// Wait for context cancellation
	<-ctx.Done()
	c.logger.Info("Client context cancelled")
	return nil
}

func (c *Client) connectionManager(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.reconnectCh:
			if c.stopped.Load() {
				return
			}
			c.connect(ctx)
		}
	}
}

func (c *Client) connect(ctx context.Context) {
	backoff := time.Second
	maxBackoff := 30 * time.Second
	
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		
		c.logger.Debug("Attempting to connect to server")
		
		transportName := c.carrier.Name()
		
		conn, err := c.carrier.Dial(ctx, c.cfg.Server.Addr, c.cfg.Server.SNI, c.cfg.Server.InsecureSkipVerify)
		if err != nil {
			c.logger.Warn("Connection failed, retrying", zap.Error(err), zap.Duration("backoff", backoff))
			if c.metrics != nil {
				c.metrics.RecordConnectionOpened(transportName, false)
			}
			time.Sleep(backoff)
			backoff = minDuration(backoff*2, maxBackoff)
			continue
		}
		
		c.logger.Info("Connected to server", zap.String("remote", conn.RemoteAddr().String()))
		
		if c.metrics != nil {
			c.metrics.RecordConnectionOpened(transportName, true)
			c.metrics.SetActiveConnections(transportName, 1)
		}
		
		// Create session
		limits, _ := c.cfg.GetParsedLimits()
		sess := session.NewSession("client-session", conn, limits, c.logger, c.metrics)
		
		// Perform authentication
		if err := c.authenticate(ctx, sess); err != nil {
			c.logger.Error("Authentication failed", zap.Error(err))
			conn.Close()
			if c.metrics != nil {
				c.metrics.RecordConnectionClosed(transportName, "auth_failed")
			}
			time.Sleep(backoff)
			backoff = minDuration(backoff*2, maxBackoff)
			continue
		}
		
		sess.SetState(session.StateActive)
		
		// Store connection and session
		c.connMu.Lock()
		c.conn = conn
		c.session = sess
		c.connMu.Unlock()
		
		// Update proxy server with new session
		if c.proxyServer != nil {
			c.proxyServer.SetSession(sess)
		}
		
		// Start stream handler
		go c.handleStreams(ctx, sess)
		
		// Wait for session to end
		<-sess.Done()
		
		c.logger.Info("Session ended, reconnecting...")
		
		// Clear proxy server session
		if c.proxyServer != nil {
			c.proxyServer.SetSession(nil)
		}
		
		if c.metrics != nil {
			c.metrics.SetActiveConnections(transportName, 0)
			c.metrics.RecordConnectionClosed(transportName, "session_ended")
		}
		
		// Trigger reconnect
		c.scheduleReconnect()
		return
	}
}

func (c *Client) authenticate(ctx context.Context, sess *session.Session) error {
	token := c.getToken()
	if token == "" {
		return nil // No token required (mTLS)
	}
	
	// Send AUTH frame
	authFrame := frame.NewAuthFrame(token)
	stream, err := sess.OpenStream(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()
	
	if err := authFrame.EncodeTo(stream); err != nil {
		return err
	}
	
	// Read response
	respFrame, err := frame.DecodeFrom(stream)
	if err != nil {
		return err
	}
	
	switch respFrame.Type {
	case frame.FrameTypeAuthOK:
		c.logger.Debug("Authentication successful")
		return nil
	case frame.FrameTypeAuthFail:
		msg, _ := frame.ParseAuthFailPayload(respFrame.Payload)
		return errors.New("auth failed: " + msg)
	default:
		return errors.New("unexpected auth response")
	}
}

func (c *Client) getToken() string {
	// For now, use first token from config
	if len(c.cfg.Auth.Tokens) > 0 {
		return c.cfg.Auth.Tokens[0]
	}
	return ""
}

func (c *Client) handleStreams(ctx context.Context, sess *session.Session) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-sess.Done():
			return
		default:
			// Accept incoming streams from server (for reverse connections if needed)
			stream, err := sess.AcceptStream(ctx)
			if err != nil {
				if ctx.Err() != nil || sess.State() != session.StateActive {
					return
				}
				c.logger.Warn("Accept stream error", zap.Error(err))
				continue
			}
			
			// Handle incoming stream (read first frame to determine type)
			go c.handleIncomingStream(stream)
		}
	}
}

func (c *Client) handleIncomingStream(strm carrier.Stream) {
	// Read first frame
	// For now, we only handle outbound connections from proxy
	strm.Close()
}

func (c *Client) scheduleReconnect() {
	select {
	case c.reconnectCh <- struct{}{}:
	default:
		// Channel full, reconnect already scheduled
	}
}

func (c *Client) Shutdown(ctx context.Context) error {
	c.stopOnce.Do(func() {
		c.stopped.Store(true)
		close(c.reconnectCh)
	})
	
	c.connMu.Lock()
	if c.session != nil {
		c.session.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
	c.connMu.Unlock()
	
	if c.proxyServer != nil {
		c.proxyServer.Stop()
	}
	
	if c.tokenMgr != nil {
		c.tokenMgr.Close()
	}
	
	return nil
}

// GetSession returns the current session for proxy use
func (c *Client) GetSession() *session.Session {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	return c.session
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}