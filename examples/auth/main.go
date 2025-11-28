// Example: Broker with username/password authentication.
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

	// Add authentication hook with static credentials
	_ = b.AddHook(new(hooks.AuthHook), &hooks.AuthConfig{
		Credentials: map[string]string{
			"admin":  "secret",
			"user1":  "password1",
			"user2":  "password2",
			"device": "devicepass",
		},
	})

	// Start TCP listener
	ln, err := b.ListenTCP(":1883")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	log.Println("MQTT broker with auth listening on :1883")
	log.Println("Users: admin/secret, user1/password1, user2/password2, device/devicepass")

	// Wait for shutdown signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down...")
	b.Shutdown(context.Background())
}
