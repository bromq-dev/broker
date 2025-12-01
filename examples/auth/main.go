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
	"github.com/bromq-dev/broker/pkg/listeners"
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

	// Add TCP listener
	tcp := listeners.NewTCP("tcp", ":1883", nil)
	if err := b.AddListener(tcp); err != nil {
		log.Fatal(err)
	}

	log.Println("MQTT broker with auth listening on :1883")
	log.Println("Users: admin/secret, user1/password1, user2/password2, device/devicepass")

	// Wait for shutdown signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down...")
	b.Shutdown(context.Background())
}
