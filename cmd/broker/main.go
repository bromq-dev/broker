package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" // Import for side effects - registers /debug/pprof handlers
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/hooks"
	"github.com/bromq-dev/broker/pkg/listeners"
)

var (
	mqttAddr  = flag.String("addr", ":1883", "MQTT listen address")
	mqttsAddr = flag.String("tls-addr", ":8883", "MQTTS listen address")
	wsAddr    = flag.String("ws-addr", ":8083", "WebSocket listen address")
	wsPath    = flag.String("ws-path", "/mqtt", "WebSocket listen path")

	credentials credentialMap
	acls        aclSlice

	certFile  = flag.String("cert", "", "TLS certificate file (optional)")
	keyFile   = flag.String("key", "", "TLS private key file (optional)")
	pprofAddr = flag.String("pprof", "", "pprof HTTP server address (e.g. :6060)")
)

// Custom flag type for accumulating credentials
type credentialMap map[string]string

func (c *credentialMap) String() string { return "" }
func (c *credentialMap) Set(s string) error {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid credential format: %s (expected username:password)", s)
	}
	if *c == nil {
		*c = make(map[string]string)
	}
	(*c)[parts[0]] = parts[1]
	return nil
}

// Custom flag type for accumulating ACL rules
type aclSlice []hooks.ACLRule

func (a *aclSlice) String() string {
	rules := []string{}
	for _, rule := range *a {
		perm := ""
		if rule.Read {
			perm += "r"
		}
		if rule.Write {
			perm += "w"
		}
		rules = append(rules, fmt.Sprintf("%s:%s:%s", rule.Username, rule.TopicFilter, perm))
	}
	return strings.Join(rules, "\n")
}
func (a *aclSlice) Set(s string) error {
	// Parse from the ends: permissions are last, username is first
	// This allows topic filters to contain colons
	lastColon := strings.LastIndex(s, ":")
	if lastColon == -1 {
		return fmt.Errorf("invalid ACL format: %s (expected username:topicFilter:permissions)", s)
	}
	perm := s[lastColon+1:]
	rest := s[:lastColon]

	firstColon := strings.Index(rest, ":")
	if firstColon == -1 {
		return fmt.Errorf("invalid ACL format: %s (expected username:topicFilter:permissions)", s)
	}
	username := rest[:firstColon]
	topic := rest[firstColon+1:]

	*a = append(*a, hooks.ACLRule{
		Username:    username,
		TopicFilter: topic,
		Read:        strings.Contains(perm, "r"),
		Write:       strings.Contains(perm, "w"),
	})
	return nil
}

func init() {
	flag.Var(&credentials, "credential", "Credential: username:password (can be repeated)")
	flag.Var(&acls, "acl", "ACL rule: username:topicFilter:permissions (can be repeated)")
}

func main() {
	flag.Parse()

	// Create broker with default configuration
	b := broker.New(nil)

	// Add $SYS metrics - auto-starts on registration
	_ = b.AddHook(new(hooks.SysHook), nil)

	if len(credentials) > 0 {
		_ = b.AddHook(new(hooks.AuthHook), &hooks.AuthConfig{
			Credentials: credentials,
		})
		log.Printf("Authentication enabled for %d user(s)", len(credentials))
	}

	if len(acls) > 0 {
		_ = b.AddHook(new(hooks.ACLHook), &hooks.ACLConfig{
			Rules: acls,
		})
		log.Printf("ACL enabled with %d rule(s):\n%s", len(acls), acls.String())
	}

	// Add TCP listener
	tcp := listeners.NewTCP("tcp", *mqttAddr, nil)
	if err := b.AddListener(tcp); err != nil {
		log.Fatalf("Failed to add listener: %v", err)
	}

	// Add WebSocket listener
	ws := listeners.NewWebSocket("ws", *wsAddr, &listeners.WebSocketConfig{
		Path: *wsPath,
	})
	if err := b.AddListener(ws); err != nil {
		log.Fatalf("Failed to add WebSocket listener: %v", err)
	}

	if *certFile != "" && *keyFile != "" {
		cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
		if err != nil {
			log.Fatalf("Failed to load TLS certificate: %v", err)
		}

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
		if err != nil {
			log.Fatalf("Failed to load TLS certificate: %v", err)
		}
		// Add TLS TCP listener
		tlsListener := listeners.NewTCP("tcp+tls", *mqttsAddr, &listeners.TCPConfig{
			TLSConfig: tlsConfig,
		})
		if err := b.AddListener(tlsListener); err != nil {
			log.Fatalf("Failed to add TLS listener: %v", err)
		}
		log.Printf("TLS listening on %s", *mqttsAddr)
	}

	// Start pprof server if enabled
	if *pprofAddr != "" {
		go func() {
			log.Printf("pprof server listening on http://%s/debug/pprof/", *pprofAddr)
			if err := http.ListenAndServe(*pprofAddr, nil); err != nil {
				log.Printf("pprof server error: %v", err)
			}
		}()
	}

	log.Printf("MQTT broker listening on %s", *mqttAddr)
	log.Printf("WebSocket listening on %s%s", *wsAddr, *wsPath)
	log.Println("Subscribe to $SYS/# to see broker metrics")

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := b.Shutdown(ctx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}

	log.Println("Broker stopped")
}
