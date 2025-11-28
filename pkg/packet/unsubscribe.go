package packet

// Unsubscribe represents an MQTT UNSUBSCRIBE packet.
// MQTT 3.1.1 Section 3.10, MQTT 5.0 Section 3.10
type Unsubscribe struct {
	PacketID     uint16
	TopicFilters []string
	Properties   *Properties // MQTT 5.0 only
	Version      Version
}

// Type returns TypeUnsubscribe.
func (u *Unsubscribe) Type() Type {
	return TypeUnsubscribe
}

// EncodedSize returns the total size of the encoded UNSUBSCRIBE packet.
func (u *Unsubscribe) EncodedSize() int {
	varHeaderSize := 2 // Packet ID

	if u.Version == Version5 {
		if u.Properties != nil {
			varHeaderSize += u.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
	}

	payloadSize := 0
	for _, topic := range u.TopicFilters {
		payloadSize += 2 + len(topic)
	}

	remainingLength := varHeaderSize + payloadSize
	return FixedHeaderSize(uint32(remainingLength)) + remainingLength
}

// Encode encodes the UNSUBSCRIBE packet into buf.
func (u *Unsubscribe) Encode(buf []byte) int {
	size := u.EncodedSize()
	if len(buf) < size {
		return 0
	}

	varHeaderSize := 2
	if u.Version == Version5 {
		if u.Properties != nil {
			varHeaderSize += u.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
	}

	payloadSize := 0
	for _, topic := range u.TopicFilters {
		payloadSize += 2 + len(topic)
	}

	remainingLength := uint32(varHeaderSize + payloadSize)

	// Fixed header (UNSUBSCRIBE has reserved flags 0010)
	pos := EncodeFixedHeader(buf, TypeUnsubscribe, UnsubscribeFlags, remainingLength)
	if pos == 0 {
		return 0
	}

	pos += EncodeUint16(buf[pos:], u.PacketID)

	if u.Version == Version5 {
		if u.Properties != nil {
			pos += u.Properties.Encode(buf[pos:])
		} else {
			buf[pos] = 0
			pos++
		}
	}

	for _, topic := range u.TopicFilters {
		pos += EncodeString(buf[pos:], topic)
	}

	return pos
}

// DecodeUnsubscribe decodes an UNSUBSCRIBE packet from buf.
func DecodeUnsubscribe(buf []byte, version Version) (*Unsubscribe, error) {
	if len(buf) < 4 { // Minimum: packet ID + one topic filter
		return nil, ErrIncompletePacket
	}

	u := &Unsubscribe{Version: version}
	pos := 0

	packetID, n, ok := DecodeUint16(buf[pos:])
	if !ok {
		return nil, ErrIncompletePacket
	}
	if packetID == 0 {
		return nil, ErrInvalidPacketID
	}
	u.PacketID = packetID
	pos += n

	if version == Version5 {
		props, n, err := DecodeProperties(buf[pos:])
		if err != nil {
			return nil, err
		}
		u.Properties = props
		pos += n
	}

	// Payload: topic filters
	for pos < len(buf) {
		topic, n, ok := DecodeStringCopy(buf[pos:])
		if !ok {
			return nil, ErrIncompletePacket
		}
		u.TopicFilters = append(u.TopicFilters, topic)
		pos += n
	}

	// Must have at least one topic filter
	if len(u.TopicFilters) == 0 {
		return nil, ErrMalformedPacket
	}

	return u, nil
}

// Unsuback represents an MQTT UNSUBACK packet.
// MQTT 3.1.1 Section 3.11, MQTT 5.0 Section 3.11
type Unsuback struct {
	PacketID    uint16
	ReasonCodes []byte      // MQTT 5.0 only (v3.1.1 has no reason codes)
	Properties  *Properties // MQTT 5.0 only
	Version     Version
}

// Type returns TypeUnsuback.
func (u *Unsuback) Type() Type {
	return TypeUnsuback
}

// EncodedSize returns the total size of the encoded UNSUBACK packet.
func (u *Unsuback) EncodedSize() int {
	varHeaderSize := 2 // Packet ID

	payloadSize := 0
	if u.Version == Version5 {
		if u.Properties != nil {
			varHeaderSize += u.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
		payloadSize = len(u.ReasonCodes)
	}

	remainingLength := varHeaderSize + payloadSize
	return FixedHeaderSize(uint32(remainingLength)) + remainingLength
}

// Encode encodes the UNSUBACK packet into buf.
func (u *Unsuback) Encode(buf []byte) int {
	size := u.EncodedSize()
	if len(buf) < size {
		return 0
	}

	varHeaderSize := 2
	payloadSize := 0
	if u.Version == Version5 {
		if u.Properties != nil {
			varHeaderSize += u.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
		payloadSize = len(u.ReasonCodes)
	}

	remainingLength := uint32(varHeaderSize + payloadSize)

	pos := EncodeFixedHeader(buf, TypeUnsuback, 0, remainingLength)
	if pos == 0 {
		return 0
	}

	pos += EncodeUint16(buf[pos:], u.PacketID)

	if u.Version == Version5 {
		if u.Properties != nil {
			pos += u.Properties.Encode(buf[pos:])
		} else {
			buf[pos] = 0
			pos++
		}

		copy(buf[pos:], u.ReasonCodes)
		pos += len(u.ReasonCodes)
	}

	return pos
}

// DecodeUnsuback decodes an UNSUBACK packet from buf.
func DecodeUnsuback(buf []byte, version Version) (*Unsuback, error) {
	if len(buf) < 2 {
		return nil, ErrIncompletePacket
	}

	u := &Unsuback{Version: version}
	pos := 0

	packetID, n, ok := DecodeUint16(buf[pos:])
	if !ok {
		return nil, ErrIncompletePacket
	}
	u.PacketID = packetID
	pos += n

	if version == Version5 {
		props, n, err := DecodeProperties(buf[pos:])
		if err != nil {
			return nil, err
		}
		u.Properties = props
		pos += n

		// Remaining bytes are reason codes
		if pos < len(buf) {
			u.ReasonCodes = make([]byte, len(buf)-pos)
			copy(u.ReasonCodes, buf[pos:])
		}
	}

	return u, nil
}

// NewUnsuback creates a new UNSUBACK packet.
func NewUnsuback(version Version, packetID uint16, codes []byte) *Unsuback {
	return &Unsuback{
		Version:     version,
		PacketID:    packetID,
		ReasonCodes: codes,
	}
}
