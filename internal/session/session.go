package session

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qtunnel/qtunnel/internal/carrier"
	"github.com/qtunnel/qtunnel/internal/config"
	"github.com/qtunnel/qtunnel/internal/observability"
	"go.uber.org/zap"
)

var (
	ErrSessionClosed   = errors.New("session closed")
	ErrStreamNotFound  = errors.New("stream not found")
	ErrTooManyStreams  = errors.New("too many streams")
	ErrInvalidStreamID = errors.New("invalid stream ID")
)

// SessionState represents the session lifecycle state
type SessionState int

const (
	StateConnecting SessionState = iota
	StateActive
	StateClosing
	StateClosed
)

func (s SessionState) String() string {
	switch s {
	case StateConnecting:
		return "connecting"
	case StateActive:
		return "active"
	case StateClosing:
		return "closing"
	case StateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// Session manages a client-server session
type Session struct {
	id          string
	state       SessionState
	stateMu     sync.RWMutex
	conn        carrier.Connection
	streams     map[uint64]*sessionStream
	streamsMu   sync.RWMutex
	nextStreamID atomic.Uint64
	limits      *config.ParsedLimits
	logger      *zap.Logger
	metrics     *observability.Metrics
	closeOnce   sync.Once
	closeCh     chan struct{}
	doneCh      chan struct{}
	
	// Flow control
	connWindow     int64
	connWindowMu   sync.Mutex
	connWindowCond *sync.Cond
}

// sessionStream wraps a carrier stream with session management
type sessionStream struct {
	id          uint64
	stream      carrier.Stream
	session     *Session
	readClosed  atomic.Bool
	writeClosed atomic.Bool
	window      int64
	windowMu    sync.Mutex
	windowCond  *sync.Cond
}

func newSessionStream(id uint64, s carrier.Stream, sess *Session, initialWindow int64) *sessionStream {
	ss := &sessionStream{
		id:     id,
		stream: s,
		session: sess,
		window: initialWindow,
	}
	ss.windowCond = sync.NewCond(&ss.windowMu)
	return ss
}

func (ss *sessionStream) ID() uint64 { return ss.id }

func (ss *sessionStream) Read(p []byte) (int, error) {
	n, err := ss.stream.Read(p)
	if n > 0 {
		// Update stream window
		ss.windowMu.Lock()
		ss.window += int64(n)
		ss.windowMu.Unlock()
		ss.windowCond.Signal()
		
		// Update connection window
		ss.session.addConnWindow(int64(n))
	}
	return n, err
}

func (ss *sessionStream) Write(p []byte) (int, error) {
	// Check stream window
	ss.windowMu.Lock()
	for ss.window <= 0 && !ss.writeClosed.Load() {
		ss.windowCond.Wait()
	}
	if ss.writeClosed.Load() {
		ss.windowMu.Unlock()
		return 0, ErrSessionClosed
	}
	if int64(len(p)) > ss.window {
		p = p[:ss.window]
	}
	ss.window -= int64(len(p))
	ss.windowMu.Unlock()

	// Check connection window
	if err := ss.session.waitConnWindow(int64(len(p))); err != nil {
		return 0, err
	}

	return ss.stream.Write(p)
}

func (ss *sessionStream) Close() error {
	if !ss.writeClosed.Swap(true) {
		ss.stream.Close()
		// Remove from session's stream map
		ss.session.RemoveStream(ss.id)
	}
	return nil
}

func (ss *sessionStream) CloseLocked() error {
	if !ss.writeClosed.Swap(true) {
		ss.stream.Close()
		// Remove from session's stream map (caller must hold streamsMu)
		ss.session.RemoveStreamLocked(ss.id)
}
	return nil
}

func (ss *sessionStream) Context() context.Context { return ss.stream.Context() }
func (ss *sessionStream) SetDeadline(t time.Time) error { return ss.stream.SetDeadline(t) }
func (ss *sessionStream) SetReadDeadline(t time.Time) error { return ss.stream.SetReadDeadline(t) }
func (ss *sessionStream) SetWriteDeadline(t time.Time) error { return ss.stream.SetWriteDeadline(t) }
func (ss *sessionStream) LocalAddr() string { return ss.stream.LocalAddr() }
func (ss *sessionStream) RemoteAddr() string { return ss.stream.RemoteAddr() }

// addConnWindow adds to connection window and signals waiters
func (s *Session) addConnWindow(n int64) {
	s.connWindowMu.Lock()
	s.connWindow += n
	s.connWindowCond.Signal()
	s.connWindowMu.Unlock()
}

// waitConnWindow waits for connection window credit
func (s *Session) waitConnWindow(n int64) error {
	s.connWindowMu.Lock()
	defer s.connWindowMu.Unlock()
	for s.connWindow < n {
		select {
		case <-s.closeCh:
			return ErrSessionClosed
		default:
			s.connWindowCond.Wait()
		}
	}
	s.connWindow -= n
	return nil
}

// NewSession creates a new session
func NewSession(id string, conn carrier.Connection, limits *config.ParsedLimits, logger *zap.Logger, metrics *observability.Metrics) *Session {
	s := &Session{
		id:         id,
		state:      StateConnecting,
		conn:       conn,
		streams:    make(map[uint64]*sessionStream),
		limits:     limits,
		logger:     logger,
		metrics:    metrics,
		closeCh:    make(chan struct{}),
		doneCh:     make(chan struct{}),
		connWindow: int64(limits.ConnBufferSize),
	}
	s.connWindowCond = sync.NewCond(&s.connWindowMu)
	return s
}

// ID returns the session ID
func (s *Session) ID() string { return s.id }

// State returns the current session state
func (s *Session) State() SessionState {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.state
}

// SetState updates the session state
func (s *Session) SetState(state SessionState) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.state = state
}

// OpenStream opens a new stream
func (s *Session) OpenStream(ctx context.Context) (carrier.Stream, error) {
	if s.State() != StateActive {
		return nil, ErrSessionClosed
	}

	streamID := s.nextStreamID.Add(1)
	if streamID%2 == 0 {
		// Client-initiated bidirectional streams are even
		if streamID%2 != 0 {
			streamID = s.nextStreamID.Add(1)
		}
	}

	// Check stream limit
	s.streamsMu.RLock()
	streamCount := len(s.streams)
	s.streamsMu.RUnlock()

	if streamCount >= s.limits.MaxStreamsPerConn {
		return nil, ErrTooManyStreams
	}

	carrierStream, err := s.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}

	ss := newSessionStream(streamID, carrierStream, s, int64(s.limits.StreamBufferSize))

	s.streamsMu.Lock()
	s.streams[streamID] = ss
	s.streamsMu.Unlock()

	if s.metrics != nil {
		s.metrics.RecordStreamOpened("outbound", true)
	}

	return ss, nil
}

