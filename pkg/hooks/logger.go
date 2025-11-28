package hooks

import (
	"context"
	"log/slog"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/packet"
)

// LoggerHook logs broker events using slog.
type LoggerHook struct {
	broker.HookBase
	logger *slog.Logger
	level  LogLevel
}

// LogLevel controls which events are logged.
type LogLevel int

const (
	// LogLevelConnection logs connect/disconnect events.
	LogLevelConnection LogLevel = 1 << iota
	// LogLevelSubscribe logs subscribe/unsubscribe events.
	LogLevelSubscribe
	// LogLevelPublish logs publish events.
	LogLevelPublish
	// LogLevelSession logs session lifecycle events.
	LogLevelSession
	// LogLevelAll logs all events.
	LogLevelAll = LogLevelConnection | LogLevelSubscribe | LogLevelPublish | LogLevelSession
)

// LoggerConfig configures the logger hook.
type LoggerConfig struct {
	// Logger is the slog.Logger to use (default: slog.Default()).
	Logger *slog.Logger

	// Level controls which events are logged (default: LogLevelAll).
	Level LogLevel
}

func (h *LoggerHook) ID() string { return "logger" }

// Provides indicates which events this hook handles.
func (h *LoggerHook) Provides(event byte) bool {
	switch event {
	case broker.OnConnectedEvent, broker.OnDisconnectEvent:
		return h.level&LogLevelConnection != 0
	case broker.OnSubscribeEvent:
		return h.level&LogLevelSubscribe != 0
	case broker.OnPublishReceivedEvent, broker.OnPublishDeliverEvent:
		return h.level&LogLevelPublish != 0
	case broker.OnSessionCreatedEvent, broker.OnSessionResumedEvent, broker.OnSessionEndedEvent:
		return h.level&LogLevelSession != 0
	}
	return false
}

// Init is called when the hook is registered with the broker.
func (h *LoggerHook) Init(opts *broker.HookOptions, config any) error {
	if err := h.HookBase.Init(opts, config); err != nil {
		return err
	}

	// Apply config if provided
	if cfg, ok := config.(*LoggerConfig); ok && cfg != nil {
		h.logger = cfg.Logger
		h.level = cfg.Level
	}

	// Apply defaults
	if h.logger == nil {
		h.logger = slog.Default()
	}
	if h.level == 0 {
		h.level = LogLevelAll
	}

	return nil
}

// OnConnected logs client connections.
func (h *LoggerHook) OnConnected(ctx context.Context, client broker.ClientInfo) {
	if h.level&LogLevelConnection == 0 {
		return
	}
	h.logger.Info("client connected",
		"client_id", client.ClientID(),
		"username", client.Username(),
		"remote_addr", client.RemoteAddr(),
		"protocol", client.ProtocolVersion(),
		"clean_start", client.CleanStart(),
	)
}

// OnDisconnect logs client disconnections.
func (h *LoggerHook) OnDisconnect(ctx context.Context, client broker.ClientInfo, err error) {
	if h.level&LogLevelConnection == 0 {
		return
	}
	if err != nil {
		h.logger.Info("client disconnected",
			"client_id", client.ClientID(),
			"error", err.Error(),
		)
	} else {
		h.logger.Info("client disconnected",
			"client_id", client.ClientID(),
		)
	}
}

// OnSubscribe logs subscriptions.
func (h *LoggerHook) OnSubscribe(ctx context.Context, client broker.ClientInfo, subs []packet.Subscription) ([]packet.Subscription, error) {
	if h.level&LogLevelSubscribe == 0 {
		return subs, nil
	}
	for _, sub := range subs {
		h.logger.Info("client subscribed",
			"client_id", client.ClientID(),
			"topic", sub.TopicFilter,
			"qos", sub.QoS,
		)
	}
	return subs, nil
}

// OnPublishReceived logs received messages.
func (h *LoggerHook) OnPublishReceived(ctx context.Context, client broker.ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	if h.level&LogLevelPublish == 0 {
		return pkt, nil
	}
	h.logger.Debug("message received",
		"client_id", client.ClientID(),
		"topic", pkt.TopicName,
		"qos", pkt.QoS,
		"retain", pkt.Retain,
		"payload_size", len(pkt.Payload),
	)
	return pkt, nil
}

// OnPublishDeliver logs delivered messages.
func (h *LoggerHook) OnPublishDeliver(ctx context.Context, subscriber broker.ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	if h.level&LogLevelPublish == 0 {
		return pkt, nil
	}
	h.logger.Debug("message delivered",
		"client_id", subscriber.ClientID(),
		"topic", pkt.TopicName,
		"qos", pkt.QoS,
	)
	return pkt, nil
}

// OnSessionCreated logs session creation.
func (h *LoggerHook) OnSessionCreated(ctx context.Context, client broker.ClientInfo) {
	if h.level&LogLevelSession == 0 {
		return
	}
	h.logger.Info("session created",
		"client_id", client.ClientID(),
	)
}

// OnSessionResumed logs session resumption.
func (h *LoggerHook) OnSessionResumed(ctx context.Context, client broker.ClientInfo) {
	if h.level&LogLevelSession == 0 {
		return
	}
	h.logger.Info("session resumed",
		"client_id", client.ClientID(),
	)
}

// OnSessionEnded logs session termination.
func (h *LoggerHook) OnSessionEnded(ctx context.Context, clientID string) {
	if h.level&LogLevelSession == 0 {
		return
	}
	h.logger.Info("session ended",
		"client_id", clientID,
	)
}
