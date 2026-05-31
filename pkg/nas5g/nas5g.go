// Package nas5g implements 5G NAS (Non-Access Stratum) message encoding/decoding
// per 3GPP TS 24.501.
//
// 5G NAS uses a 2-byte extended protocol discriminator + message type header.
// Messages for 5GMM (Mobility Management) use EPD 0x7E; 5GSM (Session Management)
// uses EPD 0x2E.
package nas5g

import "fmt"

// ExtendedProtocolDiscriminator (TS 24.007 §11.2.3.1a)
type ExtendedProtocolDiscriminator uint8

const (
	EPD5GMM ExtendedProtocolDiscriminator = 0x7E // 5GS Mobility Management
	EPD5GSM ExtendedProtocolDiscriminator = 0x2E // 5GS Session Management
)

// SecurityHeaderType for 5GMM (TS 24.501 §9.3)
type SecurityHeaderType uint8

const (
	SecurityHeaderPlainNAS                         SecurityHeaderType = 0x00
	SecurityHeaderIntegrityProtected               SecurityHeaderType = 0x01
	SecurityHeaderIntegrityProtectedCiphered       SecurityHeaderType = 0x02
	SecurityHeaderIntegrityProtectedNewCtx         SecurityHeaderType = 0x03
	SecurityHeaderIntegrityProtectedNewCtxCiphered SecurityHeaderType = 0x04
)

// MessageType identifies the 5GNas message (TS 24.501 §9.7)
type MessageType uint8

// 5GMM message types
const (
	MsgTypeRegistrationRequest          MessageType = 0x41
	MsgTypeRegistrationAccept           MessageType = 0x42
	MsgTypeRegistrationComplete         MessageType = 0x43
	MsgTypeRegistrationReject           MessageType = 0x44
	MsgTypeDeregistrationRequestUEOrig  MessageType = 0x45
	MsgTypeDeregistrationAcceptUEOrig   MessageType = 0x46
	MsgTypeDeregistrationRequestUETerm  MessageType = 0x47
	MsgTypeDeregistrationAcceptUETerm   MessageType = 0x48
	MsgTypeServiceRequest               MessageType = 0x4C
	MsgTypeServiceAccept                MessageType = 0x4E
	MsgTypeServiceReject                MessageType = 0x4F
	MsgTypeConfigurationUpdateCommand   MessageType = 0x54
	MsgTypeConfigurationUpdateComplete  MessageType = 0x55
	MsgTypeAuthenticationRequest        MessageType = 0x56
	MsgTypeAuthenticationResponse       MessageType = 0x57
	MsgTypeAuthenticationReject         MessageType = 0x58
	MsgTypeAuthenticationFailure        MessageType = 0x59
	MsgTypeAuthenticationResult         MessageType = 0x5A
	MsgTypeIdentityRequest              MessageType = 0x5B
	MsgTypeIdentityResponse             MessageType = 0x5C
	MsgTypeSecurityModeCommand          MessageType = 0x5D
	MsgTypeSecurityModeComplete         MessageType = 0x5E
	MsgTypeSecurityModeReject           MessageType = 0x5F
	MsgType5GMMStatus                   MessageType = 0x64
	MsgTypeNotification                 MessageType = 0x65
	MsgTypeNotificationResponse         MessageType = 0x66
	MsgTypeULNASTransport               MessageType = 0x67
	MsgTypeDLNASTransport               MessageType = 0x68

	// 5GSM (session management) message types — TS 24.501 Table 9.7.4-1.
	// These are carried inside UL/DL NAS Transport containers.
	MsgTypePDUSessionEstablishmentRequest  MessageType = 0xC1
	MsgTypePDUSessionEstablishmentAccept   MessageType = 0xC2
	MsgTypePDUSessionEstablishmentReject   MessageType = 0xC3
)

