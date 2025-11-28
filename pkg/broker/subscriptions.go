package broker

import (
	"sync"

	"github.com/bromq-dev/broker/pkg/packet"
	"github.com/bromq-dev/broker/pkg/topic"
)

// Subscriber represents a subscription entry.
type Subscriber struct {
	Client            *Client
	QoS               packet.QoS
	NoLocal           bool   // MQTT 5.0: don't receive own messages
	RetainAsPublished bool   // MQTT 5.0: keep original retain flag
	RetainHandling    byte   // MQTT 5.0: retain handling options
	SubscriptionID    uint32 // MQTT 5.0: subscription identifier
	ShareName         string // Non-empty for shared subscriptions
}

// SubscriptionTree is a trie-based structure for efficient topic matching.
// It supports wildcard subscriptions (+, #) and shared subscriptions.
type SubscriptionTree struct {
	mu   sync.RWMutex
	root *trieNode
}

// trieNode represents a node in the subscription trie.
type trieNode struct {
	children    map[string]*trieNode
	subscribers map[*Client]*Subscriber // Direct subscribers at this level
	shared      map[string]*sharedGroup // Shared subscription groups
}

// sharedGroup holds subscribers to a shared subscription.
type sharedGroup struct {
	subscribers []*Subscriber
	nextIdx     int // Round-robin index
	mu          sync.Mutex
}

// NewSubscriptionTree creates a new subscription tree.
func NewSubscriptionTree() *SubscriptionTree {
	return &SubscriptionTree{
		root: newTrieNode(),
	}
}

func newTrieNode() *trieNode {
	return &trieNode{
		children:    make(map[string]*trieNode),
		subscribers: make(map[*Client]*Subscriber),
		shared:      make(map[string]*sharedGroup),
	}
}

// Subscribe adds a subscription to the tree.
func (t *SubscriptionTree) Subscribe(filter string, sub *Subscriber) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Check for shared subscription
	shareName, actualFilter, isShared := topic.IsShared(filter)
	if isShared {
		sub.ShareName = shareName
		filter = actualFilter
	}

	levels := topic.Levels(filter)
	node := t.root

	for _, level := range levels {
		child, ok := node.children[level]
		if !ok {
			child = newTrieNode()
			node.children[level] = child
		}
		node = child
	}

	if isShared {
		group, ok := node.shared[shareName]
		if !ok {
			group = &sharedGroup{}
			node.shared[shareName] = group
		}
		// Remove existing subscription from same client
		for i, s := range group.subscribers {
			if s.Client == sub.Client {
				group.subscribers = append(group.subscribers[:i], group.subscribers[i+1:]...)
				break
			}
		}
		group.subscribers = append(group.subscribers, sub)
	} else {
		node.subscribers[sub.Client] = sub
	}
}

// Unsubscribe removes a subscription from the tree.
func (t *SubscriptionTree) Unsubscribe(filter string, client *Client) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	shareName, actualFilter, isShared := topic.IsShared(filter)
	if isShared {
		filter = actualFilter
	}

	levels := topic.Levels(filter)
	node := t.root

	for _, level := range levels {
		child, ok := node.children[level]
		if !ok {
			return false
		}
		node = child
	}

	if isShared {
		group, ok := node.shared[shareName]
		if !ok {
			return false
		}
		for i, s := range group.subscribers {
			if s.Client == client {
				group.subscribers = append(group.subscribers[:i], group.subscribers[i+1:]...)
				if len(group.subscribers) == 0 {
					delete(node.shared, shareName)
				}
				return true
			}
		}
		return false
	}

	if _, ok := node.subscribers[client]; ok {
		delete(node.subscribers, client)
		return true
	}
	return false
}

// UnsubscribeAll removes all subscriptions for a client.
func (t *SubscriptionTree) UnsubscribeAll(client *Client) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.unsubscribeAllRecursive(t.root, client)
}

