package hooks

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/packet"
	"github.com/redis/go-redis/v9"
)

// RedisHook provides distributed state using Redis/Valkey.
// Features:
//   - Retained message storage (survives broker restart)
//   - Cross-node message routing via Redis pub/sub
//   - Session persistence for cluster failover
type RedisHook struct {
	broker.HookBase
	client    *redis.Client
	nodeID    string
	keyPrefix string

	// For cross-node routing
	pubsub *redis.PubSub
	cancel context.CancelFunc
}

// RedisConfig configures the Redis hook.
type RedisConfig struct {
	// Addr is the Redis server address (default: "localhost:6379").
	Addr string

	// Password for Redis authentication (optional).
	Password string

	// DB is the Redis database number (default: 0).
	DB int

	// KeyPrefix is prepended to all Redis keys (default: "mqtt:").
	KeyPrefix string

	// NodeID uniquely identifies this broker node (required for clustering).
	// If empty, cross-node routing is disabled.
	NodeID string

	// Client allows providing a pre-configured Redis client.
	// If set, Addr/Password/DB are ignored.
	Client *redis.Client
}

// retainedMessage is the JSON structure for stored retained messages.
type retainedMessage struct {
	Topic   string `json:"topic"`
	Payload []byte `json:"payload"`
	QoS     byte   `json:"qos"`
}

// clusterMessage is the JSON structure for cross-node pub/sub.
type clusterMessage struct {
	FromNode string `json:"from"`
	Topic    string `json:"topic"`
	Payload  []byte `json:"payload"`
	QoS      byte   `json:"qos"`
	Retain   bool   `json:"retain"`
}

func (h *RedisHook) ID() string { return "redis" }

// Provides indicates which events this hook handles.
func (h *RedisHook) Provides(event byte) bool {
	return event == broker.StoreRetainedEvent ||
		event == broker.GetRetainedEvent ||
		event == broker.OnPublishReceivedEvent
}

// Init connects to Redis and starts the cluster subscription listener.
func (h *RedisHook) Init(opts *broker.HookOptions, config any) error {
	if err := h.HookBase.Init(opts, config); err != nil {
		return err
	}

	// Apply config
	cfg := &RedisConfig{}
	if c, ok := config.(*RedisConfig); ok && c != nil {
		cfg = c
	}

	// Apply defaults
	if cfg.Addr == "" {
		cfg.Addr = "localhost:6379"
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "mqtt:"
	}

	h.keyPrefix = cfg.KeyPrefix
	h.nodeID = cfg.NodeID

	// Use provided client or create new one
	if cfg.Client != nil {
		h.client = cfg.Client
	} else {
		h.client = redis.NewClient(&redis.Options{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			DB:       cfg.DB,
		})
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.client.Ping(ctx).Err(); err != nil {
		return err
	}

	// Start cluster pub/sub listener if nodeID is set
	if h.nodeID != "" {
		h.startClusterListener()
	}

	if h.Log != nil {
		h.Log.Info("redis hook initialized",
			"addr", cfg.Addr,
			"prefix", cfg.KeyPrefix,
			"node_id", cfg.NodeID,
		)
	}

	return nil
}

// Stop closes the Redis connection and stops the cluster listener.
func (h *RedisHook) Stop() error {
	if h.cancel != nil {
		h.cancel()
	}
	if h.pubsub != nil {
		h.pubsub.Close()
	}
	if h.client != nil {
		return h.client.Close()
	}
	return nil
}

// StoreRetained stores a retained message in Redis.
func (h *RedisHook) StoreRetained(ctx context.Context, topic string, pkt *packet.Publish) error {
	key := h.keyPrefix + "retained:" + topic

	// Empty payload = delete retained message
	if len(pkt.Payload) == 0 {
		return h.client.Del(ctx, key).Err()
	}

	msg := retainedMessage{
		Topic:   topic,
		Payload: pkt.Payload,
		QoS:     byte(pkt.QoS),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// Store in hash for efficient pattern matching
	return h.client.HSet(ctx, h.keyPrefix+"retained", topic, data).Err()
}

// GetRetained retrieves retained messages matching a topic filter.
func (h *RedisHook) GetRetained(ctx context.Context, filter string) ([]*packet.Publish, error) {
	// Get all retained messages
	all, err := h.client.HGetAll(ctx, h.keyPrefix+"retained").Result()
	if err != nil {
		return nil, err
	}

	var results []*packet.Publish
	for topic, data := range all {
		if !topicMatchesFilter(topic, filter) {
			continue
		}

		var msg retainedMessage
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			continue
		}

		results = append(results, &packet.Publish{
			TopicName: msg.Topic,
			Payload:   msg.Payload,
			QoS:       packet.QoS(msg.QoS),
			Retain:    true,
		})
	}

	return results, nil
}

// OnPublishReceived forwards messages to other nodes via Redis pub/sub.
func (h *RedisHook) OnPublishReceived(ctx context.Context, client broker.ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	// Only forward if clustering is enabled
	if h.nodeID == "" {
		return pkt, nil
	}

	msg := clusterMessage{
		FromNode: h.nodeID,
		Topic:    pkt.TopicName,
		Payload:  pkt.Payload,
		QoS:      byte(pkt.QoS),
		Retain:   pkt.Retain,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return pkt, nil // Don't fail the publish
	}

	// Publish to Redis channel for other nodes
	h.client.Publish(ctx, h.keyPrefix+"cluster", data)

	return pkt, nil
}

// startClusterListener subscribes to Redis pub/sub for cross-node messages.
func (h *RedisHook) startClusterListener() {
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel

	h.pubsub = h.client.Subscribe(ctx, h.keyPrefix+"cluster")

	go func() {
		ch := h.pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				h.handleClusterMessage(msg.Payload)
			}
		}
	}()
}

// handleClusterMessage processes messages from other nodes.
func (h *RedisHook) handleClusterMessage(data string) {
	var msg clusterMessage
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return
	}

	// Ignore our own messages
	if msg.FromNode == h.nodeID {
		return
	}

	// Inject the message into the local broker
	if h.Opts != nil && h.Opts.Broker != nil {
		h.Opts.Broker.Publish(msg.Topic, msg.Payload, msg.Retain)
	}
}

// topicMatchesFilter checks if a topic matches an MQTT topic filter.
func topicMatchesFilter(topic, filter string) bool {
	if filter == "#" {
		return true
	}

	topicParts := strings.Split(topic, "/")
	filterParts := strings.Split(filter, "/")

	for i, fp := range filterParts {
		if fp == "#" {
			return true
		}
		if i >= len(topicParts) {
			return false
		}
		if fp != "+" && fp != topicParts[i] {
			return false
		}
	}

	return len(topicParts) == len(filterParts)
}

// Client returns the underlying Redis client for advanced usage.
func (h *RedisHook) Client() *redis.Client {
	return h.client
}
