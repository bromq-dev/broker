// Package grpc provides a gRPC-based Router for direct node-to-node messaging.
package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/bromq-dev/broker/pkg/cluster/types"
	"github.com/bromq-dev/broker/pkg/packet"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Router implements cluster.Router using gRPC for direct messaging.
type Router struct {
	cfg     *Config
	server  *grpc.Server
	handler func(*packet.Publish)

	mu    sync.RWMutex
	conns map[string]*grpc.ClientConn // nodeID -> connection

	log *slog.Logger
}

// Config configures the gRPC router.
type Config struct {
	// ListenAddr is the address to listen on (e.g., ":7947").
	ListenAddr string

	// DialTimeout is the timeout for connecting to other nodes.
	DialTimeout time.Duration

	// Logger for logging. If nil, uses slog.Default().
	Logger *slog.Logger
}

// NewRouter creates a new gRPC router.
func NewRouter(cfg *Config) *Router {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":7947"
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Router{
		cfg:   cfg,
		conns: make(map[string]*grpc.ClientConn),
		log:   cfg.Logger,
	}
}

func (r *Router) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", r.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("grpc router: listen: %w", err)
	}

	r.server = grpc.NewServer()
	RegisterRouterServiceServer(r.server, &routerServer{router: r})

	go func() {
		if err := r.server.Serve(ln); err != nil {
			r.log.Error("grpc server error", "error", err)
		}
	}()

	r.log.Info("grpc router started", "addr", r.cfg.ListenAddr)
	return nil
}

func (r *Router) Stop() error {
	if r.server != nil {
		r.server.GracefulStop()
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, conn := range r.conns {
		conn.Close()
	}
	r.conns = make(map[string]*grpc.ClientConn)

	return nil
}

func (r *Router) Send(ctx context.Context, nodes []types.NodeInfo, msg *packet.Publish) error {
	req := &RouteRequest{
		Topic:   msg.TopicName,
		Payload: msg.Payload,
		Qos:     uint32(msg.QoS),
		Retain:  msg.Retain,
	}

	var errs []error
	for _, node := range nodes {
		if err := r.sendToNode(ctx, node, req); err != nil {
			errs = append(errs, fmt.Errorf("node %s: %w", node.ID, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to send to %d/%d nodes", len(errs), len(nodes))
	}
	return nil
}

func (r *Router) sendToNode(ctx context.Context, node types.NodeInfo, req *RouteRequest) error {
	conn, err := r.getConn(node)
	if err != nil {
		return err
	}

	client := NewRouterServiceClient(conn)
	_, err = client.Route(ctx, req)
	return err
}

func (r *Router) getConn(node types.NodeInfo) (*grpc.ClientConn, error) {
	r.mu.RLock()
	conn, ok := r.conns[node.ID]
	r.mu.RUnlock()
	if ok {
		return conn, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check
	if conn, ok := r.conns[node.ID]; ok {
		return conn, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.cfg.DialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, node.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", node.Addr, err)
	}

	r.conns[node.ID] = conn
	return conn, nil
}

func (r *Router) OnReceive(handler func(*packet.Publish)) {
	r.handler = handler
}

func (r *Router) handleMessage(req *RouteRequest) {
	if r.handler == nil {
		return
	}

	msg := &packet.Publish{
		TopicName: req.Topic,
		Payload:   req.Payload,
		QoS:       packet.QoS(req.Qos),
		Retain:    req.Retain,
	}
	r.handler(msg)
}

// RemoveConn removes a connection from the pool (e.g., when a node leaves).
func (r *Router) RemoveConn(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if conn, ok := r.conns[nodeID]; ok {
		conn.Close()
		delete(r.conns, nodeID)
	}
}

// Verify interface implementation
var _ types.Router = (*Router)(nil)
