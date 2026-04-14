// Package nas implements NAS (Non-Access Stratum) message encoding/decoding
// for LTE per 3GPP TS 24.301.
//
// NAS messages use TLV (Type-Length-Value) encoding, which is straightforward
// compared to S1AP's ASN.1 PER. Each message has a fixed header followed by
// mandatory IEs at fixed positions, then optional IEs in TLV format.
package nas

import "fmt"

// SecurityHeaderType (TS 24.301 Section 9.3.1)
type SecurityHeaderType uint8

const (
	SecurityHeaderPlainNAS                  SecurityHeaderType = 0
	SecurityHeaderIntegrityProtected        SecurityHeaderType = 1
	SecurityHeaderIntegrityProtectedCiphered SecurityHeaderType = 2
	SecurityHeaderIntegrityProtectedNewCtx  SecurityHeaderType = 3
	SecurityHeaderIntegrityProtectedNewCtxCiphered SecurityHeaderType = 4
)

// ProtocolDiscriminator (TS 24.007)
type ProtocolDiscriminator uint8

const (
	EPSMobilityManagement ProtocolDiscriminator = 0x07 // EMM
	EPSSessionManagement  ProtocolDiscriminator = 0x02 // ESM
)

// MessageType identifies the NAS message (TS 24.301 Section 8.1)
type MessageType uint8

// EMM message types (TS 24.301 Table 8.1)
const (
	MsgTypeAttachRequest          MessageType = 0x41
	MsgTypeAttachAccept           MessageType = 0x42
	MsgTypeAttachComplete         MessageType = 0x43
	MsgTypeAttachReject           MessageType = 0x44
	MsgTypeDetachRequest          MessageType = 0x45
	MsgTypeDetachAccept           MessageType = 0x46
	MsgTypeAuthenticationRequest  MessageType = 0x52
	MsgTypeAuthenticationResponse MessageType = 0x53
	MsgTypeAuthenticationReject   MessageType = 0x54
	MsgTypeAuthenticationFailure  MessageType = 0x5C
	MsgTypeIdentityRequest        MessageType = 0x55
	MsgTypeIdentityResponse       MessageType = 0x56
	MsgTypeSecurityModeCommand    MessageType = 0x5D
	MsgTypeSecurityModeComplete   MessageType = 0x5E
	MsgTypeSecurityModeReject     MessageType = 0x5F
	MsgTypeEMMStatus              MessageType = 0x60
	MsgTypeServiceRequest         MessageType = 0x4C
)

func (m MessageType) String() string {
	switch m {
	case MsgTypeAttachRequest:
		return "AttachRequest"
	case MsgTypeAttachAccept:
		return "AttachAccept"
	case MsgTypeAttachComplete:
		return "AttachComplete"
	case MsgTypeAuthenticationRequest:
		return "AuthenticationRequest"
	case MsgTypeAuthenticationResponse:
		return "AuthenticationResponse"
	case MsgTypeSecurityModeCommand:
		return "SecurityModeCommand"
	case MsgTypeSecurityModeComplete:
		return "SecurityModeComplete"
	case MsgTypeIdentityRequest:
		return "IdentityRequest"
	case MsgTypeIdentityResponse:
		return "IdentityResponse"
	default:
		return fmt.Sprintf("MessageType(0x%02x)", uint8(m))
	}
}

// Header is the common NAS message header.
type Header struct {
	SecurityHeader SecurityHeaderType
	Protocol       ProtocolDiscriminator
	MessageType    MessageType
}

