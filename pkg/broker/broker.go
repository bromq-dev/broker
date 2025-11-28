package broker

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/bromq-dev/broker/pkg/packet"
)

// Config holds broker configuration.
type Config struct {
	// MaxConnections limits the number of concurrent connections (0 = unlimited).
	MaxConnections int

	// ConnectTimeout is the time allowed for a client to send CONNECT after connecting.
	ConnectTimeout time.Duration

	// MaxPacketSize limits the maximum packet size (0 = protocol max ~256MB).
	MaxPacketSize uint32

	// MaxInflight limits the number of unacknowledged QoS 1/2 messages per client.
	MaxInflight int

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
		MaxConnections:       0, // Unlimited
		ConnectTimeout:       10 * time.Second,
		MaxPacketSize:        0, // Protocol max
		MaxInflight:          65535,
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
		ctx:           ctx,
		cancel:        cancel,
	}
}

// RegisterHook registers a hook for extending broker behavior.
func (b *Broker) RegisterHook(hook Hook) {
	b.hooks.Register(hook)
}

// HandleConnection handles a new client connection.
// This should be called by the transport layer when a new connection is accepted.
func (b *Broker) HandleConnection(conn net.Conn) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
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
