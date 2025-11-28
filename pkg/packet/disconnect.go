package packet

// Disconnect represents an MQTT DISCONNECT packet.
// MQTT 3.1.1 Section 3.14, MQTT 5.0 Section 3.14
type Disconnect struct {
	ReasonCode ReasonCode  // MQTT 5.0 only (default: Normal Disconnection = 0x00)
	Properties *Properties // MQTT 5.0 only
	Version    Version
}

// Type returns TypeDisconnect.
func (d *Disconnect) Type() Type {
	return TypeDisconnect
}

// EncodedSize returns the total size of the encoded DISCONNECT packet.
func (d *Disconnect) EncodedSize() int {
	// MQTT 3.1.1: No variable header or payload
	if d.Version != Version5 {
		return 2 // Fixed header only
	}

	// MQTT 5.0: Can omit reason code if 0x00 and no properties
	if d.ReasonCode == ReasonSuccess && d.Properties == nil {
		return 2
	}

	// Include reason code
	varHeaderSize := 1

	// Include properties if present
	if d.Properties != nil {
		varHeaderSize += d.Properties.EncodedSize()
	} else if d.ReasonCode != ReasonSuccess {
		// Need empty properties after non-zero reason code
		varHeaderSize += 1
	}

	return FixedHeaderSize(uint32(varHeaderSize)) + varHeaderSize
}

// Encode encodes the DISCONNECT packet into buf.
func (d *Disconnect) Encode(buf []byte) int {
	size := d.EncodedSize()
	if len(buf) < size {
		return 0
	}

	// MQTT 3.1.1 or v5.0 with default reason and no properties
	if d.Version != Version5 || (d.ReasonCode == ReasonSuccess && d.Properties == nil) {
		buf[0] = byte(TypeDisconnect) << 4
		buf[1] = 0
		return 2
	}

	varHeaderSize := 1
	if d.Properties != nil {
		varHeaderSize += d.Properties.EncodedSize()
	} else if d.ReasonCode != ReasonSuccess {
		varHeaderSize += 1
	}

	pos := EncodeFixedHeader(buf, TypeDisconnect, 0, uint32(varHeaderSize))
	if pos == 0 {
		return 0
	}

	buf[pos] = byte(d.ReasonCode)
	pos++

	if d.Properties != nil {
		pos += d.Properties.Encode(buf[pos:])
	} else if d.ReasonCode != ReasonSuccess {
		buf[pos] = 0 // Empty properties
		pos++
	}

	return pos
}

// DecodeDisconnect decodes a DISCONNECT packet from buf.
func DecodeDisconnect(buf []byte, version Version) (*Disconnect, error) {
	d := &Disconnect{
		Version:    version,
		ReasonCode: ReasonSuccess,
	}

	// MQTT 3.1.1: No variable header or payload
	if version != Version5 {
		if len(buf) != 0 {
			return nil, ErrMalformedPacket
		}
		return d, nil
	}

	// MQTT 5.0: Empty packet means normal disconnection
	if len(buf) == 0 {
		return d, nil
	}

	// Reason code
	d.ReasonCode = ReasonCode(buf[0])

	// Properties (optional)
	if len(buf) > 1 {
		props, _, err := DecodeProperties(buf[1:])
		if err != nil {
			return nil, err
		}
		d.Properties = props
	}

	return d, nil
}

// NewDisconnect creates a new DISCONNECT packet.
func NewDisconnect(version Version) *Disconnect {
	return &Disconnect{
		Version:    version,
		ReasonCode: ReasonSuccess,
	}
}

// NewDisconnectV5 creates a new DISCONNECT packet for MQTT 5.0 with a reason code.
func NewDisconnectV5(code ReasonCode, props *Properties) *Disconnect {
	return &Disconnect{
		Version:    Version5,
		ReasonCode: code,
		Properties: props,
	}
}
