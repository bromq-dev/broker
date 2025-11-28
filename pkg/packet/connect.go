package packet

// Connect represents an MQTT CONNECT packet.
// MQTT 3.1.1 Section 3.1, MQTT 5.0 Section 3.1
type Connect struct {
	// Protocol identification
	ProtocolName    string  // Must be "MQTT"
	ProtocolVersion Version // 4 for v3.1.1, 5 for v5.0

	// Connect flags
	CleanStart   bool // Clean Session (v3.1.1) or Clean Start (v5.0)
	WillFlag     bool
	WillQoS      QoS
	WillRetain   bool
	PasswordFlag bool
	UsernameFlag bool

	// Keep alive (seconds)
	KeepAlive uint16

	// Properties (MQTT 5.0 only)
	Properties *Properties

	// Payload fields
	ClientID    string
	WillProps   *Properties // Will properties (MQTT 5.0 only)
	WillTopic   string
	WillPayload []byte
	Username    string
	Password    []byte
}

// Type returns TypeConnect.
func (c *Connect) Type() Type {
	return TypeConnect
}

// connectFlagBits defines the bit positions in the connect flags byte.
const (
	connectFlagCleanStart   = 1 << 1
	connectFlagWill         = 1 << 2
	connectFlagWillQoS0     = 0 << 3
	connectFlagWillQoS1     = 1 << 3
	connectFlagWillQoS2     = 2 << 3
	connectFlagWillRetain   = 1 << 5
	connectFlagPassword     = 1 << 6
	connectFlagUsername     = 1 << 7
)

// EncodedSize returns the total size of the encoded CONNECT packet.
func (c *Connect) EncodedSize() int {
	// Variable header: protocol name (2 + len) + version (1) + flags (1) + keepalive (2)
	varHeaderSize := 2 + len(c.ProtocolName) + 1 + 1 + 2

	// Properties (MQTT 5.0)
	if c.ProtocolVersion == Version5 {
		if c.Properties != nil {
			varHeaderSize += c.Properties.EncodedSize()
		} else {
			varHeaderSize += 1 // Empty properties (just the length byte = 0)
		}
	}

	// Payload
	payloadSize := 2 + len(c.ClientID) // Client ID always present

	if c.WillFlag {
		if c.ProtocolVersion == Version5 {
			if c.WillProps != nil {
				payloadSize += c.WillProps.EncodedSize()
			} else {
				payloadSize += 1
			}
		}
		payloadSize += 2 + len(c.WillTopic)
		payloadSize += 2 + len(c.WillPayload)
	}

	if c.UsernameFlag {
		payloadSize += 2 + len(c.Username)
	}

	if c.PasswordFlag {
		payloadSize += 2 + len(c.Password)
	}

	remainingLength := varHeaderSize + payloadSize
	return FixedHeaderSize(uint32(remainingLength)) + remainingLength
}

// Encode encodes the CONNECT packet into buf.
// Returns the number of bytes written, or 0 on error.
func (c *Connect) Encode(buf []byte) int {
	size := c.EncodedSize()
	if len(buf) < size {
		return 0
	}

	// Calculate remaining length
	varHeaderSize := 2 + len(c.ProtocolName) + 1 + 1 + 2
	if c.ProtocolVersion == Version5 {
		if c.Properties != nil {
			varHeaderSize += c.Properties.EncodedSize()
		} else {
			varHeaderSize += 1
		}
	}

	payloadSize := 2 + len(c.ClientID)
	if c.WillFlag {
		if c.ProtocolVersion == Version5 {
			if c.WillProps != nil {
				payloadSize += c.WillProps.EncodedSize()
			} else {
				payloadSize += 1
			}
		}
		payloadSize += 2 + len(c.WillTopic)
		payloadSize += 2 + len(c.WillPayload)
	}
	if c.UsernameFlag {
		payloadSize += 2 + len(c.Username)
	}
	if c.PasswordFlag {
		payloadSize += 2 + len(c.Password)
	}

	remainingLength := uint32(varHeaderSize + payloadSize)

	// Fixed header
	pos := EncodeFixedHeader(buf, TypeConnect, 0, remainingLength)
	if pos == 0 {
		return 0
	}

	// Variable header
	// Protocol name
	pos += EncodeString(buf[pos:], c.ProtocolName)

	// Protocol version
	buf[pos] = byte(c.ProtocolVersion)
	pos++

	// Connect flags
	var flags byte
	if c.CleanStart {
		flags |= connectFlagCleanStart
	}
	if c.WillFlag {
		flags |= connectFlagWill
		flags |= byte(c.WillQoS) << 3
		if c.WillRetain {
			flags |= connectFlagWillRetain
		}
	}
	if c.PasswordFlag {
		flags |= connectFlagPassword
	}
	if c.UsernameFlag {
		flags |= connectFlagUsername
	}
	buf[pos] = flags
	pos++

	// Keep alive
	pos += EncodeUint16(buf[pos:], c.KeepAlive)

	// Properties (MQTT 5.0)
	if c.ProtocolVersion == Version5 {
		if c.Properties != nil {
			pos += c.Properties.Encode(buf[pos:])
		} else {
			buf[pos] = 0 // Empty properties
			pos++
		}
	}

	// Payload
	// Client ID
	pos += EncodeString(buf[pos:], c.ClientID)

	// Will
	if c.WillFlag {
		// Will properties (MQTT 5.0)
		if c.ProtocolVersion == Version5 {
			if c.WillProps != nil {
				pos += c.WillProps.Encode(buf[pos:])
			} else {
				buf[pos] = 0
				pos++
			}
		}
		// Will topic
		pos += EncodeString(buf[pos:], c.WillTopic)
		// Will payload
		pos += EncodeBytes(buf[pos:], c.WillPayload)
	}

	// Username
	if c.UsernameFlag {
		pos += EncodeString(buf[pos:], c.Username)
	}

	// Password
	if c.PasswordFlag {
		pos += EncodeBytes(buf[pos:], c.Password)
	}

	return pos
}

