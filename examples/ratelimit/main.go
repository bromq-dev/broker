// Example: Broker with rate limiting.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/hooks"
)

func main() {
	b := broker.New(nil)

	// Add rate limiting - 10 messages per second, burst of 20
	_ = b.AddHook(new(hooks.RateLimitHook), &hooks.RateLimitConfig{
		PublishRate: 10,
		Interval:    time.Second,
		BurstSize:   20,
	})

	// Add logging to see rate limit rejections
	_ = b.AddHook(new(hooks.LoggerHook), &hooks.LoggerConfig{
		Level: hooks.LogLevelConnection | hooks.LogLevelPublish,
	})

	// Start TCP listener
	ln, err := b.ListenTCP(":1883")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	log.Println("MQTT broker with rate limiting on :1883")
	log.Println("Rate limit: 10 messages/second, burst: 20")

	// Wait for shutdown signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down...")
	b.Shutdown(context.Background())
}
