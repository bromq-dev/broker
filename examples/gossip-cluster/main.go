// Example: Gossip-based clustering with no external dependencies.
//
// This example demonstrates running MQTT broker nodes that cluster via gossip
// protocol. No external services required - nodes discover and sync with each other.
//
// Features:
//   - Zero external dependencies (no Redis, no etcd)
//   - Gossip-based state replication (subscriptions, retained messages)
//   - gRPC for direct message routing
//   - DNS-based discovery for Kubernetes
//   - Supports scale-to-1 (single node works fine)
//
// Usage:
//
//	# Single node (works alone)
//	go run ./examples/gossip-cluster -port 1883 -gossip-port 7946
//
//	# Add second node (joins first)
//	go run ./examples/gossip-cluster -port 1884 -gossip-port 7956 -join localhost:7946
//
//	# Add third node
//	go run ./examples/gossip-cluster -port 1885 -gossip-port 7966 -join localhost:7946
//
//	# Test cross-node messaging
//	mosquitto_sub -p 1883 -t "test/#" &
//	mosquitto_pub -p 1884 -t "test/hello" -m "from node2"
//
// Kubernetes:
//
//	go run ./examples/gossip-cluster -dns mqtt-broker.default.svc.cluster.local
//
// With Docker Compose:
//
//	cd examples/gossip-cluster
//	docker compose up -d
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/cluster"
	"github.com/bromq-dev/broker/pkg/hooks"
	"github.com/bromq-dev/broker/pkg/listeners"
)

func main() {
	port := flag.Int("port", 1883, "MQTT listen port")
	nodeID := flag.String("node", "", "Unique node ID (default: hostname)")
	gossipPort := flag.Int("gossip-port", 7946, "Gossip protocol port")
	grpcPort := flag.Int("grpc-port", 7947, "gRPC routing port")
	join := flag.String("join", "", "Comma-separated list of nodes to join (host:port)")
	dns := flag.String("dns", "", "DNS name for discovery (K8s headless service)")
	flag.Parse()

	if *nodeID == "" {
		if hostname, err := os.Hostname(); err == nil {
			*nodeID = hostname
		} else {
			*nodeID = fmt.Sprintf("node-%d", os.Getpid())
		}
	}

	// Parse join addresses
	var joinAddrs []string
	if *join != "" {
		for _, addr := range strings.Split(*join, ",") {
			joinAddrs = append(joinAddrs, strings.TrimSpace(addr))
		}
	}

	// Configure logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Create broker
	b := broker.New(nil)

	// Add gossip cluster hook using the new composable cluster package
	clusterHook := cluster.NewGossipCluster(&cluster.GossipConfig{
		NodeID:     *nodeID,
		GossipPort: *gossipPort,
		GRPCAddr:   fmt.Sprintf(":%d", *grpcPort),
		JoinAddrs:  joinAddrs,
		DNSName:    *dns,
	})
	if err := b.AddHook(clusterHook, nil); err != nil {
		slog.Error("Failed to initialize gossip cluster", "error", err)
		os.Exit(1)
	}

	// Add logging
	_ = b.AddHook(new(hooks.LoggerHook), &hooks.LoggerConfig{
		Logger: logger,
		Level:  hooks.LogLevelConnection | hooks.LogLevelPublish,
	})

	// Add $SYS metrics
	_ = b.AddHook(new(hooks.SysHook), &hooks.SysConfig{
		Version: "1.0.0-gossip-cluster",
	})

	// Add TCP listener
	addr := fmt.Sprintf(":%d", *port)
	tcp := listeners.NewTCP("tcp", addr, nil)
	if err := b.AddListener(tcp); err != nil {
		slog.Error("Failed to add listener", "error", err)
		os.Exit(1)
	}

	slog.Info("Gossip-clustered MQTT broker started",
		"addr", addr,
		"node_id", *nodeID,
		"gossip_port", *gossipPort,
		"grpc_port", *grpcPort,
	)

	if len(joinAddrs) > 0 {
		slog.Info("Joining cluster", "peers", joinAddrs)
	} else if *dns != "" {
		slog.Info("Using DNS discovery", "dns", *dns)
	} else {
		slog.Info("Starting as single node (use -join to add peers)")
	}

	slog.Info("Features: zero dependencies, gossip state sync, gRPC routing, scale-to-1")

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
