// Package redis provides a Redis-backed Store for distributed state.
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bromq-dev/broker/pkg/cluster/types"
	"github.com/bromq-dev/broker/pkg/packet"
	"github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
)

// Store implements cluster.Store using Redis for distributed state.
type Store struct {
	client    redis.UniversalClient
	nodeID    string
	keyPrefix string

	heartbeatInterval time.Duration
	nodeTTL           time.Duration
	heartbeatCancel   context.CancelFunc

	// Local route cache
	routeCacheMu sync.RWMutex
	routeCache   map[string]bool

	log *slog.Logger
}

// Config configures the Redis store.
type Config struct {
	// Addr is the Redis server address (default: "localhost:6379").
	Addr string

	// Addrs is a list of addresses for cluster mode.
	Addrs []string

	// Password for authentication.
	Password string

	// DB is the database number (ignored in cluster mode).
	DB int

	// KeyPrefix is prepended to all keys (default: "mqtt:").
	KeyPrefix string

	// NodeID uniquely identifies this node (required).
	NodeID string

	// HeartbeatInterval is how often to refresh node TTL (default: 3s).
	HeartbeatInterval time.Duration

	// NodeTTL is how long before a node is considered dead (default: 10s).
	NodeTTL time.Duration

	// Client allows providing a pre-configured Redis client.
	Client redis.UniversalClient

	// Logger for logging. If nil, uses slog.Default().
	Logger *slog.Logger
}

type retainedMsg struct {
	Payload []byte `msgpack:"p"`
	QoS     byte   `msgpack:"q"`
}

type subInfo struct {
	QoS  byte   `json:"qos"`
	Addr string `json:"addr,omitempty"`
}

// NewStore creates a new Redis-backed store.
func NewStore(cfg *Config) (*Store, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.NodeID == "" {
		hostname, _ := os.Hostname()
		cfg.NodeID = hostname
	}
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
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	var client redis.UniversalClient
	if cfg.Client != nil {
		client = cfg.Client
	} else if len(cfg.Addrs) > 0 {
		client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    cfg.Addrs,
			Password: cfg.Password,
		})
	} else {
		client = redis.NewClient(&redis.Options{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			DB:       cfg.DB,
		})
	}

	return &Store{
		client:            client,
		nodeID:            cfg.NodeID,
		keyPrefix:         cfg.KeyPrefix,
		heartbeatInterval: cfg.HeartbeatInterval,
		nodeTTL:           cfg.NodeTTL,
		routeCache:        make(map[string]bool),
		log:               cfg.Logger,
	}, nil
}

func (s *Store) Start(ctx context.Context) error {
	// Test connection
	testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.client.Ping(testCtx).Err(); err != nil {
		return fmt.Errorf("redis connection failed: %w", err)
	}

	// Register node
	if err := s.registerNode(ctx); err != nil {
		return fmt.Errorf("failed to register node: %w", err)
	}

	// Start heartbeat
	s.startHeartbeat()

	s.log.Info("redis store started",
		"node_id", s.nodeID,
		"prefix", s.keyPrefix,
	)

	return nil
}

func (s *Store) Stop() error {
	if s.heartbeatCancel != nil {
		s.heartbeatCancel()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.client.Del(ctx, s.nodeKey())

	return s.client.Close()
}

func (s *Store) NodeID() string {
	return s.nodeID
}

func (s *Store) nodeKey() string {
	return s.keyPrefix + "node:" + s.nodeID
}

func (s *Store) registerNode(ctx context.Context) error {
	data, _ := json.Marshal(map[string]any{
		"started": time.Now().Unix(),
	})
	return s.client.SetEx(ctx, s.nodeKey(), data, s.nodeTTL).Err()
}

func (s *Store) startHeartbeat() {
	ctx, cancel := context.WithCancel(context.Background())
	s.heartbeatCancel = cancel

	go func() {
		ticker := time.NewTicker(s.heartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.client.Expire(context.Background(), s.nodeKey(), s.nodeTTL)
			}
		}
	}()
}

func (s *Store) RegisterNode(ctx context.Context, info types.NodeInfo) error {
	key := s.keyPrefix + "node:" + info.ID
	data, _ := json.Marshal(map[string]any{
		"addr": info.Addr,
		"meta": info.Meta,
	})
	return s.client.SetEx(ctx, key, data, s.nodeTTL).Err()
}

func (s *Store) DeregisterNode(ctx context.Context) error {
	return s.client.Del(ctx, s.nodeKey()).Err()
}

func (s *Store) GetNodes(ctx context.Context) ([]types.NodeInfo, error) {
	pattern := s.keyPrefix + "node:*"
	keys, err := s.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	prefix := s.keyPrefix + "node:"
	nodes := make([]types.NodeInfo, 0, len(keys))
	for _, key := range keys {
		nodeID := strings.TrimPrefix(key, prefix)
		data, err := s.client.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		var info struct {
			Addr string            `json:"addr"`
			Meta map[string]string `json:"meta"`
		}
		json.Unmarshal(data, &info)
		nodes = append(nodes, types.NodeInfo{
			ID:   nodeID,
			Addr: info.Addr,
			Meta: info.Meta,
		})
	}
	return nodes, nil
}

