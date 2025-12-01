// Example: Redis-based clustered broker using Redis/Valkey for distributed state.
//
// This example demonstrates running multiple broker nodes that share full
// distributed state via Redis/Valkey with eventual consistency.
//
// Features:
//   - Node registry with heartbeat and auto-expiry
//   - Subscription storage and route index for targeted fan-out
//   - Retained message storage with index
//   - Redis pub/sub for message routing
//
// Usage:
//
//	# Start Redis (or Valkey)
//	docker run -d -p 6379:6379 redis:alpine
//
//	# Start node 1
//	go run ./examples/redis-cluster -port 1883 -node node1
//
//	# Start node 2 (in another terminal)
//	go run ./examples/redis-cluster -port 1884 -node node2
//
//	# Test cross-node messaging
//	mosquitto_sub -p 1883 -t "test/#" &
//	mosquitto_pub -p 1884 -t "test/hello" -m "from node2"
//	# Subscriber on node1 receives the message!
//
// With Docker Compose:
//
//	cd examples/redis-cluster
//	docker compose up -d
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
	"github.com/bromq-dev/broker/pkg/cluster"
	"github.com/bromq-dev/broker/pkg/hooks"
	"github.com/bromq-dev/broker/pkg/listeners"
)

func main() {
	port := flag.Int("port", 1883, "MQTT listen port")
	nodeID := flag.String("node", "", "Unique node ID (required for clustering)")
	redisAddr := flag.String("redis", "localhost:6379", "Redis server address")
	flag.Parse()

	if *nodeID == "" {
		// Default to hostname (container ID in Docker, pod name in K8s)
		if hostname, err := os.Hostname(); err == nil {
			*nodeID = hostname
		} else {
			*nodeID = fmt.Sprintf("node-%d", os.Getpid())
		}
	}

	// Configure logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Create broker with full capabilities
	b := broker.New(nil)

	// Add Redis cluster hook using the composable cluster package
	clusterHook, err := cluster.NewRedisCluster(&cluster.RedisConfig{
		Addr:   *redisAddr,
		NodeID: *nodeID,
	})
	if err != nil {
		slog.Error("Failed to create Redis cluster", "error", err, "addr", *redisAddr)
		slog.Info("Make sure Redis is running: docker run -d -p 6379:6379 redis:alpine")
		os.Exit(1)
	}

	if err := b.AddHook(clusterHook, nil); err != nil {
		slog.Error("Failed to add cluster hook", "error", err)
		os.Exit(1)
	}

	// Add logging
	_ = b.AddHook(new(hooks.LoggerHook), &hooks.LoggerConfig{
		Logger: logger,
		Level:  hooks.LogLevelConnection | hooks.LogLevelPublish,
	})

	// Add $SYS metrics
	_ = b.AddHook(new(hooks.SysHook), &hooks.SysConfig{
		Version: "1.0.0-redis-cluster",
	})

	// Add TCP listener
	addr := fmt.Sprintf(":%d", *port)
	tcp := listeners.NewTCP("tcp", addr, nil)
	if err := b.AddListener(tcp); err != nil {
		slog.Error("Failed to add listener", "error", err)
		os.Exit(1)
	}

	slog.Info("Redis-clustered MQTT broker started",
		"addr", addr,
		"node_id", *nodeID,
		"redis", *redisAddr,
	)
	slog.Info("Features: eventual consistency, subscription sync, retained messages, redis pub/sub routing")

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
