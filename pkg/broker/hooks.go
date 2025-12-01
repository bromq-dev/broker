package broker

import (
	"context"
	"log/slog"
	"sync"

	"github.com/bromq-dev/broker/pkg/packet"
)

// Hooks manages registered hooks and dispatches events.
type Hooks struct {
	mu  sync.RWMutex
	all []Hook

	// Cached lists of hooks that provide specific events
	onConnect         []Hook
	onConnected       []Hook
	onDisconnect      []Hook
	onSubscribe       []Hook
	onPublish         []Hook
	onPublishReceived []Hook
	onPublishDeliver  []Hook
	onMessageDropped  []Hook
	onSessionCreated  []Hook
	onSessionResumed  []Hook
	onSessionEnded    []Hook
	onWillPublish     []Hook
	storeRetained     []Hook
	getRetained       []Hook
}

// NewHooks creates a new hook manager.
func NewHooks() *Hooks {
	return &Hooks{}
}

// Register registers a hook. The hook is categorized by which events it provides.
// Init is NOT called here - it's called by the broker after registration.
func (h *Hooks) Register(hook Hook) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.all = append(h.all, hook)

	// Cache hooks by event type they provide
	if hook.Provides(OnConnectEvent) {
		h.onConnect = append(h.onConnect, hook)
	}
	if hook.Provides(OnConnectedEvent) {
		h.onConnected = append(h.onConnected, hook)
	}
	if hook.Provides(OnDisconnectEvent) {
		h.onDisconnect = append(h.onDisconnect, hook)
	}
	if hook.Provides(OnSubscribeEvent) {
		h.onSubscribe = append(h.onSubscribe, hook)
	}
	if hook.Provides(OnPublishEvent) {
		h.onPublish = append(h.onPublish, hook)
	}
	if hook.Provides(OnPublishReceivedEvent) {
		h.onPublishReceived = append(h.onPublishReceived, hook)
	}
	if hook.Provides(OnPublishDeliverEvent) {
		h.onPublishDeliver = append(h.onPublishDeliver, hook)
	}
	if hook.Provides(OnMessageDroppedEvent) {
		h.onMessageDropped = append(h.onMessageDropped, hook)
	}
	if hook.Provides(OnSessionCreatedEvent) {
		h.onSessionCreated = append(h.onSessionCreated, hook)
	}
	if hook.Provides(OnSessionResumedEvent) {
		h.onSessionResumed = append(h.onSessionResumed, hook)
	}
	if hook.Provides(OnSessionEndedEvent) {
		h.onSessionEnded = append(h.onSessionEnded, hook)
	}
	if hook.Provides(OnWillPublishEvent) {
		h.onWillPublish = append(h.onWillPublish, hook)
	}
	if hook.Provides(StoreRetainedEvent) {
		h.storeRetained = append(h.storeRetained, hook)
	}
	if hook.Provides(GetRetainedEvent) {
		h.getRetained = append(h.getRetained, hook)
	}
}

// StopAll calls Stop on all registered hooks.
func (h *Hooks) StopAll() {
	h.mu.RLock()
	hooks := h.all
	h.mu.RUnlock()

	for _, hook := range hooks {
		hook.Stop()
	}
}

// OnConnect calls all hooks that provide OnConnectEvent.
// Returns error if any hook rejects the connection.
func (h *Hooks) OnConnect(ctx context.Context, client ClientInfo, pkt *packet.Connect) error {
	h.mu.RLock()
	hooks := h.onConnect
	h.mu.RUnlock()

	for _, hook := range hooks {
		if err := hook.OnConnect(ctx, client, pkt); err != nil {
			return err
		}
	}
	return nil
}

// OnConnected notifies all hooks that provide OnConnectedEvent.
func (h *Hooks) OnConnected(ctx context.Context, client ClientInfo) {
	h.mu.RLock()
	hooks := h.onConnected
	h.mu.RUnlock()

	for _, hook := range hooks {
		hook.OnConnected(ctx, client)
	}
}

// OnDisconnect notifies all hooks that provide OnDisconnectEvent.
func (h *Hooks) OnDisconnect(ctx context.Context, client ClientInfo, err error) {
	h.mu.RLock()
	hooks := h.onDisconnect
	h.mu.RUnlock()

	for _, hook := range hooks {
		hook.OnDisconnect(ctx, client, err)
	}
}

