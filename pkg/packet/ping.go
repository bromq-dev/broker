package packet

// Pingreq represents an MQTT PINGREQ packet.
// MQTT 3.1.1 Section 3.12, MQTT 5.0 Section 3.12
type Pingreq struct{}

// Type returns TypePingreq.
func (p *Pingreq) Type() Type {
	return TypePingreq
}

// EncodedSize returns the total size of the encoded PINGREQ packet.
func (p *Pingreq) EncodedSize() int {
	return 2 // Fixed header only, remaining length = 0
}

// Encode encodes the PINGREQ packet into buf.
func (p *Pingreq) Encode(buf []byte) int {
	if len(buf) < 2 {
		return 0
	}
	buf[0] = byte(TypePingreq) << 4
	buf[1] = 0 // Remaining length
	return 2
}

// DecodePingreq decodes a PINGREQ packet.
// PINGREQ has no variable header or payload, so buf should be empty.
func DecodePingreq(buf []byte) (*Pingreq, error) {
	if len(buf) != 0 {
		return nil, ErrMalformedPacket
	}
	return &Pingreq{}, nil
}

// Pingresp represents an MQTT PINGRESP packet.
// MQTT 3.1.1 Section 3.13, MQTT 5.0 Section 3.13
type Pingresp struct{}

// Type returns TypePingresp.
func (p *Pingresp) Type() Type {
	return TypePingresp
}

// EncodedSize returns the total size of the encoded PINGRESP packet.
func (p *Pingresp) EncodedSize() int {
	return 2 // Fixed header only, remaining length = 0
}

// Encode encodes the PINGRESP packet into buf.
func (p *Pingresp) Encode(buf []byte) int {
	if len(buf) < 2 {
		return 0
	}
	buf[0] = byte(TypePingresp) << 4
	buf[1] = 0 // Remaining length
	return 2
}

// DecodePingresp decodes a PINGRESP packet.
// PINGRESP has no variable header or payload, so buf should be empty.
func DecodePingresp(buf []byte) (*Pingresp, error) {
	if len(buf) != 0 {
		return nil, ErrMalformedPacket
	}
	return &Pingresp{}, nil
}
