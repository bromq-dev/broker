package broker

import (
	"maps"
	"sync"
	"time"

	"github.com/bromq-dev/broker/pkg/packet"
)

// Session represents an MQTT session.
// Sessions persist across connections when Clean Session/Start is false.
type Session struct {
	mu sync.RWMutex

	// Identity
	ClientID string

	// Subscriptions: topic filter -> subscription details
	subscriptions map[string]*sessionSub

	// Pending messages for offline client (QoS 1/2)
	pending []*pendingMsg

	// State
	createdAt int64 // Unix timestamp
	expiresAt int64 // Unix timestamp (0 = never, MQTT 5.0)

	// Current client (nil if disconnected)
	client *Client
}

// sessionSub holds subscription details.
type sessionSub struct {
	QoS               packet.QoS
	NoLocal           bool   // MQTT 5.0
	RetainAsPublished bool   // MQTT 5.0
	RetainHandling    byte   // MQTT 5.0
	SubscriptionID    uint32 // MQTT 5.0
}

// pendingMsg is a message queued for delivery.
type pendingMsg struct {
	publish   *packet.Publish
	timestamp time.Time
}

// newSession creates a new session.
func newSession(clientID string) *Session {
	return &Session{
		ClientID:      clientID,
		subscriptions: make(map[string]*sessionSub),
		createdAt:     time.Now().Unix(),
	}
}

// Subscribe adds or updates a subscription.
func (s *Session) Subscribe(filter string, sub *sessionSub) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.subscriptions[filter] = sub
}

// Unsubscribe removes a subscription.
func (s *Session) Unsubscribe(filter string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.subscriptions[filter]; ok {
		delete(s.subscriptions, filter)
		return true
	}
	return false
}

// GetSubscription returns a subscription by filter.
func (s *Session) GetSubscription(filter string) (*sessionSub, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sub, ok := s.subscriptions[filter]
	return sub, ok
}

// GetAllSubscriptions returns a copy of all subscriptions.
func (s *Session) GetAllSubscriptions() map[string]*sessionSub {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*sessionSub, len(s.subscriptions))
	maps.Copy(result, s.subscriptions)
	return result
}

// QueueMessage queues a message for delivery when the client reconnects.
func (s *Session) QueueMessage(pkt *packet.Publish) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pending = append(s.pending, &pendingMsg{
		publish:   pkt,
		timestamp: time.Now(),
	})
}

// DrainPending returns and clears all pending messages.
func (s *Session) DrainPending() []*packet.Publish {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]*packet.Publish, len(s.pending))
	for i, p := range s.pending {
		result[i] = p.publish
	}
	s.pending = nil
	return result
}

// SetClient associates a client with this session.
func (s *Session) SetClient(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.client = c
}

// GetClient returns the associated client.
func (s *Session) GetClient() *Client {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.client
}

// IsExpired checks if the session has expired.
func (s *Session) IsExpired() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.expiresAt == 0 {
		return false // Never expires
	}
	return time.Now().Unix() > s.expiresAt
}

// SetExpiry sets the session expiry time.
func (s *Session) SetExpiry(seconds uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if seconds == 0 {
		s.expiresAt = 0
		return
	}
	if seconds == 0xFFFFFFFF {
		s.expiresAt = 0 // Never expires
		return
	}
	s.expiresAt = time.Now().Unix() + int64(seconds)
}

// SessionManager manages all sessions.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewSessionManager creates a new session manager.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// Get returns a session by client ID.
func (m *SessionManager) Get(clientID string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.sessions[clientID]
}

// GetOrCreate returns an existing session or creates a new one.
func (m *SessionManager) GetOrCreate(clientID string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sess, ok := m.sessions[clientID]; ok {
		return sess, true // Existing session
	}

	sess := newSession(clientID)
	m.sessions[clientID] = sess
	return sess, false // New session
}

// Create creates a new session, replacing any existing one.
func (m *SessionManager) Create(clientID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess := newSession(clientID)
	m.sessions[clientID] = sess
	return sess
}

// Delete removes a session.
func (m *SessionManager) Delete(clientID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, clientID)
}

// CleanExpired removes all expired sessions.
func (m *SessionManager) CleanExpired() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var expired []string
	for id, sess := range m.sessions {
		if sess.IsExpired() && sess.GetClient() == nil {
			delete(m.sessions, id)
			expired = append(expired, id)
		}
	}
	return expired
}

// Count returns the number of sessions.
func (m *SessionManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.sessions)
}
