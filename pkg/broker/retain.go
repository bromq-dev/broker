package broker

import (
	"context"

	"github.com/bromq-dev/broker/pkg/packet"
	"github.com/bromq-dev/broker/pkg/topic"
)

// storeRetained stores or deletes a retained message.
func (b *Broker) storeRetained(topicName string, pkt *packet.Publish) {
	// Try hooks first
	if err := b.hooks.StoreRetained(context.Background(), topicName, pkt); err == nil {
		return
	}

	// Fall back to in-memory storage
	b.retainedMu.Lock()
	defer b.retainedMu.Unlock()

	if len(pkt.Payload) == 0 {
		delete(b.retained, topicName)
	} else {
		// Store copy
		stored := &packet.Publish{
			TopicName: pkt.TopicName,
			Payload:   make([]byte, len(pkt.Payload)),
			QoS:       pkt.QoS,
			Retain:    true,
		}
		copy(stored.Payload, pkt.Payload)
		b.retained[topicName] = stored
	}
}

// sendRetained sends retained messages matching a filter to a client.
func (b *Broker) sendRetained(client *Client, filter string, maxQoS packet.QoS) {
	// Try hooks first
	messages, err := b.hooks.GetRetained(context.Background(), filter)
	if err == nil && messages != nil {
		for _, msg := range messages {
			b.deliverRetained(client, msg, maxQoS)
		}
		return
	}

	// Fall back to in-memory storage
	b.retainedMu.RLock()
	defer b.retainedMu.RUnlock()

	for topicName, msg := range b.retained {
		if topic.Match(filter, topicName) {
			b.deliverRetained(client, msg, maxQoS)
		}
	}
}

func (b *Broker) deliverRetained(client *Client, msg *packet.Publish, maxQoS packet.QoS) {
	deliverQoS := min(maxQoS, msg.QoS)

	pkt := &packet.Publish{
		Version:   client.version,
		TopicName: msg.TopicName,
		Payload:   msg.Payload,
		QoS:       deliverQoS,
		Retain:    true,
	}

	if deliverQoS > packet.QoS0 {
		pkt.PacketID = client.nextID()
		client.trackInflight(pkt)
	}

	client.Send(pkt)
}
