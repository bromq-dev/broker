package broker

import (
	"context"
	"errors"
	"sync"

	"github.com/bromq-dev/broker/pkg/packet"
)

// Hooks manages registered hooks and dispatches events.
type Hooks struct {
	mu sync.RWMutex

	auth        []AuthHook
	authz       []AuthzHook
	session     []SessionHook
	message     []MessageHook
	connection  []ConnectionHook
	will        []WillHook
	retain      RetainHook      // Single retain hook (storage backend)
	persistence PersistenceHook // Single persistence hook (storage backend)
}

// NewHooks creates a new hook manager.
func NewHooks() *Hooks {
	return &Hooks{}
}

// Register registers a hook. The hook is checked for all supported interfaces.
func (h *Hooks) Register(hook Hook) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if ah, ok := hook.(AuthHook); ok {
		h.auth = append(h.auth, ah)
	}
	if az, ok := hook.(AuthzHook); ok {
		h.authz = append(h.authz, az)
	}
	if sh, ok := hook.(SessionHook); ok {
		h.session = append(h.session, sh)
	}
	if mh, ok := hook.(MessageHook); ok {
		h.message = append(h.message, mh)
	}
	if ch, ok := hook.(ConnectionHook); ok {
		h.connection = append(h.connection, ch)
	}
	if wh, ok := hook.(WillHook); ok {
		h.will = append(h.will, wh)
	}
	if rh, ok := hook.(RetainHook); ok {
		h.retain = rh
	}
	if ph, ok := hook.(PersistenceHook); ok {
		h.persistence = ph
	}
}

// OnConnect calls all auth hooks for connection authentication.
// Returns error if any hook rejects the connection.
func (h *Hooks) OnConnect(ctx context.Context, client ClientInfo, pkt *packet.Connect) error {
	h.mu.RLock()
	hooks := h.auth
	h.mu.RUnlock()

	for _, hook := range hooks {
		if err := hook.OnConnect(ctx, client, pkt); err != nil {
			return err
		}
	}
	return nil
}

// OnSubscribe calls all authz hooks for subscription authorization.
// Returns the final (possibly modified) subscriptions.
func (h *Hooks) OnSubscribe(ctx context.Context, client ClientInfo, subs []packet.Subscription) ([]packet.Subscription, error) {
	h.mu.RLock()
	hooks := h.authz
	h.mu.RUnlock()

	result := subs
	for _, hook := range hooks {
		modified, err := hook.OnSubscribe(ctx, client, result)
		if err != nil {
			return nil, err
		}
		if modified != nil {
			result = modified
		}
	}
	return result, nil
}

// OnPublish calls all authz hooks for publish authorization.
func (h *Hooks) OnPublish(ctx context.Context, client ClientInfo, pkt *packet.Publish) error {
	h.mu.RLock()
	hooks := h.authz
	h.mu.RUnlock()

	for _, hook := range hooks {
		if err := hook.OnPublish(ctx, client, pkt); err != nil {
			return err
		}
	}
	return nil
}

// CanRead checks if a client can receive messages on a topic.
func (h *Hooks) CanRead(ctx context.Context, client ClientInfo, topic string) bool {
	h.mu.RLock()
	hooks := h.authz
	h.mu.RUnlock()

	for _, hook := range hooks {
		if !hook.CanRead(ctx, client, topic) {
			return false
		}
	}
	return true
}

// OnSessionCreated notifies all session hooks of new session creation.
func (h *Hooks) OnSessionCreated(ctx context.Context, client ClientInfo) {
	h.mu.RLock()
	hooks := h.session
	h.mu.RUnlock()

	for _, hook := range hooks {
		hook.OnSessionCreated(ctx, client)
	}
}

// OnSessionResumed notifies all session hooks of session resumption.
func (h *Hooks) OnSessionResumed(ctx context.Context, client ClientInfo) {
	h.mu.RLock()
	hooks := h.session
	h.mu.RUnlock()

	for _, hook := range hooks {
		hook.OnSessionResumed(ctx, client)
	}
}

