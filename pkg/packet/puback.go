package packet

// Puback represents an MQTT PUBACK packet (QoS 1 acknowledgment).
// MQTT 3.1.1 Section 3.4, MQTT 5.0 Section 3.4
type Puback struct {
	PacketID   uint16      // Packet identifier
	ReasonCode ReasonCode  // MQTT 5.0 only (default: Success)
	Properties *Properties // MQTT 5.0 only
	Version    Version
}

// Type returns TypePuback.
func (p *Puback) Type() Type {
	return TypePuback
}

// EncodedSize returns the total size of the encoded PUBACK packet.
func (p *Puback) EncodedSize() int {
	// Packet ID
	varHeaderSize := 2

	// MQTT 5.0: Reason code and properties
	if p.Version == Version5 {
		// Only include reason code if not Success or if properties exist
		if p.ReasonCode != ReasonSuccess || p.Properties != nil {
			varHeaderSize += 1 // Reason code
			if p.Properties != nil {
				varHeaderSize += p.Properties.EncodedSize()
			} else {
				varHeaderSize += 1 // Empty properties
			}
		}
	}

	return FixedHeaderSize(uint32(varHeaderSize)) + varHeaderSize
}

// Encode encodes the PUBACK packet into buf.
func (p *Puback) Encode(buf []byte) int {
	size := p.EncodedSize()
	if len(buf) < size {
		return 0
	}

	varHeaderSize := 2
	if p.Version == Version5 && (p.ReasonCode != ReasonSuccess || p.Properties != nil) {
		varHeaderSize += 1
		if p.Properties != nil {
			varHeaderSize += p.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
	}

	// Fixed header
	pos := EncodeFixedHeader(buf, TypePuback, 0, uint32(varHeaderSize))
	if pos == 0 {
		return 0
	}

	// Packet ID
	pos += EncodeUint16(buf[pos:], p.PacketID)

	// MQTT 5.0: Reason code and properties
	if p.Version == Version5 && (p.ReasonCode != ReasonSuccess || p.Properties != nil) {
		buf[pos] = byte(p.ReasonCode)
		pos++
		if p.Properties != nil {
			pos += p.Properties.Encode(buf[pos:])
		} else {
			buf[pos] = 0
			pos++
		}
	}

	return pos
}

// DecodePuback decodes a PUBACK packet from buf.
func DecodePuback(buf []byte, version Version) (*Puback, error) {
	if len(buf) < 2 {
		return nil, ErrIncompletePacket
	}

	p := &Puback{Version: version, ReasonCode: ReasonSuccess}
	pos := 0

	// Packet ID
	packetID, n, ok := DecodeUint16(buf[pos:])
	if !ok {
		return nil, ErrIncompletePacket
	}
	p.PacketID = packetID
	pos += n

	// MQTT 5.0: Reason code and properties (optional)
	if version == Version5 && pos < len(buf) {
		p.ReasonCode = ReasonCode(buf[pos])
		pos++

		if pos < len(buf) {
			props, n, err := DecodeProperties(buf[pos:])
			if err != nil {
				return nil, err
			}
			p.Properties = props
			pos += n
		}
	}

	return p, nil
}

// Pubrec represents an MQTT PUBREC packet (QoS 2 step 1).
// MQTT 3.1.1 Section 3.5, MQTT 5.0 Section 3.5
type Pubrec struct {
	PacketID   uint16
	ReasonCode ReasonCode  // MQTT 5.0 only
	Properties *Properties // MQTT 5.0 only
	Version    Version
}

// Type returns TypePubrec.
func (p *Pubrec) Type() Type {
	return TypePubrec
}

// EncodedSize returns the total size of the encoded PUBREC packet.
func (p *Pubrec) EncodedSize() int {
	varHeaderSize := 2

	if p.Version == Version5 && (p.ReasonCode != ReasonSuccess || p.Properties != nil) {
		varHeaderSize += 1
		if p.Properties != nil {
			varHeaderSize += p.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
	}

	return FixedHeaderSize(uint32(varHeaderSize)) + varHeaderSize
}

// Encode encodes the PUBREC packet into buf.
func (p *Pubrec) Encode(buf []byte) int {
	size := p.EncodedSize()
	if len(buf) < size {
		return 0
	}

	varHeaderSize := 2
	if p.Version == Version5 && (p.ReasonCode != ReasonSuccess || p.Properties != nil) {
		varHeaderSize += 1
		if p.Properties != nil {
			varHeaderSize += p.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
	}

	pos := EncodeFixedHeader(buf, TypePubrec, 0, uint32(varHeaderSize))
	if pos == 0 {
		return 0
	}

	pos += EncodeUint16(buf[pos:], p.PacketID)

	if p.Version == Version5 && (p.ReasonCode != ReasonSuccess || p.Properties != nil) {
		buf[pos] = byte(p.ReasonCode)
		pos++
		if p.Properties != nil {
			pos += p.Properties.Encode(buf[pos:])
		} else {
			buf[pos] = 0
			pos++
		}
	}

	return pos
}

// DecodePubrec decodes a PUBREC packet from buf.
func DecodePubrec(buf []byte, version Version) (*Pubrec, error) {
	if len(buf) < 2 {
		return nil, ErrIncompletePacket
	}

	p := &Pubrec{Version: version, ReasonCode: ReasonSuccess}
	pos := 0

	packetID, n, ok := DecodeUint16(buf[pos:])
	if !ok {
		return nil, ErrIncompletePacket
	}
	p.PacketID = packetID
	pos += n

	if version == Version5 && pos < len(buf) {
		p.ReasonCode = ReasonCode(buf[pos])
		pos++

		if pos < len(buf) {
			props, n, err := DecodeProperties(buf[pos:])
			if err != nil {
				return nil, err
			}
			p.Properties = props
			pos += n
		}
	}

	return p, nil
}

// Pubrel represents an MQTT PUBREL packet (QoS 2 step 2).
// MQTT 3.1.1 Section 3.6, MQTT 5.0 Section 3.6
type Pubrel struct {
	PacketID   uint16
	ReasonCode ReasonCode  // MQTT 5.0 only
	Properties *Properties // MQTT 5.0 only
	Version    Version
}

// Type returns TypePubrel.
func (p *Pubrel) Type() Type {
	return TypePubrel
}

// EncodedSize returns the total size of the encoded PUBREL packet.
func (p *Pubrel) EncodedSize() int {
	varHeaderSize := 2

	if p.Version == Version5 && (p.ReasonCode != ReasonSuccess || p.Properties != nil) {
		varHeaderSize += 1
		if p.Properties != nil {
			varHeaderSize += p.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
	}

	return FixedHeaderSize(uint32(varHeaderSize)) + varHeaderSize
}

// Encode encodes the PUBREL packet into buf.
func (p *Pubrel) Encode(buf []byte) int {
	size := p.EncodedSize()
	if len(buf) < size {
		return 0
	}

	varHeaderSize := 2
	if p.Version == Version5 && (p.ReasonCode != ReasonSuccess || p.Properties != nil) {
		varHeaderSize += 1
		if p.Properties != nil {
			varHeaderSize += p.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
	}

	// PUBREL has fixed flags 0010 (0x02)
	pos := EncodeFixedHeader(buf, TypePubrel, PubrelFlags, uint32(varHeaderSize))
	if pos == 0 {
		return 0
	}

	pos += EncodeUint16(buf[pos:], p.PacketID)

	if p.Version == Version5 && (p.ReasonCode != ReasonSuccess || p.Properties != nil) {
		buf[pos] = byte(p.ReasonCode)
		pos++
		if p.Properties != nil {
			pos += p.Properties.Encode(buf[pos:])
		} else {
			buf[pos] = 0
			pos++
		}
	}

	return pos
}

// DecodePubrel decodes a PUBREL packet from buf.
func DecodePubrel(buf []byte, version Version) (*Pubrel, error) {
	if len(buf) < 2 {
		return nil, ErrIncompletePacket
	}

	p := &Pubrel{Version: version, ReasonCode: ReasonSuccess}
	pos := 0

	packetID, n, ok := DecodeUint16(buf[pos:])
	if !ok {
		return nil, ErrIncompletePacket
	}
	p.PacketID = packetID
	pos += n

	if version == Version5 && pos < len(buf) {
		p.ReasonCode = ReasonCode(buf[pos])
		pos++

		if pos < len(buf) {
			props, n, err := DecodeProperties(buf[pos:])
			if err != nil {
				return nil, err
			}
			p.Properties = props
			pos += n
		}
	}

	return p, nil
}

// Pubcomp represents an MQTT PUBCOMP packet (QoS 2 step 3).
// MQTT 3.1.1 Section 3.7, MQTT 5.0 Section 3.7
type Pubcomp struct {
	PacketID   uint16
	ReasonCode ReasonCode  // MQTT 5.0 only
	Properties *Properties // MQTT 5.0 only
	Version    Version
}

// Type returns TypePubcomp.
func (p *Pubcomp) Type() Type {
	return TypePubcomp
}

// EncodedSize returns the total size of the encoded PUBCOMP packet.
func (p *Pubcomp) EncodedSize() int {
	varHeaderSize := 2

	if p.Version == Version5 && (p.ReasonCode != ReasonSuccess || p.Properties != nil) {
		varHeaderSize += 1
		if p.Properties != nil {
			varHeaderSize += p.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
	}

	return FixedHeaderSize(uint32(varHeaderSize)) + varHeaderSize
}

// Encode encodes the PUBCOMP packet into buf.
func (p *Pubcomp) Encode(buf []byte) int {
	size := p.EncodedSize()
	if len(buf) < size {
		return 0
	}

	varHeaderSize := 2
	if p.Version == Version5 && (p.ReasonCode != ReasonSuccess || p.Properties != nil) {
		varHeaderSize += 1
		if p.Properties != nil {
			varHeaderSize += p.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
	}

	pos := EncodeFixedHeader(buf, TypePubcomp, 0, uint32(varHeaderSize))
	if pos == 0 {
		return 0
	}

	pos += EncodeUint16(buf[pos:], p.PacketID)

	if p.Version == Version5 && (p.ReasonCode != ReasonSuccess || p.Properties != nil) {
		buf[pos] = byte(p.ReasonCode)
		pos++
		if p.Properties != nil {
			pos += p.Properties.Encode(buf[pos:])
		} else {
			buf[pos] = 0
			pos++
		}
	}

	return pos
}

// DecodePubcomp decodes a PUBCOMP packet from buf.
func DecodePubcomp(buf []byte, version Version) (*Pubcomp, error) {
	if len(buf) < 2 {
		return nil, ErrIncompletePacket
	}

	p := &Pubcomp{Version: version, ReasonCode: ReasonSuccess}
	pos := 0

	packetID, n, ok := DecodeUint16(buf[pos:])
	if !ok {
		return nil, ErrIncompletePacket
	}
	p.PacketID = packetID
	pos += n

	if version == Version5 && pos < len(buf) {
		p.ReasonCode = ReasonCode(buf[pos])
		pos++

		if pos < len(buf) {
			props, n, err := DecodeProperties(buf[pos:])
			if err != nil {
				return nil, err
			}
			p.Properties = props
			pos += n
		}
	}

	return p, nil
}
