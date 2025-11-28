package packet

// PropertyID represents an MQTT 5.0 property identifier.
// MQTT 5.0 Section 2.2.2.2
type PropertyID byte

// Property identifiers as defined in MQTT 5.0 Table 2-4
const (
	PropPayloadFormat          PropertyID = 0x01 // Byte - PUBLISH, Will Properties
	PropMessageExpiry          PropertyID = 0x02 // Four Byte Integer - PUBLISH, Will Properties
	PropContentType            PropertyID = 0x03 // UTF-8 String - PUBLISH, Will Properties
	PropResponseTopic          PropertyID = 0x08 // UTF-8 String - PUBLISH, Will Properties
	PropCorrelationData        PropertyID = 0x09 // Binary Data - PUBLISH, Will Properties
	PropSubscriptionID         PropertyID = 0x0B // Variable Byte Integer - PUBLISH, SUBSCRIBE
	PropSessionExpiry          PropertyID = 0x11 // Four Byte Integer - CONNECT, CONNACK, DISCONNECT
	PropAssignedClientID       PropertyID = 0x12 // UTF-8 String - CONNACK
	PropServerKeepAlive        PropertyID = 0x13 // Two Byte Integer - CONNACK
	PropAuthMethod             PropertyID = 0x15 // UTF-8 String - CONNECT, CONNACK, AUTH
	PropAuthData               PropertyID = 0x16 // Binary Data - CONNECT, CONNACK, AUTH
	PropRequestProblemInfo     PropertyID = 0x17 // Byte - CONNECT
	PropWillDelayInterval      PropertyID = 0x18 // Four Byte Integer - Will Properties
	PropRequestResponseInfo    PropertyID = 0x19 // Byte - CONNECT
	PropResponseInfo           PropertyID = 0x1A // UTF-8 String - CONNACK
	PropServerReference        PropertyID = 0x1C // UTF-8 String - CONNACK, DISCONNECT
	PropReasonString           PropertyID = 0x1F // UTF-8 String - All except CONNECT
	PropReceiveMax             PropertyID = 0x21 // Two Byte Integer - CONNECT, CONNACK
	PropTopicAliasMax          PropertyID = 0x22 // Two Byte Integer - CONNECT, CONNACK
	PropTopicAlias             PropertyID = 0x23 // Two Byte Integer - PUBLISH
	PropMaxQoS                 PropertyID = 0x24 // Byte - CONNACK
	PropRetainAvailable        PropertyID = 0x25 // Byte - CONNACK
	PropUserProperty           PropertyID = 0x26 // UTF-8 String Pair - All packets
	PropMaxPacketSize          PropertyID = 0x27 // Four Byte Integer - CONNECT, CONNACK
	PropWildcardSubAvailable   PropertyID = 0x28 // Byte - CONNACK
	PropSubIDAvailable         PropertyID = 0x29 // Byte - CONNACK
	PropSharedSubAvailable     PropertyID = 0x2A // Byte - CONNACK
)

// PropertyType represents the data type of a property.
type PropertyType byte

const (
	PropertyTypeByte       PropertyType = iota // Single byte
	PropertyTypeTwoByteInt                     // Two byte integer
	PropertyTypeFourByteInt                    // Four byte integer
	PropertyTypeVarInt                         // Variable byte integer
	PropertyTypeString                         // UTF-8 encoded string
	PropertyTypeBinary                         // Binary data
	PropertyTypeStringPair                     // UTF-8 string pair
)

// Type returns the data type for a property identifier.
func (p PropertyID) Type() PropertyType {
	switch p {
	case PropPayloadFormat, PropRequestProblemInfo, PropRequestResponseInfo,
		PropMaxQoS, PropRetainAvailable, PropWildcardSubAvailable,
		PropSubIDAvailable, PropSharedSubAvailable:
		return PropertyTypeByte

	case PropServerKeepAlive, PropReceiveMax, PropTopicAliasMax, PropTopicAlias:
		return PropertyTypeTwoByteInt

	case PropMessageExpiry, PropSessionExpiry, PropWillDelayInterval, PropMaxPacketSize:
		return PropertyTypeFourByteInt

	case PropSubscriptionID:
		return PropertyTypeVarInt

	case PropContentType, PropResponseTopic, PropAssignedClientID, PropAuthMethod,
		PropResponseInfo, PropServerReference, PropReasonString:
		return PropertyTypeString

	case PropCorrelationData, PropAuthData:
		return PropertyTypeBinary

	case PropUserProperty:
		return PropertyTypeStringPair

	default:
		return PropertyTypeByte // Unknown properties treated as byte
	}
}

