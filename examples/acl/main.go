// Example: Broker with ACL-based topic authorization.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/hooks"
)

func main() {
	b := broker.New(nil)

	// Add authentication
	b.RegisterHook(hooks.NewAuthHook(hooks.AuthConfig{
		Credentials: map[string]string{
			"admin":   "admin",
			"sensor":  "sensor",
			"display": "display",
		},
	}))

	// Add ACL authorization
	b.RegisterHook(hooks.NewACLHook(hooks.ACLConfig{
		Rules: []hooks.ACLRule{
			// Admin can read/write everything
			{Username: "admin", TopicFilter: "#", Read: true, Write: true},

			// Sensors can only publish to sensor topics
			{Username: "sensor", TopicFilter: "sensors/#", Read: false, Write: true},

			// Display can only subscribe to sensor topics
			{Username: "display", TopicFilter: "sensors/#", Read: true, Write: false},

			// Everyone can use the public topic
			{TopicFilter: "public/#", Read: true, Write: true},
		},
		DenyByDefault: true,
	}))

	// Add logging to see what's happening
	b.RegisterHook(hooks.NewLoggerHook(hooks.LoggerConfig{
		Level: hooks.LogLevelConnection | hooks.LogLevelSubscribe,
	}))

	// Start TCP listener
	ln, err := b.ListenTCP(":1883")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	log.Println("MQTT broker with ACL listening on :1883")
	log.Println("Users:")
	log.Println("  admin/admin   - full access to all topics")
	log.Println("  sensor/sensor - can publish to sensors/#")
	log.Println("  display/display - can subscribe to sensors/#")
	log.Println("  anyone - can use public/#")

	// Wait for shutdown signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down...")
	b.Shutdown(context.Background())
}
