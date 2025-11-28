package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/packet"
	"github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
)

// ClusterHook provides full distributed state using Redis/Valkey.
//
// Features:
//   - Node registry with heartbeat and auto-expiry
//   - Session persistence across nodes
//   - Subscription storage and route index for targeted fan-out
//   - Offline message queue using Redis Streams
//   - QoS 2 in-flight state tracking
//   - Retained message storage with index for wildcard matching
//   - Per-node pub/sub channels for efficient message routing
//
// Key structure (prefix: "mqtt:"):
//
//	mqtt:node:{nodeID}         → STRING (with TTL) - node registry
//	mqtt:session:{clientID}    → HASH - session state
//	mqtt:subs:{clientID}       → HASH - client subscriptions
//	mqtt:route:{filter}        → SET - nodes with subscribers to filter
//	mqtt:queue:{clientID}      → STREAM - offline message queue
//	mqtt:inflight:{clientID}   → HASH - QoS 2 in-flight state
//	mqtt:retained:{topic}      → STRING - retained message (msgpack)
//	mqtt:retained:_idx         → SET - retained message index
//	mqtt:fanout:{nodeID}       → PUB/SUB channel - per-node messaging
type ClusterHook struct {
	broker.HookBase

	client    redis.UniversalClient
	nodeID    string
	keyPrefix string

	// Node registry
	heartbeatInterval time.Duration
	nodeTTL           time.Duration
	heartbeatCancel   context.CancelFunc

	// Local route cache (filter -> true)
	routeCacheMu sync.RWMutex
	routeCache   map[string]bool

	// Pub/sub
	pubsub *redis.PubSub
	cancel context.CancelFunc
}

// ClusterConfig configures the cluster hook.
type ClusterConfig struct {
	// Addr is the Redis server address (default: "localhost:6379").
	// For Redis Cluster, use comma-separated addresses.
	Addr string

	// Addrs is a list of Redis addresses for cluster mode.
	Addrs []string

	// Password for Redis authentication (optional).
	Password string

	// DB is the Redis database number (default: 0).
	// Ignored in cluster mode.
	DB int

	// KeyPrefix is prepended to all Redis keys (default: "mqtt:").
	KeyPrefix string

	// NodeID uniquely identifies this broker node (required).
	NodeID string

	// HeartbeatInterval is how often to refresh the node TTL (default: 3s).
	HeartbeatInterval time.Duration

	// NodeTTL is how long before a node is considered dead (default: 10s).
	NodeTTL time.Duration

	// Client allows providing a pre-configured Redis client.
	// If set, Addr/Addrs/Password/DB are ignored.
	Client redis.UniversalClient
}

// Session state stored in Redis.
type clusterSession struct {
	Connected   bool   `json:"connected"`
	Node        string `json:"node"`
	Protocol    byte   `json:"protocol"`
	CreatedAt   int64  `json:"created_at"`
	ConnectedAt int64  `json:"connected_at"`
	Expiry      uint32 `json:"expiry"`
	WillTopic   string `json:"will_topic,omitempty"`
	WillPayload []byte `json:"will_payload,omitempty"`
	WillQoS     byte   `json:"will_qos,omitempty"`
	WillRetain  bool   `json:"will_retain,omitempty"`
}

// Subscription options stored in Redis.
type clusterSub struct {
	QoS            byte   `json:"qos"`
	NoLocal        bool   `json:"nl,omitempty"`
	RetainAsPublished bool `json:"rap,omitempty"`
	SubscriptionID uint32 `json:"sid,omitempty"`
}

// Message for inter-node fan-out.
type clusterMsg struct {
	Type    string `json:"type"` // "publish", "disconnect", "will"
	Topic   string `json:"topic,omitempty"`
	Payload []byte `json:"payload,omitempty"`
	QoS     byte   `json:"qos,omitempty"`
	Retain  bool   `json:"retain,omitempty"`
}

// Retained message stored as msgpack.
type retainedMsg struct {
	Payload    []byte            `msgpack:"p"`
	QoS        byte              `msgpack:"q"`
	Properties map[string]string `msgpack:"r,omitempty"`
}

