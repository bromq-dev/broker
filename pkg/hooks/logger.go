package hooks

import (
	"context"
	"log/slog"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/packet"
)

// LoggerHook logs broker events using slog.
type LoggerHook struct {
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

// NewLoggerHook creates a new logging hook.
func NewLoggerHook(cfg LoggerConfig) *LoggerHook {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Level == 0 {
		cfg.Level = LogLevelAll
	}
	return &LoggerHook{
		logger: cfg.Logger,
		level:  cfg.Level,
	}
}

func (h *LoggerHook) ID() string { return "logger" }

// ConnectionHook implementation

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

// AuthzHook implementation (for subscribe logging)

func (h *LoggerHook) OnSubscribe(ctx context.Context, client broker.ClientInfo, subs []packet.Subscription) ([]packet.Subscription, error) {
	if h.level&LogLevelSubscribe == 0 {
		return nil, nil
	}
	for _, sub := range subs {
		h.logger.Info("client subscribed",
			"client_id", client.ClientID(),
			"topic", sub.TopicFilter,
			"qos", sub.QoS,
		)
	}
	return nil, nil
}

func (h *LoggerHook) OnPublish(ctx context.Context, client broker.ClientInfo, pkt *packet.Publish) error {
	// No-op for authz - we log on receive
	return nil
}

func (h *LoggerHook) CanRead(ctx context.Context, client broker.ClientInfo, topic string) bool {
	return true
}

// MessageHook implementation

func (h *LoggerHook) OnPublishReceived(ctx context.Context, client broker.ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	if h.level&LogLevelPublish == 0 {
		return nil, nil
	}
	h.logger.Debug("message received",
		"client_id", client.ClientID(),
		"topic", pkt.TopicName,
		"qos", pkt.QoS,
		"retain", pkt.Retain,
		"payload_size", len(pkt.Payload),
	)
	return nil, nil
}

func (h *LoggerHook) OnPublishDeliver(ctx context.Context, subscriber broker.ClientInfo, pkt *packet.Publish) (*packet.Publish, error) {
	if h.level&LogLevelPublish == 0 {
		return nil, nil
	}
	h.logger.Debug("message delivered",
		"client_id", subscriber.ClientID(),
		"topic", pkt.TopicName,
		"qos", pkt.QoS,
	)
	return nil, nil
}

// SessionHook implementation

func (h *LoggerHook) OnSessionCreated(ctx context.Context, client broker.ClientInfo) {
	if h.level&LogLevelSession == 0 {
		return
	}
	h.logger.Info("session created",
		"client_id", client.ClientID(),
	)
}

func (h *LoggerHook) OnSessionResumed(ctx context.Context, client broker.ClientInfo) {
	if h.level&LogLevelSession == 0 {
		return
	}
	h.logger.Info("session resumed",
		"client_id", client.ClientID(),
	)
}

func (h *LoggerHook) OnSessionEnded(ctx context.Context, clientID string) {
	if h.level&LogLevelSession == 0 {
		return
	}
	h.logger.Info("session ended",
		"client_id", clientID,
	)
}
