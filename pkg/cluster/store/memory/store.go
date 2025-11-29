// Package memory provides an in-memory Store for single-node operation.
package memory

import (
	"context"
	"sync"

	"github.com/bromq-dev/broker/pkg/cluster/types"
	"github.com/bromq-dev/broker/pkg/packet"
)

// Store implements cluster.Store with in-memory storage.
// Useful for single-node deployments or testing.
type Store struct {
	nodeID string

	mu            sync.RWMutex
	subscriptions map[string]byte          // filter -> qos
	retained      map[string]*packet.Publish // topic -> message
}

// Config configures the memory store.
type Config struct {
	// NodeID is the unique identifier for this node.
	// If empty, defaults to "local".
	NodeID string
}

// NewStore creates a new in-memory store.
func NewStore(cfg *Config) *Store {
	nodeID := "local"
	if cfg != nil && cfg.NodeID != "" {
		nodeID = cfg.NodeID
	}
	return &Store{
		nodeID:        nodeID,
		subscriptions: make(map[string]byte),
		retained:      make(map[string]*packet.Publish),
	}
}

func (s *Store) Start(ctx context.Context) error {
	return nil
}

func (s *Store) Stop() error {
	return nil
}

func (s *Store) NodeID() string {
	return s.nodeID
}

func (s *Store) RegisterNode(ctx context.Context, info types.NodeInfo) error {
	// Single node, nothing to register
	return nil
}

func (s *Store) DeregisterNode(ctx context.Context) error {
	// Single node, nothing to deregister
	return nil
}

func (s *Store) GetNodes(ctx context.Context) ([]types.NodeInfo, error) {
	// Return just this node
	return []types.NodeInfo{{ID: s.nodeID}}, nil
}

func (s *Store) Subscribe(ctx context.Context, filter string, qos byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriptions[filter] = qos
	return nil
}

func (s *Store) Unsubscribe(ctx context.Context, filter string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subscriptions, filter)
	return nil
}

func (s *Store) GetNodesForTopic(ctx context.Context, topic string) ([]types.SubscriptionInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []types.SubscriptionInfo
	for filter, qos := range s.subscriptions {
		if topicMatchesFilter(topic, filter) {
			result = append(result, types.SubscriptionInfo{
				NodeInfo: types.NodeInfo{ID: s.nodeID},
				QoS:      qos,
			})
			break // Only one node (self)
		}
	}
	return result, nil
}

func (s *Store) StoreRetained(ctx context.Context, topic string, msg *packet.Publish) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if msg == nil || len(msg.Payload) == 0 {
		delete(s.retained, topic)
	} else {
		s.retained[topic] = msg
	}
	return nil
}

func (s *Store) GetRetained(ctx context.Context, filter string) ([]*packet.Publish, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*packet.Publish
	for topic, msg := range s.retained {
		if topicMatchesFilter(topic, filter) {
			result = append(result, msg)
		}
	}
	return result, nil
}

// topicMatchesFilter checks if a topic matches an MQTT filter.
func topicMatchesFilter(topic, filter string) bool {
	if filter == "#" {
		return true
	}
	if filter == topic {
		return true
	}

	ti, fi := 0, 0
	for fi < len(filter) {
		if filter[fi] == '#' {
			return true
		}
		if filter[fi] == '+' {
			// Skip to next / in topic
			for ti < len(topic) && topic[ti] != '/' {
				ti++
			}
			fi++
			continue
		}
		if ti >= len(topic) || topic[ti] != filter[fi] {
			return false
		}
		ti++
		fi++
	}
	return ti == len(topic)
}

// Verify interface implementation
var _ types.Store = (*Store)(nil)
