package listeners

import (
	"crypto/tls"
	"errors"
	"net"
	"sync"
)

// TCPConfig holds configuration for TCP listeners.
type TCPConfig struct {
	// TLSConfig enables TLS if set.
	TLSConfig *tls.Config
}

// TCP is a TCP listener.
type TCP struct {
	id       string
	addr     string
	config   *TCPConfig
	listener net.Listener
	wg       sync.WaitGroup
	closed   chan struct{}
	mu       sync.Mutex
}

// NewTCP creates a new TCP listener.
// Use config.TLSConfig to enable TLS.
func NewTCP(id, addr string, config *TCPConfig) *TCP {
	if config == nil {
		config = &TCPConfig{}
	}
	return &TCP{
		id:     id,
		addr:   addr,
		config: config,
		closed: make(chan struct{}),
	}
}

// ID returns the listener ID.
func (t *TCP) ID() string {
	return t.id
}

// Addr returns the listener's address.
// Returns nil if the listener hasn't started.
func (t *TCP) Addr() net.Addr {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.listener == nil {
		return nil
	}
	return t.listener.Addr()
}

// Serve starts the listener and accepts connections.
func (t *TCP) Serve(handler ConnectionHandler) error {
	var l net.Listener
	var err error

	if t.config.TLSConfig != nil {
		l, err = tls.Listen("tcp", t.addr, t.config.TLSConfig)
	} else {
		l, err = net.Listen("tcp", t.addr)
	}
	if err != nil {
		return err
	}

	t.mu.Lock()
	t.listener = l
	t.mu.Unlock()

	t.wg.Add(1)
	defer t.wg.Done()

	for {
		conn, err := l.Accept()
		if err != nil {
			select {
			case <-t.closed:
				return nil
			default:
				// Transient error, continue
				continue
			}
		}
		handler.HandleConnection(conn)
	}
}

// Close stops the listener.
func (t *TCP) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	select {
	case <-t.closed:
		return errors.New("listener already closed")
	default:
		close(t.closed)
	}

	if t.listener != nil {
		t.listener.Close()
	}
	t.wg.Wait()
	return nil
}
