// Example: Basic broker with no hooks.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bromq-dev/broker/pkg/broker"
)

func main() {
	b := broker.New(nil)

	// Start TCP listener
	ln, err := b.ListenTCP(":1883")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	log.Println("MQTT broker listening on :1883")

	// Wait for shutdown signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down...")
	b.Shutdown(context.Background())
}
