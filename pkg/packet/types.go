// Package packet provides MQTT packet encoding and decoding for MQTT 3.1.1 and 5.0.
// It is designed for zero-copy operations where possible and minimal allocations.
package packet

// Type represents an MQTT control packet type.
type Type byte

// MQTT Control Packet types as defined in MQTT 3.1.1 Section 2.2.1 and MQTT 5.0 Section 2.1.2
const (
	TypeReserved0   Type = 0  // Reserved
	TypeConnect     Type = 1  // Client request to connect to Server
	TypeConnack     Type = 2  // Connect acknowledgment
	TypePublish     Type = 3  // Publish message
	TypePuback      Type = 4  // Publish acknowledgment (QoS 1)
	TypePubrec      Type = 5  // Publish received (QoS 2 part 1)
	TypePubrel      Type = 6  // Publish release (QoS 2 part 2)
	TypePubcomp     Type = 7  // Publish complete (QoS 2 part 3)
	TypeSubscribe   Type = 8  // Subscribe request
	TypeSuback      Type = 9  // Subscribe acknowledgment
	TypeUnsubscribe Type = 10 // Unsubscribe request
	TypeUnsuback    Type = 11 // Unsubscribe acknowledgment
	TypePingreq     Type = 12 // PING request
	TypePingresp    Type = 13 // PING response
	TypeDisconnect  Type = 14 // Disconnect notification
	TypeAuth        Type = 15 // Authentication exchange (MQTT 5.0 only)
)

// String returns the string representation of the packet type.
func (t Type) String() string {
	switch t {
	case TypeConnect:
		return "CONNECT"
	case TypeConnack:
		return "CONNACK"
	case TypePublish:
		return "PUBLISH"
	case TypePuback:
		return "PUBACK"
	case TypePubrec:
		return "PUBREC"
	case TypePubrel:
		return "PUBREL"
	case TypePubcomp:
		return "PUBCOMP"
	case TypeSubscribe:
		return "SUBSCRIBE"
	case TypeSuback:
		return "SUBACK"
	case TypeUnsubscribe:
		return "UNSUBSCRIBE"
	case TypeUnsuback:
		return "UNSUBACK"
	case TypePingreq:
		return "PINGREQ"
	case TypePingresp:
		return "PINGRESP"
	case TypeDisconnect:
		return "DISCONNECT"
	case TypeAuth:
		return "AUTH"
	default:
		return "RESERVED"
	}
}

// Valid returns true if the packet type is valid.
func (t Type) Valid() bool {
	return t >= TypeConnect && t <= TypeAuth
}

// Version represents an MQTT protocol version.
type Version byte

const (
	Version31  Version = 3 // MQTT 3.1
	Version311 Version = 4 // MQTT 3.1.1
	Version5   Version = 5 // MQTT 5.0
)

// String returns the string representation of the MQTT version.
func (v Version) String() string {
	switch v {
	case Version31:
		return "3.1"
	case Version311:
		return "3.1.1"
	case Version5:
		return "5.0"
	default:
		return "unknown"
	}
}

// QoS represents MQTT Quality of Service level.
type QoS byte

const (
	QoS0 QoS = 0 // At most once delivery
	QoS1 QoS = 1 // At least once delivery
	QoS2 QoS = 2 // Exactly once delivery
)

// Valid returns true if the QoS level is valid.
func (q QoS) Valid() bool {
	return q <= QoS2
}

// String returns the string representation of the QoS level.
func (q QoS) String() string {
	switch q {
	case QoS0:
		return "QoS0"
	case QoS1:
		return "QoS1"
	case QoS2:
		return "QoS2"
	default:
		return "invalid"
	}
}

// Fixed header flag bits for specific packet types.
const (
	// PUBLISH flags (bits 3-0 of first byte)
	PublishFlagRetain = 1 << 0 // Bit 0: RETAIN flag
	PublishFlagQoS1   = 1 << 1 // Bit 1: QoS LSB
	PublishFlagQoS2   = 1 << 2 // Bit 2: QoS MSB
	PublishFlagDup    = 1 << 3 // Bit 3: DUP flag

	// Reserved flags that MUST be set for certain packet types
	PubrelFlags      = 0x02 // PUBREL MUST have flags 0010
	SubscribeFlags   = 0x02 // SUBSCRIBE MUST have flags 0010
	UnsubscribeFlags = 0x02 // UNSUBSCRIBE MUST have flags 0010
)

// MaxRemainingLength is the maximum remaining length value (256MB - 1).
const MaxRemainingLength = 268435455

// MaxPacketSize is the maximum total packet size including fixed header.
const MaxPacketSize = MaxRemainingLength + 5 // 5 bytes max for fixed header