// String returns the name of the property.
func (p PropertyID) String() string {
	switch p {
	case PropPayloadFormat:
		return "Payload Format Indicator"
	case PropMessageExpiry:
		return "Message Expiry Interval"
	case PropContentType:
		return "Content Type"
	case PropResponseTopic:
		return "Response Topic"
	case PropCorrelationData:
		return "Correlation Data"
	case PropSubscriptionID:
		return "Subscription Identifier"
	case PropSessionExpiry:
		return "Session Expiry Interval"
	case PropAssignedClientID:
		return "Assigned Client Identifier"
	case PropServerKeepAlive:
		return "Server Keep Alive"
	case PropAuthMethod:
		return "Authentication Method"
	case PropAuthData:
		return "Authentication Data"
	case PropRequestProblemInfo:
		return "Request Problem Information"
	case PropWillDelayInterval:
		return "Will Delay Interval"
	case PropRequestResponseInfo:
		return "Request Response Information"
	case PropResponseInfo:
		return "Response Information"
	case PropServerReference:
		return "Server Reference"
	case PropReasonString:
		return "Reason String"
	case PropReceiveMax:
		return "Receive Maximum"
	case PropTopicAliasMax:
		return "Topic Alias Maximum"
	case PropTopicAlias:
		return "Topic Alias"
	case PropMaxQoS:
		return "Maximum QoS"
	case PropRetainAvailable:
		return "Retain Available"
	case PropUserProperty:
		return "User Property"
	case PropMaxPacketSize:
		return "Maximum Packet Size"
	case PropWildcardSubAvailable:
		return "Wildcard Subscription Available"
	case PropSubIDAvailable:
		return "Subscription Identifier Available"
	case PropSharedSubAvailable:
		return "Shared Subscription Available"
	default:
		return "Unknown Property"
	}
}

// StringPair represents a key-value pair of UTF-8 strings.
type StringPair struct {
	Key   string
	Value string
}

// Properties holds MQTT 5.0 properties for a packet.
// Properties can appear multiple times for User Property and Subscription Identifier.
type Properties struct {
	// Single-value properties
	PayloadFormat       *byte   // 0x01
	MessageExpiry       *uint32 // 0x02
	ContentType         string  // 0x03
	ResponseTopic       string  // 0x08
	CorrelationData     []byte  // 0x09
	SessionExpiry       *uint32 // 0x11
	AssignedClientID    string  // 0x12
	ServerKeepAlive     *uint16 // 0x13
	AuthMethod          string  // 0x15
	AuthData            []byte  // 0x16
	RequestProblemInfo  *byte   // 0x17
	WillDelayInterval   *uint32 // 0x18
	RequestResponseInfo *byte   // 0x19
	ResponseInfo        string  // 0x1A
	ServerReference     string  // 0x1C
	ReasonString        string  // 0x1F
	ReceiveMax          *uint16 // 0x21
	TopicAliasMax       *uint16 // 0x22
	TopicAlias          *uint16 // 0x23
	MaxQoS              *byte   // 0x24
	RetainAvailable     *byte   // 0x25
	MaxPacketSize       *uint32 // 0x27
	WildcardSubAvail    *byte   // 0x28
	SubIDAvail          *byte   // 0x29
	SharedSubAvail      *byte   // 0x2A

	// Multi-value properties (can appear multiple times)
	SubscriptionIDs []uint32     // 0x0B - Variable byte integer, can appear multiple times in PUBLISH
	UserProperties  []StringPair // 0x26 - Can appear multiple times
}

