// Package broker provides the core MQTT broker functionality.
package broker

import (
	"context"

	"github.com/bromq-dev/broker/pkg/packet"
)

// Hook provides extension points for customizing broker behavior.
// Implementations can intercept and modify packet handling at various stages.
// All methods are optional - return nil/zero values to use default behavior.
//
// Hook methods are called synchronously. For long-running operations,
// implementations should spawn goroutines internally.
type Hook interface {
	// ID returns a unique identifier for this hook.
	ID() string
}

// AuthHook handles client authentication.
type AuthHook interface {
	Hook

	// OnConnect is called when a client sends a CONNECT packet.
	// Return nil to accept the connection, or an error to reject.
	// For MQTT 5.0, return a ReasonCode error to send specific reason code.
	OnConnect(ctx context.Context, client ClientInfo, pkt *packet.Connect) error
}

// AuthzHook handles client authorization for operations.
type AuthzHook interface {
	Hook

	// OnSubscribe is called before processing a SUBSCRIBE packet.
	// Return modified subscriptions or nil to use original.
	// Return error to reject the entire subscription.
	OnSubscribe(ctx context.Context, client ClientInfo, subs []packet.Subscription) ([]packet.Subscription, error)

	// OnPublish is called before processing a PUBLISH packet.
	// Return nil to allow, error to reject.
	OnPublish(ctx context.Context, client ClientInfo, pkt *packet.Publish) error

	// CanRead checks if a client can receive messages on a topic.
	// Called during message delivery. Return false to skip delivery.
	CanRead(ctx context.Context, client ClientInfo, topic string) bool
}

// SessionHook handles session lifecycle events.
type SessionHook interface {
	Hook

	// OnSessionCreated is called when a new session is created.
	OnSessionCreated(ctx context.Context, client ClientInfo)

	// OnSessionResumed is called when an existing session is resumed.
	OnSessionResumed(ctx context.Context, client ClientInfo)

	// OnSessionEnded is called when a session ends (expires or cleaned).
	OnSessionEnded(ctx context.Context, clientID string)
}

// MessageHook handles message interception and transformation.
type MessageHook interface {
	Hook

	// OnPublishReceived is called when a PUBLISH is received from a client.
	// Return modified packet or nil to use original.
	// Return error to reject the publish.
	OnPublishReceived(ctx context.Context, client ClientInfo, pkt *packet.Publish) (*packet.Publish, error)

	// OnPublishDeliver is called before delivering a message to a subscriber.
	// Return modified packet or nil to use original.
	// Return error to skip delivery to this subscriber.
	OnPublishDeliver(ctx context.Context, subscriber ClientInfo, pkt *packet.Publish) (*packet.Publish, error)
}

// ConnectionHook handles connection lifecycle events.
type ConnectionHook interface {
	Hook

	// OnConnected is called after a client successfully connects.
	OnConnected(ctx context.Context, client ClientInfo)

	// OnDisconnect is called when a client disconnects.
	OnDisconnect(ctx context.Context, client ClientInfo, err error)
}

// WillHook handles will message processing.
type WillHook interface {
	Hook

	// OnWillPublish is called before publishing a will message.
	// Return modified will or nil to use original.
	// Return error to prevent will publication.
	OnWillPublish(ctx context.Context, clientID string, will *packet.Publish) (*packet.Publish, error)
}

// RetainHook handles retained message storage.
type RetainHook interface {
	Hook

	// StoreRetained stores a retained message for a topic.
	// Pass nil payload to delete the retained message.
	StoreRetained(ctx context.Context, topic string, pkt *packet.Publish) error

	// GetRetained retrieves retained messages matching a filter.
	GetRetained(ctx context.Context, filter string) ([]*packet.Publish, error)
}

// PersistenceHook handles session and message persistence.
type PersistenceHook interface {
	Hook

	// SaveSession persists session state.
	SaveSession(ctx context.Context, session *SessionState) error

	// LoadSession loads session state.
	LoadSession(ctx context.Context, clientID string) (*SessionState, error)

	// DeleteSession removes session state.
	DeleteSession(ctx context.Context, clientID string) error

	// SaveInflight saves an inflight message.
	SaveInflight(ctx context.Context, clientID string, pkt *packet.Publish) error

	// LoadInflight loads all inflight messages for a client.
	LoadInflight(ctx context.Context, clientID string) ([]*packet.Publish, error)

	// DeleteInflight removes an inflight message.
	DeleteInflight(ctx context.Context, clientID string, packetID uint16) error
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
