// Package types defines shared types for the cluster package.
package types

import (
	"context"

	"github.com/bromq-dev/broker/pkg/packet"
)

// NodeInfo represents a node in the cluster.
type NodeInfo struct {
	// ID is the unique identifier for this node.
	ID string

	// Addr is the address for routing (e.g., "10.0.0.1:7947").
	Addr string

	// Meta contains optional metadata about the node.
	Meta map[string]string
}

// SubscriptionInfo represents a subscription on a node.
type SubscriptionInfo struct {
	NodeInfo
	QoS byte
}

// Store manages cluster state: subscriptions, nodes, and retained messages.
type Store interface {
	// Start initializes the store.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the store.
	Stop() error

	// NodeID returns this node's identifier.
	NodeID() string

	// RegisterNode registers this node in the cluster.
	RegisterNode(ctx context.Context, info NodeInfo) error

	// DeregisterNode removes this node from the cluster.
	DeregisterNode(ctx context.Context) error

	// GetNodes returns all active nodes in the cluster.
	GetNodes(ctx context.Context) ([]NodeInfo, error)

	// Subscribe registers a subscription on this node.
	Subscribe(ctx context.Context, filter string, qos byte) error

	// Unsubscribe removes a subscription from this node.
	Unsubscribe(ctx context.Context, filter string) error

	// GetNodesForTopic returns nodes with subscriptions matching the topic.
	GetNodesForTopic(ctx context.Context, topic string) ([]SubscriptionInfo, error)

	// StoreRetained stores a retained message.
	StoreRetained(ctx context.Context, topic string, msg *packet.Publish) error

	// GetRetained retrieves retained messages matching the filter.
	GetRetained(ctx context.Context, filter string) ([]*packet.Publish, error)
}

// Router handles message delivery between cluster nodes.
type Router interface {
	// Start initializes the router.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the router.
	Stop() error

	// Send delivers a message to the specified nodes.
	Send(ctx context.Context, nodes []NodeInfo, msg *packet.Publish) error

	// OnReceive sets the handler for messages from other nodes.
	OnReceive(handler func(*packet.Publish))
}