func (t *SubscriptionTree) unsubscribeAllRecursive(node *trieNode, client *Client) {
	delete(node.subscribers, client)

	for name, group := range node.shared {
		for i, s := range group.subscribers {
			if s.Client == client {
				group.subscribers = append(group.subscribers[:i], group.subscribers[i+1:]...)
				break
			}
		}
		if len(group.subscribers) == 0 {
			delete(node.shared, name)
		}
	}

	for _, child := range node.children {
		t.unsubscribeAllRecursive(child, client)
	}
}

// Match returns all subscribers that match a topic name.
// For shared subscriptions, only one subscriber per group is returned (round-robin).
func (t *SubscriptionTree) Match(topicName string) []*Subscriber {
	t.mu.RLock()
	defer t.mu.RUnlock()

	levels := topic.Levels(topicName)
	var result []*Subscriber
	seen := make(map[*Client]bool)

	// Check if topic starts with $ (system topic)
	isSysTopic := topic.IsSysTopic(topicName)

	t.matchRecursive(t.root, levels, 0, isSysTopic, &result, seen)

	return result
}

func (t *SubscriptionTree) matchRecursive(node *trieNode, levels []string, idx int, isSysTopic bool, result *[]*Subscriber, seen map[*Client]bool) {
	if idx == len(levels) {
		// Add direct subscribers
		for _, sub := range node.subscribers {
			if !seen[sub.Client] {
				seen[sub.Client] = true
				*result = append(*result, sub)
			}
		}

		// Add one subscriber from each shared group (round-robin)
		for _, group := range node.shared {
			if len(group.subscribers) > 0 {
				group.mu.Lock()
				sub := group.subscribers[group.nextIdx%len(group.subscribers)]
				group.nextIdx++
				group.mu.Unlock()

				if !seen[sub.Client] {
					seen[sub.Client] = true
					*result = append(*result, sub)
				}
			}
		}

		// Check for # wildcard at this level
		if hashNode, ok := node.children["#"]; ok {
			for _, sub := range hashNode.subscribers {
				if !seen[sub.Client] {
					seen[sub.Client] = true
					*result = append(*result, sub)
				}
			}
			for _, group := range hashNode.shared {
				if len(group.subscribers) > 0 {
					group.mu.Lock()
					sub := group.subscribers[group.nextIdx%len(group.subscribers)]
					group.nextIdx++
					group.mu.Unlock()

					if !seen[sub.Client] {
						seen[sub.Client] = true
						*result = append(*result, sub)
					}
				}
			}
		}
		return
	}

	level := levels[idx]

	// Exact match
	if child, ok := node.children[level]; ok {
		t.matchRecursive(child, levels, idx+1, isSysTopic, result, seen)
	}

	// System topics don't match wildcards at the first level
	if isSysTopic && idx == 0 {
		return
	}

	// Single-level wildcard (+)
	if plusNode, ok := node.children["+"]; ok {
		t.matchRecursive(plusNode, levels, idx+1, isSysTopic, result, seen)
	}

	// Multi-level wildcard (#)
	if hashNode, ok := node.children["#"]; ok {
		for _, sub := range hashNode.subscribers {
			if !seen[sub.Client] {
				seen[sub.Client] = true
				*result = append(*result, sub)
			}
		}
		for _, group := range hashNode.shared {
			if len(group.subscribers) > 0 {
				group.mu.Lock()
				sub := group.subscribers[group.nextIdx%len(group.subscribers)]
				group.nextIdx++
				group.mu.Unlock()

				if !seen[sub.Client] {
					seen[sub.Client] = true
					*result = append(*result, sub)
				}
			}
		}
	}
}

// Count returns the total number of subscriptions.
func (t *SubscriptionTree) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.countRecursive(t.root)
}

func (t *SubscriptionTree) countRecursive(node *trieNode) int {
	count := len(node.subscribers)
	for _, group := range node.shared {
		count += len(group.subscribers)
	}
	for _, child := range node.children {
		count += t.countRecursive(child)
	}
	return count
}
