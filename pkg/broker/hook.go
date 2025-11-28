// Package broker provides the core MQTT broker functionality.
package broker

import (
	"context"
	"errors"
	"log/slog"

	"github.com/bromq-dev/broker/pkg/packet"
)

// Hook provides extension points for customizing broker behavior.
// Implementations can intercept and modify packet handling at various stages.
//
// Embed HookBase in your hook struct to get default implementations of all methods,
// then override only the methods you need.
type Hook interface {
	// ID returns a unique identifier for this hook.
	ID() string

	// Init is called when the hook is added to the broker.
	// The config parameter is hook-specific configuration (can be nil).
	Init(opts *HookOptions, config any) error

	// Stop is called when the broker is shutting down.
	Stop() error

	// Provides returns true if the hook handles the given event type.
	Provides(event byte) bool

	// Connection lifecycle
	OnConnect(ctx context.Context, client ClientInfo, pkt *packet.Connect) error
	OnConnected(ctx context.Context, client ClientInfo)
	OnDisconnect(ctx context.Context, client ClientInfo, err error)

	// Authorization
	OnSubscribe(ctx context.Context, client ClientInfo, subs []packet.Subscription) ([]packet.Subscription, error)
	OnPublish(ctx context.Context, client ClientInfo, pkt *packet.Publish) error
	CanRead(ctx context.Context, client ClientInfo, topic string) bool

	// Message handling
	OnPublishReceived(ctx context.Context, client ClientInfo, pkt *packet.Publish) (*packet.Publish, error)
	OnPublishDeliver(ctx context.Context, subscriber ClientInfo, pkt *packet.Publish) (*packet.Publish, error)

	// Session lifecycle
	OnSessionCreated(ctx context.Context, client ClientInfo)
	OnSessionResumed(ctx context.Context, client ClientInfo)
	OnSessionEnded(ctx context.Context, clientID string)

	// Will message
	OnWillPublish(ctx context.Context, clientID string, will *packet.Publish) (*packet.Publish, error)

	// Retained messages
	StoreRetained(ctx context.Context, topic string, pkt *packet.Publish) error
	GetRetained(ctx context.Context, filter string) ([]*packet.Publish, error)
}

// ErrNoRetainHook indicates no retain hook is registered.
var ErrNoRetainHook = errors.New("no retain hook registered")

// Event types for Provides().
const (
	OnConnectEvent byte = iota
	OnConnectedEvent
	OnDisconnectEvent
	OnSubscribeEvent
	OnPublishEvent
	OnPublishReceivedEvent
	OnPublishDeliverEvent
	OnSessionCreatedEvent
	OnSessionResumedEvent
	OnSessionEndedEvent
	OnWillPublishEvent
	StoreRetainedEvent
	GetRetainedEvent
)

// HookOptions provides hooks with access to broker capabilities.
type HookOptions struct {
	Broker *Broker
	Log    *slog.Logger
}

// HookBase provides default implementations for all Hook methods.
// Embed this in your hook struct, then override only what you need.
type HookBase struct {
	Opts *HookOptions
	Log  *slog.Logger
}

func (h *HookBase) ID() string                      { return "base" }
func (h *HookBase) Init(opts *HookOptions, _ any) error {
	h.Opts = opts
	h.Log = opts.Log
	return nil
}
func (h *HookBase) Stop() error                     { return nil }
func (h *HookBase) Provides(event byte) bool        { return false }

func (h *HookBase) OnConnect(ctx context.Context, client ClientInfo, pkt *packet.Connect) error {
	return nil
}
func (h *HookBase) OnConnected(ctx context.Context, client ClientInfo)                {}
func (h *HookBase) OnDisconnect(ctx context.Context, client ClientInfo, err error)    {}
func (h *HookBase) OnSubscribe(ctx context.Context, client ClientInfo, subs []packet.Subscription) ([]packet.Subscription, error) {
	return subs, nil
}
func (h *HookBase) OnPublish(ctx context.Context, client ClientInfo, pkt *packet.Publish) error {
	return nil
}
func (h *HookBase) CanRead(ctx context.Context, client ClientInfo, topic string) bool { return true }
func (h *HookBase) OnPublishReceived(ctx context.Context, client ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	return pkt, nil
}
func (h *HookBase) OnPublishDeliver(ctx context.Context, subscriber ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	return pkt, nil
}
func (h *HookBase) OnSessionCreated(ctx context.Context, client ClientInfo) {}
func (h *HookBase) OnSessionResumed(ctx context.Context, client ClientInfo) {}
func (h *HookBase) OnSessionEnded(ctx context.Context, clientID string)     {}
func (h *HookBase) OnWillPublish(ctx context.Context, clientID string, will *packet.Publish) (*packet.Publish, error) {
	return will, nil
}
func (h *HookBase) StoreRetained(ctx context.Context, topic string, pkt *packet.Publish) error {
	return ErrNoRetainHook
}
func (h *HookBase) GetRetained(ctx context.Context, filter string) ([]*packet.Publish, error) {
	return nil, nil
}

// ClientInfo provides read-only information about a connected client.
type ClientInfo interface {
	// ClientID returns the client identifier.
	ClientID() string

	// Username returns the username if provided during connect.
	Username() string

	// RemoteAddr returns the remote address of the client.
	RemoteAddr() string

	// ProtocolVersion returns the MQTT protocol version.
	ProtocolVersion() packet.Version

	// CleanStart returns whether this is a clean session/start.
	CleanStart() bool

	// KeepAlive returns the keep-alive interval in seconds.
	KeepAlive() uint16

	// Properties returns MQTT 5.0 connect properties (nil for v3.1.1).
	Properties() *packet.Properties
}

// SessionState represents persistent session state.
type SessionState struct {
	ClientID      string
	Subscriptions map[string]packet.QoS // topic filter -> QoS
	InflightIn    []uint16              // Packet IDs of incomplete inbound QoS 2
	InflightOut   []uint16              // Packet IDs of incomplete outbound QoS 1/2
	CreatedAt     int64                 // Unix timestamp
	ExpiresAt     int64                 // Unix timestamp (0 = never)
}

// ReasonCodeError is an error that carries an MQTT 5.0 reason code.
type ReasonCodeError struct {
	Code    packet.ReasonCode
	Message string
}

func (e *ReasonCodeError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code.String()
}

// NewReasonCodeError creates a new reason code error.
func NewReasonCodeError(code packet.ReasonCode, msg string) *ReasonCodeError {
	return &ReasonCodeError{Code: code, Message: msg}
}
