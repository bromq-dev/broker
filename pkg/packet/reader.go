package packet

import (
	"io"
	"sync"
)

// Reader reads MQTT packets from an io.Reader.
// It manages buffer pooling for efficient memory usage.
type Reader struct {
	r       io.Reader
	buf     []byte
	pos     int
	end     int
	version Version // Protocol version for decoding
}

// NewReader creates a new packet reader.
func NewReader(r io.Reader, bufSize int) *Reader {
	if bufSize < 1024 {
		bufSize = 1024
	}
	return &Reader{
		r:       r,
		buf:     make([]byte, bufSize),
		version: Version311, // Default, updated after CONNECT
	}
}

// SetVersion sets the protocol version for packet decoding.
func (r *Reader) SetVersion(v Version) {
	r.version = v
}

// Version returns the current protocol version.
func (r *Reader) Version() Version {
	return r.version
}

// fill reads more data into the buffer.
func (r *Reader) fill() error {
	// Shift remaining data to the beginning
	if r.pos > 0 {
		copy(r.buf, r.buf[r.pos:r.end])
		r.end -= r.pos
		r.pos = 0
	}

	// Grow buffer if needed
	if r.end == len(r.buf) {
		newBuf := make([]byte, len(r.buf)*2)
		copy(newBuf, r.buf)
		r.buf = newBuf
	}

	n, err := r.r.Read(r.buf[r.end:])
	if n > 0 {
		r.end += n
	}
	return err
}

// available returns the number of unread bytes in the buffer.
func (r *Reader) available() int {
	return r.end - r.pos
}

// ReadPacket reads the next packet from the reader.
// Returns the packet and any error encountered.
func (r *Reader) ReadPacket() (Packet, error) {
	// Read until we have at least 2 bytes for the fixed header
	for r.available() < 2 {
		if err := r.fill(); err != nil {
			return nil, err
		}
	}

	// Parse fixed header
	packetType, flags, remainingLength, headerLen, ok := DecodeFixedHeader(r.buf[r.pos:r.end])
	if !ok {
		// Need more data for remaining length
		for !ok && r.available() < 5 {
			if err := r.fill(); err != nil {
				return nil, err
			}
			packetType, flags, remainingLength, headerLen, ok = DecodeFixedHeader(r.buf[r.pos:r.end])
		}
		if !ok {
			return nil, ErrMalformedRemainingLength
		}
	}

	// Check packet size
	if remainingLength > MaxRemainingLength {
		return nil, ErrPacketTooLarge
	}

	totalLen := headerLen + int(remainingLength)

	// Read until we have the complete packet
	for r.available() < totalLen {
		if err := r.fill(); err != nil {
			return nil, err
		}
	}

	// Extract packet data (after fixed header)
	data := r.buf[r.pos+headerLen : r.pos+totalLen]
	r.pos += totalLen

	// Decode the packet based on type
	return r.decodePacket(packetType, flags, data)
}

// decodePacket decodes a packet given its type, flags, and payload data.
func (r *Reader) decodePacket(packetType Type, flags byte, data []byte) (Packet, error) {
	// Validate reserved flags for packet types that require specific values
	switch packetType {
	case TypeConnect, TypeConnack, TypePuback, TypePubrec, TypePubcomp,
		TypeSuback, TypeUnsuback, TypePingreq, TypePingresp, TypeDisconnect, TypeAuth:
		if flags != 0 {
			return nil, ErrInvalidFlags
		}
	case TypePubrel, TypeSubscribe, TypeUnsubscribe:
		if flags != 0x02 {
			return nil, ErrInvalidFlags
		}
	case TypePublish:
		// PUBLISH flags are validated in DecodePublish
	default:
		return nil, ErrInvalidPacketType
	}

	switch packetType {
	case TypeConnect:
		pkt, err := DecodeConnect(data)
		if err != nil {
			return nil, err
		}
		// Update version for subsequent packets
		r.version = pkt.ProtocolVersion
		return pkt, nil

	case TypeConnack:
		return DecodeConnack(data, r.version)

	case TypePublish:
		return DecodePublish(flags, data, r.version)

	case TypePuback:
		return DecodePuback(data, r.version)

	case TypePubrec:
		return DecodePubrec(data, r.version)

	case TypePubrel:
		return DecodePubrel(data, r.version)

	case TypePubcomp:
		return DecodePubcomp(data, r.version)

	case TypeSubscribe:
		return DecodeSubscribe(data, r.version)

	case TypeSuback:
		return DecodeSuback(data, r.version)

	case TypeUnsubscribe:
		return DecodeUnsubscribe(data, r.version)

	case TypeUnsuback:
		return DecodeUnsuback(data, r.version)

	case TypePingreq:
		return DecodePingreq(data)

	case TypePingresp:
		return DecodePingresp(data)

	case TypeDisconnect:
		return DecodeDisconnect(data, r.version)

	case TypeAuth:
		if r.version != Version5 {
			return nil, ErrInvalidPacketType
		}
		return DecodeAuth(data)

	default:
		return nil, ErrInvalidPacketType
	}
}

// BufferPool provides a pool of reusable buffers for packet encoding.
var BufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 4096)
		return &buf
	},
}

// GetBuffer returns a buffer from the pool.
func GetBuffer() []byte {
	return *BufferPool.Get().(*[]byte)
}

// PutBuffer returns a buffer to the pool.
func PutBuffer(buf []byte) {
	// Only return buffers of reasonable size
	if cap(buf) <= 65536 {
		buf = buf[:cap(buf)]
		BufferPool.Put(&buf)
	}
}
