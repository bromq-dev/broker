// Package cluster provides composable clustering for the MQTT broker.
//
// The clustering system is built on two interfaces:
//   - Store: Manages subscription state, node registry, and retained messages
//   - Router: Handles message delivery between nodes
//
// These can be mixed and matched for different deployment scenarios:
//
//	// Zero dependencies - gossip for state, gRPC for routing
//	cluster.New(gossip.NewStore(cfg), grpc.NewRouter(cfg))
//
//	// Redis for state, gRPC for fast routing
//	cluster.New(redis.NewStore(client), grpc.NewRouter(cfg))
//
//	// All Redis - simple deployment
//	cluster.New(redis.NewStore(client), redis.NewRouter(client))
//
// For convenience, pre-composed bundles are available:
//
//	cluster.NewGossipCluster(cfg)  // gossip + gRPC
//	cluster.NewRedisCluster(cfg)   // redis + redis pubsub
//
// Single-node mode (no clustering):
//
//	cluster.New(memory.NewStore(), noop.NewRouter())
package cluster

import (
	"context"
	"sync"
	"time"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/cluster/router/grpc"
	"github.com/bromq-dev/broker/pkg/cluster/router/noop"
	redisrouter "github.com/bromq-dev/broker/pkg/cluster/router/redis"
	"github.com/bromq-dev/broker/pkg/cluster/store/gossip"
	"github.com/bromq-dev/broker/pkg/cluster/store/memory"
	redisstore "github.com/bromq-dev/broker/pkg/cluster/store/redis"
	"github.com/bromq-dev/broker/pkg/cluster/types"
	"github.com/bromq-dev/broker/pkg/packet"
	"github.com/redis/go-redis/v9"
)

// Re-export types for convenience
type (
	NodeInfo         = types.NodeInfo
	SubscriptionInfo = types.SubscriptionInfo
	Store            = types.Store
	Router           = types.Router
)

// Hook implements broker.Hook using composable Store and Router.
type Hook struct {
	broker.HookBase

	store  Store
	router Router

	localNode NodeInfo
	mu        sync.RWMutex
}

// New creates a new cluster Hook with the given Store and Router.
func New(store Store, router Router) *Hook {
	return &Hook{
		store:  store,
		router: router,
	}
}

// Option configures the cluster Hook.
type Option func(*Hook)

// WithStore sets the Store implementation.
func WithStore(s Store) Option {
	return func(h *Hook) {
		h.store = s
	}
}

// WithRouter sets the Router implementation.
func WithRouter(r Router) Option {
	return func(h *Hook) {
		h.router = r
	}
}

// NewWithOptions creates a Hook with functional options.
func NewWithOptions(opts ...Option) *Hook {
	h := &Hook{}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *Hook) ID() string { return "cluster" }

func (h *Hook) Provides(event byte) bool {
	switch event {
	case broker.OnConnectedEvent,
		broker.OnDisconnectEvent,
		broker.OnSubscribeEvent,
		broker.OnPublishReceivedEvent,
		broker.StoreRetainedEvent,
		broker.GetRetainedEvent:
		return true
	}
	return false
}

func (h *Hook) Init(opts *broker.HookOptions, config any) error {
	if err := h.HookBase.Init(opts, config); err != nil {
		return err
	}

	ctx := context.Background()

	// Start store
	if h.store != nil {
		if err := h.store.Start(ctx); err != nil {
			return err
		}

		// Register this node
		h.localNode = NodeInfo{
			ID: h.store.NodeID(),
		}
	}

	// Start router and set up receive handler
	if h.router != nil {
		h.router.OnReceive(h.handleRemoteMessage)
		if err := h.router.Start(ctx); err != nil {
			return err
		}
	}

	if h.Log != nil {
		h.Log.Info("cluster hook initialized",
			"node_id", h.localNode.ID,
			"store", storeName(h.store),
			"router", routerName(h.router),
		)
	}

	return nil
}

func (h *Hook) Stop() error {
	ctx := context.Background()

	if h.store != nil {
		h.store.DeregisterNode(ctx)
		h.store.Stop()
	}

	if h.router != nil {
		h.router.Stop()
	}

	return nil
}

// OnConnected registers the node when the first client connects.
func (h *Hook) OnConnected(ctx context.Context, client broker.ClientInfo) {
	// Node registration happens in Init, nothing to do per-client
}

// OnDisconnect handles client disconnection.
func (h *Hook) OnDisconnect(ctx context.Context, client broker.ClientInfo, err error) {
	// Subscription cleanup is handled by the broker's session manager
}