// AcceptStream accepts an incoming stream
func (s *Session) AcceptStream(ctx context.Context) (carrier.Stream, error) {
	if s.State() != StateActive {
		return nil, ErrSessionClosed
	}

	carrierStream, err := s.conn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}

	// Stream ID comes from carrier
	streamID := carrierStream.ID()
	if streamID == 0 {
		streamID = s.nextStreamID.Add(1)
	}

	// Check stream limit
	s.streamsMu.RLock()
	streamCount := len(s.streams)
	s.streamsMu.RUnlock()

	if streamCount >= s.limits.MaxStreamsPerConn {
		carrierStream.Close()
		return nil, ErrTooManyStreams
	}

	ss := newSessionStream(streamID, carrierStream, s, int64(s.limits.StreamBufferSize))

	s.streamsMu.Lock()
	s.streams[streamID] = ss
	s.streamsMu.Unlock()

	if s.metrics != nil {
		s.metrics.RecordStreamOpened("inbound", true)
	}

	return ss, nil
}

// GetStream returns a stream by ID
func (s *Session) GetStream(id uint64) (carrier.Stream, bool) {
	s.streamsMu.RLock()
	defer s.streamsMu.RUnlock()
	ss, ok := s.streams[id]
	return ss, ok
}

// RemoveStream removes a stream from the session
func (s *Session) RemoveStream(id uint64) {
	s.streamsMu.Lock()
	delete(s.streams, id)
	s.streamsMu.Unlock()

	if s.metrics != nil {
		s.metrics.RecordStreamClosed("bidirectional", "completed")
	}
}

// RemoveStreamLocked removes a stream from the session (caller must hold streamsMu)
func (s *Session) RemoveStreamLocked(id uint64) {
	delete(s.streams, id)

	if s.metrics != nil {
		s.metrics.RecordStreamClosed("bidirectional", "completed")
	}
}

// Close closes the session
func (s *Session) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.SetState(StateClosing)
		close(s.closeCh)

		// Close all streams
		s.streamsMu.Lock()
		for _, ss := range s.streams {
			ss.CloseLocked()
		}
		s.streams = nil
		s.streamsMu.Unlock()

		// Close underlying connection
		err = s.conn.Close()

		s.SetState(StateClosed)
		close(s.doneCh)
	})
	return err
}

// Done returns a channel that closes when session is fully closed
func (s *Session) Done() <-chan struct{} { return s.doneCh }

// Context returns the session context
func (s *Session) Context() context.Context { return s.conn.Context() }

// StreamCount returns the number of active streams
func (s *Session) StreamCount() int {
	s.streamsMu.RLock()
	defer s.streamsMu.RUnlock()
	return len(s.streams)
}

// SessionManager manages multiple sessions
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	limits   *config.ParsedLimits
	logger   *zap.Logger
	metrics  *observability.Metrics
}

func NewSessionManager(limits *config.ParsedLimits, logger *zap.Logger, metrics *observability.Metrics) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		limits:   limits,
		logger:   logger,
		metrics:  metrics,
	}
}

func (sm *SessionManager) AddSession(id string, conn carrier.Connection) *Session {
	sess := NewSession(id, conn, sm.limits, sm.logger, sm.metrics)
	
	sm.mu.Lock()
	// Check connection limit
	if len(sm.sessions) >= sm.limits.MaxConnections {
		sm.mu.Unlock()
		conn.Close()
		return nil
	}
	sm.sessions[id] = sess
	sm.mu.Unlock()

	if sm.metrics != nil {
		sm.metrics.RecordConnectionOpened("quic", true)
	}

	return sess
}

func (sm *SessionManager) GetSession(id string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	sess, ok := sm.sessions[id]
	return sess, ok
}

func (sm *SessionManager) RemoveSession(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, id)

	if sm.metrics != nil {
		sm.metrics.RecordConnectionClosed("quic", "session_ended")
	}
}

func (sm *SessionManager) SessionCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

func (sm *SessionManager) CloseAll() {
	sm.mu.Lock()
	sessions := make([]*Session, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		sessions = append(sessions, s)
	}
	sm.sessions = nil
	sm.mu.Unlock()

	for _, s := range sessions {
		s.Close()
	}
}