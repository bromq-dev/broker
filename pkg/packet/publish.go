package packet

// Publish represents an MQTT PUBLISH packet.
// MQTT 3.1.1 Section 3.3, MQTT 5.0 Section 3.3
type Publish struct {
	// Fixed header flags
	Dup    bool // Duplicate delivery flag
	QoS    QoS  // Quality of Service level
	Retain bool // Retain flag

	// Variable header
	TopicName string // Topic name
	PacketID  uint16 // Packet identifier (only for QoS > 0)

	// Properties (MQTT 5.0 only)
	Properties *Properties

	// Payload
	Payload []byte

	// Version to determine encoding
	Version Version
}

// Type returns TypePublish.
func (p *Publish) Type() Type {
	return TypePublish
}

// flags returns the fixed header flags for this PUBLISH packet.
func (p *Publish) flags() byte {
	var flags byte
	if p.Retain {
		flags |= PublishFlagRetain
	}
	flags |= byte(p.QoS) << 1
	if p.Dup {
		flags |= PublishFlagDup
	}
	return flags
}

// EncodedSize returns the total size of the encoded PUBLISH packet.
func (p *Publish) EncodedSize() int {
	// Topic name
	varHeaderSize := 2 + len(p.TopicName)

	// Packet ID (only for QoS > 0)
	if p.QoS > QoS0 {
		varHeaderSize += 2
	}

	// Properties (MQTT 5.0)
	if p.Version == Version5 {
		if p.Properties != nil {
			varHeaderSize += p.Properties.EncodedSize()
		} else {
			varHeaderSize += 1 // Empty properties
		}
	}

	// Payload
	payloadSize := len(p.Payload)

	remainingLength := varHeaderSize + payloadSize
	return FixedHeaderSize(uint32(remainingLength)) + remainingLength
}

// Encode encodes the PUBLISH packet into buf.
// Returns the number of bytes written, or 0 on error.
func (p *Publish) Encode(buf []byte) int {
	size := p.EncodedSize()
	if len(buf) < size {
		return 0
	}

	// Calculate remaining length
	varHeaderSize := 2 + len(p.TopicName)
	if p.QoS > QoS0 {
		varHeaderSize += 2
	}
	if p.Version == Version5 {
		if p.Properties != nil {
			varHeaderSize += p.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
	}
	payloadSize := len(p.Payload)
	remainingLength := uint32(varHeaderSize + payloadSize)

	// Fixed header
	pos := EncodeFixedHeader(buf, TypePublish, p.flags(), remainingLength)
	if pos == 0 {
		return 0
	}

	// Variable header
	// Topic name
	pos += EncodeString(buf[pos:], p.TopicName)

	// Packet ID (only for QoS > 0)
	if p.QoS > QoS0 {
		pos += EncodeUint16(buf[pos:], p.PacketID)
	}

	// Properties (MQTT 5.0)
	if p.Version == Version5 {
		if p.Properties != nil {
			pos += p.Properties.Encode(buf[pos:])
		} else {
			buf[pos] = 0
			pos++
		}
	}

	// Payload
	copy(buf[pos:], p.Payload)
	pos += len(p.Payload)

	return pos
}

// DecodePublish decodes a PUBLISH packet from buf.
// flags are the fixed header flags (lower 4 bits of first byte).
// buf should contain the packet data starting after the fixed header.
func DecodePublish(flags byte, buf []byte, version Version) (*Publish, error) {
	p := &Publish{Version: version}
	pos := 0

	// Parse flags
	p.Retain = flags&PublishFlagRetain != 0
	p.QoS = QoS((flags >> 1) & 0x03)
	p.Dup = flags&PublishFlagDup != 0

	// Validate QoS
	if !p.QoS.Valid() {
		return nil, ErrInvalidQoS
	}

	// DUP must be 0 for QoS 0
	if p.QoS == QoS0 && p.Dup {
		return nil, ErrMalformedPacket
	}

	// Topic name
	topic, n, ok := DecodeStringCopy(buf[pos:])
	if !ok {
		return nil, ErrIncompletePacket
	}
	p.TopicName = topic
	pos += n

	// Packet ID (only for QoS > 0)
	if p.QoS > QoS0 {
		packetID, n, ok := DecodeUint16(buf[pos:])
		if !ok {
			return nil, ErrIncompletePacket
		}
		if packetID == 0 {
			return nil, ErrInvalidPacketID
		}
		p.PacketID = packetID
		pos += n
	}

	// Properties (MQTT 5.0)
	if version == Version5 {
		props, n, err := DecodeProperties(buf[pos:])
		if err != nil {
			return nil, err
		}
		p.Properties = props
		pos += n
	}

	// Payload (remaining bytes)
	if pos < len(buf) {
		p.Payload = make([]byte, len(buf)-pos)
		copy(p.Payload, buf[pos:])
	}

	return p, nil
}

// NewPublish creates a new PUBLISH packet.
func NewPublish(version Version, topic string, payload []byte, qos QoS, retain bool) *Publish {
	return &Publish{
		Version:   version,
		TopicName: topic,
		Payload:   payload,
		QoS:       qos,
		Retain:    retain,
	}
}