// OnSubscribe stores the subscription in the cluster.
func (h *Hook) OnSubscribe(ctx context.Context, client broker.ClientInfo, subs []packet.Subscription) ([]packet.Subscription, error) {
	if h.store == nil {
		return subs, nil
	}

	for _, sub := range subs {
		if err := h.store.Subscribe(ctx, sub.TopicFilter, byte(sub.QoS)); err != nil {
			if h.Log != nil {
				h.Log.Warn("failed to store subscription",
					"filter", sub.TopicFilter,
					"error", err,
				)
			}
		}
	}

	return subs, nil
}

// OnPublishReceived routes the message to other nodes.
func (h *Hook) OnPublishReceived(ctx context.Context, client broker.ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	if h.store == nil || h.router == nil {
		return pkt, nil
	}

	// Get nodes with matching subscriptions
	nodes, err := h.store.GetNodesForTopic(ctx, pkt.TopicName)
	if err != nil {
		if h.Log != nil {
			h.Log.Warn("failed to get nodes for topic",
				"topic", pkt.TopicName,
				"error", err,
			)
		}
		return pkt, nil
	}

	// Filter out self and convert to NodeInfo
	var targets []NodeInfo
	for _, sub := range nodes {
		if sub.ID != h.localNode.ID {
			targets = append(targets, sub.NodeInfo)
		}
	}

	// Route to other nodes
	if len(targets) > 0 {
		if err := h.router.Send(ctx, targets, pkt); err != nil {
			if h.Log != nil {
				h.Log.Warn("failed to route message",
					"topic", pkt.TopicName,
					"targets", len(targets),
					"error", err,
				)
			}
		}
	}

	return pkt, nil
}

// StoreRetained stores a retained message in the cluster.
func (h *Hook) StoreRetained(ctx context.Context, topic string, pkt *packet.Publish) error {
	if h.store == nil {
		return nil
	}
	return h.store.StoreRetained(ctx, topic, pkt)
}

// GetRetained retrieves retained messages from the cluster.
func (h *Hook) GetRetained(ctx context.Context, filter string) ([]*packet.Publish, error) {
	if h.store == nil {
		return nil, nil
	}
	return h.store.GetRetained(ctx, filter)
}

// handleRemoteMessage processes messages received from other nodes.
func (h *Hook) handleRemoteMessage(pkt *packet.Publish) {
	if h.Opts != nil && h.Opts.Broker != nil {
		h.Opts.Broker.Publish(pkt.TopicName, pkt.Payload, pkt.Retain)
	}
}

// Helper functions for logging
func storeName(s Store) string {
	if s == nil {
		return "none"
	}
	switch s.(type) {
	default:
		return "custom"
	}
}

func routerName(r Router) string {
	if r == nil {
		return "none"
	}
	switch r.(type) {
	default:
		return "custom"
	}
}

// ============================================================================
// Convenience Bundles
// ============================================================================

// GossipConfig configures a gossip-based cluster.
type GossipConfig struct {
	// NodeID uniquely identifies this node. Defaults to hostname.
	NodeID string

	// GossipAddr is the address to bind for gossip. Default: "0.0.0.0".
	GossipAddr string

	// GossipPort is the port for gossip protocol. Default: 7946.
	GossipPort int

	// GRPCAddr is the address to bind for gRPC. Default: ":7947".
	GRPCAddr string

	// JoinAddrs is a list of existing nodes to join.
	JoinAddrs []string

	// DNSName for DNS-based discovery (K8s headless service).
	DNSName string

	// DNSRefreshInterval is how often to refresh DNS. Default: 30s.
	DNSRefreshInterval time.Duration
}

// NewGossipCluster creates a cluster using gossip for state and gRPC for routing.
// This is the recommended configuration for most deployments as it requires
// no external dependencies.
func NewGossipCluster(cfg *GossipConfig) *Hook {
	if cfg == nil {
		cfg = &GossipConfig{}
	}

	gossipPort := cfg.GossipPort
	if gossipPort == 0 {
		gossipPort = 7946
	}

	grpcAddr := cfg.GRPCAddr
	if grpcAddr == "" {
		grpcAddr = ":7947"
	}

	// Extract gRPC port for routing address
	grpcPort := "7947"
	if len(grpcAddr) > 0 && grpcAddr[0] == ':' {
		grpcPort = grpcAddr[1:]
	}

	// Build routing address from node ID and gRPC port
	// This allows other nodes to connect via hostname (works in Docker/K8s)
	routingAddr := cfg.NodeID + ":" + grpcPort

	store := gossip.NewStore(&gossip.Config{
		NodeID:             cfg.NodeID,
		BindAddr:           cfg.GossipAddr,
		BindPort:           gossipPort,
		RoutingAddr:        routingAddr,
		JoinAddrs:          cfg.JoinAddrs,
		DNSName:            cfg.DNSName,
		DNSRefreshInterval: cfg.DNSRefreshInterval,
	})

	router := grpc.NewRouter(&grpc.Config{
		ListenAddr: grpcAddr,
	})

	return New(store, router)
}

