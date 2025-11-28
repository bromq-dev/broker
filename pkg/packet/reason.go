package packet

// ReasonCode represents an MQTT 5.0 reason code.
// Reason codes less than 0x80 indicate success, 0x80 or greater indicate failure.
// MQTT 5.0 Section 2.4
type ReasonCode byte

// Success reason codes (< 0x80)
const (
	ReasonSuccess              ReasonCode = 0x00 // Success / Normal disconnection / Granted QoS 0
	ReasonGrantedQoS1          ReasonCode = 0x01 // Granted QoS 1
	ReasonGrantedQoS2          ReasonCode = 0x02 // Granted QoS 2
	ReasonDisconnectWithWill   ReasonCode = 0x04 // Disconnect with Will Message
	ReasonNoMatchingSubscriber ReasonCode = 0x10 // No matching subscribers
	ReasonNoSubscriptionExist  ReasonCode = 0x11 // No subscription existed
	ReasonContinueAuth         ReasonCode = 0x18 // Continue authentication
	ReasonReAuthenticate       ReasonCode = 0x19 // Re-authenticate
)

// Error reason codes (>= 0x80)
const (
	ReasonUnspecifiedError           ReasonCode = 0x80 // Unspecified error
	ReasonMalformedPacket            ReasonCode = 0x81 // Malformed Packet
	ReasonProtocolError              ReasonCode = 0x82 // Protocol Error
	ReasonImplSpecificError          ReasonCode = 0x83 // Implementation specific error
	ReasonUnsupportedProtocolVersion ReasonCode = 0x84 // Unsupported Protocol Version
	ReasonClientIDNotValid           ReasonCode = 0x85 // Client Identifier not valid
	ReasonBadUserNameOrPassword      ReasonCode = 0x86 // Bad User Name or Password
	ReasonNotAuthorized              ReasonCode = 0x87 // Not authorized
	ReasonServerUnavailable          ReasonCode = 0x88 // Server unavailable
	ReasonServerBusy                 ReasonCode = 0x89 // Server busy
	ReasonBanned                     ReasonCode = 0x8A // Banned
	ReasonServerShuttingDown         ReasonCode = 0x8B // Server shutting down
	ReasonBadAuthMethod              ReasonCode = 0x8C // Bad authentication method
	ReasonKeepAliveTimeout           ReasonCode = 0x8D // Keep Alive timeout
	ReasonSessionTakenOver           ReasonCode = 0x8E // Session taken over
	ReasonTopicFilterInvalid         ReasonCode = 0x8F // Topic Filter invalid
	ReasonTopicNameInvalid           ReasonCode = 0x90 // Topic Name invalid
	ReasonPacketIDInUse              ReasonCode = 0x91 // Packet Identifier in use
	ReasonPacketIDNotFound           ReasonCode = 0x92 // Packet Identifier not found
	ReasonReceiveMaxExceeded         ReasonCode = 0x93 // Receive Maximum exceeded
	ReasonTopicAliasInvalid          ReasonCode = 0x94 // Topic Alias invalid
	ReasonPacketTooLarge             ReasonCode = 0x95 // Packet too large
	ReasonMessageRateTooHigh         ReasonCode = 0x96 // Message rate too high
	ReasonQuotaExceeded              ReasonCode = 0x97 // Quota exceeded
	ReasonAdminAction                ReasonCode = 0x98 // Administrative action
	ReasonPayloadFormatInvalid       ReasonCode = 0x99 // Payload format invalid
	ReasonRetainNotSupported         ReasonCode = 0x9A // Retain not supported
	ReasonQoSNotSupported            ReasonCode = 0x9B // QoS not supported
	ReasonUseAnotherServer           ReasonCode = 0x9C // Use another server
	ReasonServerMoved                ReasonCode = 0x9D // Server moved
	ReasonSharedSubsNotSupported     ReasonCode = 0x9E // Shared Subscriptions not supported
	ReasonConnectionRateExceeded     ReasonCode = 0x9F // Connection rate exceeded
	ReasonMaxConnectTime             ReasonCode = 0xA0 // Maximum connect time
	ReasonSubIDsNotSupported         ReasonCode = 0xA1 // Subscription Identifiers not supported
	ReasonWildcardSubsNotSupported   ReasonCode = 0xA2 // Wildcard Subscriptions not supported
)

// IsSuccess returns true if the reason code indicates success.
func (r ReasonCode) IsSuccess() bool {
	return r < 0x80
}

// IsError returns true if the reason code indicates an error.
func (r ReasonCode) IsError() bool {
	return r >= 0x80
}

