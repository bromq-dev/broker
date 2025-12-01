// Example: Broker with TLS on all transports.
//
// This example demonstrates secure MQTT connections using TLS for:
//   - TCP (mqtts:// on port 8883)
//   - WebSocket (wss:// on port 8084)
//
// Generate test certificates:
//
//	# Generate CA
//	openssl genrsa -out ca.key 2048
//	openssl req -new -x509 -days 365 -key ca.key -out ca.crt -subj "/CN=MQTT CA"
//
//	# Generate server cert
//	openssl genrsa -out server.key 2048
//	openssl req -new -key server.key -out server.csr -subj "/CN=localhost"
//	openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt
//
// Usage:
//
//	go run ./examples/tls -cert server.crt -key server.key
//
// Test with mosquitto:
//
//	mosquitto_sub -h localhost -p 8883 --cafile ca.crt -t "test/#" -v
//	mosquitto_pub -h localhost -p 8883 --cafile ca.crt -t "test/hello" -m "secure"
//
// Test with MQTT.js (browser/Node.js):
//
//	const mqtt = require('mqtt')
//	const client = mqtt.connect('wss://localhost:8084/mqtt', {
//	  rejectUnauthorized: false // for self-signed certs
//	})
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/hooks"
	"github.com/bromq-dev/broker/pkg/listeners"
)

func main() {
	certFile := flag.String("cert", "server.crt", "TLS certificate file")
	keyFile := flag.String("key", "server.key", "TLS private key file")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load TLS certificate
	cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
	if err != nil {
		slog.Error("Failed to load TLS certificate", "error", err)
		slog.Info("Generate test certs with:")
		slog.Info("  openssl req -x509 -newkey rsa:2048 -keyout server.key -out server.crt -days 365 -nodes -subj '/CN=localhost'")
		os.Exit(1)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	b := broker.New(nil)

	// Add logging
	_ = b.AddHook(new(hooks.LoggerHook), &hooks.LoggerConfig{
		Logger: logger,
		Level:  hooks.LogLevelConnection,
	})

	// Plain TCP on :1883 (optional, for local testing)
	tcp := listeners.NewTCP("tcp", ":1883", nil)
	if err := b.AddListener(tcp); err != nil {
		slog.Error("Failed to add TCP listener", "error", err)
		os.Exit(1)
	}

	// TLS TCP on :8883 (standard MQTTS port)
	tcpTLS := listeners.NewTCP("tcp+tls", ":8883", &listeners.TCPConfig{
		TLSConfig: tlsConfig,
	})
	if err := b.AddListener(tcpTLS); err != nil {
		slog.Error("Failed to add TLS listener", "error", err)
		os.Exit(1)
	}

	// Plain WebSocket on :8083 (optional, for local testing)
	ws := listeners.NewWebSocket("ws", ":8083", &listeners.WebSocketConfig{
		Path: "/mqtt",
	})
	if err := b.AddListener(ws); err != nil {
		slog.Error("Failed to add WebSocket listener", "error", err)
		os.Exit(1)
	}

	// TLS WebSocket on :8084 (secure WebSocket)
	wss := listeners.NewWebSocket("wss", ":8084", &listeners.WebSocketConfig{
		Path:      "/mqtt",
		TLSConfig: tlsConfig,
	})
	if err := b.AddListener(wss); err != nil {
		slog.Error("Failed to add WSS listener", "error", err)
		os.Exit(1)
	}

	slog.Info("MQTT broker started with TLS",
		"tcp", ":1883",
		"tcp+tls", ":8883",
		"ws", "ws://localhost:8083/mqtt",
		"wss", "wss://localhost:8084/mqtt",
	)

	// Wait for shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	slog.Info("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := b.Shutdown(ctx); err != nil {
		slog.Error("Shutdown error", "error", err)
	}
}
