// Example: Full-featured broker with all hooks.
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
	// Configure structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Create broker with custom config
	b := broker.New(&broker.Config{
		MaxConnections:  1000,
		ConnectTimeout:  10 * time.Second,
		MaxInflight:     100,
		RetainAvailable: true,
	})

	// 1. Authentication - username/password
	_ = b.AddHook(new(hooks.AuthHook), &hooks.AuthConfig{
		Credentials: map[string]string{
			"admin":   "supersecret",
			"device":  "devicekey",
			"monitor": "viewonly",
		},
	})

	// 2. Authorization - ACL rules
	_ = b.AddHook(new(hooks.ACLHook), &hooks.ACLConfig{
		Rules: []hooks.ACLRule{
			// Admin has full access
			{Username: "admin", TopicFilter: "#", Read: true, Write: true},

			// Devices can publish telemetry and subscribe to commands
			{Username: "device", TopicFilter: "telemetry/#", Read: false, Write: true},
			{Username: "device", TopicFilter: "commands/#", Read: true, Write: false},

			// Monitor can only read
			{Username: "monitor", TopicFilter: "#", Read: true, Write: false},

			// Public topics for everyone
			{TopicFilter: "public/#", Read: true, Write: true},
		},
		DenyByDefault: true,
	})

	// 3. Rate limiting - prevent abuse
	_ = b.AddHook(new(hooks.RateLimitHook), &hooks.RateLimitConfig{
		PublishRate: 100, // 100 msg/sec per client
		BurstSize:   200,
	})

	// 4. Logging - track activity
	_ = b.AddHook(new(hooks.LoggerHook), &hooks.LoggerConfig{
		Logger: logger,
		Level:  hooks.LogLevelConnection | hooks.LogLevelSession,
	})

	// 5. $SYS metrics - auto-starts on registration
	_ = b.AddHook(new(hooks.SysHook), &hooks.SysConfig{
		Version:  "1.0.0",
		Interval: 30 * time.Second,
	})

	// Add TCP listener
	tcp := listeners.NewTCP("tcp", ":1883", nil)
	if err := b.AddListener(tcp); err != nil {
		slog.Error("Failed to add listener", "error", err)
		os.Exit(1)
	}

	slog.Info("Full-featured MQTT broker started",
		"addr", ":1883",
		"features", "auth, acl, ratelimit, logging, metrics",
	)
	slog.Info("Credentials",
		"admin", "admin/supersecret (full access)",
		"device", "device/devicekey (telemetry write, commands read)",
		"monitor", "monitor/viewonly (read-only)",
	)

	// Wait for shutdown signal
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
