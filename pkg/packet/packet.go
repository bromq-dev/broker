package packet

// Packet is the interface implemented by all MQTT control packets.
type Packet interface {
	// Type returns the packet type.
	Type() Type

	// Encode encodes the packet into buf.
	// Returns the number of bytes written, or 0 on error.
	Encode(buf []byte) int

	// EncodedSize returns the total size of the encoded packet.
	EncodedSize() int
}

// Will represents an MQTT Will Message configuration.
type Will struct {
	Topic      string
	Payload    []byte
	QoS        QoS
	Retain     bool
	Properties *Properties // MQTT 5.0 only
}
