// Package hooks provides composable hook implementations for the MQTT broker.
package hooks

import (
	"context"
	"fmt"
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
	broker.HookBase
	interval time.Duration
	version  string

	// Metrics
	startTime    time.Time
	connected    atomic.Int64
	totalClients atomic.Int64
	msgsReceived atomic.Int64
	msgsSent     atomic.Int64
	bytesRecv    atomic.Int64
	bytesSent    atomic.Int64
	subsCount    atomic.Int64

	cancel context.CancelFunc
}

// SysConfig configures the $SYS hook.
type SysConfig struct {
	// Interval is how often to publish metrics (default: 10s).
	Interval time.Duration

	// Version is the broker version string (default: "1.0.0").
	Version string
}

func (h *SysHook) ID() string { return "sys" }

// Provides indicates which events this hook handles.
func (h *SysHook) Provides(event byte) bool {
	return event == broker.OnConnectedEvent ||
		event == broker.OnDisconnectEvent ||
		event == broker.OnPublishReceivedEvent ||
		event == broker.OnPublishDeliverEvent
}

// Init is called when the hook is registered with the broker.
func (h *SysHook) Init(opts *broker.HookOptions, config any) error {
	if err := h.HookBase.Init(opts, config); err != nil {
		return err
	}

	// Apply config if provided
	if cfg, ok := config.(*SysConfig); ok && cfg != nil {
		h.interval = cfg.Interval
		h.version = cfg.Version
	}

	// Apply defaults for zero values
	if h.interval == 0 {
		h.interval = 10 * time.Second
	}
	if h.version == "" {
		h.version = "1.0.0"
	}
	if h.startTime.IsZero() {
		h.startTime = time.Now()
	}

	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel

	// Publish static values
	h.publish("$SYS/broker/version", h.version)

	go h.loop(ctx)
	return nil
}

// Stop is called when the broker shuts down.
func (h *SysHook) Stop() error {
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	return nil
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
	if h.Opts != nil && h.Opts.Broker != nil {
		h.Opts.Broker.Publish(topic, []byte(value), true)
	}
}

// OnConnected tracks connected clients.
func (h *SysHook) OnConnected(ctx context.Context, client broker.ClientInfo) {
	h.connected.Add(1)
	h.totalClients.Add(1)
}

// OnDisconnect tracks disconnected clients.
func (h *SysHook) OnDisconnect(ctx context.Context, client broker.ClientInfo, err error) {
	h.connected.Add(-1)
}

// OnPublishReceived tracks received messages.
func (h *SysHook) OnPublishReceived(ctx context.Context, client broker.ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	h.msgsReceived.Add(1)
	h.bytesRecv.Add(int64(len(pkt.Payload)))
	return pkt, nil
}

// OnPublishDeliver tracks sent messages.
func (h *SysHook) OnPublishDeliver(ctx context.Context, subscriber broker.ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	h.msgsSent.Add(1)
	h.bytesSent.Add(int64(len(pkt.Payload)))
	return pkt, nil
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
