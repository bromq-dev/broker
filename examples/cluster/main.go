// Example: Clustered broker using Redis/Valkey for cross-node communication.
//
// This example demonstrates running multiple broker nodes that share state
// via Redis. Messages published to one node are delivered to subscribers
// on all nodes.
//
// Usage:
//
//	# Start Redis (or Valkey)
//	docker run -d -p 6379:6379 redis:alpine
//
//	# Start node 1
//	go run ./examples/cluster -port 1883 -node node1
//
//	# Start node 2 (in another terminal)
//	go run ./examples/cluster -port 1884 -node node2
//
//	# Test cross-node messaging
//	mosquitto_sub -p 1883 -t "test/#" &
//	mosquitto_pub -p 1884 -t "test/hello" -m "from node2"
//	# Subscriber on node1 receives the message!
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/hooks"
)

func main() {
	port := flag.Int("port", 1883, "MQTT listen port")
	nodeID := flag.String("node", "", "Unique node ID (required for clustering)")
	redisAddr := flag.String("redis", "localhost:6379", "Redis server address")
	flag.Parse()

	if *nodeID == "" {
		// Generate a unique node ID if not provided
		*nodeID = fmt.Sprintf("node-%d", os.Getpid())
	}

	// Configure logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Create broker
	b := broker.New(&broker.Config{
		RetainAvailable: true,
	})

	// Add Redis hook for clustering
	if err := b.AddHook(new(hooks.RedisHook), &hooks.RedisConfig{
		Addr:      *redisAddr,
		KeyPrefix: "mqtt:",
		NodeID:    *nodeID,
	}); err != nil {
		slog.Error("Failed to connect to Redis", "error", err, "addr", *redisAddr)
		slog.Info("Make sure Redis is running: docker run -d -p 6379:6379 redis:alpine")
		os.Exit(1)
	}

	// Add logging
	_ = b.AddHook(new(hooks.LoggerHook), &hooks.LoggerConfig{
		Logger: logger,
		Level:  hooks.LogLevelConnection | hooks.LogLevelPublish,
	})

	// Add $SYS metrics
	_ = b.AddHook(new(hooks.SysHook), &hooks.SysConfig{
		Version: "1.0.0-cluster",
	})

	// Start listener
	addr := fmt.Sprintf(":%d", *port)
	ln, err := b.ListenTCP(addr)
	if err != nil {
		slog.Error("Failed to listen", "error", err)
		os.Exit(1)
	}
	defer ln.Close()

	slog.Info("Clustered MQTT broker started",
		"addr", addr,
		"node_id", *nodeID,
		"redis", *redisAddr,
	)
	slog.Info("Features: Redis-backed retained messages, cross-node pub/sub")

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
