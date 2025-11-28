package packet

// Connack represents an MQTT CONNACK packet.
// MQTT 3.1.1 Section 3.2, MQTT 5.0 Section 3.2
type Connack struct {
	// Session present flag
	SessionPresent bool

	// Return code (v3.1.1) or Reason code (v5.0)
	// For v3.1.1: Use ConnackReturnCode values
	// For v5.0: Use ReasonCode values
	ReasonCode byte

	// Properties (MQTT 5.0 only)
	Properties *Properties

	// Version to determine encoding
	Version Version
}

// Type returns TypeConnack.
func (c *Connack) Type() Type {
	return TypeConnack
}

// EncodedSize returns the total size of the encoded CONNACK packet.
func (c *Connack) EncodedSize() int {
	// Variable header: Acknowledge flags (1) + Return/Reason code (1)
	varHeaderSize := 2

	// Properties (MQTT 5.0)
	if c.Version == Version5 {
		if c.Properties != nil {
			varHeaderSize += c.Properties.EncodedSize()
		} else {
			varHeaderSize += 1 // Empty properties
		}
	}

	return FixedHeaderSize(uint32(varHeaderSize)) + varHeaderSize
}

// Encode encodes the CONNACK packet into buf.
// Returns the number of bytes written, or 0 on error.
func (c *Connack) Encode(buf []byte) int {
	size := c.EncodedSize()
	if len(buf) < size {
		return 0
	}

	// Calculate remaining length
	varHeaderSize := 2
	if c.Version == Version5 {
		if c.Properties != nil {
			varHeaderSize += c.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
	}

	// Fixed header
	pos := EncodeFixedHeader(buf, TypeConnack, 0, uint32(varHeaderSize))
	if pos == 0 {
		return 0
	}

	// Variable header
	// Acknowledge flags (only bit 0 - session present)
	if c.SessionPresent {
		buf[pos] = 0x01
	} else {
		buf[pos] = 0x00
	}
	pos++

	// Return/Reason code
	buf[pos] = c.ReasonCode
	pos++

	// Properties (MQTT 5.0)
	if c.Version == Version5 {
		if c.Properties != nil {
			pos += c.Properties.Encode(buf[pos:])
		} else {
			buf[pos] = 0 // Empty properties
			pos++
		}
	}

	return pos
}

// DecodeConnack decodes a CONNACK packet from buf.
// buf should contain the packet data starting after the fixed header.
// version should be the protocol version from the connection.
func DecodeConnack(buf []byte, version Version) (*Connack, error) {
	if len(buf) < 2 {
		return nil, ErrIncompletePacket
	}

	c := &Connack{Version: version}
	pos := 0

	// Acknowledge flags
	ackFlags := buf[pos]
	pos++

	// Bits 7-1 must be 0
	if ackFlags&0xFE != 0 {
		return nil, ErrMalformedPacket
	}
	c.SessionPresent = ackFlags&0x01 != 0

	// Return/Reason code
	c.ReasonCode = buf[pos]
	pos++

	// Properties (MQTT 5.0)
	if version == Version5 {
		props, n, err := DecodeProperties(buf[pos:])
		if err != nil {
			return nil, err
		}
		c.Properties = props
		pos += n
	}

	return c, nil
}

// NewConnack creates a new CONNACK packet.
func NewConnack(version Version, sessionPresent bool, code byte) *Connack {
	return &Connack{
		Version:        version,
		SessionPresent: sessionPresent,
		ReasonCode:     code,
	}
}

// NewConnackV311 creates a new CONNACK packet for MQTT 3.1.1.
func NewConnackV311(sessionPresent bool, code ConnackReturnCode) *Connack {
	return &Connack{
		Version:        Version311,
		SessionPresent: sessionPresent,
		ReasonCode:     byte(code),
	}
}

// NewConnackV5 creates a new CONNACK packet for MQTT 5.0.
func NewConnackV5(sessionPresent bool, code ReasonCode, props *Properties) *Connack {
	return &Connack{
		Version:        Version5,
		SessionPresent: sessionPresent,
		ReasonCode:     byte(code),
		Properties:     props,
	}
}