func (h *ClusterHook) ID() string { return "cluster" }

// Provides indicates which events this hook handles.
func (h *ClusterHook) Provides(event byte) bool {
	switch event {
	case broker.OnConnectEvent,
		broker.OnConnectedEvent,
		broker.OnDisconnectEvent,
		broker.OnSubscribeEvent,
		broker.OnPublishReceivedEvent,
		broker.OnSessionCreatedEvent,
		broker.OnSessionResumedEvent,
		broker.OnSessionEndedEvent,
		broker.StoreRetainedEvent,
		broker.GetRetainedEvent:
		return true
	}
	return false
}

// Init connects to Redis and starts background workers.
func (h *ClusterHook) Init(opts *broker.HookOptions, config any) error {
	if err := h.HookBase.Init(opts, config); err != nil {
		return err
	}

	cfg := &ClusterConfig{}
	if c, ok := config.(*ClusterConfig); ok && c != nil {
		cfg = c
	}

	// Validate required config
	if cfg.NodeID == "" {
		return fmt.Errorf("cluster hook requires NodeID")
	}

	// Apply defaults
	if cfg.Addr == "" && len(cfg.Addrs) == 0 {
		cfg.Addr = "localhost:6379"
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "mqtt:"
	}
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 3 * time.Second
	}
	if cfg.NodeTTL == 0 {
		cfg.NodeTTL = 10 * time.Second
	}

	h.keyPrefix = cfg.KeyPrefix
	h.nodeID = cfg.NodeID
	h.heartbeatInterval = cfg.HeartbeatInterval
	h.nodeTTL = cfg.NodeTTL
	h.routeCache = make(map[string]bool)

	// Create Redis client
	if cfg.Client != nil {
		h.client = cfg.Client
	} else if len(cfg.Addrs) > 0 {
		// Cluster mode
		h.client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    cfg.Addrs,
			Password: cfg.Password,
		})
	} else {
		// Single node
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
		return fmt.Errorf("redis connection failed: %w", err)
	}

	// Register node
	if err := h.registerNode(ctx); err != nil {
		return fmt.Errorf("failed to register node: %w", err)
	}

	// Start heartbeat
	h.startHeartbeat()

	// Start fan-out listener
	h.startFanoutListener()

	if h.Log != nil {
		h.Log.Info("cluster hook initialized",
			"node_id", h.nodeID,
			"prefix", h.keyPrefix,
			"heartbeat", h.heartbeatInterval,
			"ttl", h.nodeTTL,
		)
	}

	return nil
}

// Stop gracefully shuts down the cluster hook.
func (h *ClusterHook) Stop() error {
	// Stop heartbeat
	if h.heartbeatCancel != nil {
		h.heartbeatCancel()
	}

	// Stop fan-out listener
	if h.cancel != nil {
		h.cancel()
	}
	if h.pubsub != nil {
		h.pubsub.Close()
	}

	// Unregister node
	if h.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		h.client.Del(ctx, h.nodeKey())
		return h.client.Close()
	}

	return nil
}

// ============================================================================
// Node Registry
// ============================================================================

func (h *ClusterHook) nodeKey() string {
	return h.keyPrefix + "node:" + h.nodeID
}

func (h *ClusterHook) registerNode(ctx context.Context) error {
	data, _ := json.Marshal(map[string]any{
		"started": time.Now().Unix(),
	})
	return h.client.SetEx(ctx, h.nodeKey(), data, h.nodeTTL).Err()
}

func (h *ClusterHook) startHeartbeat() {
	ctx, cancel := context.WithCancel(context.Background())
	h.heartbeatCancel = cancel

	go func() {
		ticker := time.NewTicker(h.heartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.client.Expire(context.Background(), h.nodeKey(), h.nodeTTL)
			}
		}
	}()
}

// ============================================================================
// Session Persistence
// ============================================================================

func (h *ClusterHook) sessionKey(clientID string) string {
	return h.keyPrefix + "session:" + clientID
}

func (h *ClusterHook) subsKey(clientID string) string {
	return h.keyPrefix + "subs:" + clientID
}

