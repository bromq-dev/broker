// Package redis provides a Redis pub/sub based Router.
package redis

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"

	"github.com/bromq-dev/broker/pkg/cluster/types"
	"github.com/bromq-dev/broker/pkg/packet"
	"github.com/redis/go-redis/v9"
)

// Router implements cluster.Router using Redis pub/sub for messaging.
type Router struct {
	client    redis.UniversalClient
	nodeID    string
	keyPrefix string

	pubsub  *redis.PubSub
	handler func(*packet.Publish)
	cancel  context.CancelFunc

	log *slog.Logger
}

// Config configures the Redis router.
type Config struct {
	// Addr is the Redis server address (default: "localhost:6379").
	Addr string

	// Addrs is a list of addresses for cluster mode.
	Addrs []string

	// Password for authentication.
	Password string

	// DB is the database number (ignored in cluster mode).
	DB int

	// KeyPrefix is prepended to channel names (default: "mqtt:").
	KeyPrefix string

	// NodeID uniquely identifies this node (required).
	NodeID string

	// Client allows providing a pre-configured Redis client.
	Client redis.UniversalClient

	// Logger for logging. If nil, uses slog.Default().
	Logger *slog.Logger
}

type routeMsg struct {
	Type    string `json:"type"`
	Topic   string `json:"topic"`
	Payload []byte `json:"payload"`
	QoS     byte   `json:"qos"`
	Retain  bool   `json:"retain"`
}

// NewRouter creates a new Redis pub/sub router.
func NewRouter(cfg *Config) (*Router, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.NodeID == "" {
		hostname, _ := os.Hostname()
		cfg.NodeID = hostname
	}
	if cfg.Addr == "" && len(cfg.Addrs) == 0 && cfg.Client == nil {
		cfg.Addr = "localhost:6379"
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "mqtt:"
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

	return &Router{
		client:    client,
		nodeID:    cfg.NodeID,
		keyPrefix: cfg.KeyPrefix,
		log:       cfg.Logger,
	}, nil
}

// NewRouterWithClient creates a router using an existing Redis client.
func NewRouterWithClient(client redis.UniversalClient, nodeID, keyPrefix string) *Router {
	if keyPrefix == "" {
		keyPrefix = "mqtt:"
	}
	return &Router{
		client:    client,
		nodeID:    nodeID,
		keyPrefix: keyPrefix,
		log:       slog.Default(),
	}
}

func (r *Router) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel

	// Subscribe to this node's channel
	r.pubsub = r.client.Subscribe(ctx, r.channelKey(r.nodeID))

	go r.listen(ctx)

	r.log.Info("redis router started", "node_id", r.nodeID)
	return nil
}

func (r *Router) Stop() error {
	if r.cancel != nil {
		r.cancel()
	}
	if r.pubsub != nil {
		return r.pubsub.Close()
	}
	return nil
}

func (r *Router) channelKey(nodeID string) string {
	return r.keyPrefix + "fanout:" + nodeID
}

func (r *Router) Send(ctx context.Context, nodes []types.NodeInfo, msg *packet.Publish) error {
	data, err := json.Marshal(routeMsg{
		Type:    "publish",
		Topic:   msg.TopicName,
		Payload: msg.Payload,
		QoS:     byte(msg.QoS),
		Retain:  msg.Retain,
	})
	if err != nil {
		return err
	}

	for _, node := range nodes {
		if node.ID == r.nodeID {
			continue
		}
		r.client.Publish(ctx, r.channelKey(node.ID), data)
	}

	return nil
}

func (r *Router) OnReceive(handler func(*packet.Publish)) {
	r.handler = handler
}

func (r *Router) listen(ctx context.Context) {
	ch := r.pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			r.handleMessage(msg.Payload)
		}
	}
}

func (r *Router) handleMessage(data string) {
	if r.handler == nil {
		return
	}

	var msg routeMsg
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return
	}

	if msg.Type == "publish" {
		r.handler(&packet.Publish{
			TopicName: msg.Topic,
			Payload:   msg.Payload,
			QoS:       packet.QoS(msg.QoS),
			Retain:    msg.Retain,
		})
	}
}

// Client returns the underlying Redis client.
func (r *Router) Client() redis.UniversalClient {
	return r.client
}

// Verify interface implementation
var _ types.Router = (*Router)(nil)