// RedisConfig configures a Redis-based cluster.
type RedisConfig struct {
	// NodeID uniquely identifies this node.
	NodeID string

	// Addr is the Redis server address. Default: "localhost:6379".
	Addr string

	// Addrs is a list of addresses for Redis cluster mode.
	Addrs []string

	// Password for Redis authentication.
	Password string

	// DB is the Redis database number. Ignored in cluster mode.
	DB int

	// KeyPrefix is prepended to all keys. Default: "mqtt:".
	KeyPrefix string

	// HeartbeatInterval is how often to refresh node TTL. Default: 3s.
	HeartbeatInterval time.Duration

	// NodeTTL is how long before a node is considered dead. Default: 10s.
	NodeTTL time.Duration

	// Client allows providing a pre-configured Redis client.
	Client redis.UniversalClient
}

// NewRedisCluster creates a cluster using Redis for both state and routing.
// Simple deployment with Redis as the only external dependency.
func NewRedisCluster(cfg *RedisConfig) (*Hook, error) {
	if cfg == nil {
		cfg = &RedisConfig{}
	}

	store, err := redisstore.NewStore(&redisstore.Config{
		NodeID:            cfg.NodeID,
		Addr:              cfg.Addr,
		Addrs:             cfg.Addrs,
		Password:          cfg.Password,
		DB:                cfg.DB,
		KeyPrefix:         cfg.KeyPrefix,
		HeartbeatInterval: cfg.HeartbeatInterval,
		NodeTTL:           cfg.NodeTTL,
		Client:            cfg.Client,
	})
	if err != nil {
		return nil, err
	}

	router, err := redisrouter.NewRouter(&redisrouter.Config{
		NodeID:    cfg.NodeID,
		Addr:      cfg.Addr,
		Addrs:     cfg.Addrs,
		Password:  cfg.Password,
		DB:        cfg.DB,
		KeyPrefix: cfg.KeyPrefix,
		Client:    cfg.Client,
	})
	if err != nil {
		return nil, err
	}

	return New(store, router), nil
}

// HybridConfig configures a hybrid cluster (Redis state + gRPC routing).
type HybridConfig struct {
	// NodeID uniquely identifies this node.
	NodeID string

	// Redis configuration
	RedisAddr     string
	RedisAddrs    []string
	RedisPassword string
	RedisDB       int
	RedisPrefix   string
	RedisClient   redis.UniversalClient

	// gRPC configuration
	GRPCAddr string

	// Node TTL settings
	HeartbeatInterval time.Duration
	NodeTTL           time.Duration
}

// NewHybridCluster creates a cluster using Redis for state and gRPC for routing.
// This provides faster message delivery than pure Redis while maintaining
// Redis for shared state.
func NewHybridCluster(cfg *HybridConfig) (*Hook, error) {
	if cfg == nil {
		cfg = &HybridConfig{}
	}

	store, err := redisstore.NewStore(&redisstore.Config{
		NodeID:            cfg.NodeID,
		Addr:              cfg.RedisAddr,
		Addrs:             cfg.RedisAddrs,
		Password:          cfg.RedisPassword,
		DB:                cfg.RedisDB,
		KeyPrefix:         cfg.RedisPrefix,
		HeartbeatInterval: cfg.HeartbeatInterval,
		NodeTTL:           cfg.NodeTTL,
		Client:            cfg.RedisClient,
	})
	if err != nil {
		return nil, err
	}

	grpcAddr := cfg.GRPCAddr
	if grpcAddr == "" {
		grpcAddr = ":7947"
	}

	router := grpc.NewRouter(&grpc.Config{
		ListenAddr: grpcAddr,
	})

	return New(store, router), nil
}

// NewLocalCluster creates a single-node "cluster" with in-memory state.
// Useful for development and testing.
func NewLocalCluster() *Hook {
	return New(memory.NewStore(nil), noop.NewRouter())
}
