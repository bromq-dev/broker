// Package noop provides a no-op Router for single-node operation.
package noop

import (
	"context"

	"github.com/bromq-dev/broker/pkg/cluster/types"
	"github.com/bromq-dev/broker/pkg/packet"
)

// Router implements cluster.Router as a no-op.
// Useful for single-node deployments where no routing is needed.
type Router struct{}

// NewRouter creates a new no-op router.
func NewRouter() *Router {
	return &Router{}
}

func (r *Router) Start(ctx context.Context) error {
	return nil
}

func (r *Router) Stop() error {
	return nil
}

func (r *Router) Send(ctx context.Context, nodes []types.NodeInfo, msg *packet.Publish) error {
	// No-op: single node, nothing to route
	return nil
}

func (r *Router) OnReceive(handler func(*packet.Publish)) {
	// No-op: single node, nothing to receive
}

// Verify interface implementation
var _ types.Router = (*Router)(nil)