// EncodedSize returns the total size of encoded properties (including length prefix).
func (p *Properties) EncodedSize() int {
	if p == nil {
		return 1 // Just the zero-length varint
	}

	size := 0

	// Calculate size of each property
	if p.PayloadFormat != nil {
		size += 1 + 1 // ID + byte
	}
	if p.MessageExpiry != nil {
		size += 1 + 4 // ID + uint32
	}
	if p.ContentType != "" {
		size += 1 + 2 + len(p.ContentType) // ID + len + string
	}
	if p.ResponseTopic != "" {
		size += 1 + 2 + len(p.ResponseTopic)
	}
	if len(p.CorrelationData) > 0 {
		size += 1 + 2 + len(p.CorrelationData)
	}
	if p.SessionExpiry != nil {
		size += 1 + 4
	}
	if p.AssignedClientID != "" {
		size += 1 + 2 + len(p.AssignedClientID)
	}
	if p.ServerKeepAlive != nil {
		size += 1 + 2
	}
	if p.AuthMethod != "" {
		size += 1 + 2 + len(p.AuthMethod)
	}
	if len(p.AuthData) > 0 {
		size += 1 + 2 + len(p.AuthData)
	}
	if p.RequestProblemInfo != nil {
		size += 1 + 1
	}
	if p.WillDelayInterval != nil {
		size += 1 + 4
	}
	if p.RequestResponseInfo != nil {
		size += 1 + 1
	}
	if p.ResponseInfo != "" {
		size += 1 + 2 + len(p.ResponseInfo)
	}
	if p.ServerReference != "" {
		size += 1 + 2 + len(p.ServerReference)
	}
	if p.ReasonString != "" {
		size += 1 + 2 + len(p.ReasonString)
	}
	if p.ReceiveMax != nil {
		size += 1 + 2
	}
	if p.TopicAliasMax != nil {
		size += 1 + 2
	}
	if p.TopicAlias != nil {
		size += 1 + 2
	}
	if p.MaxQoS != nil {
		size += 1 + 1
	}
	if p.RetainAvailable != nil {
		size += 1 + 1
	}
	if p.MaxPacketSize != nil {
		size += 1 + 4
	}
	if p.WildcardSubAvail != nil {
		size += 1 + 1
	}
	if p.SubIDAvail != nil {
		size += 1 + 1
	}
	if p.SharedSubAvail != nil {
		size += 1 + 1
	}

	// Multi-value properties
	for _, subID := range p.SubscriptionIDs {
		size += 1 + VarIntSize(subID)
	}
	for _, up := range p.UserProperties {
		size += 1 + 2 + len(up.Key) + 2 + len(up.Value)
	}

	// Property length prefix (variable byte integer)
	return VarIntSize(uint32(size)) + size
}

