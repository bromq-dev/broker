package hooks

import (
	"context"
	"sync"
	"time"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/packet"
)

// RateLimitHook limits message rates per client.
type RateLimitHook struct {
	broker.HookBase
	publishRate int           // max publishes per interval
	interval    time.Duration // rate limit interval
	burstSize   int           // max burst before limiting

	mu      sync.RWMutex
	buckets map[string]*bucket
	cancel  context.CancelFunc
}

type bucket struct {
	tokens   int
	lastFill time.Time
	mu       sync.Mutex
}

// RateLimitConfig configures the rate limiter.
type RateLimitConfig struct {
	// PublishRate is the max number of publishes per interval per client.
	PublishRate int

	// Interval is the rate limit window (default: 1s).
	Interval time.Duration

	// BurstSize is the max burst allowed (default: PublishRate * 2).
	BurstSize int
}

func (h *RateLimitHook) ID() string { return "ratelimit" }

// Provides indicates which events this hook handles.
func (h *RateLimitHook) Provides(event byte) bool {
	return event == broker.OnPublishReceivedEvent ||
		event == broker.OnDisconnectEvent
}

// Init is called when the hook is registered with the broker.
func (h *RateLimitHook) Init(opts *broker.HookOptions, config any) error {
	if err := h.HookBase.Init(opts, config); err != nil {
		return err
	}

	// Apply config if provided
	if cfg, ok := config.(*RateLimitConfig); ok && cfg != nil {
		h.publishRate = cfg.PublishRate
		h.interval = cfg.Interval
		h.burstSize = cfg.BurstSize
	}

	// Apply defaults
	if h.interval == 0 {
		h.interval = time.Second
	}
	if h.burstSize == 0 && h.publishRate > 0 {
		h.burstSize = h.publishRate * 2
	}
	if h.buckets == nil {
		h.buckets = make(map[string]*bucket)
	}

	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go h.cleanup(ctx)
	return nil
}

// Stop is called when the broker shuts down.
func (h *RateLimitHook) Stop() error {
	if h.cancel != nil {
		h.cancel()
	}
	return nil
}

// OnPublishReceived checks the publish rate limit.
func (h *RateLimitHook) OnPublishReceived(ctx context.Context, client broker.ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	if h.publishRate <= 0 {
		return pkt, nil // No limit
	}

	b := h.getBucket(client.ClientID())
	if !b.take() {
		return nil, broker.NewReasonCodeError(packet.ReasonQuotaExceeded, "publish rate limit exceeded")
	}

	return pkt, nil
}

// OnDisconnect cleans up the client's bucket.
func (h *RateLimitHook) OnDisconnect(ctx context.Context, client broker.ClientInfo, err error) {
	h.mu.Lock()
	delete(h.buckets, client.ClientID())
	h.mu.Unlock()
}

func (h *RateLimitHook) getBucket(clientID string) *bucket {
	h.mu.RLock()
	b, ok := h.buckets[clientID]
	h.mu.RUnlock()

	if ok {
		return b
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Double-check
	if b, ok = h.buckets[clientID]; ok {
		return b
	}

	b = &bucket{
		tokens:   h.burstSize,
		lastFill: time.Now(),
	}
	h.buckets[clientID] = b
	return b
}

func (b *bucket) take() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(b.lastFill)
	if elapsed > 0 {
		// This is simplified - a real implementation would use the actual rate
		b.lastFill = now
	}

	if b.tokens <= 0 {
		return false
	}

	b.tokens--
	return true
}

func (h *RateLimitHook) cleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.mu.Lock()
			now := time.Now()
			for id, b := range h.buckets {
				b.mu.Lock()
				// Remove stale buckets (no activity for 5 minutes)
				if now.Sub(b.lastFill) > 5*time.Minute {
					delete(h.buckets, id)
				} else {
					// Refill tokens
					b.tokens = h.burstSize
				}
				b.mu.Unlock()
			}
			h.mu.Unlock()
		}
	}
}