// DecodeConnect decodes a CONNECT packet from buf.
// buf should contain the packet data starting after the fixed header.
func DecodeConnect(buf []byte) (*Connect, error) {
	if len(buf) < 10 {
		return nil, ErrIncompletePacket
	}

	c := &Connect{}
	pos := 0

	// Protocol name
	name, n, ok := DecodeStringCopy(buf[pos:])
	if !ok {
		return nil, ErrIncompletePacket
	}
	c.ProtocolName = name
	pos += n

	// Validate protocol name
	if c.ProtocolName != "MQTT" && c.ProtocolName != "MQIsdp" {
		return nil, ErrInvalidProtocolName
	}

	// Protocol version
	if pos >= len(buf) {
		return nil, ErrIncompletePacket
	}
	c.ProtocolVersion = Version(buf[pos])
	pos++

	// Validate version
	if c.ProtocolVersion != Version31 && c.ProtocolVersion != Version311 && c.ProtocolVersion != Version5 {
		return nil, ErrInvalidProtocolVersion
	}

	// Connect flags
	if pos >= len(buf) {
		return nil, ErrIncompletePacket
	}
	flags := buf[pos]
	pos++

	// Reserved bit must be 0
	if flags&0x01 != 0 {
		return nil, ErrMalformedPacket
	}

	c.CleanStart = flags&connectFlagCleanStart != 0
	c.WillFlag = flags&connectFlagWill != 0
	c.WillQoS = QoS((flags >> 3) & 0x03)
	c.WillRetain = flags&connectFlagWillRetain != 0
	c.PasswordFlag = flags&connectFlagPassword != 0
	c.UsernameFlag = flags&connectFlagUsername != 0

	// Validate will flags
	if !c.WillFlag {
		if c.WillQoS != 0 || c.WillRetain {
			return nil, ErrMalformedPacket
		}
	} else {
		if !c.WillQoS.Valid() {
			return nil, ErrInvalidQoS
		}
	}

	// v3.1.1: Password requires Username
	if c.ProtocolVersion != Version5 && c.PasswordFlag && !c.UsernameFlag {
		return nil, ErrMalformedPacket
	}

	// Keep alive
	if pos+2 > len(buf) {
		return nil, ErrIncompletePacket
	}
	c.KeepAlive, _, _ = DecodeUint16(buf[pos:])
	pos += 2

	// Properties (MQTT 5.0 only)
	if c.ProtocolVersion == Version5 {
		props, n, err := DecodeProperties(buf[pos:])
		if err != nil {
			return nil, err
		}
		c.Properties = props
		pos += n
	}

	// Payload

	// Client ID (always present)
	clientID, n, ok := DecodeStringCopy(buf[pos:])
	if !ok {
		return nil, ErrIncompletePacket
	}
	c.ClientID = clientID
	pos += n

	// Will
	if c.WillFlag {
		// Will properties (MQTT 5.0)
		if c.ProtocolVersion == Version5 {
			props, n, err := DecodeProperties(buf[pos:])
			if err != nil {
				return nil, err
			}
			c.WillProps = props
			pos += n
		}

		// Will topic
		topic, n, ok := DecodeStringCopy(buf[pos:])
		if !ok {
			return nil, ErrIncompletePacket
		}
		c.WillTopic = topic
		pos += n

		// Will payload
		payload, n, ok := DecodeString(buf[pos:])
		if !ok {
			return nil, ErrIncompletePacket
		}
		c.WillPayload = make([]byte, len(payload))
		copy(c.WillPayload, payload)
		pos += n
	}

	// Username
	if c.UsernameFlag {
		username, n, ok := DecodeStringCopy(buf[pos:])
		if !ok {
			return nil, ErrIncompletePacket
		}
		c.Username = username
		pos += n
	}

	// Password
	if c.PasswordFlag {
		password, n, ok := DecodeString(buf[pos:])
		if !ok {
			return nil, ErrIncompletePacket
		}
		c.Password = make([]byte, len(password))
		copy(c.Password, password)
		pos += n
	}

	return c, nil
}
