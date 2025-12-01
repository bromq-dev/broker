// Package listeners provides transport listeners for the MQTT broker.
package listeners

import "net"

// ConnectionHandler handles new connections from listeners.
type ConnectionHandler interface {
	HandleConnection(conn net.Conn)
}

// Listener is the interface that all transport listeners implement.
type Listener interface {
	// ID returns the unique identifier for this listener.
	ID() string

	// Addr returns the listener's address.
	Addr() net.Addr

	// Serve starts accepting connections and passes them to the handler.
	// This should be called in a goroutine as it blocks until Close is called.
	Serve(handler ConnectionHandler) error

	// Close stops the listener.
	Close() error
}
