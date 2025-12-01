package broker

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"github.com/bromq-dev/broker/pkg/listeners"
	"github.com/bromq-dev/broker/pkg/packet"
)

// Config holds broker configuration.
type Config struct {
	// MaxConnections limits the number of concurrent connections (0 = unlimited).
	MaxConnections int

	// ConnectTimeout is the time allowed for a client to send CONNECT after connecting.
	ConnectTimeout time.Duration

	// MaxPacketSize limits the maximum inbound packet size in bytes (0 = protocol max ~256MB).
	// For outbound, the broker respects each client's advertised Maximum Packet Size (MQTT 5.0).
	MaxPacketSize uint32

	// MaxInflight limits the number of unacknowledged QoS 1/2 messages per client (0 = unlimited).
	MaxInflight int

	// MaxSessionQueue limits the number of pending messages queued for offline clients (0 = unlimited).
	MaxSessionQueue int

	// ClientOutboundBuffer is the number of messages in each client's outbound queue.
	// QoS 0 messages are dropped when this buffer is full. Default: 256.
	// Memory bound per client ≈ MaxPacketSize × ClientOutboundBuffer.
	ClientOutboundBuffer int

	// RetainAvailable indicates whether retained messages are supported.
	RetainAvailable bool

	// WildcardSubAvailable indicates whether wildcard subscriptions are supported.
	WildcardSubAvailable bool

	// SubIDAvailable indicates whether subscription identifiers are supported (MQTT 5.0).
	SubIDAvailable bool

	// SharedSubAvailable indicates whether shared subscriptions are supported.
	SharedSubAvailable bool

	// MaxQoS is the maximum QoS level supported.
	MaxQoS packet.QoS
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		MaxConnections:       0,     // Unlimited
		ConnectTimeout:       10 * time.Second,
		MaxPacketSize:        0,     // Protocol max
		MaxInflight:          65535, // MQTT spec max
		MaxSessionQueue:      1000,  // Limit offline queue
		ClientOutboundBuffer: 256, // Per-client outbound queue
		RetainAvailable:      true,
		WildcardSubAvailable: true,
		SubIDAvailable:       true,
		SharedSubAvailable:   true,
		MaxQoS:               packet.QoS2,
	}
}

// Broker is the core MQTT broker.
type Broker struct {
	config *Config
	hooks  *Hooks

	// Client management
	clientsMu sync.RWMutex
	clients   map[string]*Client // clientID -> client

	// Session management
	sessions *SessionManager

	// Subscriptions
	subscriptions *SubscriptionTree

	// Retained messages (in-memory default)
	retainedMu sync.RWMutex
	retained   map[string]*packet.Publish

	// Listeners
	listenersMu sync.Mutex
	listeners   map[string]listeners.Listener

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new broker with the given configuration.
func New(config *Config) *Broker {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Broker{
		config:        config,
		hooks:         NewHooks(),
		clients:       make(map[string]*Client),
		sessions:      NewSessionManager(),
		subscriptions: NewSubscriptionTree(),
		retained:      make(map[string]*packet.Publish),
		listeners:     make(map[string]listeners.Listener),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// AddHook registers a hook for extending broker behavior.
// The hook's Init method is called immediately with HookOptions.
// The config parameter is hook-specific configuration (can be nil).
func (b *Broker) AddHook(hook Hook, config any) error {
	opts := &HookOptions{
		Broker: b,
		Log:    defaultLog(),
	}
	if err := hook.Init(opts, config); err != nil {
		return err
	}
	b.hooks.Register(hook)
	return nil
}

// AddListener registers and starts a listener.
// The listener will begin accepting connections immediately.
func (b *Broker) AddListener(l listeners.Listener) error {
	b.listenersMu.Lock()
	if _, exists := b.listeners[l.ID()]; exists {
		b.listenersMu.Unlock()
		return fmt.Errorf("listener %q already exists", l.ID())
	}
	b.listeners[l.ID()] = l
	b.listenersMu.Unlock()

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		l.Serve(b)
	}()

	return nil
}

// HandleConnection handles a new client connection.
// This should be called by the transport layer when a new connection is accepted.
func (b *Broker) HandleConnection(conn net.Conn) {
	// Enforce MaxConnections
	if b.config.MaxConnections > 0 {
		b.clientsMu.RLock()
		count := len(b.clients)
		b.clientsMu.RUnlock()
		if count >= b.config.MaxConnections {
			conn.Close()
			return
		}
	}

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in connection handler",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				conn.Close()
			}
		}()
		b.handleConnection(conn)
	}()
}

func (b *Broker) handleConnection(conn net.Conn) {
	client := newClient(conn, b)

	// Set connect timeout
	if b.config.ConnectTimeout > 0 {
		conn.SetReadDeadline(time.Now().Add(b.config.ConnectTimeout))
	}

	// Read CONNECT packet
	pkt, err := client.reader.ReadPacket()
	if err != nil {
		conn.Close()
		return
	}

	connectPkt, ok := pkt.(*packet.Connect)
	if !ok {
		// First packet must be CONNECT
		conn.Close()
		return
	}

	// Handle CONNECT
	if err := b.handleConnect(client, connectPkt); err != nil {
		conn.Close()
		return
	}

	// Clear read deadline
	conn.SetReadDeadline(time.Time{})

	// Start read/write loops
	go client.writeLoop()
	client.readLoop() // Blocking
}

// Shutdown gracefully shuts down the broker.
func (b *Broker) Shutdown(ctx context.Context) error {
	b.cancel()

	// Close all listeners first
	b.listenersMu.Lock()
	for _, l := range b.listeners {
		l.Close()
	}
	b.listenersMu.Unlock()

	// Stop all hooks
	b.hooks.StopAll()

	// Disconnect all clients
	b.clientsMu.Lock()
	for _, client := range b.clients {
		if client.version == packet.Version5 {
			client.Send(&packet.Disconnect{
				Version:    packet.Version5,
				ReasonCode: packet.ReasonServerShuttingDown,
			})
		}
		client.Close()
	}
	b.clientsMu.Unlock()

	// Wait for all goroutines
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stats returns broker statistics.
func (b *Broker) Stats() Stats {
	b.clientsMu.RLock()
	clientCount := len(b.clients)
	b.clientsMu.RUnlock()

	b.retainedMu.RLock()
	retainedCount := len(b.retained)
	b.retainedMu.RUnlock()

	return Stats{
		Clients:       clientCount,
		Sessions:      b.sessions.Count(),
		Subscriptions: b.subscriptions.Count(),
		Retained:      retainedCount,
	}
}

// Stats holds broker statistics.
type Stats struct {
	Clients       int
	Sessions      int
	Subscriptions int
	Retained      int
}

// Publish injects a message into the broker for delivery to subscribers.
// This is useful for $SYS topics and other internal messages.
func (b *Broker) Publish(topic string, payload []byte, retain bool) {
	pkt := &packet.Publish{
		TopicName: topic,
		Payload:   payload,
		QoS:       packet.QoS0,
		Retain:    retain,
	}

	if retain {
		b.storeRetained(topic, pkt)
	}

	b.routeMessage(nil, pkt)
}

// generateClientID generates a unique client ID.
func generateClientID() string {
	return fmt.Sprintf("auto-%d", time.Now().UnixNano())
}
