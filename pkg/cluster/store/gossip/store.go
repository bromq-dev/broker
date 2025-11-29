// Package gossip provides a gossip-based Store using HashiCorp memberlist.
package gossip

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/bromq-dev/broker/pkg/cluster/types"
	"github.com/bromq-dev/broker/pkg/packet"
	"github.com/hashicorp/memberlist"
)

// Store implements cluster.Store using gossip-based replication.
// State is replicated across all nodes using HashiCorp memberlist.
type Store struct {
	cfg    *Config
	nodeID string

	memberlist *memberlist.Memberlist
	broadcasts *memberlist.TransmitLimitedQueue

	// Subscription state (replicated via gossip)
	subsMu sync.RWMutex
	subs   map[string]map[string]*nodeSubInfo // filter -> nodeID -> info

	// Retained messages (replicated via gossip)
	retainedMu sync.RWMutex
	retained   map[string]*retainedMsg // topic -> message

	// Node registry
	nodesMu sync.RWMutex
	nodes   map[string]*types.NodeInfo // nodeID -> info

	cancel context.CancelFunc
	log    *slog.Logger
}

type nodeSubInfo struct {
	Addr string `json:"addr"` // Routing address
	QoS  byte   `json:"qos"`
}

type retainedMsg struct {
	Payload []byte `json:"payload"`
	QoS     byte   `json:"qos"`
}

// Config configures the gossip store.
type Config struct {
	// NodeID uniquely identifies this node. Defaults to hostname.
	NodeID string

	// BindAddr is the address to bind for gossip. Default: "0.0.0.0".
	BindAddr string

	// BindPort is the port for gossip protocol. Default: 7946.
	BindPort int

	// AdvertiseAddr is the address to advertise to other nodes.
	// If empty, auto-detected.
	AdvertiseAddr string

	// AdvertisePort is the port to advertise. Default: same as BindPort.
	AdvertisePort int

	// RoutingAddr is the address other nodes use to connect for message routing.
	// This should be the gRPC router address (e.g., "node1:7947").
	// If empty, derived from NodeID and gRPC port.
	RoutingAddr string

	// JoinAddrs is a list of existing nodes to join. Format: "host:port".
	JoinAddrs []string

	// DNSName for DNS-based discovery (K8s headless service).
	DNSName string

	// DNSRefreshInterval is how often to refresh DNS. Default: 30s.
	DNSRefreshInterval time.Duration

	// Logger for logging. If nil, uses slog.Default().
	Logger *slog.Logger
}

// Gossip message types
const (
	msgSubscribe byte = iota + 1
	msgUnsubscribe
	msgRetain
	msgDeleteRetain
	msgNodeRegister
)

type gossipMsg struct {
	Type   byte   `json:"t"`
	NodeID string `json:"n"`
	Filter string `json:"f,omitempty"`
	Topic  string `json:"p,omitempty"`
	Addr   string `json:"a,omitempty"`
	QoS    byte   `json:"q,omitempty"`
	Data   []byte `json:"d,omitempty"`
}

// NewStore creates a new gossip-based store.
func NewStore(cfg *Config) *Store {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.NodeID == "" {
		hostname, _ := os.Hostname()
		cfg.NodeID = hostname
	}
	if cfg.BindAddr == "" {
		cfg.BindAddr = "0.0.0.0"
	}
	if cfg.BindPort == 0 {
		cfg.BindPort = 7946
	}
	if cfg.AdvertisePort == 0 {
		cfg.AdvertisePort = cfg.BindPort
	}
	if cfg.DNSRefreshInterval == 0 {
		cfg.DNSRefreshInterval = 30 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &Store{
		cfg:      cfg,
		nodeID:   cfg.NodeID,
		subs:     make(map[string]map[string]*nodeSubInfo),
		retained: make(map[string]*retainedMsg),
		nodes:    make(map[string]*types.NodeInfo),
		log:      cfg.Logger,
	}
}

func (s *Store) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Initialize memberlist
	mlCfg := memberlist.DefaultLANConfig()
	mlCfg.Name = s.nodeID
	mlCfg.BindAddr = s.cfg.BindAddr
	mlCfg.BindPort = s.cfg.BindPort
	mlCfg.AdvertisePort = s.cfg.AdvertisePort
	if s.cfg.AdvertiseAddr != "" {
		mlCfg.AdvertiseAddr = s.cfg.AdvertiseAddr
	}

	mlCfg.Delegate = &gossipDelegate{store: s}
	mlCfg.Events = &gossipEvents{store: s}

	ml, err := memberlist.Create(mlCfg)
	if err != nil {
		return fmt.Errorf("memberlist create: %w", err)
	}
	s.memberlist = ml

	s.broadcasts = &memberlist.TransmitLimitedQueue{
		NumNodes:       func() int { return ml.NumMembers() },
		RetransmitMult: 3,
	}

	// Join cluster
	if err := s.joinCluster(); err != nil {
		s.log.Warn("failed to join cluster, starting as single node", "error", err)
	}

	// DNS refresh loop
	if s.cfg.DNSName != "" {
		go s.dnsRefreshLoop(ctx)
	}

	s.log.Info("gossip store started",
		"node_id", s.nodeID,
		"bind", fmt.Sprintf("%s:%d", s.cfg.BindAddr, s.cfg.BindPort),
	)

	return nil
}

