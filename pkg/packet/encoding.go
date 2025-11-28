package packet

import (
	"encoding/binary"
	"unicode/utf8"
)

// EncodeVarInt encodes a variable byte integer into buf and returns the number of bytes written.
// Returns 0 if the value is too large or the buffer is too small.
// MQTT 5.0 Section 1.5.5, MQTT 3.1.1 Section 2.2.3
func EncodeVarInt(buf []byte, value uint32) int {
	if value > MaxRemainingLength {
		return 0
	}

	i := 0
	for {
		if i >= len(buf) {
			return 0
		}
		encodedByte := byte(value & 0x7F)
		value >>= 7
		if value > 0 {
			encodedByte |= 0x80
		}
		buf[i] = encodedByte
		i++
		if value == 0 {
			break
		}
	}
	return i
}

// DecodeVarInt decodes a variable byte integer from buf.
// Returns the value, number of bytes consumed, and success flag.
// Returns (0, 0, false) on error or incomplete data.
// MQTT 5.0 Section 1.5.5, MQTT 3.1.1 Section 2.2.3
func DecodeVarInt(buf []byte) (value uint32, n int, ok bool) {
	var multiplier uint32 = 1

	for i := 0; i < len(buf) && i < 4; i++ {
		encodedByte := buf[i]
		value += uint32(encodedByte&0x7F) * multiplier

		if encodedByte&0x80 == 0 {
			return value, i + 1, true
		}

		multiplier *= 128
		if multiplier > 128*128*128 {
			return 0, 0, false // Malformed: more than 4 bytes
		}
	}

	return 0, 0, false // Incomplete
}

// VarIntSize returns the number of bytes needed to encode a value as a variable byte integer.
func VarIntSize(value uint32) int {
	switch {
	case value < 128:
		return 1
	case value < 16384:
		return 2
	case value < 2097152:
		return 3
	default:
		return 4
	}
}

// EncodeUint16 encodes a 16-bit unsigned integer in big-endian order.
// Returns 2 on success, 0 if buffer is too small.
func EncodeUint16(buf []byte, value uint16) int {
	if len(buf) < 2 {
		return 0
	}
	binary.BigEndian.PutUint16(buf, value)
	return 2
}

// DecodeUint16 decodes a 16-bit unsigned integer from big-endian bytes.
// Returns the value, 2 bytes consumed, and success flag.
func DecodeUint16(buf []byte) (value uint16, n int, ok bool) {
	if len(buf) < 2 {
		return 0, 0, false
	}
	return binary.BigEndian.Uint16(buf), 2, true
}

// EncodeUint32 encodes a 32-bit unsigned integer in big-endian order.
// Returns 4 on success, 0 if buffer is too small.
func EncodeUint32(buf []byte, value uint32) int {
	if len(buf) < 4 {
		return 0
	}
	binary.BigEndian.PutUint32(buf, value)
	return 4
}

// DecodeUint32 decodes a 32-bit unsigned integer from big-endian bytes.
// Returns the value, 4 bytes consumed, and success flag.
func DecodeUint32(buf []byte) (value uint32, n int, ok bool) {
	if len(buf) < 4 {
		return 0, 0, false
	}
	return binary.BigEndian.Uint32(buf), 4, true
}

// EncodeString encodes a UTF-8 string with a 2-byte length prefix.
// Returns the number of bytes written, or 0 on error.
// MQTT 5.0 Section 1.5.4, MQTT 3.1.1 Section 1.5.3
func EncodeString(buf []byte, s string) int {
	slen := len(s)
	if slen > 65535 {
		return 0
	}
	if len(buf) < 2+slen {
		return 0
	}
	binary.BigEndian.PutUint16(buf, uint16(slen))
	copy(buf[2:], s)
	return 2 + slen
}

// EncodeBytes encodes binary data with a 2-byte length prefix.
// Returns the number of bytes written, or 0 on error.
// MQTT 5.0 Section 1.5.6
func EncodeBytes(buf []byte, data []byte) int {
	dlen := len(data)
	if dlen > 65535 {
		return 0
	}
	if len(buf) < 2+dlen {
		return 0
	}
	binary.BigEndian.PutUint16(buf, uint16(dlen))
	copy(buf[2:], data)
	return 2 + dlen
}

// DecodeString decodes a length-prefixed UTF-8 string from buf.
// Returns a slice referencing the original buffer (zero-copy), bytes consumed, and success flag.
// The caller should copy the string if needed beyond the buffer's lifetime.
func DecodeString(buf []byte) (s []byte, n int, ok bool) {
	if len(buf) < 2 {
		return nil, 0, false
	}
	slen := int(binary.BigEndian.Uint16(buf))
	if len(buf) < 2+slen {
		return nil, 0, false
	}
	return buf[2 : 2+slen], 2 + slen, true
}

// DecodeStringCopy decodes a length-prefixed UTF-8 string from buf.
// Returns a copied string, bytes consumed, and success flag.
func DecodeStringCopy(buf []byte) (s string, n int, ok bool) {
	data, n, ok := DecodeString(buf)
	if !ok {
		return "", 0, false
	}
	return string(data), n, true
}

// ValidateUTF8String validates that a byte slice is valid UTF-8 without null characters.
// Returns nil on success or an appropriate error.
// MQTT 5.0 Section 1.5.4, MQTT 3.1.1 Section 1.5.3
func ValidateUTF8String(data []byte) error {
	if !utf8.Valid(data) {
		return ErrInvalidUTF8
	}

	// Check for null characters and surrogate pairs (U+D800 to U+DFFF)
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r == 0 {
			return ErrInvalidUTF8NullChar
		}
		// Surrogate pairs are invalid in UTF-8 (but utf8.Valid already handles this)
		if r >= 0xD800 && r <= 0xDFFF {
			return ErrInvalidUTF8
		}
		i += size
	}

	return nil
}

// FixedHeaderSize calculates the size of the fixed header for a given remaining length.
func FixedHeaderSize(remainingLength uint32) int {
	return 1 + VarIntSize(remainingLength)
}

// EncodeFixedHeader encodes the fixed header into buf.
// Returns the number of bytes written, or 0 on error.
func EncodeFixedHeader(buf []byte, packetType Type, flags byte, remainingLength uint32) int {
	if len(buf) < 1 {
		return 0
	}
	buf[0] = byte(packetType)<<4 | (flags & 0x0F)
	n := EncodeVarInt(buf[1:], remainingLength)
	if n == 0 {
		return 0
	}
	return 1 + n
}

// DecodeFixedHeader decodes the fixed header from buf.
// Returns packet type, flags, remaining length, bytes consumed, and success flag.
func DecodeFixedHeader(buf []byte) (packetType Type, flags byte, remainingLength uint32, n int, ok bool) {
	if len(buf) < 2 {
		return 0, 0, 0, 0, false
	}

	packetType = Type(buf[0] >> 4)
	flags = buf[0] & 0x0F

	remainingLength, varIntLen, ok := DecodeVarInt(buf[1:])
	if !ok {
		return 0, 0, 0, 0, false
	}

	return packetType, flags, remainingLength, 1 + varIntLen, true
}
