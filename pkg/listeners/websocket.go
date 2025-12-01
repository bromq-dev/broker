package listeners

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketConfig holds configuration for WebSocket listeners.
type WebSocketConfig struct {
	// TLSConfig enables TLS if set.
	TLSConfig *tls.Config

	// Path is the URL path to listen on. Default: "/mqtt".
	Path string

	// CheckOrigin is a function to validate the Origin header.
	// If nil, all origins are allowed.
	CheckOrigin func(r *http.Request) bool
}

// WebSocket is a WebSocket listener.
type WebSocket struct {
	id       string
	addr     string
	config   *WebSocketConfig
	server   *http.Server
	upgrader websocket.Upgrader
	handler  ConnectionHandler
	wg       sync.WaitGroup
	closed   chan struct{}
	mu       sync.Mutex
}

// NewWebSocket creates a new WebSocket listener.
func NewWebSocket(id, addr string, config *WebSocketConfig) *WebSocket {
	if config == nil {
		config = &WebSocketConfig{}
	}
	if config.Path == "" {
		config.Path = "/mqtt"
	}
	checkOrigin := config.CheckOrigin
	if checkOrigin == nil {
		checkOrigin = func(r *http.Request) bool { return true }
	}

	return &WebSocket{
		id:     id,
		addr:   addr,
		config: config,
		upgrader: websocket.Upgrader{
			Subprotocols: []string{"mqtt"},
			CheckOrigin:  checkOrigin,
		},
		closed: make(chan struct{}),
	}
}

// ID returns the listener ID.
func (w *WebSocket) ID() string {
	return w.id
}

// Addr returns the listener's address.
func (w *WebSocket) Addr() net.Addr {
	return &wsListenerAddr{addr: w.addr}
}

// Serve starts the WebSocket server.
func (w *WebSocket) Serve(handler ConnectionHandler) error {
	w.handler = handler

	mux := http.NewServeMux()
	mux.HandleFunc(w.config.Path, w.handleWebSocket)

	w.server = &http.Server{
		Addr:    w.addr,
		Handler: mux,
	}

	var ln net.Listener
	var err error

	if w.config.TLSConfig != nil {
		w.server.TLSConfig = w.config.TLSConfig
		ln, err = tls.Listen("tcp", w.addr, w.config.TLSConfig)
	} else {
		ln, err = net.Listen("tcp", w.addr)
	}
	if err != nil {
		return err
	}

	w.wg.Add(1)
	defer w.wg.Done()

	err = w.server.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (w *WebSocket) handleWebSocket(rw http.ResponseWriter, r *http.Request) {
	select {
	case <-w.closed:
		http.Error(rw, "server closing", http.StatusServiceUnavailable)
		return
	default:
	}

	ws, err := w.upgrader.Upgrade(rw, r, nil)
	if err != nil {
		return
	}

	conn := &wsConn{
		Conn:       ws,
		remoteAddr: r.RemoteAddr,
	}

	w.handler.HandleConnection(conn)
}

// Close stops the WebSocket server.
func (w *WebSocket) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	select {
	case <-w.closed:
		return errors.New("listener already closed")
	default:
		close(w.closed)
	}

	if w.server != nil {
		w.server.Close()
	}
	w.wg.Wait()
	return nil
}

// wsConn wraps websocket.Conn to implement net.Conn.
type wsConn struct {
	*websocket.Conn
	reader     io.Reader
	remoteAddr string
	mu         sync.Mutex
}

func (c *wsConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for {
		if c.reader == nil {
			messageType, r, err := c.Conn.NextReader()
			if err != nil {
				return 0, err
			}
			// MQTT over WebSocket uses binary messages
			if messageType != websocket.BinaryMessage {
				continue
			}
			c.reader = r
		}

		n, err := c.reader.Read(p)
		if err == io.EOF {
			c.reader = nil
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func (c *wsConn) Write(p []byte) (int, error) {
	err := c.Conn.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *wsConn) RemoteAddr() net.Addr {
	return &wsAddr{addr: c.remoteAddr}
}

func (c *wsConn) LocalAddr() net.Addr {
	return c.Conn.LocalAddr()
}

func (c *wsConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

func (c *wsConn) SetReadDeadline(t time.Time) error {
	return c.Conn.SetReadDeadline(t)
}

func (c *wsConn) SetWriteDeadline(t time.Time) error {
	return c.Conn.SetWriteDeadline(t)
}

// wsAddr implements net.Addr for WebSocket connections.
type wsAddr struct {
	addr string
}

func (a *wsAddr) Network() string { return "websocket" }
func (a *wsAddr) String() string  { return a.addr }

// wsListenerAddr implements net.Addr for WebSocket listener.
type wsListenerAddr struct {
	addr string
}

func (a *wsListenerAddr) Network() string { return "tcp" }
func (a *wsListenerAddr) String() string  { return a.addr }