func (s *Store) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.memberlist != nil {
		s.memberlist.Leave(5 * time.Second)
		s.memberlist.Shutdown()
	}
	return nil
}

func (s *Store) NodeID() string {
	return s.nodeID
}

func (s *Store) RegisterNode(ctx context.Context, info types.NodeInfo) error {
	s.nodesMu.Lock()
	s.nodes[info.ID] = &info
	s.nodesMu.Unlock()

	s.broadcast(gossipMsg{
		Type:   msgNodeRegister,
		NodeID: info.ID,
		Addr:   info.Addr,
	})

	return nil
}

func (s *Store) DeregisterNode(ctx context.Context) error {
	s.nodesMu.Lock()
	delete(s.nodes, s.nodeID)
	s.nodesMu.Unlock()
	return nil
}

func (s *Store) GetNodes(ctx context.Context) ([]types.NodeInfo, error) {
	s.nodesMu.RLock()
	defer s.nodesMu.RUnlock()

	result := make([]types.NodeInfo, 0, len(s.nodes))
	for _, info := range s.nodes {
		result = append(result, *info)
	}
	return result, nil
}

func (s *Store) Subscribe(ctx context.Context, filter string, qos byte) error {
	s.subsMu.Lock()
	if s.subs[filter] == nil {
		s.subs[filter] = make(map[string]*nodeSubInfo)
	}

	// Get this node's routing address
	addr := s.getRoutingAddr()
	s.subs[filter][s.nodeID] = &nodeSubInfo{
		Addr: addr,
		QoS:  qos,
	}
	s.subsMu.Unlock()

	s.broadcast(gossipMsg{
		Type:   msgSubscribe,
		NodeID: s.nodeID,
		Filter: filter,
		Addr:   addr,
		QoS:    qos,
	})

	return nil
}

func (s *Store) Unsubscribe(ctx context.Context, filter string) error {
	s.subsMu.Lock()
	if s.subs[filter] != nil {
		delete(s.subs[filter], s.nodeID)
		if len(s.subs[filter]) == 0 {
			delete(s.subs, filter)
		}
	}
	s.subsMu.Unlock()

	s.broadcast(gossipMsg{
		Type:   msgUnsubscribe,
		NodeID: s.nodeID,
		Filter: filter,
	})

	return nil
}

func (s *Store) GetNodesForTopic(ctx context.Context, topic string) ([]types.SubscriptionInfo, error) {
	s.subsMu.RLock()
	defer s.subsMu.RUnlock()

	// Track unique nodes with highest QoS
	nodeMap := make(map[string]types.SubscriptionInfo)
	for filter, nodes := range s.subs {
		if topicMatchesFilter(topic, filter) {
			for nodeID, info := range nodes {
				if existing, ok := nodeMap[nodeID]; !ok || info.QoS > existing.QoS {
					nodeMap[nodeID] = types.SubscriptionInfo{
						NodeInfo: types.NodeInfo{
							ID:   nodeID,
							Addr: info.Addr,
						},
						QoS: info.QoS,
					}
				}
			}
		}
	}

	result := make([]types.SubscriptionInfo, 0, len(nodeMap))
	for _, info := range nodeMap {
		result = append(result, info)
	}
	return result, nil
}

