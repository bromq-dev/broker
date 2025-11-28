package packet

import "errors"

// Sentinel errors for packet parsing and encoding.
var (
	// ErrMalformedPacket indicates the packet structure is invalid.
	ErrMalformedPacket = errors.New("malformed packet")

	// ErrMalformedRemainingLength indicates the remaining length encoding is invalid.
	ErrMalformedRemainingLength = errors.New("malformed remaining length")

	// ErrPacketTooLarge indicates the packet exceeds maximum allowed size.
	ErrPacketTooLarge = errors.New("packet too large")

	// ErrInvalidPacketType indicates an unknown or reserved packet type.
	ErrInvalidPacketType = errors.New("invalid packet type")

	// ErrInvalidFlags indicates invalid fixed header flags for the packet type.
	ErrInvalidFlags = errors.New("invalid packet flags")

	// ErrInvalidQoS indicates an invalid QoS level.
	ErrInvalidQoS = errors.New("invalid QoS level")

	// ErrInvalidProtocolName indicates an unrecognized protocol name.
	ErrInvalidProtocolName = errors.New("invalid protocol name")

	// ErrInvalidProtocolVersion indicates an unsupported protocol version.
	ErrInvalidProtocolVersion = errors.New("invalid protocol version")

	// ErrInvalidUTF8 indicates a string contains invalid UTF-8.
	ErrInvalidUTF8 = errors.New("invalid UTF-8 string")

	// ErrInvalidUTF8NullChar indicates a string contains a null character.
	ErrInvalidUTF8NullChar = errors.New("UTF-8 string contains null character")

	// ErrInvalidTopicName indicates an invalid topic name.
	ErrInvalidTopicName = errors.New("invalid topic name")

	// ErrInvalidTopicFilter indicates an invalid topic filter.
	ErrInvalidTopicFilter = errors.New("invalid topic filter")

	// ErrInvalidPacketID indicates an invalid packet identifier (zero or duplicate).
	ErrInvalidPacketID = errors.New("invalid packet identifier")

	// ErrShortBuffer indicates insufficient buffer space for encoding.
	ErrShortBuffer = errors.New("buffer too short")

	// ErrIncompletePacket indicates more data is needed to complete the packet.
	ErrIncompletePacket = errors.New("incomplete packet")

	// ErrInvalidPropertyID indicates an unknown or invalid property identifier (MQTT 5.0).
	ErrInvalidPropertyID = errors.New("invalid property identifier")

	// ErrDuplicateProperty indicates a property that must be unique appears multiple times.
	ErrDuplicateProperty = errors.New("duplicate property")

	// ErrInvalidReasonCode indicates an unknown or invalid reason code (MQTT 5.0).
	ErrInvalidReasonCode = errors.New("invalid reason code")
)
