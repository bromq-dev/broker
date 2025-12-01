// Example: Broker with both TCP and WebSocket transports.
//
// This allows browser-based MQTT clients (like MQTT.js) to connect
// while also supporting standard TCP connections.
//
// Usage:
//
//	go run ./examples/websocket
//
// Test with mosquitto (TCP):
//
//	mosquitto_sub -t "test/#" -v
//	mosquitto_pub -t "test/hello" -m "from TCP"
//
// Test with websocat (WebSocket):
//
//	websocat ws://localhost:8083/mqtt --binary
//
// Test with MQTT.js (browser/Node.js):
//
//	const mqtt = require('mqtt')
//	const client = mqtt.connect('ws://localhost:8083/mqtt')
//	client.subscribe('test/#')
//	client.on('message', (topic, msg) => console.log(topic, msg.toString()))
package main

import (
	"context"
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
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	b := broker.New(nil)

	// Add logging
	_ = b.AddHook(new(hooks.LoggerHook), &hooks.LoggerConfig{
		Logger: logger,
		Level:  hooks.LogLevelConnection,
	})

	// Add $SYS metrics
	_ = b.AddHook(new(hooks.SysHook), nil)

	// Add TCP listener on :1883
	tcp := listeners.NewTCP("tcp", ":1883", nil)
	if err := b.AddListener(tcp); err != nil {
		slog.Error("Failed to add TCP listener", "error", err)
		os.Exit(1)
	}

	// Add WebSocket listener on :8083/mqtt
	ws := listeners.NewWebSocket("ws", ":8083", &listeners.WebSocketConfig{
		Path: "/mqtt",
	})
	if err := b.AddListener(ws); err != nil {
		slog.Error("Failed to add WebSocket listener", "error", err)
		os.Exit(1)
	}

	slog.Info("MQTT broker started",
		"tcp", ":1883",
		"websocket", "ws://localhost:8083/mqtt",
	)
	slog.Info("Browser clients can connect via WebSocket using MQTT.js")

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
