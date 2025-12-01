package broker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bromq-dev/broker/pkg/packet"
)

// Client represents a connected MQTT client.
type Client struct {
	// Connection
	conn   net.Conn
	reader *packet.Reader

	// Client info (set after CONNECT)
	clientID      string
	username      string
	version       packet.Version
	cleanStart    bool
	keepAlive     uint16
	properties    *packet.Properties
	maxPacketSize uint32 // Client's max packet size (0 = no limit)

	// Session
	session *Session

	// State
	connected atomic.Bool
	closed    atomic.Bool

	// Outbound queue (channel-based for non-blocking)
	outbound chan packet.Packet

	// Packet ID management
	packetIDMu   sync.Mutex
	nextPacketID uint16
	inFlightOut  map[uint16]*inflightMsg  // Outbound QoS 1/2 waiting for ack
	inFlightIn   map[uint16]*packet.Publish // Inbound QoS 2 waiting for PUBREL

	// Will message
	will       *packet.Publish
	willProps  *packet.Properties
	willDelay  uint32 // MQTT 5.0 will delay interval

	// Keep-alive
	lastActivity time.Time
	keepAliveMu  sync.Mutex

	// Broker reference
	broker *Broker

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// inflightMsg tracks an outbound message waiting for acknowledgment.
type inflightMsg struct {
	pkt       *packet.Publish
	timestamp time.Time
	retries   int
}

// newClient creates a new client from a network connection.
func newClient(conn net.Conn, broker *Broker) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	bufferSize := broker.config.ClientOutboundBuffer
	if bufferSize <= 0 {
		bufferSize = 4096
	}
	reader := packet.NewReader(conn, 8192)
	if broker.config.MaxPacketSize > 0 {
		reader.SetMaxPacketSize(broker.config.MaxPacketSize)
	}
	return &Client{
		conn:         conn,
		reader:       reader,
		outbound:     make(chan packet.Packet, bufferSize),
		nextPacketID: 1,
		inFlightOut:  make(map[uint16]*inflightMsg),
		inFlightIn:   make(map[uint16]*packet.Publish),
		lastActivity: time.Now(),
		broker:       broker,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// ClientID returns the client identifier.
func (c *Client) ClientID() string {
	return c.clientID
}

// Username returns the username if provided during connect.
func (c *Client) Username() string {
	return c.username
}

// RemoteAddr returns the remote address of the client.
func (c *Client) RemoteAddr() string {
	if c.conn != nil {
		return c.conn.RemoteAddr().String()
	}
	return ""
}

// ProtocolVersion returns the MQTT protocol version.
func (c *Client) ProtocolVersion() packet.Version {
	return c.version
}

// CleanStart returns whether this is a clean session/start.
func (c *Client) CleanStart() bool {
	return c.cleanStart
}

// KeepAlive returns the keep-alive interval in seconds.
func (c *Client) KeepAlive() uint16 {
	return c.keepAlive
}

// Properties returns MQTT 5.0 connect properties (nil for v3.1.1).
func (c *Client) Properties() *packet.Properties {
	return c.properties
}

// Send queues a packet for sending to the client.
// This is non-blocking; if the queue is full, the packet is dropped.
func (c *Client) Send(pkt packet.Packet) bool {
	if c.closed.Load() {
		return false
	}

	select {
	case c.outbound <- pkt:
		return true
	default:
		// Queue full, packet dropped
		return false
	}
}

// SendSync sends a packet synchronously, blocking until sent.
func (c *Client) SendSync(pkt packet.Packet) error {
	if c.closed.Load() {
		return errors.New("client closed")
	}

	buf := packet.GetBuffer()
	defer packet.PutBuffer(buf)

	size := pkt.EncodedSize()
	if size > len(buf) {
		buf = make([]byte, size)
	}

	n := pkt.Encode(buf)
	if n == 0 {
		return errors.New("failed to encode packet")
	}

	_, err := c.conn.Write(buf[:n])
	return err
}

// Close closes the client connection.
func (c *Client) Close() error {
	if c.closed.Swap(true) {
		return nil // Already closed
	}

	c.cancel()
	close(c.outbound)
	return c.conn.Close()
}

// nextID generates the next packet identifier.
func (c *Client) nextID() uint16 {
	c.packetIDMu.Lock()
	defer c.packetIDMu.Unlock()

	id := c.nextPacketID
	c.nextPacketID++
	if c.nextPacketID == 0 {
		c.nextPacketID = 1 // Skip 0
	}
	return id
}

// trackInflight tracks an outbound message for QoS 1/2.
// Returns false if the inflight limit has been reached.
func (c *Client) trackInflight(pkt *packet.Publish) bool {
	c.packetIDMu.Lock()
	defer c.packetIDMu.Unlock()

	// Enforce MaxInflight limit
	maxInflight := c.broker.config.MaxInflight
	if maxInflight > 0 && len(c.inFlightOut) >= maxInflight {
		return false
	}

	c.inFlightOut[pkt.PacketID] = &inflightMsg{
		pkt:       pkt,
		timestamp: time.Now(),
	}
	return true
}

// ackInflight acknowledges an outbound message.
func (c *Client) ackInflight(packetID uint16) *packet.Publish {
	c.packetIDMu.Lock()
	defer c.packetIDMu.Unlock()

	if msg, ok := c.inFlightOut[packetID]; ok {
		delete(c.inFlightOut, packetID)
		return msg.pkt
	}
	return nil
}

// trackInbound tracks an inbound QoS 2 message waiting for PUBREL.
func (c *Client) trackInbound(pkt *packet.Publish) {
	c.packetIDMu.Lock()
	defer c.packetIDMu.Unlock()

	c.inFlightIn[pkt.PacketID] = pkt
}

// releaseInbound releases an inbound QoS 2 message after PUBREL.
func (c *Client) releaseInbound(packetID uint16) *packet.Publish {
	c.packetIDMu.Lock()
	defer c.packetIDMu.Unlock()

	if pkt, ok := c.inFlightIn[packetID]; ok {
		delete(c.inFlightIn, packetID)
		return pkt
	}
	return nil
}

// updateActivity updates the last activity timestamp.
func (c *Client) updateActivity() {
	c.keepAliveMu.Lock()
	c.lastActivity = time.Now()
	c.keepAliveMu.Unlock()
}

// checkKeepAlive checks if the client has exceeded the keep-alive timeout.
func (c *Client) checkKeepAlive() bool {
	if c.keepAlive == 0 {
		return true // Keep-alive disabled
	}

	c.keepAliveMu.Lock()
	last := c.lastActivity
	c.keepAliveMu.Unlock()

	// Timeout is 1.5 times the keep-alive interval
	timeout := time.Duration(c.keepAlive) * time.Second * 3 / 2
	return time.Since(last) < timeout
}

// readLoop reads packets from the connection.
func (c *Client) readLoop() {
	defer c.broker.handleDisconnect(c, nil)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		// Set read deadline based on keep-alive
		if c.keepAlive > 0 {
			timeout := time.Duration(c.keepAlive) * time.Second * 2
			c.conn.SetReadDeadline(time.Now().Add(timeout))
		}

		pkt, err := c.reader.ReadPacket()
		if err != nil {
			if err == io.EOF || errors.Is(err, net.ErrClosed) {
				return
			}
			// Protocol error or timeout
			c.broker.handleDisconnect(c, err)
			return
		}

		c.updateActivity()

		if err := c.broker.handlePacket(c, pkt); err != nil {
			c.broker.handleDisconnect(c, err)
			return
		}
	}
}

// writeLoop sends packets to the connection.
func (c *Client) writeLoop() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in write loop",
				"client", c.clientID,
				"panic", r,
				"stack", string(debug.Stack()),
			)
			c.Close()
		}
	}()

	buf := make([]byte, 65536)

	for pkt := range c.outbound {
		size := pkt.EncodedSize()
		if size > len(buf) {
			buf = make([]byte, size)
		}

		n := pkt.Encode(buf)
		if n == 0 {
			continue
		}

		if _, err := c.conn.Write(buf[:n]); err != nil {
			return
		}
	}
}
