package hooks

import (
	"context"
	"strings"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/packet"
	"github.com/bromq-dev/broker/pkg/topic"
)

// ACLHook provides topic-based access control.
type ACLHook struct {
	broker.HookBase
	rules         []ACLRule
	denyByDefault bool
}

// ACLRule defines an access control rule.
type ACLRule struct {
	// ClientID pattern (supports * wildcard, empty = any).
	ClientID string

	// Username pattern (supports * wildcard, empty = any).
	Username string

	// TopicFilter pattern (supports MQTT wildcards + and #).
	TopicFilter string

	// Access permissions.
	Read  bool
	Write bool
}

// ACLConfig configures the ACL hook.
type ACLConfig struct {
	// Rules defines the access control rules (evaluated in order).
	Rules []ACLRule

	// DenyByDefault denies access if no rule matches (default: false = allow).
	DenyByDefault bool
}

func (h *ACLHook) ID() string { return "acl" }

// Provides indicates which events this hook handles.
func (h *ACLHook) Provides(event byte) bool {
	return event == broker.OnSubscribeEvent ||
		event == broker.OnPublishEvent
}

// Init is called when the hook is registered with the broker.
func (h *ACLHook) Init(opts *broker.HookOptions, config any) error {
	if err := h.HookBase.Init(opts, config); err != nil {
		return err
	}

	// Apply config if provided
	if cfg, ok := config.(*ACLConfig); ok && cfg != nil {
		h.rules = cfg.Rules
		h.denyByDefault = cfg.DenyByDefault
	}

	return nil
}

// OnSubscribe filters subscriptions based on read permissions.
func (h *ACLHook) OnSubscribe(ctx context.Context, client broker.ClientInfo, subs []packet.Subscription) ([]packet.Subscription, error) {
	result := make([]packet.Subscription, 0, len(subs))
	for _, sub := range subs {
		if h.canAccess(client, sub.TopicFilter, true) {
			result = append(result, sub)
		}
	}
	if len(result) == 0 {
		return nil, broker.NewReasonCodeError(packet.ReasonNotAuthorized, "subscription denied by ACL")
	}
	return result, nil
}

// OnPublish checks write permissions for the topic.
func (h *ACLHook) OnPublish(ctx context.Context, client broker.ClientInfo, pkt *packet.Publish) error {
	if !h.canAccess(client, pkt.TopicName, false) {
		return broker.NewReasonCodeError(packet.ReasonNotAuthorized, "publish denied by ACL")
	}
	return nil
}

// CanRead checks if a client can receive messages on a topic.
func (h *ACLHook) CanRead(ctx context.Context, client broker.ClientInfo, topicName string) bool {
	return h.canAccess(client, topicName, true)
}

func (h *ACLHook) canAccess(client broker.ClientInfo, topicPattern string, read bool) bool {
	for _, rule := range h.rules {
		if !h.matchClient(rule, client) {
			continue
		}
		if !h.matchTopic(rule.TopicFilter, topicPattern) {
			continue
		}
		// Rule matches - check permission
		if read {
			return rule.Read
		}
		return rule.Write
	}
	// No rule matched
	return !h.denyByDefault
}

func (h *ACLHook) matchClient(rule ACLRule, client broker.ClientInfo) bool {
	if rule.ClientID != "" && !matchPattern(rule.ClientID, client.ClientID()) {
		return false
	}
	if rule.Username != "" && !matchPattern(rule.Username, client.Username()) {
		return false
	}
	return true
}

func (h *ACLHook) matchTopic(ruleFilter, clientTopic string) bool {
	// Use MQTT topic matching
	return topic.Match(ruleFilter, clientTopic)
}

// matchPattern matches a simple wildcard pattern (* = any).
func matchPattern(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		return strings.Contains(value, pattern[1:len(pattern)-1])
	}
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(value, pattern[1:])
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, pattern[:len(pattern)-1])
	}
	return pattern == value
}

// AddRule adds a rule at runtime.
func (h *ACLHook) AddRule(rule ACLRule) {
	h.rules = append(h.rules, rule)
}

// ClearRules removes all rules.
func (h *ACLHook) ClearRules() {
	h.rules = nil
}