// OnSessionEnded notifies all session hooks of session termination.
func (h *Hooks) OnSessionEnded(ctx context.Context, clientID string) {
	h.mu.RLock()
	hooks := h.session
	h.mu.RUnlock()

	for _, hook := range hooks {
		hook.OnSessionEnded(ctx, clientID)
	}
}

// OnPublishReceived calls all message hooks when a PUBLISH is received.
func (h *Hooks) OnPublishReceived(ctx context.Context, client ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	h.mu.RLock()
	hooks := h.message
	h.mu.RUnlock()

	result := pkt
	for _, hook := range hooks {
		modified, err := hook.OnPublishReceived(ctx, client, result)
		if err != nil {
			return nil, err
		}
		if modified != nil {
			result = modified
		}
	}
	return result, nil
}

// OnPublishDeliver calls all message hooks before delivering a message.
func (h *Hooks) OnPublishDeliver(ctx context.Context, subscriber ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	h.mu.RLock()
	hooks := h.message
	h.mu.RUnlock()

	result := pkt
	for _, hook := range hooks {
		modified, err := hook.OnPublishDeliver(ctx, subscriber, result)
		if err != nil {
			return nil, err
		}
		if modified != nil {
			result = modified
		}
	}
	return result, nil
}

// OnConnected notifies all connection hooks of successful connection.
func (h *Hooks) OnConnected(ctx context.Context, client ClientInfo) {
	h.mu.RLock()
	hooks := h.connection
	h.mu.RUnlock()

	for _, hook := range hooks {
		hook.OnConnected(ctx, client)
	}
}

// OnDisconnect notifies all connection hooks of disconnection.
func (h *Hooks) OnDisconnect(ctx context.Context, client ClientInfo, err error) {
	h.mu.RLock()
	hooks := h.connection
	h.mu.RUnlock()

	for _, hook := range hooks {
		hook.OnDisconnect(ctx, client, err)
	}
}

// OnWillPublish calls all will hooks before publishing a will message.
func (h *Hooks) OnWillPublish(ctx context.Context, clientID string, will *packet.Publish) (*packet.Publish, error) {
	h.mu.RLock()
	hooks := h.will
	h.mu.RUnlock()

	result := will
	for _, hook := range hooks {
		modified, err := hook.OnWillPublish(ctx, clientID, result)
		if err != nil {
			return nil, err
		}
		if modified != nil {
			result = modified
		}
	}
	return result, nil
}

// ErrNoRetainHook indicates no retain hook is registered.
var ErrNoRetainHook = errors.New("no retain hook registered")

// StoreRetained stores a retained message (delegates to retain hook).
func (h *Hooks) StoreRetained(ctx context.Context, topic string, pkt *packet.Publish) error {
	h.mu.RLock()
	rh := h.retain
	h.mu.RUnlock()

	if rh != nil {
		return rh.StoreRetained(ctx, topic, pkt)
	}
	return ErrNoRetainHook
}

// GetRetained retrieves retained messages (delegates to retain hook).
func (h *Hooks) GetRetained(ctx context.Context, filter string) ([]*packet.Publish, error) {
	h.mu.RLock()
	rh := h.retain
	h.mu.RUnlock()

	if rh != nil {
		return rh.GetRetained(ctx, filter)
	}
	return nil, nil
}

// SaveSession saves session state (delegates to persistence hook).
func (h *Hooks) SaveSession(ctx context.Context, session *SessionState) error {
	h.mu.RLock()
	ph := h.persistence
	h.mu.RUnlock()

	if ph != nil {
		return ph.SaveSession(ctx, session)
	}
	return nil
}

// LoadSession loads session state (delegates to persistence hook).
func (h *Hooks) LoadSession(ctx context.Context, clientID string) (*SessionState, error) {
	h.mu.RLock()
	ph := h.persistence
	h.mu.RUnlock()

	if ph != nil {
		return ph.LoadSession(ctx, clientID)
	}
	return nil, nil
}

// DeleteSession deletes session state (delegates to persistence hook).
func (h *Hooks) DeleteSession(ctx context.Context, clientID string) error {
	h.mu.RLock()
	ph := h.persistence
	h.mu.RUnlock()

	if ph != nil {
		return ph.DeleteSession(ctx, clientID)
	}
	return nil
}
