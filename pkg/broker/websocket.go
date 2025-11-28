package broker

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketListener accepts WebSocket connections and hands them to the broker.
type WebSocketListener struct {
	broker   *Broker
	server   *http.Server
	upgrader websocket.Upgrader
	path     string
	wg       sync.WaitGroup
	closed   chan struct{}
}

// ListenWebSocket creates a WebSocket listener on the given address.
// The path parameter specifies the URL path (default: "/mqtt").
func (b *Broker) ListenWebSocket(addr, path string) (*WebSocketListener, error) {
	if path == "" {
		path = "/mqtt"
	}

	wsl := &WebSocketListener{
		broker: b,
		path:   path,
		upgrader: websocket.Upgrader{
			Subprotocols: []string{"mqtt"},
			CheckOrigin:  func(r *http.Request) bool { return true },
		},
		closed: make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc(path, wsl.handleWebSocket)

	wsl.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	wsl.wg.Add(1)
	go func() {
		defer wsl.wg.Done()
		wsl.server.Serve(ln)
	}()

	return wsl, nil
}

// ListenWebSocketTLS creates a TLS WebSocket listener.
func (b *Broker) ListenWebSocketTLS(addr, path string, config *tls.Config) (*WebSocketListener, error) {
	if path == "" {
		path = "/mqtt"
	}

	wsl := &WebSocketListener{
		broker: b,
		path:   path,
		upgrader: websocket.Upgrader{
			Subprotocols: []string{"mqtt"},
			CheckOrigin:  func(r *http.Request) bool { return true },
		},
		closed: make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc(path, wsl.handleWebSocket)

	wsl.server = &http.Server{
		Addr:      addr,
		Handler:   mux,
		TLSConfig: config,
	}

	ln, err := tls.Listen("tcp", addr, config)
	if err != nil {
		return nil, err
	}

	wsl.wg.Add(1)
	go func() {
		defer wsl.wg.Done()
		wsl.server.Serve(ln)
	}()

	return wsl, nil
}

func (wsl *WebSocketListener) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	select {
	case <-wsl.closed:
		http.Error(w, "server closing", http.StatusServiceUnavailable)
		return
	default:
	}

	ws, err := wsl.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	// Wrap WebSocket connection to implement net.Conn
	conn := &wsConn{
		Conn:       ws,
		remoteAddr: r.RemoteAddr,
	}

	wsl.broker.HandleConnection(conn)
}

// Close closes the WebSocket listener.
func (wsl *WebSocketListener) Close() error {
	close(wsl.closed)
	err := wsl.server.Close()
	wsl.wg.Wait()
	return err
}

// Addr returns the listener address.
func (wsl *WebSocketListener) Addr() string {
	return wsl.server.Addr
}

// wsConn wraps websocket.Conn to implement net.Conn for the broker.
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
			continue // Get next message
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