// OnConnect stores/updates session state.
func (h *ClusterHook) OnConnect(ctx context.Context, client broker.ClientInfo, pkt *packet.Connect) error {
	sess := clusterSession{
		Connected:   true,
		Node:        h.nodeID,
		Protocol:    byte(client.ProtocolVersion()),
		ConnectedAt: time.Now().Unix(),
	}

	// Store will message if present
	if pkt.WillFlag {
		sess.WillTopic = pkt.WillTopic
		sess.WillPayload = pkt.WillPayload
		sess.WillQoS = byte(pkt.WillQoS)
		sess.WillRetain = pkt.WillRetain
	}

	data, _ := json.Marshal(sess)
	key := h.sessionKey(client.ClientID())

	pipe := h.client.Pipeline()
	pipe.HSet(ctx, key, "data", data)

	// Set TTL if session expiry is set
	if props := pkt.Properties; props != nil && props.SessionExpiry != nil {
		expiry := *props.SessionExpiry
		if expiry > 0 {
			pipe.Expire(ctx, key, time.Duration(expiry)*time.Second)
		}
	}

	_, err := pipe.Exec(ctx)
	return err
}

// OnConnected is called after successful connection.
func (h *ClusterHook) OnConnected(ctx context.Context, client broker.ClientInfo) {
	// Restore subscriptions from Redis to local broker
	h.restoreSubscriptions(ctx, client)

	// Deliver queued messages
	h.deliverQueuedMessages(ctx, client)
}

// OnDisconnect updates session state and handles cleanup.
func (h *ClusterHook) OnDisconnect(ctx context.Context, client broker.ClientInfo, err error) {
	key := h.sessionKey(client.ClientID())

	// Mark as disconnected
	h.client.HSet(ctx, key, "connected", false)

	// Remove this node from route index for client's subscriptions
	h.removeClientFromRoutes(ctx, client.ClientID())
}

// ============================================================================
// Subscription Storage & Route Index
// ============================================================================

func (h *ClusterHook) routeKey(filter string) string {
	return h.keyPrefix + "route:" + filter
}

// OnSubscribe stores subscriptions and updates route index.
func (h *ClusterHook) OnSubscribe(ctx context.Context, client broker.ClientInfo, subs []packet.Subscription) ([]packet.Subscription, error) {
	if len(subs) == 0 {
		return subs, nil
	}

	subsKey := h.subsKey(client.ClientID())
	pipe := h.client.Pipeline()

	for _, sub := range subs {
		// Store subscription details
		cs := clusterSub{
			QoS:               byte(sub.QoS),
			NoLocal:           sub.NoLocal,
			RetainAsPublished: sub.RetainAsPublished,
		}
		data, _ := json.Marshal(cs)
		pipe.HSet(ctx, subsKey, sub.TopicFilter, data)

		// Add node to route index
		pipe.SAdd(ctx, h.routeKey(sub.TopicFilter), h.nodeID)

		// Update local cache
		h.routeCacheMu.Lock()
		h.routeCache[sub.TopicFilter] = true
		h.routeCacheMu.Unlock()
	}

	_, err := pipe.Exec(ctx)
	return subs, err
}

// removeClientFromRoutes removes this node from route index when client disconnects.
func (h *ClusterHook) removeClientFromRoutes(ctx context.Context, clientID string) {
	subsKey := h.subsKey(clientID)

	// Get all subscriptions for this client
	subs, err := h.client.HGetAll(ctx, subsKey).Result()
	if err != nil || len(subs) == 0 {
		return
	}

	// For each filter, check if any other local clients have this subscription
	// For simplicity, we remove from route index (other nodes will re-add on their next subscription)
	pipe := h.client.Pipeline()
	for filter := range subs {
		pipe.SRem(ctx, h.routeKey(filter), h.nodeID)

		h.routeCacheMu.Lock()
		delete(h.routeCache, filter)
		h.routeCacheMu.Unlock()
	}
	pipe.Exec(ctx)
}

