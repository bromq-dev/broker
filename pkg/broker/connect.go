package broker

import (
	"errors"
	"fmt"

	"github.com/bromq-dev/broker/pkg/packet"
)

// handleConnect processes a CONNECT packet.
func (b *Broker) handleConnect(client *Client, pkt *packet.Connect) error {
	// Set client info
	client.clientID = pkt.ClientID
	client.username = pkt.Username
	client.version = pkt.ProtocolVersion
	client.cleanStart = pkt.CleanStart
	client.keepAlive = pkt.KeepAlive
	client.properties = pkt.Properties
	client.reader.SetVersion(pkt.ProtocolVersion)

	// Extract client's max packet size (MQTT 5.0)
	if pkt.Properties != nil && pkt.Properties.MaxPacketSize != nil {
		client.maxPacketSize = *pkt.Properties.MaxPacketSize
	}

	// Validate client ID length (protect against DoS)
	const maxClientIDLen = 256 // Reasonable limit, spec allows up to 65535
	if len(client.clientID) > maxClientIDLen {
		if pkt.ProtocolVersion == packet.Version5 {
			return b.sendConnackAndClose(client, pkt.ProtocolVersion, false, byte(packet.ReasonClientIDNotValid))
		}
		return b.sendConnackAndClose(client, pkt.ProtocolVersion, false, byte(packet.ConnackIdentifierRejected))
	}

	// Generate client ID if empty
	if client.clientID == "" {
		if !pkt.CleanStart {
			// v3.1.1: Empty client ID requires clean session
			return b.sendConnackAndClose(client, pkt.ProtocolVersion, false, byte(packet.ConnackIdentifierRejected))
		}
		client.clientID = generateClientID()
	}

	// Call auth hooks
	if err := b.hooks.OnConnect(client.ctx, client, pkt); err != nil {
		var rcErr *ReasonCodeError
		if errors.As(err, &rcErr) {
			return b.sendConnackAndClose(client, pkt.ProtocolVersion, false, byte(rcErr.Code))
		}
		if pkt.ProtocolVersion == packet.Version5 {
			return b.sendConnackAndClose(client, pkt.ProtocolVersion, false, byte(packet.ReasonNotAuthorized))
		}
		return b.sendConnackAndClose(client, pkt.ProtocolVersion, false, byte(packet.ConnackNotAuthorized))
	}

	// Handle existing client with same ID
	b.clientsMu.Lock()
	if existing, ok := b.clients[client.clientID]; ok {
		// Disconnect existing client (session takeover)
		if pkt.ProtocolVersion == packet.Version5 {
			existing.Send(&packet.Disconnect{
				Version:    packet.Version5,
				ReasonCode: packet.ReasonSessionTakenOver,
			})
		}
		existing.Close()
		delete(b.clients, client.clientID)
	}
	b.clients[client.clientID] = client
	b.clientsMu.Unlock()

	// Handle session
	var sessionPresent bool
	var session *Session

	if pkt.CleanStart {
		// Create new session
		session = b.sessions.Create(client.clientID)
		b.hooks.OnSessionCreated(client.ctx, client)
	} else {
		// Try to resume existing session
		session, sessionPresent = b.sessions.GetOrCreate(client.clientID)
		if sessionPresent {
			b.hooks.OnSessionResumed(client.ctx, client)
		} else {
			b.hooks.OnSessionCreated(client.ctx, client)
		}
	}

	// Set session expiry (MQTT 5.0)
	if pkt.ProtocolVersion == packet.Version5 && pkt.Properties != nil && pkt.Properties.SessionExpiry != nil {
		session.SetExpiry(*pkt.Properties.SessionExpiry)
	}

	session.SetClient(client)
	client.session = session

	// Store will message
	if pkt.WillFlag {
		client.will = &packet.Publish{
			Version:   pkt.ProtocolVersion,
			TopicName: pkt.WillTopic,
			Payload:   pkt.WillPayload,
			QoS:       pkt.WillQoS,
			Retain:    pkt.WillRetain,
		}
		client.willProps = pkt.WillProps
		if pkt.ProtocolVersion == packet.Version5 && pkt.WillProps != nil && pkt.WillProps.WillDelayInterval != nil {
			client.willDelay = *pkt.WillProps.WillDelayInterval
		}
	}

	// Send CONNACK
	connack := &packet.Connack{
		Version:        pkt.ProtocolVersion,
		SessionPresent: sessionPresent && !pkt.CleanStart,
		ReasonCode:     0, // Success
	}

	// Add MQTT 5.0 properties
	if pkt.ProtocolVersion == packet.Version5 {
		connack.Properties = &packet.Properties{}

		// Assigned Client Identifier
		if pkt.ClientID == "" {
			connack.Properties.AssignedClientID = client.clientID
		}

		// Server capabilities
		if b.config.MaxQoS < packet.QoS2 {
			maxQoS := byte(b.config.MaxQoS)
			connack.Properties.MaxQoS = &maxQoS
		}

		retainAvail := byte(0)
		if b.config.RetainAvailable {
			retainAvail = 1
		}
		connack.Properties.RetainAvailable = &retainAvail

		wildcardAvail := byte(0)
		if b.config.WildcardSubAvailable {
			wildcardAvail = 1
		}
		connack.Properties.WildcardSubAvail = &wildcardAvail

		subIDAvail := byte(0)
		if b.config.SubIDAvailable {
			subIDAvail = 1
		}
		connack.Properties.SubIDAvail = &subIDAvail

		sharedAvail := byte(0)
		if b.config.SharedSubAvailable {
			sharedAvail = 1
		}
		connack.Properties.SharedSubAvail = &sharedAvail
	}

	if err := client.SendSync(connack); err != nil {
		return err
	}

	client.connected.Store(true)
	b.hooks.OnConnected(client.ctx, client)

	// Restore subscriptions if session was resumed
	if sessionPresent && !pkt.CleanStart {
		for filter, sub := range session.GetAllSubscriptions() {
			b.subscriptions.Subscribe(filter, &Subscriber{
				Client:            client,
				QoS:               sub.QoS,
				NoLocal:           sub.NoLocal,
				RetainAsPublished: sub.RetainAsPublished,
				RetainHandling:    sub.RetainHandling,
				SubscriptionID:    sub.SubscriptionID,
			})
		}

		// Deliver pending messages
		for _, pending := range session.DrainPending() {
			pending.PacketID = client.nextID()
			client.Send(pending)
			if pending.QoS > packet.QoS0 {
				client.trackInflight(pending)
			}
		}
	}

	return nil
}

func (b *Broker) sendConnackAndClose(client *Client, version packet.Version, sessionPresent bool, code byte) error {
	connack := &packet.Connack{
		Version:        version,
		SessionPresent: sessionPresent,
		ReasonCode:     code,
	}
	client.SendSync(connack)
	return fmt.Errorf("connection rejected: %d", code)
}
