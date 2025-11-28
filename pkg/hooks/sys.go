// Package hooks provides composable hook implementations for the MQTT broker.
package hooks

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/packet"
)

// SysHook publishes broker metrics to $SYS topics like Mosquitto.
// Topics published:
//   - $SYS/broker/version
//   - $SYS/broker/uptime
//   - $SYS/broker/clients/connected
//   - $SYS/broker/clients/total
//   - $SYS/broker/messages/received
//   - $SYS/broker/messages/sent
//   - $SYS/broker/bytes/received
//   - $SYS/broker/bytes/sent
//   - $SYS/broker/subscriptions/count
type SysHook struct {
	publisher SysPublisher
	interval  time.Duration
	version   string

	// Metrics
	startTime    time.Time
	connected    atomic.Int64
	totalClients atomic.Int64
	msgsReceived atomic.Int64
	msgsSent     atomic.Int64
	bytesRecv    atomic.Int64
	bytesSent    atomic.Int64
	subsCount    atomic.Int64

	mu     sync.Mutex
	cancel context.CancelFunc
}

// SysPublisher is called to publish $SYS messages.
type SysPublisher func(topic string, payload []byte, retain bool)

// SysConfig configures the $SYS hook.
type SysConfig struct {
	// Publisher is called to publish $SYS messages.
	Publisher SysPublisher

	// Interval is how often to publish metrics (default: 10s).
	Interval time.Duration

	// Version is the broker version string.
	Version string
}

// NewSysHook creates a new $SYS metrics hook.
func NewSysHook(cfg SysConfig) *SysHook {
	if cfg.Interval == 0 {
		cfg.Interval = 10 * time.Second
	}
	if cfg.Version == "" {
		cfg.Version = "1.0.0"
	}

	return &SysHook{
		publisher: cfg.Publisher,
		interval:  cfg.Interval,
		version:   cfg.Version,
		startTime: time.Now(),
	}
}

func (h *SysHook) ID() string { return "sys" }

// Start begins publishing $SYS metrics.
func (h *SysHook) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel

	// Publish static values
	h.publish("$SYS/broker/version", h.version)

	go h.loop(ctx)
}

// Stop stops publishing $SYS metrics.
func (h *SysHook) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
}

func (h *SysHook) loop(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.publishMetrics()
		}
	}
}

func (h *SysHook) publishMetrics() {
	uptime := int64(time.Since(h.startTime).Seconds())

	h.publish("$SYS/broker/uptime", fmt.Sprintf("%d", uptime))
	h.publish("$SYS/broker/clients/connected", fmt.Sprintf("%d", h.connected.Load()))
	h.publish("$SYS/broker/clients/total", fmt.Sprintf("%d", h.totalClients.Load()))
	h.publish("$SYS/broker/messages/received", fmt.Sprintf("%d", h.msgsReceived.Load()))
	h.publish("$SYS/broker/messages/sent", fmt.Sprintf("%d", h.msgsSent.Load()))
	h.publish("$SYS/broker/bytes/received", fmt.Sprintf("%d", h.bytesRecv.Load()))
	h.publish("$SYS/broker/bytes/sent", fmt.Sprintf("%d", h.bytesSent.Load()))
	h.publish("$SYS/broker/subscriptions/count", fmt.Sprintf("%d", h.subsCount.Load()))
}

func (h *SysHook) publish(topic, value string) {
	if h.publisher != nil {
		h.publisher(topic, []byte(value), true)
	}
}

// ConnectionHook implementation

func (h *SysHook) OnConnected(ctx context.Context, client broker.ClientInfo) {
	h.connected.Add(1)
	h.totalClients.Add(1)
}

func (h *SysHook) OnDisconnect(ctx context.Context, client broker.ClientInfo, err error) {
	h.connected.Add(-1)
}

// MessageHook implementation

func (h *SysHook) OnPublishReceived(ctx context.Context, client broker.ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	h.msgsReceived.Add(1)
	h.bytesRecv.Add(int64(len(pkt.Payload)))
	return nil, nil
}

func (h *SysHook) OnPublishDeliver(ctx context.Context, subscriber broker.ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	h.msgsSent.Add(1)
	h.bytesSent.Add(int64(len(pkt.Payload)))
	return nil, nil
}

// Metrics provides direct access to current metrics.
func (h *SysHook) Metrics() SysMetrics {
	return SysMetrics{
		Uptime:           time.Since(h.startTime),
		ClientsConnected: h.connected.Load(),
		ClientsTotal:     h.totalClients.Load(),
		MessagesReceived: h.msgsReceived.Load(),
		MessagesSent:     h.msgsSent.Load(),
		BytesReceived:    h.bytesRecv.Load(),
		BytesSent:        h.bytesSent.Load(),
		Subscriptions:    h.subsCount.Load(),
	}
}

// IncrementSubscriptions increments the subscription count.
func (h *SysHook) IncrementSubscriptions(n int64) {
	h.subsCount.Add(n)
}

// DecrementSubscriptions decrements the subscription count.
func (h *SysHook) DecrementSubscriptions(n int64) {
	h.subsCount.Add(-n)
}

// SysMetrics holds current broker metrics.
type SysMetrics struct {
	Uptime           time.Duration
	ClientsConnected int64
	ClientsTotal     int64
	MessagesReceived int64
	MessagesSent     int64
	BytesReceived    int64
	BytesSent        int64
	Subscriptions    int64
}