func (m MessageType) String() string {
	switch m {
	case MsgTypeRegistrationRequest:
		return "RegistrationRequest"
	case MsgTypeRegistrationAccept:
		return "RegistrationAccept"
	case MsgTypeRegistrationComplete:
		return "RegistrationComplete"
	case MsgTypeRegistrationReject:
		return "RegistrationReject"
	case MsgTypeAuthenticationRequest:
		return "AuthenticationRequest"
	case MsgTypeAuthenticationResponse:
		return "AuthenticationResponse"
	case MsgTypeAuthenticationReject:
		return "AuthenticationReject"
	case MsgTypeAuthenticationFailure:
		return "AuthenticationFailure"
	case MsgTypeIdentityRequest:
		return "IdentityRequest"
	case MsgTypeIdentityResponse:
		return "IdentityResponse"
	case MsgTypeSecurityModeCommand:
		return "SecurityModeCommand"
	case MsgTypeSecurityModeComplete:
		return "SecurityModeComplete"
	case MsgTypeSecurityModeReject:
		return "SecurityModeReject"
	case MsgTypeDeregistrationRequestUEOrig:
		return "DeregistrationRequest(UE-originated)"
	default:
		return fmt.Sprintf("MessageType(0x%02X)", uint8(m))
	}
}

// Header is the 5GNas plain (unprotected) message header.
// TS 24.501 §9.2: EPD(1) + SecurityHeader(1) + MessageType(1)
type Header struct {
	EPD                ExtendedProtocolDiscriminator
	SecurityHeaderType SecurityHeaderType
	MessageType        MessageType
}

// EncodeHeader encodes a 3-byte plain NAS header.
func EncodeHeader(h Header) []byte {
	return []byte{uint8(h.EPD), uint8(h.SecurityHeaderType), uint8(h.MessageType)}
}

// DecodeHeader decodes the 3-byte plain NAS header.
func DecodeHeader(data []byte) (Header, error) {
	if len(data) < 3 {
		return Header{}, fmt.Errorf("nas5g: header too short: %d bytes", len(data))
	}
	return Header{
		EPD:                ExtendedProtocolDiscriminator(data[0]),
		SecurityHeaderType: SecurityHeaderType(data[1]),
		MessageType:        MessageType(data[2]),
	}, nil
}

// SecurityProtectedHeader is the header for integrity/ciphered NAS messages.
// TS 24.501 §9.1.1: EPD(1) + SecurityHeader(1) + MAC(4) + SequenceNumber(1) + PlainNASMessage
type SecurityProtectedHeader struct {
	EPD                ExtendedProtocolDiscriminator
	SecurityHeaderType SecurityHeaderType
	MAC                [4]byte
	SequenceNumber     uint8
}

// EncodeSecurityProtectedHeader encodes a 7-byte security-protected NAS header.
func EncodeSecurityProtectedHeader(h SecurityProtectedHeader) []byte {
	out := make([]byte, 7)
	out[0] = uint8(h.EPD)
	out[1] = uint8(h.SecurityHeaderType)
	copy(out[2:6], h.MAC[:])
	out[6] = h.SequenceNumber
	return out
}

// DecodeSecurityProtectedHeader decodes a 7-byte security-protected NAS header.
func DecodeSecurityProtectedHeader(data []byte) (SecurityProtectedHeader, error) {
	if len(data) < 7 {
		return SecurityProtectedHeader{}, fmt.Errorf("nas5g: security header too short: %d bytes", len(data))
	}
	h := SecurityProtectedHeader{
		EPD:                ExtendedProtocolDiscriminator(data[0]),
		SecurityHeaderType: SecurityHeaderType(data[1]),
		SequenceNumber:     data[6],
	}
	copy(h.MAC[:], data[2:6])
	return h, nil
}

// Cause5GMM — 5GMM cause values (TS 24.501 §9.11.3.2)
type Cause5GMM uint8