// restoreSubscriptions restores subscriptions from Redis on reconnect.
func (h *ClusterHook) restoreSubscriptions(ctx context.Context, client broker.ClientInfo) {
	subsKey := h.subsKey(client.ClientID())

	subs, err := h.client.HGetAll(ctx, subsKey).Result()
	if err != nil || len(subs) == 0 {
		return
	}

	// Re-add this node to route index for all filters
	pipe := h.client.Pipeline()
	for filter := range subs {
		pipe.SAdd(ctx, h.routeKey(filter), h.nodeID)

		h.routeCacheMu.Lock()
		h.routeCache[filter] = true
		h.routeCacheMu.Unlock()
	}
	pipe.Exec(ctx)

	// Note: The broker already restores subscriptions from its session manager
}

// ============================================================================
// Targeted Fan-out
// ============================================================================

func (h *ClusterHook) fanoutKey(nodeID string) string {
	return h.keyPrefix + "fanout:" + nodeID
}

// OnPublishReceived routes messages to other nodes via targeted fan-out.
func (h *ClusterHook) OnPublishReceived(ctx context.Context, client broker.ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	// Find all nodes that have subscribers for this topic
	nodes := h.findTargetNodes(ctx, pkt.TopicName)

	if len(nodes) == 0 {
		return pkt, nil
	}

	// Prepare message
	msg := clusterMsg{
		Type:    "publish",
		Topic:   pkt.TopicName,
		Payload: pkt.Payload,
		QoS:     byte(pkt.QoS),
		Retain:  pkt.Retain,
	}
	data, _ := json.Marshal(msg)

	// Publish to each target node's channel
	for _, nodeID := range nodes {
		if nodeID == h.nodeID {
			continue // Skip self
		}
		h.client.Publish(ctx, h.fanoutKey(nodeID), data)
	}

	return pkt, nil
}

// findTargetNodes finds all nodes with matching subscriptions for a topic.
func (h *ClusterHook) findTargetNodes(ctx context.Context, topic string) []string {
	// Generate all possible matching filters
	filters := generateMatchingFilters(topic)

	// Lookup each filter in route index
	nodeSet := make(map[string]bool)
	pipe := h.client.Pipeline()
	cmds := make([]*redis.StringSliceCmd, len(filters))

	for i, filter := range filters {
		cmds[i] = pipe.SMembers(ctx, h.routeKey(filter))
	}
	pipe.Exec(ctx)

	for _, cmd := range cmds {
		nodes, _ := cmd.Result()
		for _, n := range nodes {
			nodeSet[n] = true
		}
	}

	// Convert to slice
	result := make([]string, 0, len(nodeSet))
	for n := range nodeSet {
		result = append(result, n)
	}
	return result
}

// generateMatchingFilters generates all possible MQTT filters that could match a topic.
// For topic "a/b/c", generates: "a/b/c", "a/b/+", "a/+/c", "+/b/c", "a/+/+", "+/+/c", "+/b/+", "+/+/+", "a/b/#", "a/#", "#"
func generateMatchingFilters(topic string) []string {
	parts := strings.Split(topic, "/")
	n := len(parts)

	filters := make([]string, 0, 1<<n+n+1)

	// Exact match
	filters = append(filters, topic)

	// Multi-level wildcard at each position
	for i := 0; i <= n; i++ {
		if i == 0 {
			filters = append(filters, "#")
		} else {
			filters = append(filters, strings.Join(parts[:i], "/")+"/#")
		}
	}

	// Single-level wildcards (generate all combinations)
	if n <= 8 { // Limit to avoid explosion
		for mask := 1; mask < (1 << n); mask++ {
			filterParts := make([]string, n)
			for i := 0; i < n; i++ {
				if mask&(1<<i) != 0 {
					filterParts[i] = "+"
				} else {
					filterParts[i] = parts[i]
				}
			}
			filters = append(filters, strings.Join(filterParts, "/"))
		}
	}

	return filters
}

// startFanoutListener subscribes to this node's fan-out channel.
func (h *ClusterHook) startFanoutListener() {
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel

	h.pubsub = h.client.Subscribe(ctx, h.fanoutKey(h.nodeID))

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
				h.handleFanoutMessage(msg.Payload)
			}
		}
	}()
}

