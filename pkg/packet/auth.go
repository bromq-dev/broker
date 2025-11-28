package packet

// Auth represents an MQTT AUTH packet (MQTT 5.0 only).
// MQTT 5.0 Section 3.15
type Auth struct {
	ReasonCode ReasonCode  // Default: Success (0x00)
	Properties *Properties
}

// Type returns TypeAuth.
func (a *Auth) Type() Type {
	return TypeAuth
}

// EncodedSize returns the total size of the encoded AUTH packet.
func (a *Auth) EncodedSize() int {
	// Can omit reason code if 0x00 and no properties
	if a.ReasonCode == ReasonSuccess && a.Properties == nil {
		return 2 // Fixed header only
	}

	varHeaderSize := 1 // Reason code

	if a.Properties != nil {
		varHeaderSize += a.Properties.EncodedSize()
	} else {
		varHeaderSize += 1 // Empty properties
	}

	return FixedHeaderSize(uint32(varHeaderSize)) + varHeaderSize
}

// Encode encodes the AUTH packet into buf.
func (a *Auth) Encode(buf []byte) int {
	size := a.EncodedSize()
	if len(buf) < size {
		return 0
	}

	// Minimal encoding if success with no properties
	if a.ReasonCode == ReasonSuccess && a.Properties == nil {
		buf[0] = byte(TypeAuth) << 4
		buf[1] = 0
		return 2
	}

	varHeaderSize := 1
	if a.Properties != nil {
		varHeaderSize += a.Properties.EncodedSize()
	} else {
		varHeaderSize += 1
	}

	pos := EncodeFixedHeader(buf, TypeAuth, 0, uint32(varHeaderSize))
	if pos == 0 {
		return 0
	}

	buf[pos] = byte(a.ReasonCode)
	pos++

	if a.Properties != nil {
		pos += a.Properties.Encode(buf[pos:])
	} else {
		buf[pos] = 0
		pos++
	}

	return pos
}

// DecodeAuth decodes an AUTH packet from buf.
func DecodeAuth(buf []byte) (*Auth, error) {
	a := &Auth{ReasonCode: ReasonSuccess}

	// Empty packet means success with no properties
	if len(buf) == 0 {
		return a, nil
	}

	// Reason code
	a.ReasonCode = ReasonCode(buf[0])

	// Properties
	if len(buf) > 1 {
		props, _, err := DecodeProperties(buf[1:])
		if err != nil {
			return nil, err
		}
		a.Properties = props
	}

	return a, nil
}

// NewAuth creates a new AUTH packet.
func NewAuth(code ReasonCode, props *Properties) *Auth {
	return &Auth{
		ReasonCode: code,
		Properties: props,
	}
}