const (
	Cause5GMMIllegalUE                     Cause5GMM = 0x03
	Cause5GMMIllegalME                     Cause5GMM = 0x06
	Cause5GMM5GSServicesNotAllowed         Cause5GMM = 0x07
	Cause5GMMUEIdentityNotDerived          Cause5GMM = 0x09
	Cause5GMMImplicitlyDeregistered        Cause5GMM = 0x0A
	Cause5GMMPLMNNotAllowed                Cause5GMM = 0x0B
	Cause5GMMTrackingAreaNotAllowed        Cause5GMM = 0x0C
	Cause5GMMRoamingNotAllowed             Cause5GMM = 0x0D
	Cause5GMMNoSuitableCells               Cause5GMM = 0x0F
	Cause5GMMMACFailure                    Cause5GMM = 0x14
	Cause5GMMSynchFailure                  Cause5GMM = 0x15
	Cause5GMMCongestion                    Cause5GMM = 0x16
	Cause5GMMUESecCapMismatch              Cause5GMM = 0x17
	Cause5GMMSecurityModeRejectedUnspec    Cause5GMM = 0x18
	Cause5GMMNon5GAuthUnacceptable         Cause5GMM = 0x1A
	Cause5GMMN1ModeNotAllowed              Cause5GMM = 0x27
	Cause5GMMProtocolError                 Cause5GMM = 0x6F
)

// MSIN encodes a Mobile Subscriber Identification Number as BCD.
// TS 24.501 §9.11.3.4 — 5GS mobile identity
type MobileIdentityType uint8

const (
	MobileIdentityNoIdentity  MobileIdentityType = 0x00
	MobileIdentitySUCI        MobileIdentityType = 0x01
	MobileIdentity5GGUTI      MobileIdentityType = 0x02
	MobileIdentityIMEI        MobileIdentityType = 0x04
	MobileIdentity5GSTMSI     MobileIdentityType = 0x03
	MobileIdentityIMEISV      MobileIdentityType = 0x05
	MobileIdentityMAC         MobileIdentityType = 0x06
	MobileIdentityEUI64       MobileIdentityType = 0x07
)

// GUTI5G holds a 5G-GUTI.
// TS 24.501 §9.11.3.4: MCC+MNC(3) + AMFRegionID(1) + AMFSetID(10b) + AMFPointer(6b) + 5G-TMSI(4)
type GUTI5G struct {
	PLMN        [3]byte
	AMFRegionID uint8
	AMFSetID    uint16 // 10 bits
	AMFPointer  uint8  // 6 bits
	TMSI5G      [4]byte
}

// SUCI holds a Subscription Concealed Identifier (null-scheme, IMSI-based).
// TS 24.501 §9.11.3.4: SUPI format=IMSI, home network identifier (MCC+MNC),
// routing indicator, protection scheme ID, home network public key ID, MSIN
type SUCI struct {
	PLMN            [3]byte // MCC+MNC BCD
	RoutingIndicator [2]byte
	ProtectionScheme uint8
	HomeNetworkPKID  uint8
	MSIN             []byte // BCD, variable length
}

// RegistrationTypeValue (TS 24.501 §9.11.3.7)
type RegistrationTypeValue uint8

const (
	RegistrationTypeInitialRegistration     RegistrationTypeValue = 0x01
	RegistrationTypeMobilityUpdating        RegistrationTypeValue = 0x02
	RegistrationTypePeriodicUpdating        RegistrationTypeValue = 0x03
	RegistrationTypeEmergencyRegistration   RegistrationTypeValue = 0x04
)

// FollowOnRequest flag (TS 24.501 §9.11.3.7, bit 4)
const FollowOnRequestBit = 0x08

// NSSAIEntry holds SST + optional SD.
type NSSAIEntry struct {
	SST uint8
	SD  [3]byte // valid only when SDPresent=true
	SDPresent bool
}

// SNSSAI5G — used in Requested/Allowed NSSAI IEs
type SNSSAI5G = NSSAIEntry
