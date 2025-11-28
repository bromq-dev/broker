package hooks

import (
	"context"
	"crypto/subtle"

	"github.com/bromq-dev/broker/pkg/broker"
	"github.com/bromq-dev/broker/pkg/packet"
)

// AuthHook provides simple username/password authentication.
type AuthHook struct {
	broker.HookBase
	credentials map[string]string // username -> password
	validator   AuthValidator
}

// AuthValidator is a custom authentication function.
type AuthValidator func(ctx context.Context, username, password string) bool

// AuthConfig configures the auth hook.
type AuthConfig struct {
	// Credentials is a map of username -> password for simple auth.
	Credentials map[string]string

	// Validator is a custom authentication function.
	// If set, Credentials is ignored.
	Validator AuthValidator
}

func (h *AuthHook) ID() string { return "auth" }

// Provides indicates which events this hook handles.
func (h *AuthHook) Provides(event byte) bool {
	return event == broker.OnConnectEvent
}

// Init is called when the hook is registered with the broker.
func (h *AuthHook) Init(opts *broker.HookOptions, config any) error {
	if err := h.HookBase.Init(opts, config); err != nil {
		return err
	}

	// Apply config if provided
	if cfg, ok := config.(*AuthConfig); ok && cfg != nil {
		h.credentials = cfg.Credentials
		h.validator = cfg.Validator
	}

	return nil
}

// OnConnect validates client credentials.
func (h *AuthHook) OnConnect(ctx context.Context, client broker.ClientInfo, pkt *packet.Connect) error {
	username := pkt.Username
	password := string(pkt.Password)

	// Use custom validator if provided
	if h.validator != nil {
		if !h.validator(ctx, username, password) {
			return broker.NewReasonCodeError(packet.ReasonNotAuthorized, "invalid credentials")
		}
		return nil
	}

	// Use static credentials
	if h.credentials == nil {
		return nil // No auth configured
	}

	expected, ok := h.credentials[username]
	if !ok {
		return broker.NewReasonCodeError(packet.ReasonNotAuthorized, "unknown user")
	}

	if subtle.ConstantTimeCompare([]byte(password), []byte(expected)) != 1 {
		return broker.NewReasonCodeError(packet.ReasonNotAuthorized, "invalid password")
	}

	return nil
}

// AddUser adds or updates a user credential.
func (h *AuthHook) AddUser(username, password string) {
	if h.credentials == nil {
		h.credentials = make(map[string]string)
	}
	h.credentials[username] = password
}

// RemoveUser removes a user credential.
func (h *AuthHook) RemoveUser(username string) {
	delete(h.credentials, username)
}
