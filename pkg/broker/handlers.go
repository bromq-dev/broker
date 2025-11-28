package broker

import (
	"context"
	"errors"
	"time"

	"github.com/bromq-dev/broker/pkg/packet"
)

// handlePacket processes an incoming packet from a client.
func (b *Broker) handlePacket(client *Client, pkt packet.Packet) error {
	client.updateActivity()

	switch p := pkt.(type) {
	case *packet.Publish:
		return b.handlePublish(client, p)
	case *packet.Puback:
		return b.handlePuback(client, p)
	case *packet.Pubrec:
		return b.handlePubrec(client, p)
	case *packet.Pubrel:
		return b.handlePubrel(client, p)
	case *packet.Pubcomp:
		return b.handlePubcomp(client, p)
	case *packet.Subscribe:
		return b.handleSubscribe(client, p)
	case *packet.Unsubscribe:
		return b.handleUnsubscribe(client, p)
	case *packet.Pingreq:
		return b.handlePingreq(client)
	case *packet.Disconnect:
		return b.handleDisconnectPacket(client, p)
	case *packet.Auth:
		return b.handleAuth(client, p)
	default:
		return errors.New("unexpected packet type")
	}
}

func (b *Broker) handlePingreq(client *Client) error {
	client.Send(&packet.Pingresp{})
	return nil
}

func (b *Broker) handleDisconnectPacket(client *Client, pkt *packet.Disconnect) error {
	// Clean disconnect - don't send will
	if pkt.ReasonCode == packet.ReasonSuccess {
		client.will = nil
	}

	// Update session expiry (MQTT 5.0)
	if client.version == packet.Version5 && pkt.Properties != nil && pkt.Properties.SessionExpiry != nil {
		client.session.SetExpiry(*pkt.Properties.SessionExpiry)
	}

	return errors.New("client disconnected")
}

func (b *Broker) handleAuth(client *Client, pkt *packet.Auth) error {
	// Enhanced authentication not implemented in basic broker
	// Hooks can handle this
	return nil
}

// handleDisconnect handles client disconnection.
func (b *Broker) handleDisconnect(client *Client, err error) {
	if !client.connected.Load() {
		return
	}
	client.connected.Store(false)

	// Notify hooks
	b.hooks.OnDisconnect(client.ctx, client, err)

	// Remove from clients map
	b.clientsMu.Lock()
	if c, ok := b.clients[client.clientID]; ok && c == client {
		delete(b.clients, client.clientID)
	}
	b.clientsMu.Unlock()

	// Handle will message
	if client.will != nil {
		if client.willDelay > 0 {
			// Schedule delayed will (simplified - in production use a scheduler)
			go func() {
				time.Sleep(time.Duration(client.willDelay) * time.Second)
				// Check if client reconnected
				b.clientsMu.RLock()
				reconnected := b.clients[client.clientID] != nil
				b.clientsMu.RUnlock()

				if !reconnected {
					will, _ := b.hooks.OnWillPublish(context.Background(), client.clientID, client.will)
					if will != nil {
						b.routeMessage(nil, will)
						if will.Retain {
							b.storeRetained(will.TopicName, will)
						}
					}
				}
			}()
		} else {
			will, _ := b.hooks.OnWillPublish(client.ctx, client.clientID, client.will)
			if will != nil {
				b.routeMessage(nil, will)
				if will.Retain {
					b.storeRetained(will.TopicName, will)
				}
			}
		}
	}

	// Handle session
	if client.cleanStart {
		// Clean session - remove subscriptions and session
		b.subscriptions.UnsubscribeAll(client)
		b.sessions.Delete(client.clientID)
		b.hooks.OnSessionEnded(client.ctx, client.clientID)
	} else {
		// Persistent session - clear client reference
		client.session.SetClient(nil)
	}

	client.Close()
}