// Encode encodes the properties into buf.
// Returns the number of bytes written, or 0 on error.
func (p *Properties) Encode(buf []byte) int {
	if p == nil {
		if len(buf) < 1 {
			return 0
		}
		buf[0] = 0 // Zero-length properties
		return 1
	}

	// Calculate content size first
	contentSize := p.EncodedSize()
	propLenSize := VarIntSize(uint32(contentSize - VarIntSize(uint32(contentSize-1))))

	// Recalculate actual content size (excluding the length prefix)
	actualContentSize := contentSize - propLenSize
	if len(buf) < contentSize {
		return 0
	}

	// Encode property length
	n := EncodeVarInt(buf, uint32(actualContentSize))
	pos := n

	// Helper to encode a byte property
	encodeByte := func(id PropertyID, v byte) {
		buf[pos] = byte(id)
		buf[pos+1] = v
		pos += 2
	}

	// Helper to encode a uint16 property
	encodeUint16 := func(id PropertyID, v uint16) {
		buf[pos] = byte(id)
		EncodeUint16(buf[pos+1:], v)
		pos += 3
	}

	// Helper to encode a uint32 property
	encodeUint32 := func(id PropertyID, v uint32) {
		buf[pos] = byte(id)
		EncodeUint32(buf[pos+1:], v)
		pos += 5
	}

	// Helper to encode a string property
	encodeString := func(id PropertyID, s string) {
		buf[pos] = byte(id)
		pos += 1
		pos += EncodeString(buf[pos:], s)
	}

	// Helper to encode binary data property
	encodeBinary := func(id PropertyID, data []byte) {
		buf[pos] = byte(id)
		pos += 1
		pos += EncodeBytes(buf[pos:], data)
	}

	// Encode each property
	if p.PayloadFormat != nil {
		encodeByte(PropPayloadFormat, *p.PayloadFormat)
	}
	if p.MessageExpiry != nil {
		encodeUint32(PropMessageExpiry, *p.MessageExpiry)
	}
	if p.ContentType != "" {
		encodeString(PropContentType, p.ContentType)
	}
	if p.ResponseTopic != "" {
		encodeString(PropResponseTopic, p.ResponseTopic)
	}
	if len(p.CorrelationData) > 0 {
		encodeBinary(PropCorrelationData, p.CorrelationData)
	}
	for _, subID := range p.SubscriptionIDs {
		buf[pos] = byte(PropSubscriptionID)
		pos += 1
		pos += EncodeVarInt(buf[pos:], subID)
	}
	if p.SessionExpiry != nil {
		encodeUint32(PropSessionExpiry, *p.SessionExpiry)
	}
	if p.AssignedClientID != "" {
		encodeString(PropAssignedClientID, p.AssignedClientID)
	}
	if p.ServerKeepAlive != nil {
		encodeUint16(PropServerKeepAlive, *p.ServerKeepAlive)
	}
	if p.AuthMethod != "" {
		encodeString(PropAuthMethod, p.AuthMethod)
	}
	if len(p.AuthData) > 0 {
		encodeBinary(PropAuthData, p.AuthData)
	}
	if p.RequestProblemInfo != nil {
		encodeByte(PropRequestProblemInfo, *p.RequestProblemInfo)
	}
	if p.WillDelayInterval != nil {
		encodeUint32(PropWillDelayInterval, *p.WillDelayInterval)
	}
	if p.RequestResponseInfo != nil {
		encodeByte(PropRequestResponseInfo, *p.RequestResponseInfo)
	}
	if p.ResponseInfo != "" {
		encodeString(PropResponseInfo, p.ResponseInfo)
	}
	if p.ServerReference != "" {
		encodeString(PropServerReference, p.ServerReference)
	}
	if p.ReasonString != "" {
		encodeString(PropReasonString, p.ReasonString)
	}
	if p.ReceiveMax != nil {
		encodeUint16(PropReceiveMax, *p.ReceiveMax)
	}
	if p.TopicAliasMax != nil {
		encodeUint16(PropTopicAliasMax, *p.TopicAliasMax)
	}
	if p.TopicAlias != nil {
		encodeUint16(PropTopicAlias, *p.TopicAlias)
	}
	if p.MaxQoS != nil {
		encodeByte(PropMaxQoS, *p.MaxQoS)
	}
	if p.RetainAvailable != nil {
		encodeByte(PropRetainAvailable, *p.RetainAvailable)
	}
	for _, up := range p.UserProperties {
		buf[pos] = byte(PropUserProperty)
		pos += 1
		pos += EncodeString(buf[pos:], up.Key)
		pos += EncodeString(buf[pos:], up.Value)
	}
	if p.MaxPacketSize != nil {
		encodeUint32(PropMaxPacketSize, *p.MaxPacketSize)
	}
	if p.WildcardSubAvail != nil {
		encodeByte(PropWildcardSubAvailable, *p.WildcardSubAvail)
	}
	if p.SubIDAvail != nil {
		encodeByte(PropSubIDAvailable, *p.SubIDAvail)
	}
	if p.SharedSubAvail != nil {
		encodeByte(PropSharedSubAvailable, *p.SharedSubAvail)
	}

	return pos
}

