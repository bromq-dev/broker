package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/hooks"
)

func main() {
	addr := flag.String("addr", ":1883", "MQTT listen address")
	flag.Parse()

	// Create server with default configuration
	server := broker.NewServer(broker.DefaultConfig())

	// Add $SYS metrics - auto-starts on registration
	_ = server.AddHook(new(hooks.SysHook), nil)

	// Start TCP listener
	if err := server.ListenTCP(*addr); err != nil {
		log.Fatalf("Failed to start listener: %v", err)
	}

	log.Printf("MQTT broker listening on %s", *addr)
	log.Println("Subscribe to $SYS/# to see broker metrics")

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}

	log.Println("Broker stopped")
}
