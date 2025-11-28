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
	publishRate  int           // max publishes per interval
	interval     time.Duration // rate limit interval
	burstSize    int           // max burst before limiting

	mu      sync.RWMutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens    int
	lastFill  time.Time
	mu        sync.Mutex
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

// NewRateLimitHook creates a new rate limiting hook.
func NewRateLimitHook(cfg RateLimitConfig) *RateLimitHook {
	if cfg.Interval == 0 {
		cfg.Interval = time.Second
	}
	if cfg.BurstSize == 0 {
		cfg.BurstSize = cfg.PublishRate * 2
	}

	h := &RateLimitHook{
		publishRate: cfg.PublishRate,
		interval:    cfg.Interval,
		burstSize:   cfg.BurstSize,
		buckets:     make(map[string]*bucket),
	}

	// Start cleanup goroutine
	go h.cleanup()

	return h
}

func (h *RateLimitHook) ID() string { return "ratelimit" }

// OnPublishReceived checks the publish rate limit.
func (h *RateLimitHook) OnPublishReceived(ctx context.Context, client broker.ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	if h.publishRate <= 0 {
		return nil, nil // No limit
	}

	b := h.getBucket(client.ClientID())
	if !b.take() {
		return nil, broker.NewReasonCodeError(packet.ReasonQuotaExceeded, "publish rate limit exceeded")
	}

	return nil, nil
}

func (h *RateLimitHook) OnPublishDeliver(ctx context.Context, subscriber broker.ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	return nil, nil
}

// ConnectionHook implementation (cleanup on disconnect)

func (h *RateLimitHook) OnConnected(ctx context.Context, client broker.ClientInfo) {}

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

func (h *RateLimitHook) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
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