func (s *Store) StoreRetained(ctx context.Context, topic string, msg *packet.Publish) error {
	if msg == nil || len(msg.Payload) == 0 {
		s.retainedMu.Lock()
		delete(s.retained, topic)
		s.retainedMu.Unlock()

		s.broadcast(gossipMsg{
			Type:  msgDeleteRetain,
			Topic: topic,
		})
	} else {
		s.retainedMu.Lock()
		s.retained[topic] = &retainedMsg{
			Payload: msg.Payload,
			QoS:     byte(msg.QoS),
		}
		s.retainedMu.Unlock()

		s.broadcast(gossipMsg{
			Type:  msgRetain,
			Topic: topic,
			Data:  msg.Payload,
			QoS:   byte(msg.QoS),
		})
	}
	return nil
}

func (s *Store) GetRetained(ctx context.Context, filter string) ([]*packet.Publish, error) {
	s.retainedMu.RLock()
	defer s.retainedMu.RUnlock()

	var results []*packet.Publish
	for topic, msg := range s.retained {
		if topicMatchesFilter(topic, filter) {
			results = append(results, &packet.Publish{
				TopicName: topic,
				Payload:   msg.Payload,
				QoS:       packet.QoS(msg.QoS),
				Retain:    true,
			})
		}
	}
	return results, nil
}

func (s *Store) getRoutingAddr() string {
	// Use explicitly configured routing address (preferred)
	if s.cfg.RoutingAddr != "" {
		return s.cfg.RoutingAddr
	}
	// Fallback to advertise address if set
	if s.cfg.AdvertiseAddr != "" {
		return s.cfg.AdvertiseAddr
	}
	// Fallback to node ID (works in Docker/K8s where hostname is routable)
	return s.nodeID
}

func (s *Store) broadcast(msg gossipMsg) {
	data, _ := json.Marshal(msg)
	s.broadcasts.QueueBroadcast(&gossipBroadcast{data: data})
}

func (s *Store) joinCluster() error {
	var addrs []string

	// Try DNS first
	if s.cfg.DNSName != "" {
		ips, err := net.LookupHost(s.cfg.DNSName)
		if err == nil {
			for _, ip := range ips {
				addrs = append(addrs, fmt.Sprintf("%s:%d", ip, s.cfg.BindPort))
			}
		}
	}

	// Fall back to static list
	if len(addrs) == 0 {
		addrs = s.cfg.JoinAddrs
	}

	if len(addrs) == 0 {
		return nil // Single node
	}

	_, err := s.memberlist.Join(addrs)
	return err
}

func (s *Store) dnsRefreshLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.DNSRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.joinCluster()
		}
	}
}

func (s *Store) handleGossipMsg(msg gossipMsg) {
	switch msg.Type {
	case msgSubscribe:
		s.subsMu.Lock()
		if s.subs[msg.Filter] == nil {
			s.subs[msg.Filter] = make(map[string]*nodeSubInfo)
		}
		s.subs[msg.Filter][msg.NodeID] = &nodeSubInfo{
			Addr: msg.Addr,
			QoS:  msg.QoS,
		}
		s.subsMu.Unlock()

	case msgUnsubscribe:
		s.subsMu.Lock()
		if s.subs[msg.Filter] != nil {
			delete(s.subs[msg.Filter], msg.NodeID)
			if len(s.subs[msg.Filter]) == 0 {
				delete(s.subs, msg.Filter)
			}
		}
		s.subsMu.Unlock()

	case msgRetain:
		s.retainedMu.Lock()
		s.retained[msg.Topic] = &retainedMsg{
			Payload: msg.Data,
			QoS:     msg.QoS,
		}
		s.retainedMu.Unlock()

	case msgDeleteRetain:
		s.retainedMu.Lock()
		delete(s.retained, msg.Topic)
		s.retainedMu.Unlock()

	case msgNodeRegister:
		s.nodesMu.Lock()
		s.nodes[msg.NodeID] = &types.NodeInfo{
			ID:   msg.NodeID,
			Addr: msg.Addr,
		}
		s.nodesMu.Unlock()
	}
}

// topicMatchesFilter checks if a topic matches an MQTT filter.
func topicMatchesFilter(topic, filter string) bool {
	if filter == "#" {
		return true
	}
	if filter == topic {
		return true
	}

	ti, fi := 0, 0
	for fi < len(filter) {
		if filter[fi] == '#' {
			return true
		}
		if filter[fi] == '+' {
			for ti < len(topic) && topic[ti] != '/' {
				ti++
			}
			fi++
			continue
		}
		if ti >= len(topic) || topic[ti] != filter[fi] {
			return false
		}
		ti++
		fi++
	}
	return ti == len(topic)
}

// Verify interface implementation
var _ types.Store = (*Store)(nil)