// ParseHeader extracts the NAS header from raw bytes.
// Returns the header and the offset where the message body starts.
func ParseHeader(data []byte) (Header, int, error) {
	if len(data) < 2 {
		return Header{}, 0, fmt.Errorf("NAS message too short: %d bytes (need at least 2)", len(data))
	}

	h := Header{
		SecurityHeader: SecurityHeaderType(data[0] >> 4),
		Protocol:       ProtocolDiscriminator(data[0] & 0x0F),
	}

	if h.SecurityHeader != SecurityHeaderPlainNAS {
		// Security-protected NAS message: byte 0 = security header + PD,
		// bytes 1-4 = MAC, byte 5 = sequence number, bytes 6+ = plain NAS
		if len(data) < 8 {
			return Header{}, 0, fmt.Errorf("security-protected NAS too short: %d bytes", len(data))
		}
		// Skip the 6-byte outer security wrapper and parse the inner plain NAS
		innerH, innerOff, err := ParseHeader(data[6:])
		if err != nil {
			return Header{}, 0, fmt.Errorf("parsing inner NAS: %w", err)
		}
		innerH.SecurityHeader = h.SecurityHeader
		// Offset = 6 (outer sec wrapper) + innerOff (inner plain header = 2)
		return innerH, 6 + innerOff, nil
	}

	// Plain NAS: byte 0 = PD, byte 1 = message type
	h.MessageType = MessageType(data[1])
	return h, 2, nil
}

// EPS Mobile Identity type (TS 24.301 Section 9.9.3.12)
type IdentityType uint8

const (
	IdentityIMSI  IdentityType = 1
	IdentityIMEI  IdentityType = 2
	IdentityIMEISV IdentityType = 3
	IdentityTMSI  IdentityType = 4
	IdentityGUTI  IdentityType = 6
)

// DecodeIMSI extracts an IMSI from BCD-encoded EPS Mobile Identity IE.
// Format: length byte, then BCD digits with identity type in low nibble of first byte.
func DecodeIMSI(data []byte) (string, error) {
	if len(data) < 1 {
		return "", fmt.Errorf("IMSI data too short")
	}

	// First byte: odd/even indicator (bit 4) | identity digit 1 (bits 7-5) | identity type (bits 2-0)
	idType := IdentityType(data[0] & 0x07)
	if idType != IdentityIMSI {
		return "", fmt.Errorf("not an IMSI (type=%d)", idType)
	}

	odd := (data[0] >> 3) & 1
	imsi := ""

	// First digit is in high nibble of first byte
	imsi += fmt.Sprintf("%d", (data[0]>>4)&0x0F)

	// Remaining bytes: each byte has 2 BCD digits (low nibble first, then high)
	for i := 1; i < len(data); i++ {
		lo := data[i] & 0x0F
		hi := (data[i] >> 4) & 0x0F

		imsi += fmt.Sprintf("%d", lo)
		if i == len(data)-1 && odd == 0 {
			// Last byte, even number of digits: high nibble is filler (0xF)
			if hi != 0x0F {
				imsi += fmt.Sprintf("%d", hi)
			}
		} else {
			imsi += fmt.Sprintf("%d", hi)
		}
	}

	return imsi, nil
}

// EncodeIMSI encodes an IMSI string into BCD EPS Mobile Identity format.
func EncodeIMSI(imsi string) ([]byte, error) {
	if len(imsi) < 6 || len(imsi) > 15 {
		return nil, fmt.Errorf("invalid IMSI length: %d (must be 6-15 digits)", len(imsi))
	}

	odd := len(imsi) % 2
	numBytes := (len(imsi) + 1) / 2

	result := make([]byte, numBytes)

	// First byte: digit1 (high nibble) | odd/even (bit 3) | IMSI type (bits 2-0)
	firstDigit := imsi[0] - '0'
	result[0] = (firstDigit << 4) | uint8(odd<<3) | uint8(IdentityIMSI)

	// Remaining digits packed as BCD pairs
	idx := 1
	for i := 1; i < numBytes; i++ {
		lo := imsi[idx] - '0'
		idx++
		var hi byte
		if idx < len(imsi) {
			hi = imsi[idx] - '0'
			idx++
		} else {
			hi = 0x0F // filler
		}
		result[i] = (hi << 4) | lo
	}

	return result, nil
}