// String returns the string representation of the reason code.
func (r ReasonCode) String() string {
	switch r {
	case ReasonSuccess:
		return "Success"
	case ReasonGrantedQoS1:
		return "Granted QoS 1"
	case ReasonGrantedQoS2:
		return "Granted QoS 2"
	case ReasonDisconnectWithWill:
		return "Disconnect with Will Message"
	case ReasonNoMatchingSubscriber:
		return "No matching subscribers"
	case ReasonNoSubscriptionExist:
		return "No subscription existed"
	case ReasonContinueAuth:
		return "Continue authentication"
	case ReasonReAuthenticate:
		return "Re-authenticate"
	case ReasonUnspecifiedError:
		return "Unspecified error"
	case ReasonMalformedPacket:
		return "Malformed Packet"
	case ReasonProtocolError:
		return "Protocol Error"
	case ReasonImplSpecificError:
		return "Implementation specific error"
	case ReasonUnsupportedProtocolVersion:
		return "Unsupported Protocol Version"
	case ReasonClientIDNotValid:
		return "Client Identifier not valid"
	case ReasonBadUserNameOrPassword:
		return "Bad User Name or Password"
	case ReasonNotAuthorized:
		return "Not authorized"
	case ReasonServerUnavailable:
		return "Server unavailable"
	case ReasonServerBusy:
		return "Server busy"
	case ReasonBanned:
		return "Banned"
	case ReasonServerShuttingDown:
		return "Server shutting down"
	case ReasonBadAuthMethod:
		return "Bad authentication method"
	case ReasonKeepAliveTimeout:
		return "Keep Alive timeout"
	case ReasonSessionTakenOver:
		return "Session taken over"
	case ReasonTopicFilterInvalid:
		return "Topic Filter invalid"
	case ReasonTopicNameInvalid:
		return "Topic Name invalid"
	case ReasonPacketIDInUse:
		return "Packet Identifier in use"
	case ReasonPacketIDNotFound:
		return "Packet Identifier not found"
	case ReasonReceiveMaxExceeded:
		return "Receive Maximum exceeded"
	case ReasonTopicAliasInvalid:
		return "Topic Alias invalid"
	case ReasonPacketTooLarge:
		return "Packet too large"
	case ReasonMessageRateTooHigh:
		return "Message rate too high"
	case ReasonQuotaExceeded:
		return "Quota exceeded"
	case ReasonAdminAction:
		return "Administrative action"
	case ReasonPayloadFormatInvalid:
		return "Payload format invalid"
	case ReasonRetainNotSupported:
		return "Retain not supported"
	case ReasonQoSNotSupported:
		return "QoS not supported"
	case ReasonUseAnotherServer:
		return "Use another server"
	case ReasonServerMoved:
		return "Server moved"
	case ReasonSharedSubsNotSupported:
		return "Shared Subscriptions not supported"
	case ReasonConnectionRateExceeded:
		return "Connection rate exceeded"
	case ReasonMaxConnectTime:
		return "Maximum connect time"
	case ReasonSubIDsNotSupported:
		return "Subscription Identifiers not supported"
	case ReasonWildcardSubsNotSupported:
		return "Wildcard Subscriptions not supported"
	default:
		return "Unknown reason code"
	}
}

// MQTT 3.1.1 CONNACK return codes (different from v5.0 reason codes)
// MQTT 3.1.1 Section 3.2.2.3
type ConnackReturnCode byte

const (
	ConnackAccepted                    ConnackReturnCode = 0x00 // Connection Accepted
	ConnackUnacceptableProtocolVersion ConnackReturnCode = 0x01 // Connection Refused, unacceptable protocol version
	ConnackIdentifierRejected          ConnackReturnCode = 0x02 // Connection Refused, identifier rejected
	ConnackServerUnavailable           ConnackReturnCode = 0x03 // Connection Refused, Server unavailable
	ConnackBadUsernameOrPassword       ConnackReturnCode = 0x04 // Connection Refused, bad user name or password
	ConnackNotAuthorized               ConnackReturnCode = 0x05 // Connection Refused, not authorized
)

// IsAccepted returns true if the connection was accepted.
func (c ConnackReturnCode) IsAccepted() bool {
	return c == ConnackAccepted
}

// String returns the string representation of the CONNACK return code.
func (c ConnackReturnCode) String() string {
	switch c {
	case ConnackAccepted:
		return "Connection Accepted"
	case ConnackUnacceptableProtocolVersion:
		return "Connection Refused, unacceptable protocol version"
	case ConnackIdentifierRejected:
		return "Connection Refused, identifier rejected"
	case ConnackServerUnavailable:
		return "Connection Refused, Server unavailable"
	case ConnackBadUsernameOrPassword:
		return "Connection Refused, bad user name or password"
	case ConnackNotAuthorized:
		return "Connection Refused, not authorized"
	default:
		return "Unknown return code"
	}
}
