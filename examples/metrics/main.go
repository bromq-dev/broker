// Example: Broker with $SYS metrics and logging.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/hooks"
)

func main() {
	// Use structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	b := broker.New(nil)

	// Add $SYS metrics hook - auto-starts on registration
	_ = b.AddHook(new(hooks.SysHook), &hooks.SysConfig{
		Version: "1.0.0",
	})

	// Add logging hook
	_ = b.AddHook(new(hooks.LoggerHook), &hooks.LoggerConfig{
		Logger: logger,
		Level:  hooks.LogLevelAll,
	})

	// Add rate limiting (100 messages/second per client)
	_ = b.AddHook(new(hooks.RateLimitHook), &hooks.RateLimitConfig{
		PublishRate: 100,
	})

	// Start TCP listener
	ln, err := b.ListenTCP(":1883")
	if err != nil {
		slog.Error("Failed to listen", "error", err)
		os.Exit(1)
	}
	defer ln.Close()

	slog.Info("MQTT broker started", "addr", ":1883")
	slog.Info("Features enabled: $SYS metrics, logging, rate limiting (100 msg/s)")

	// Wait for shutdown signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	slog.Info("Shutting down...")
	b.Shutdown(context.Background())
}
