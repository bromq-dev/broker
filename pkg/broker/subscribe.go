package broker

import (
	"github.com/bromq-dev/broker/pkg/packet"
	"github.com/bromq-dev/broker/pkg/topic"
)

func (b *Broker) handleSubscribe(client *Client, pkt *packet.Subscribe) error {
	// Authorize subscriptions
	subs := pkt.Subscriptions
	subs, err := b.hooks.OnSubscribe(client.ctx, client, subs)
	if err != nil {
		// All subscriptions rejected
		codes := make([]byte, len(pkt.Subscriptions))
		for i := range codes {
			if client.version == packet.Version5 {
				codes[i] = byte(packet.ReasonNotAuthorized)
			} else {
				codes[i] = 0x80 // Failure
			}
		}
		client.Send(&packet.Suback{
			Version:     client.version,
			PacketID:    pkt.PacketID,
			ReasonCodes: codes,
		})
		return nil
	}

	codes := make([]byte, len(subs))

	for i, sub := range subs {
		// Validate filter
		if err := topic.ValidateFilter(sub.TopicFilter); err != nil {
			if client.version == packet.Version5 {
				codes[i] = byte(packet.ReasonTopicFilterInvalid)
			} else {
				codes[i] = 0x80
			}
			continue
		}

		// Enforce max QoS
		grantedQoS := min(sub.QoS, b.config.MaxQoS)

		// Add to subscription tree
		var subID uint32
		if pkt.Properties != nil && len(pkt.Properties.SubscriptionIDs) > 0 {
			subID = pkt.Properties.SubscriptionIDs[0]
		}

		b.subscriptions.Subscribe(sub.TopicFilter, &Subscriber{
			Client:            client,
			QoS:               grantedQoS,
			NoLocal:           sub.NoLocal,
			RetainAsPublished: sub.RetainAsPublished,
			RetainHandling:    sub.RetainHandling,
			SubscriptionID:    subID,
		})

		// Store in session
		client.session.Subscribe(sub.TopicFilter, &sessionSub{
			QoS:               grantedQoS,
			NoLocal:           sub.NoLocal,
			RetainAsPublished: sub.RetainAsPublished,
			RetainHandling:    sub.RetainHandling,
			SubscriptionID:    subID,
		})

		codes[i] = byte(grantedQoS)

		// Send retained messages
		if sub.RetainHandling != 2 { // 2 = don't send retained
			b.sendRetained(client, sub.TopicFilter, grantedQoS)
		}
	}

	client.Send(&packet.Suback{
		Version:     client.version,
		PacketID:    pkt.PacketID,
		ReasonCodes: codes,
	})

	return nil
}

func (b *Broker) handleUnsubscribe(client *Client, pkt *packet.Unsubscribe) error {
	codes := make([]byte, len(pkt.TopicFilters))

	for i, filter := range pkt.TopicFilters {
		if b.subscriptions.Unsubscribe(filter, client) {
			client.session.Unsubscribe(filter)
			codes[i] = byte(packet.ReasonSuccess)
		} else {
			if client.version == packet.Version5 {
				codes[i] = byte(packet.ReasonNoSubscriptionExist)
			} else {
				codes[i] = 0
			}
		}
	}

	client.Send(&packet.Unsuback{
		Version:     client.version,
		PacketID:    pkt.PacketID,
		ReasonCodes: codes,
	})

	return nil
}
