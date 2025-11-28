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
	b.RegisterHook(hooks.NewAuthHook(hooks.AuthConfig{
		Credentials: map[string]string{
			"admin":   "supersecret",
			"device":  "devicekey",
			"monitor": "viewonly",
		},
	}))

	// 2. Authorization - ACL rules
	b.RegisterHook(hooks.NewACLHook(hooks.ACLConfig{
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
	}))

	// 3. Rate limiting - prevent abuse
	b.RegisterHook(hooks.NewRateLimitHook(hooks.RateLimitConfig{
		PublishRate: 100, // 100 msg/sec per client
		BurstSize:   200,
	}))

	// 4. Logging - track activity
	b.RegisterHook(hooks.NewLoggerHook(hooks.LoggerConfig{
		Logger: logger,
		Level:  hooks.LogLevelConnection | hooks.LogLevelSession,
	}))

	// 5. $SYS metrics
	sysHook := hooks.NewSysHook(hooks.SysConfig{
		Publisher: func(topic string, payload []byte, retain bool) {
			// Log metrics (in production, inject into broker)
			slog.Debug("metric", "topic", topic, "value", string(payload))
		},
		Version:  "1.0.0",
		Interval: 30 * time.Second,
	})
	b.RegisterHook(sysHook)
	sysHook.Start()
	defer sysHook.Stop()

	// Start TCP listener
	ln, err := b.ListenTCP(":1883")
	if err != nil {
		slog.Error("Failed to listen", "error", err)
		os.Exit(1)
	}
	defer ln.Close()

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

	// Final stats
	stats := b.Stats()
	metrics := sysHook.Metrics()
	slog.Info("Final stats",
		"uptime", metrics.Uptime.Round(time.Second),
		"total_clients", metrics.ClientsTotal,
		"messages_in", metrics.MessagesReceived,
		"messages_out", metrics.MessagesSent,
		"retained", stats.Retained,
	)
}
