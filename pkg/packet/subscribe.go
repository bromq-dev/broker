package packet

// Subscription represents a single topic subscription.
type Subscription struct {
	TopicFilter string
	QoS         QoS

	// MQTT 5.0 subscription options
	NoLocal           bool // Don't receive own publications
	RetainAsPublished bool // Keep original retain flag
	RetainHandling    byte // 0=send retained, 1=send if new sub, 2=don't send
}

// Subscribe represents an MQTT SUBSCRIBE packet.
// MQTT 3.1.1 Section 3.8, MQTT 5.0 Section 3.8
type Subscribe struct {
	PacketID      uint16
	Subscriptions []Subscription
	Properties    *Properties // MQTT 5.0 only
	Version       Version
}

// Type returns TypeSubscribe.
func (s *Subscribe) Type() Type {
	return TypeSubscribe
}

// EncodedSize returns the total size of the encoded SUBSCRIBE packet.
func (s *Subscribe) EncodedSize() int {
	// Packet ID
	varHeaderSize := 2

	// Properties (MQTT 5.0)
	if s.Version == Version5 {
		if s.Properties != nil {
			varHeaderSize += s.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
	}

	// Payload: topic filters + subscription options
	payloadSize := 0
	for _, sub := range s.Subscriptions {
		payloadSize += 2 + len(sub.TopicFilter) + 1 // length + topic + options byte
	}

	remainingLength := varHeaderSize + payloadSize
	return FixedHeaderSize(uint32(remainingLength)) + remainingLength
}

// Encode encodes the SUBSCRIBE packet into buf.
func (s *Subscribe) Encode(buf []byte) int {
	size := s.EncodedSize()
	if len(buf) < size {
		return 0
	}

	varHeaderSize := 2
	if s.Version == Version5 {
		if s.Properties != nil {
			varHeaderSize += s.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
	}

	payloadSize := 0
	for _, sub := range s.Subscriptions {
		payloadSize += 2 + len(sub.TopicFilter) + 1
	}

	remainingLength := uint32(varHeaderSize + payloadSize)

	// Fixed header (SUBSCRIBE has reserved flags 0010)
	pos := EncodeFixedHeader(buf, TypeSubscribe, SubscribeFlags, remainingLength)
	if pos == 0 {
		return 0
	}

	// Packet ID
	pos += EncodeUint16(buf[pos:], s.PacketID)

	// Properties (MQTT 5.0)
	if s.Version == Version5 {
		if s.Properties != nil {
			pos += s.Properties.Encode(buf[pos:])
		} else {
			buf[pos] = 0
			pos++
		}
	}

	// Payload: subscriptions
	for _, sub := range s.Subscriptions {
		pos += EncodeString(buf[pos:], sub.TopicFilter)

		// Subscription options byte
		var options byte = byte(sub.QoS)
		if s.Version == Version5 {
			if sub.NoLocal {
				options |= 0x04
			}
			if sub.RetainAsPublished {
				options |= 0x08
			}
			options |= (sub.RetainHandling & 0x03) << 4
		}
		buf[pos] = options
		pos++
	}

	return pos
}

// DecodeSubscribe decodes a SUBSCRIBE packet from buf.
func DecodeSubscribe(buf []byte, version Version) (*Subscribe, error) {
	if len(buf) < 5 { // Minimum: packet ID + one subscription
		return nil, ErrIncompletePacket
	}

	s := &Subscribe{Version: version}
	pos := 0

	// Packet ID
	packetID, n, ok := DecodeUint16(buf[pos:])
	if !ok {
		return nil, ErrIncompletePacket
	}
	if packetID == 0 {
		return nil, ErrInvalidPacketID
	}
	s.PacketID = packetID
	pos += n

	// Properties (MQTT 5.0)
	if version == Version5 {
		props, n, err := DecodeProperties(buf[pos:])
		if err != nil {
			return nil, err
		}
		s.Properties = props
		pos += n
	}

	// Payload: subscriptions
	for pos < len(buf) {
		// Topic filter
		topic, n, ok := DecodeStringCopy(buf[pos:])
		if !ok {
			return nil, ErrIncompletePacket
		}
		pos += n

		// Options byte
		if pos >= len(buf) {
			return nil, ErrIncompletePacket
		}
		options := buf[pos]
		pos++

		sub := Subscription{
			TopicFilter: topic,
			QoS:         QoS(options & 0x03),
		}

		if !sub.QoS.Valid() {
			return nil, ErrInvalidQoS
		}

		// MQTT 5.0 subscription options
		if version == Version5 {
			sub.NoLocal = options&0x04 != 0
			sub.RetainAsPublished = options&0x08 != 0
			sub.RetainHandling = (options >> 4) & 0x03

			// Reserved bits must be 0
			if options&0xC0 != 0 {
				return nil, ErrMalformedPacket
			}
		} else {
			// For v3.1.1, bits 7-2 must be 0
			if options&0xFC != 0 {
				return nil, ErrMalformedPacket
			}
		}

		s.Subscriptions = append(s.Subscriptions, sub)
	}

	// Must have at least one subscription
	if len(s.Subscriptions) == 0 {
		return nil, ErrMalformedPacket
	}

	return s, nil
}

// Suback represents an MQTT SUBACK packet.
// MQTT 3.1.1 Section 3.9, MQTT 5.0 Section 3.9
type Suback struct {
	PacketID    uint16
	ReasonCodes []byte      // Return codes (v3.1.1) or Reason codes (v5.0)
	Properties  *Properties // MQTT 5.0 only
	Version     Version
}

// Type returns TypeSuback.
func (s *Suback) Type() Type {
	return TypeSuback
}

// EncodedSize returns the total size of the encoded SUBACK packet.
func (s *Suback) EncodedSize() int {
	varHeaderSize := 2 // Packet ID

	if s.Version == Version5 {
		if s.Properties != nil {
			varHeaderSize += s.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
	}

	payloadSize := len(s.ReasonCodes)
	remainingLength := varHeaderSize + payloadSize
	return FixedHeaderSize(uint32(remainingLength)) + remainingLength
}

// Encode encodes the SUBACK packet into buf.
func (s *Suback) Encode(buf []byte) int {
	size := s.EncodedSize()
	if len(buf) < size {
		return 0
	}

	varHeaderSize := 2
	if s.Version == Version5 {
		if s.Properties != nil {
			varHeaderSize += s.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
	}

	payloadSize := len(s.ReasonCodes)
	remainingLength := uint32(varHeaderSize + payloadSize)

	pos := EncodeFixedHeader(buf, TypeSuback, 0, remainingLength)
	if pos == 0 {
		return 0
	}

	pos += EncodeUint16(buf[pos:], s.PacketID)

	if s.Version == Version5 {
		if s.Properties != nil {
			pos += s.Properties.Encode(buf[pos:])
		} else {
			buf[pos] = 0
			pos++
		}
	}

	copy(buf[pos:], s.ReasonCodes)
	pos += len(s.ReasonCodes)

	return pos
}

// DecodeSuback decodes a SUBACK packet from buf.
func DecodeSuback(buf []byte, version Version) (*Suback, error) {
	if len(buf) < 3 { // Minimum: packet ID + one reason code
		return nil, ErrIncompletePacket
	}

	s := &Suback{Version: version}
	pos := 0

	packetID, n, ok := DecodeUint16(buf[pos:])
	if !ok {
		return nil, ErrIncompletePacket
	}
	s.PacketID = packetID
	pos += n

	if version == Version5 {
		props, n, err := DecodeProperties(buf[pos:])
		if err != nil {
			return nil, err
		}
		s.Properties = props
		pos += n
	}

	// Remaining bytes are reason codes
	if pos >= len(buf) {
		return nil, ErrIncompletePacket
	}
	s.ReasonCodes = make([]byte, len(buf)-pos)
	copy(s.ReasonCodes, buf[pos:])

	return s, nil
}

// NewSuback creates a new SUBACK packet.
func NewSuback(version Version, packetID uint16, codes []byte) *Suback {
	return &Suback{
		Version:     version,
		PacketID:    packetID,
		ReasonCodes: codes,
	}
}
