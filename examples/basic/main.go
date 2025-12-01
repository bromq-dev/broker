// Example: Basic broker with no hooks.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/listeners"
)

func main() {
	b := broker.New(nil)

	// Add TCP listener
	tcp := listeners.NewTCP("tcp", ":1883", nil)
	if err := b.AddListener(tcp); err != nil {
		log.Fatal(err)
	}

	log.Println("MQTT broker listening on :1883")

	// Wait for shutdown signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down...")
	b.Shutdown(context.Background())
}
