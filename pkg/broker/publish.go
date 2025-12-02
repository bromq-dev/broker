package broker

import (
	"context"

	"github.com/bromq-dev/broker/pkg/packet"
	"github.com/bromq-dev/broker/pkg/topic"
)

func (b *Broker) handlePublish(client *Client, pkt *packet.Publish) error {
	// Validate topic
	if err := topic.ValidateName(pkt.TopicName); err != nil {
		if client.version == packet.Version5 {
			client.Send(&packet.Disconnect{
				Version:    packet.Version5,
				ReasonCode: packet.ReasonTopicNameInvalid,
			})
		}
		return err
	}

	// Check authorization
	if err := b.hooks.OnPublish(client.ctx, client, pkt); err != nil {
		if pkt.QoS == packet.QoS1 {
			client.Send(&packet.Puback{
				Version:    client.version,
				PacketID:   pkt.PacketID,
				ReasonCode: packet.ReasonNotAuthorized,
			})
		} else if pkt.QoS == packet.QoS2 {
			client.Send(&packet.Pubrec{
				Version:    client.version,
				PacketID:   pkt.PacketID,
				ReasonCode: packet.ReasonNotAuthorized,
			})
		}
		return nil // Don't disconnect
	}

	// Apply message hooks
	finalPkt, err := b.hooks.OnPublishReceived(client.ctx, client, pkt)
	if err != nil {
		return nil // Message rejected by hook
	}
	if finalPkt != nil {
		pkt = finalPkt
	}

	// Handle QoS acknowledgments
	switch pkt.QoS {
	case packet.QoS1:
		// For QoS1, always ack and route. DUP just means client is retrying,
		// but we have no state to deduplicate - that's expected for QoS1.
		// Only QoS2 provides true exactly-once via the 4-way handshake.
		client.Send(&packet.Puback{
			Version:  client.version,
			PacketID: pkt.PacketID,
		})
	case packet.QoS2:
		// Try to track - returns false if already tracking (retransmission)
		if !client.trackInbound(pkt) {
			// DUP - resend PUBREC, don't reprocess
			client.Send(&packet.Pubrec{
				Version:  client.version,
				PacketID: pkt.PacketID,
			})
			return nil
		}
		client.Send(&packet.Pubrec{
			Version:  client.version,
			PacketID: pkt.PacketID,
		})
		return nil // Don't route until PUBREL
	}

	// Handle retained message
	if pkt.Retain {
		b.storeRetained(pkt.TopicName, pkt)
	}

	// Route message
	b.routeMessage(client, pkt)

	return nil
}

func (b *Broker) handlePuback(client *Client, pkt *packet.Puback) error {
	client.ackInflight(pkt.PacketID)
	return nil
}

func (b *Broker) handlePubrec(client *Client, pkt *packet.Pubrec) error {
	// For outbound QoS 2
	client.Send(&packet.Pubrel{
		Version:  client.version,
		PacketID: pkt.PacketID,
	})
	return nil
}

func (b *Broker) handlePubrel(client *Client, pkt *packet.Pubrel) error {
	// Complete inbound QoS 2
	pub := client.releaseInbound(pkt.PacketID)
	if pub == nil {
		// Packet ID not found
		if client.version == packet.Version5 {
			client.Send(&packet.Pubcomp{
				Version:    packet.Version5,
				PacketID:   pkt.PacketID,
				ReasonCode: packet.ReasonPacketIDNotFound,
			})
		}
		return nil
	}

	client.Send(&packet.Pubcomp{
		Version:  client.version,
		PacketID: pkt.PacketID,
	})

	// Handle retained message
	if pub.Retain {
		b.storeRetained(pub.TopicName, pub)
	}

	// Now route the message
	b.routeMessage(client, pub)

	return nil
}

func (b *Broker) handlePubcomp(client *Client, pkt *packet.Pubcomp) error {
	client.ackInflight(pkt.PacketID)
	return nil
}

