package broker

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
)

// Listener accepts connections and hands them to the broker.
type Listener struct {
	broker   *Broker
	listener net.Listener
	addr     string
	wg       sync.WaitGroup
	closed   chan struct{}
}

// ListenTCP creates a TCP listener on the given address.
func (b *Broker) ListenTCP(addr string) (*Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	listener := &Listener{
		broker:   b,
		listener: l,
		addr:     addr,
		closed:   make(chan struct{}),
	}

	listener.wg.Add(1)
	go listener.acceptLoop()

	return listener, nil
}

// ListenTLS creates a TLS listener on the given address.
func (b *Broker) ListenTLS(addr string, config *tls.Config) (*Listener, error) {
	l, err := tls.Listen("tcp", addr, config)
	if err != nil {
		return nil, err
	}

	listener := &Listener{
		broker:   b,
		listener: l,
		addr:     addr,
		closed:   make(chan struct{}),
	}

	listener.wg.Add(1)
	go listener.acceptLoop()

	return listener, nil
}

// Addr returns the listener address.
func (l *Listener) Addr() net.Addr {
	return l.listener.Addr()
}

// Close closes the listener.
func (l *Listener) Close() error {
	close(l.closed)
	err := l.listener.Close()
	l.wg.Wait()
	return err
}

func (l *Listener) acceptLoop() {
	defer l.wg.Done()

	for {
		conn, err := l.listener.Accept()
		if err != nil {
			select {
			case <-l.closed:
				return
			default:
				continue
			}
		}

		l.broker.HandleConnection(conn)
	}
}

// Server combines the broker with listeners for easy setup.
type Server struct {
	Broker       *Broker
	listeners    []*Listener
	wsListeners  []*WebSocketListener
	mu           sync.Mutex
}

// NewServer creates a new server with the given configuration.
func NewServer(config *Config) *Server {
	return &Server{
		Broker: New(config),
	}
}

// ListenTCP adds a TCP listener.
func (s *Server) ListenTCP(addr string) error {
	l, err := s.Broker.ListenTCP(addr)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.listeners = append(s.listeners, l)
	s.mu.Unlock()

	return nil
}

// ListenTLS adds a TLS listener.
func (s *Server) ListenTLS(addr string, config *tls.Config) error {
	l, err := s.Broker.ListenTLS(addr, config)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.listeners = append(s.listeners, l)
	s.mu.Unlock()

	return nil
}

// ListenWebSocket adds a WebSocket listener.
// The path parameter specifies the URL path (default: "/mqtt").
func (s *Server) ListenWebSocket(addr, path string) error {
	l, err := s.Broker.ListenWebSocket(addr, path)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.wsListeners = append(s.wsListeners, l)
	s.mu.Unlock()

	return nil
}

// ListenWebSocketTLS adds a TLS WebSocket listener.
func (s *Server) ListenWebSocketTLS(addr, path string, config *tls.Config) error {
	l, err := s.Broker.ListenWebSocketTLS(addr, path, config)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.wsListeners = append(s.wsListeners, l)
	s.mu.Unlock()

	return nil
}

// AddHook registers a hook with the broker.
// The config parameter is hook-specific configuration (can be nil).
func (s *Server) AddHook(hook Hook, config any) error {
	return s.Broker.AddHook(hook, config)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	// Close all listeners first
	s.mu.Lock()
	for _, l := range s.listeners {
		l.Close()
	}
	for _, l := range s.wsListeners {
		l.Close()
	}
	s.mu.Unlock()

	// Then shutdown broker
	return s.Broker.Shutdown(ctx)
}