// OnSubscribe calls all hooks that provide OnSubscribeEvent.
// Returns the final (possibly modified) subscriptions.
func (h *Hooks) OnSubscribe(ctx context.Context, client ClientInfo, subs []packet.Subscription) ([]packet.Subscription, error) {
	h.mu.RLock()
	hooks := h.onSubscribe
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

// OnPublish calls all hooks that provide OnPublishEvent for publish authorization.
func (h *Hooks) OnPublish(ctx context.Context, client ClientInfo, pkt *packet.Publish) error {
	h.mu.RLock()
	hooks := h.onPublish
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
	hooks := h.onSubscribe // Use subscribe hooks for read authorization
	h.mu.RUnlock()

	for _, hook := range hooks {
		if !hook.CanRead(ctx, client, topic) {
			return false
		}
	}
	return true
}

// OnPublishReceived calls all hooks that provide OnPublishReceivedEvent.
func (h *Hooks) OnPublishReceived(ctx context.Context, client ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	h.mu.RLock()
	hooks := h.onPublishReceived
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

// OnPublishDeliver calls all hooks that provide OnPublishDeliverEvent.
func (h *Hooks) OnPublishDeliver(ctx context.Context, subscriber ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	h.mu.RLock()
	hooks := h.onPublishDeliver
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

// OnMessageDropped notifies all hooks that provide OnMessageDroppedEvent.
func (h *Hooks) OnMessageDropped(client ClientInfo, pkt *packet.Publish, reason DropReason) {
	h.mu.RLock()
	hooks := h.onMessageDropped
	h.mu.RUnlock()

	for _, hook := range hooks {
		hook.OnMessageDropped(client, pkt, reason)
	}
}

// OnSessionCreated notifies all hooks that provide OnSessionCreatedEvent.
func (h *Hooks) OnSessionCreated(ctx context.Context, client ClientInfo) {
	h.mu.RLock()
	hooks := h.onSessionCreated
	h.mu.RUnlock()

	for _, hook := range hooks {
		hook.OnSessionCreated(ctx, client)
	}
}

// OnSessionResumed notifies all hooks that provide OnSessionResumedEvent.
func (h *Hooks) OnSessionResumed(ctx context.Context, client ClientInfo) {
	h.mu.RLock()
	hooks := h.onSessionResumed
	h.mu.RUnlock()

	for _, hook := range hooks {
		hook.OnSessionResumed(ctx, client)
	}
}

// OnSessionEnded notifies all hooks that provide OnSessionEndedEvent.
func (h *Hooks) OnSessionEnded(ctx context.Context, clientID string) {
	h.mu.RLock()
	hooks := h.onSessionEnded
	h.mu.RUnlock()

	for _, hook := range hooks {
		hook.OnSessionEnded(ctx, clientID)
	}
}

// OnWillPublish calls all hooks that provide OnWillPublishEvent.
func (h *Hooks) OnWillPublish(ctx context.Context, clientID string, will *packet.Publish) (*packet.Publish, error) {
	h.mu.RLock()
	hooks := h.onWillPublish
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

// StoreRetained stores a retained message (delegates to first hook that provides it).
func (h *Hooks) StoreRetained(ctx context.Context, topic string, pkt *packet.Publish) error {
	h.mu.RLock()
	hooks := h.storeRetained
	h.mu.RUnlock()

	for _, hook := range hooks {
		if err := hook.StoreRetained(ctx, topic, pkt); err != nil {
			return err
		}
		return nil // Only use first provider
	}
	return ErrNoRetainHook
}

// GetRetained retrieves retained messages (delegates to first hook that provides it).
func (h *Hooks) GetRetained(ctx context.Context, filter string) ([]*packet.Publish, error) {
	h.mu.RLock()
	hooks := h.getRetained
	h.mu.RUnlock()

	for _, hook := range hooks {
		return hook.GetRetained(ctx, filter)
	}
	return nil, nil
}

// defaultLog returns a default logger if none provided.
func defaultLog() *slog.Logger {
	return slog.Default()
}