func (s *Store) routeKey(filter string) string {
	return s.keyPrefix + "route:" + filter
}

func (s *Store) Subscribe(ctx context.Context, filter string, qos byte) error {
	// Add node to route index
	err := s.client.SAdd(ctx, s.routeKey(filter), s.nodeID).Err()
	if err != nil {
		return err
	}

	// Store subscription info
	infoKey := s.keyPrefix + "subinfo:" + filter + ":" + s.nodeID
	info := subInfo{QoS: qos}
	data, _ := json.Marshal(info)
	s.client.Set(ctx, infoKey, data, 0)

	s.routeCacheMu.Lock()
	s.routeCache[filter] = true
	s.routeCacheMu.Unlock()

	return nil
}

func (s *Store) Unsubscribe(ctx context.Context, filter string) error {
	pipe := s.client.Pipeline()
	pipe.SRem(ctx, s.routeKey(filter), s.nodeID)
	pipe.Del(ctx, s.keyPrefix+"subinfo:"+filter+":"+s.nodeID)
	_, err := pipe.Exec(ctx)

	s.routeCacheMu.Lock()
	delete(s.routeCache, filter)
	s.routeCacheMu.Unlock()

	return err
}

func (s *Store) GetNodesForTopic(ctx context.Context, topic string) ([]types.SubscriptionInfo, error) {
	filters := generateMatchingFilters(topic)

	pipe := s.client.Pipeline()
	cmds := make([]*redis.StringSliceCmd, len(filters))
	for i, filter := range filters {
		cmds[i] = pipe.SMembers(ctx, s.routeKey(filter))
	}
	pipe.Exec(ctx)

	nodeSet := make(map[string]struct {
		filter string
		qos    byte
	})
	for i, cmd := range cmds {
		nodes, _ := cmd.Result()
		for _, nodeID := range nodes {
			if existing, ok := nodeSet[nodeID]; !ok {
				nodeSet[nodeID] = struct {
					filter string
					qos    byte
				}{filter: filters[i]}
			} else {
				// Keep existing if already set
				_ = existing
			}
		}
	}

	// Fetch subscription info for QoS
	result := make([]types.SubscriptionInfo, 0, len(nodeSet))
	for nodeID := range nodeSet {
		// Get node info for address
		nodeKey := s.keyPrefix + "node:" + nodeID
		nodeData, _ := s.client.Get(ctx, nodeKey).Bytes()
		var nodeInfo struct {
			Addr string `json:"addr"`
		}
		json.Unmarshal(nodeData, &nodeInfo)

		result = append(result, types.SubscriptionInfo{
			NodeInfo: types.NodeInfo{
				ID:   nodeID,
				Addr: nodeInfo.Addr,
			},
			QoS: 0, // Default QoS
		})
	}

	return result, nil
}

func (s *Store) retainedKey(topic string) string {
	return s.keyPrefix + "retained:" + topic
}

func (s *Store) retainedIndexKey() string {
	return s.keyPrefix + "retained:_idx"
}

func (s *Store) StoreRetained(ctx context.Context, topic string, msg *packet.Publish) error {
	key := s.retainedKey(topic)
	idxKey := s.retainedIndexKey()

	if msg == nil || len(msg.Payload) == 0 {
		pipe := s.client.Pipeline()
		pipe.Del(ctx, key)
		pipe.SRem(ctx, idxKey, topic)
		_, err := pipe.Exec(ctx)
		return err
	}

	retMsg := retainedMsg{
		Payload: msg.Payload,
		QoS:     byte(msg.QoS),
	}
	data, err := msgpack.Marshal(retMsg)
	if err != nil {
		return err
	}

	pipe := s.client.Pipeline()
	pipe.Set(ctx, key, data, 0)
	pipe.SAdd(ctx, idxKey, topic)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *Store) GetRetained(ctx context.Context, filter string) ([]*packet.Publish, error) {
	topics, err := s.client.SMembers(ctx, s.retainedIndexKey()).Result()
	if err != nil {
		return nil, err
	}

	var matching []string
	for _, topic := range topics {
		if topicMatchesFilter(topic, filter) {
			matching = append(matching, topic)
		}
	}

	if len(matching) == 0 {
		return nil, nil
	}

	pipe := s.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(matching))
	for i, topic := range matching {
		cmds[i] = pipe.Get(ctx, s.retainedKey(topic))
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

// Client returns the underlying Redis client.
func (s *Store) Client() redis.UniversalClient {
	return s.client
}

// generateMatchingFilters generates all possible filters that could match a topic.
func generateMatchingFilters(topic string) []string {
	parts := strings.Split(topic, "/")
	n := len(parts)

	filters := make([]string, 0, 1<<n+n+1)
	filters = append(filters, topic)

	// Multi-level wildcard
	for i := 0; i <= n; i++ {
		if i == 0 {
			filters = append(filters, "#")
		} else {
			filters = append(filters, strings.Join(parts[:i], "/")+"/#")
		}
	}

	// Single-level wildcards
	if n <= 8 {
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