// DecodeProperties decodes properties from buf.
// Returns the properties, bytes consumed, and any error.
func DecodeProperties(buf []byte) (*Properties, int, error) {
	if len(buf) < 1 {
		return nil, 0, ErrIncompletePacket
	}

	// Decode property length
	propLen, n, ok := DecodeVarInt(buf)
	if !ok {
		return nil, 0, ErrMalformedRemainingLength
	}

	if uint32(len(buf)-n) < propLen {
		return nil, 0, ErrIncompletePacket
	}

	if propLen == 0 {
		return nil, n, nil
	}

	props := &Properties{}
	pos := n
	end := n + int(propLen)

	for pos < end {
		if pos >= len(buf) {
			return nil, 0, ErrIncompletePacket
		}

		propID := PropertyID(buf[pos])
		pos++

		switch propID {
		case PropPayloadFormat:
			if pos >= end {
				return nil, 0, ErrIncompletePacket
			}
			v := buf[pos]
			props.PayloadFormat = &v
			pos++

		case PropMessageExpiry:
			v, consumed, ok := DecodeUint32(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.MessageExpiry = &v
			pos += consumed

		case PropContentType:
			s, consumed, ok := DecodeStringCopy(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.ContentType = s
			pos += consumed

		case PropResponseTopic:
			s, consumed, ok := DecodeStringCopy(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.ResponseTopic = s
			pos += consumed

		case PropCorrelationData:
			data, consumed, ok := DecodeString(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.CorrelationData = make([]byte, len(data))
			copy(props.CorrelationData, data)
			pos += consumed

		case PropSubscriptionID:
			v, consumed, ok := DecodeVarInt(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.SubscriptionIDs = append(props.SubscriptionIDs, v)
			pos += consumed

		case PropSessionExpiry:
			v, consumed, ok := DecodeUint32(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.SessionExpiry = &v
			pos += consumed

		case PropAssignedClientID:
			s, consumed, ok := DecodeStringCopy(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.AssignedClientID = s
			pos += consumed

		case PropServerKeepAlive:
			v, consumed, ok := DecodeUint16(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.ServerKeepAlive = &v
			pos += consumed

		case PropAuthMethod:
			s, consumed, ok := DecodeStringCopy(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.AuthMethod = s
			pos += consumed

		case PropAuthData:
			data, consumed, ok := DecodeString(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.AuthData = make([]byte, len(data))
			copy(props.AuthData, data)
			pos += consumed

		case PropRequestProblemInfo:
			if pos >= end {
				return nil, 0, ErrIncompletePacket
			}
			v := buf[pos]
			props.RequestProblemInfo = &v
			pos++

		case PropWillDelayInterval:
			v, consumed, ok := DecodeUint32(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.WillDelayInterval = &v
			pos += consumed

		case PropRequestResponseInfo:
			if pos >= end {
				return nil, 0, ErrIncompletePacket
			}
			v := buf[pos]
			props.RequestResponseInfo = &v
			pos++

		case PropResponseInfo:
			s, consumed, ok := DecodeStringCopy(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.ResponseInfo = s
			pos += consumed

		case PropServerReference:
			s, consumed, ok := DecodeStringCopy(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.ServerReference = s
			pos += consumed

		case PropReasonString:
			s, consumed, ok := DecodeStringCopy(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.ReasonString = s
			pos += consumed

		case PropReceiveMax:
			v, consumed, ok := DecodeUint16(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.ReceiveMax = &v
			pos += consumed

		case PropTopicAliasMax:
			v, consumed, ok := DecodeUint16(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.TopicAliasMax = &v
			pos += consumed

		case PropTopicAlias:
			v, consumed, ok := DecodeUint16(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.TopicAlias = &v
			pos += consumed

		case PropMaxQoS:
			if pos >= end {
				return nil, 0, ErrIncompletePacket
			}
			v := buf[pos]
			props.MaxQoS = &v
			pos++

		case PropRetainAvailable:
			if pos >= end {
				return nil, 0, ErrIncompletePacket
			}
			v := buf[pos]
			props.RetainAvailable = &v
			pos++

		case PropUserProperty:
			key, consumed, ok := DecodeStringCopy(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			pos += consumed
			val, consumed, ok := DecodeStringCopy(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			pos += consumed
			props.UserProperties = append(props.UserProperties, StringPair{Key: key, Value: val})

		case PropMaxPacketSize:
			v, consumed, ok := DecodeUint32(buf[pos:])
			if !ok {
				return nil, 0, ErrIncompletePacket
			}
			props.MaxPacketSize = &v
			pos += consumed

		case PropWildcardSubAvailable:
			if pos >= end {
				return nil, 0, ErrIncompletePacket
			}
			v := buf[pos]
			props.WildcardSubAvail = &v
			pos++

		case PropSubIDAvailable:
			if pos >= end {
				return nil, 0, ErrIncompletePacket
			}
			v := buf[pos]
			props.SubIDAvail = &v
			pos++

		case PropSharedSubAvailable:
			if pos >= end {
				return nil, 0, ErrIncompletePacket
			}
			v := buf[pos]
			props.SharedSubAvail = &v
			pos++

		default:
			return nil, 0, ErrInvalidPropertyID
		}
	}

	return props, end, nil
}