// handleFanoutMessage processes messages from other nodes.
func (h *ClusterHook) handleFanoutMessage(data string) {
	var msg clusterMsg
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return
	}

	switch msg.Type {
	case "publish":
		if h.Opts != nil && h.Opts.Broker != nil {
			h.Opts.Broker.Publish(msg.Topic, msg.Payload, msg.Retain)
		}
	}
}

// ============================================================================
// Offline Message Queue (Redis Streams)
// ============================================================================

func (h *ClusterHook) queueKey(clientID string) string {
	return h.keyPrefix + "queue:" + clientID
}

// QueueMessage adds a message to the client's offline queue.
func (h *ClusterHook) QueueMessage(ctx context.Context, clientID string, pkt *packet.Publish) error {
	key := h.queueKey(clientID)

	// Add to stream
	_, err := h.client.XAdd(ctx, &redis.XAddArgs{
		Stream: key,
		Values: map[string]any{
			"topic":   pkt.TopicName,
			"payload": pkt.Payload,
			"qos":     pkt.QoS,
		},
	}).Result()

	if err != nil {
		return err
	}

	// Trim to max size (keep last 1000 messages)
	h.client.XTrimMaxLen(ctx, key, 1000)

	return nil
}

// deliverQueuedMessages delivers offline messages when client reconnects.
func (h *ClusterHook) deliverQueuedMessages(ctx context.Context, client broker.ClientInfo) {
	key := h.queueKey(client.ClientID())

	// Read all messages from stream
	msgs, err := h.client.XRange(ctx, key, "-", "+").Result()
	if err != nil || len(msgs) == 0 {
		return
	}

	// Deliver each message
	for _, msg := range msgs {
		topic, _ := msg.Values["topic"].(string)
		payload, _ := msg.Values["payload"].(string)
		qos, _ := msg.Values["qos"].(string)

		var qosVal packet.QoS
		if qos == "1" {
			qosVal = packet.QoS1
		} else if qos == "2" {
			qosVal = packet.QoS2
		}

		pkt := &packet.Publish{
			TopicName: topic,
			Payload:   []byte(payload),
			QoS:       qosVal,
		}

		// Inject into broker for delivery
		if h.Opts != nil && h.Opts.Broker != nil {
			h.Opts.Broker.Publish(pkt.TopicName, pkt.Payload, false)
		}
	}

	// Clear the queue
	h.client.Del(ctx, key)
}

// ============================================================================
// QoS 2 In-Flight State
// ============================================================================

func (h *ClusterHook) inflightKey(clientID string) string {
	return h.keyPrefix + "inflight:" + clientID
}

// TrackInflight stores QoS 2 state in Redis.
func (h *ClusterHook) TrackInflight(ctx context.Context, clientID string, packetID uint16, state string, pkt *packet.Publish) error {
	key := h.inflightKey(clientID)

	data, _ := json.Marshal(map[string]any{
		"state": state,
		"topic": pkt.TopicName,
		"ts":    time.Now().Unix(),
	})

	pipe := h.client.Pipeline()
	pipe.HSet(ctx, key, fmt.Sprintf("%d", packetID), data)
	pipe.Expire(ctx, key, 60*time.Second) // Auto-cleanup after 60s
	_, err := pipe.Exec(ctx)
	return err
}

// AckInflight removes QoS 2 state from Redis.
func (h *ClusterHook) AckInflight(ctx context.Context, clientID string, packetID uint16) error {
	return h.client.HDel(ctx, h.inflightKey(clientID), fmt.Sprintf("%d", packetID)).Err()
}

// ============================================================================
// Retained Messages
// ============================================================================

func (h *ClusterHook) retainedKey(topic string) string {
	return h.keyPrefix + "retained:" + topic
}

func (h *ClusterHook) retainedIndexKey() string {
	return h.keyPrefix + "retained:_idx"
}