// routeMessage routes a message to all matching subscribers.
func (b *Broker) routeMessage(sender *Client, pkt *packet.Publish) {
	subscribers := b.subscriptions.Match(pkt.TopicName)

	for _, sub := range subscribers {
		// NoLocal check
		if sub.NoLocal && sender != nil && sub.Client == sender {
			continue
		}

		// Authorization check
		if !b.hooks.CanRead(context.Background(), sub.Client, pkt.TopicName) {
			continue
		}

		// Determine delivery QoS (minimum of message QoS and subscription QoS)
		deliverQoS := min(sub.QoS, pkt.QoS)

		// Create delivery packet
		deliverPkt := &packet.Publish{
			Version:   sub.Client.version,
			TopicName: pkt.TopicName,
			Payload:   pkt.Payload,
			QoS:       deliverQoS,
			Retain:    sub.RetainAsPublished && pkt.Retain,
		}

		// Copy MQTT 5.0 properties if subscriber supports them
		if sub.Client.version == packet.Version5 && pkt.Properties != nil {
			deliverPkt.Properties = &packet.Properties{
				PayloadFormat:   pkt.Properties.PayloadFormat,
				MessageExpiry:   pkt.Properties.MessageExpiry,
				ContentType:     pkt.Properties.ContentType,
				ResponseTopic:   pkt.Properties.ResponseTopic,
				CorrelationData: pkt.Properties.CorrelationData,
				UserProperties:  pkt.Properties.UserProperties,
			}
		}

		// Add subscription ID (MQTT 5.0)
		if sub.SubscriptionID > 0 && sub.Client.version == packet.Version5 {
			if deliverPkt.Properties == nil {
				deliverPkt.Properties = &packet.Properties{}
			}
			deliverPkt.Properties.SubscriptionIDs = []uint32{sub.SubscriptionID}
		}

		// Apply message hooks
		finalPkt, err := b.hooks.OnPublishDeliver(context.Background(), sub.Client, deliverPkt)
		if err != nil {
			continue // Delivery rejected by hook
		}
		if finalPkt != nil {
			deliverPkt = finalPkt
		}

		// Assign packet ID for QoS > 0
		if deliverQoS > packet.QoS0 {
			deliverPkt.PacketID = sub.Client.nextID()
		}

		// Check client's max packet size (MQTT 5.0)
		if sub.Client.maxPacketSize > 0 {
			pktSize := uint32(deliverPkt.EncodedSize())
			if pktSize > sub.Client.maxPacketSize {
				b.hooks.OnMessageDropped(sub.Client, deliverPkt, DropReasonPacketTooLarge)
				continue
			}
		}

		// Send to client
		if sub.Client.connected.Load() {
			if !sub.Client.Send(deliverPkt) {
				if deliverQoS > packet.QoS0 {
					// Queue full, store for later
					if !sub.Client.session.QueueMessage(deliverPkt, b.config.MaxSessionQueue) {
						b.hooks.OnMessageDropped(sub.Client, deliverPkt, DropReasonQueueFull)
					}
				} else {
					// QoS 0 dropped due to full outbound buffer
					b.hooks.OnMessageDropped(sub.Client, deliverPkt, DropReasonQueueFull)
				}
			} else if deliverQoS > packet.QoS0 {
				if !sub.Client.trackInflight(deliverPkt) {
					// Inflight limit reached, queue for later
					if !sub.Client.session.QueueMessage(deliverPkt, b.config.MaxSessionQueue) {
						b.hooks.OnMessageDropped(sub.Client, deliverPkt, DropReasonQueueFull)
					}
				}
			}
		} else {
			if deliverQoS > packet.QoS0 {
				// Client offline, queue message
				if !sub.Client.session.QueueMessage(deliverPkt, b.config.MaxSessionQueue) {
					b.hooks.OnMessageDropped(sub.Client, deliverPkt, DropReasonQueueFull)
				}
			} else {
				// QoS 0 to offline client - dropped per MQTT spec
				b.hooks.OnMessageDropped(sub.Client, deliverPkt, DropReasonClientOffline)
			}
		}
	}
}