// StoreRetained stores a retained message in Redis.
func (h *ClusterHook) StoreRetained(ctx context.Context, topic string, pkt *packet.Publish) error {
	key := h.retainedKey(topic)
	idxKey := h.retainedIndexKey()

	// Empty payload = delete retained message
	if len(pkt.Payload) == 0 {
		pipe := h.client.Pipeline()
		pipe.Del(ctx, key)
		pipe.SRem(ctx, idxKey, topic)
		_, err := pipe.Exec(ctx)
		return err
	}

	// Store as msgpack for efficiency
	msg := retainedMsg{
		Payload: pkt.Payload,
		QoS:     byte(pkt.QoS),
	}
	data, err := msgpack.Marshal(msg)
	if err != nil {
		return err
	}

	pipe := h.client.Pipeline()
	pipe.Set(ctx, key, data, 0)
	pipe.SAdd(ctx, idxKey, topic)
	_, err = pipe.Exec(ctx)
	return err
}

// GetRetained retrieves retained messages matching a topic filter.
func (h *ClusterHook) GetRetained(ctx context.Context, filter string) ([]*packet.Publish, error) {
	idxKey := h.retainedIndexKey()

	// Get all retained topics
	topics, err := h.client.SMembers(ctx, idxKey).Result()
	if err != nil {
		return nil, err
	}

	// Filter topics that match
	var matching []string
	for _, topic := range topics {
		if topicMatchesFilter(topic, filter) {
			matching = append(matching, topic)
		}
	}

	if len(matching) == 0 {
		return nil, nil
	}

	// Fetch matching messages
	pipe := h.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(matching))
	for i, topic := range matching {
		cmds[i] = pipe.Get(ctx, h.retainedKey(topic))
	}
	pipe.Exec(ctx)

	var results []*packet.Publish
	for i, cmd := range cmds {
		data, err := cmd.Bytes()
		if err != nil {
			continue
		}

		var msg retainedMsg
		if err := msgpack.Unmarshal(data, &msg); err != nil {
			continue
		}

		results = append(results, &packet.Publish{
			TopicName: matching[i],
			Payload:   msg.Payload,
			QoS:       packet.QoS(msg.QoS),
			Retain:    true,
		})
	}

	return results, nil
}

// ============================================================================
// Session Lifecycle Hooks
// ============================================================================

// OnSessionCreated initializes session in Redis.
func (h *ClusterHook) OnSessionCreated(ctx context.Context, client broker.ClientInfo) {
	sess := clusterSession{
		Connected: true,
		Node:      h.nodeID,
		Protocol:  byte(client.ProtocolVersion()),
		CreatedAt: time.Now().Unix(),
	}
	data, _ := json.Marshal(sess)
	h.client.HSet(ctx, h.sessionKey(client.ClientID()), "data", data)
}

// OnSessionResumed updates session state in Redis.
func (h *ClusterHook) OnSessionResumed(ctx context.Context, client broker.ClientInfo) {
	key := h.sessionKey(client.ClientID())
	h.client.HSet(ctx, key,
		"connected", true,
		"node", h.nodeID,
		"connected_at", time.Now().Unix(),
	)
}

// OnSessionEnded cleans up session data in Redis.
func (h *ClusterHook) OnSessionEnded(ctx context.Context, clientID string) {
	pipe := h.client.Pipeline()
	pipe.Del(ctx, h.sessionKey(clientID))
	pipe.Del(ctx, h.subsKey(clientID))
	pipe.Del(ctx, h.queueKey(clientID))
	pipe.Del(ctx, h.inflightKey(clientID))
	pipe.Exec(ctx)
}

// ============================================================================
// Utilities
// ============================================================================

// GetActiveNodes returns all active nodes in the cluster.
func (h *ClusterHook) GetActiveNodes(ctx context.Context) ([]string, error) {
	pattern := h.keyPrefix + "node:*"
	keys, err := h.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	nodes := make([]string, 0, len(keys))
	prefix := h.keyPrefix + "node:"
	for _, key := range keys {
		nodes = append(nodes, strings.TrimPrefix(key, prefix))
	}
	return nodes, nil
}

// Client returns the underlying Redis client for advanced usage.
func (h *ClusterHook) Client() redis.UniversalClient {
	return h.client
}

// NodeID returns this node's ID.
func (h *ClusterHook) NodeID() string {
	return h.nodeID
}
