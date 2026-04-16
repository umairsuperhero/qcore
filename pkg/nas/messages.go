package nas

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"time"
	"unicode/utf16"
)

// AttachRequest represents a NAS Attach Request (TS 24.301 Section 8.2.4).
type AttachRequest struct {
	AttachType           uint8  // 4 bits
	NASKeySetIdentifier  uint8  // 4 bits
	EPSMobileIdentity    []byte // variable length
	IMSI                 string // decoded from EPSMobileIdentity
	UENetworkCapability  []byte
	ESMMessageContainer  []byte
}

// AuthenticationRequest represents a NAS Authentication Request (TS 24.301 Section 8.2.7).
type AuthenticationRequest struct {
	NASKeySetIdentifier uint8    // 4 bits (TSC + key set ID)
	RAND                [16]byte
	AUTN                [16]byte
}

// AuthenticationResponse represents a NAS Authentication Response (TS 24.301 Section 8.2.8).
type AuthenticationResponse struct {
	RES []byte // 4-16 bytes
}

// SecurityModeCommand represents a NAS Security Mode Command (TS 24.301 Section 8.2.20).
type SecurityModeCommand struct {
	SelectedNASSecAlg   uint8  // octet: ciphering (high nibble) | integrity (low nibble)
	NASKeySetIdentifier uint8  // 4 bits
	ReplayedUESecCap    []byte // UE security capabilities (replayed)
}

// SecurityModeComplete represents a NAS Security Mode Complete (TS 24.301 Section 8.2.21).
type SecurityModeComplete struct {
	// No mandatory IEs beyond the header
}

// DecodeAttachRequest decodes an Attach Request from raw NAS bytes (after header).
func DecodeAttachRequest(data []byte) (*AttachRequest, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("Attach Request too short: %d bytes", len(data))
	}

	req := &AttachRequest{}

	// Byte 0: NAS key set identifier (high nibble) | EPS attach type (low nibble)
	req.NASKeySetIdentifier = (data[0] >> 4) & 0x0F
	req.AttachType = data[0] & 0x07

	// EPS mobile identity (LV format: length + value)
	if len(data) < 2 {
		return nil, fmt.Errorf("missing EPS mobile identity")
	}
	idLen := int(data[1])
	if len(data) < 2+idLen {
		return nil, fmt.Errorf("EPS mobile identity truncated: need %d, have %d", idLen, len(data)-2)
	}
	req.EPSMobileIdentity = data[2 : 2+idLen]

	// Try to decode IMSI from the mobile identity
	if idLen > 0 {
		idType := IdentityType(req.EPSMobileIdentity[0] & 0x07)
		if idType == IdentityIMSI {
			imsi, err := DecodeIMSI(req.EPSMobileIdentity)
			if err == nil {
				req.IMSI = imsi
			}
		}
	}

	// Parse remaining mandatory/optional IEs
	offset := 2 + idLen

	// UE network capability (LV)
	if offset < len(data) {
		capLen := int(data[offset])
		offset++
		if offset+capLen <= len(data) {
			req.UENetworkCapability = data[offset : offset+capLen]
			offset += capLen
		}
	}

	// ESM message container (LV-E: 2-byte length)
	if offset+2 <= len(data) {
		esmLen := int(binary.BigEndian.Uint16(data[offset:]))
		offset += 2
		if offset+esmLen <= len(data) {
			req.ESMMessageContainer = data[offset : offset+esmLen]
			offset += esmLen
		}
	}

	return req, nil
}

// EncodeAuthenticationRequest encodes a NAS Authentication Request.
func EncodeAuthenticationRequest(req *AuthenticationRequest) ([]byte, error) {
	// Plain NAS: PD + message type + IEs
	msg := make([]byte, 0, 40)

	// Header: security header (0) | protocol discriminator (EMM = 0x07)
	msg = append(msg, uint8(SecurityHeaderPlainNAS<<4)|uint8(EPSMobilityManagement))
	msg = append(msg, uint8(MsgTypeAuthenticationRequest))

	// NAS key set identifier (4 bits) | spare (4 bits)
	msg = append(msg, req.NASKeySetIdentifier&0x0F)

	// Authentication parameter RAND (TV, tag=0x21 implied — it's mandatory at fixed position)
	msg = append(msg, req.RAND[:]...)

	// Authentication parameter AUTN (TLV: tag=0x10 implied — mandatory)
	msg = append(msg, uint8(len(req.AUTN))) // length
	msg = append(msg, req.AUTN[:]...)

	return msg, nil
}

// DecodeAuthenticationResponse decodes a NAS Authentication Response.
func DecodeAuthenticationResponse(data []byte) (*AuthenticationResponse, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("Authentication Response too short")
	}

	resp := &AuthenticationResponse{}

	// Authentication response parameter (LV)
	resLen := int(data[0])
	if len(data) < 1+resLen {
		return nil, fmt.Errorf("RES truncated: need %d, have %d", resLen, len(data)-1)
	}
	resp.RES = make([]byte, resLen)
	copy(resp.RES, data[1:1+resLen])

	return resp, nil
}

// EncodeSecurityModeCommand encodes a NAS Security Mode Command.
func EncodeSecurityModeCommand(cmd *SecurityModeCommand) ([]byte, error) {
	msg := make([]byte, 0, 20)

	// Header
	msg = append(msg, uint8(SecurityHeaderPlainNAS<<4)|uint8(EPSMobilityManagement))
	msg = append(msg, uint8(MsgTypeSecurityModeCommand))

	// Selected NAS security algorithms
	msg = append(msg, cmd.SelectedNASSecAlg)

	// NAS key set identifier (4 bits) | spare (4 bits)
	msg = append(msg, cmd.NASKeySetIdentifier&0x0F)

	// Replayed UE security capabilities (LV)
	msg = append(msg, uint8(len(cmd.ReplayedUESecCap)))
	msg = append(msg, cmd.ReplayedUESecCap...)

	return msg, nil
}

// DecodeSecurityModeComplete decodes a NAS Security Mode Complete.
func DecodeSecurityModeComplete(data []byte) (*SecurityModeComplete, error) {
	// No mandatory IEs beyond the header
	return &SecurityModeComplete{}, nil
}

// VerifyAuthResponse checks if the UE's RES matches our expected XRES.
func VerifyAuthResponse(res, xres []byte) bool {
	if len(res) != len(xres) {
		return false
	}
	match := true
	for i := range res {
		if res[i] != xres[i] {
			match = false
		}
	}
	return match
}

// HexToBytes is a convenience for decoding hex strings from the HSS API.
func HexToBytes(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

// EncodeIdentityRequest encodes a NAS Identity Request (TS 24.301 §8.2.10).
// identityType: 1=IMSI, 2=IMEI, 3=IMEISV, 4=TMSI, 6=GUTI
func EncodeIdentityRequest(identityType uint8) []byte {
	return []byte{
		uint8(SecurityHeaderPlainNAS<<4) | uint8(EPSMobilityManagement),
		uint8(MsgTypeIdentityRequest),
		identityType & 0x0F, // low nibble = type, high nibble = spare/KSI = 0
	}
}

// IdentityResponse holds the identity returned by the UE.
type IdentityResponse struct {
	IdentityType uint8
	IMSI         string // populated if IdentityType == 1 (IMSI)
	RawIdentity  []byte // raw BCD bytes for other types
}

// DecodeIdentityResponse decodes a NAS Identity Response body (after the header).
// data starts after the NAS header bytes.
func DecodeIdentityResponse(data []byte) (*IdentityResponse, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("Identity Response too short: %d bytes", len(data))
	}
	resp := &IdentityResponse{}

	// Mobile identity (LV): length + identity bytes
	idLen := int(data[0])
	if len(data) < 1+idLen {
		return nil, fmt.Errorf("Identity Response mobile identity truncated: need %d, have %d", idLen, len(data)-1)
	}
	rawID := data[1 : 1+idLen]
	resp.RawIdentity = rawID

	if len(rawID) == 0 {
		return resp, nil
	}
	resp.IdentityType = rawID[0] & 0x07

	if resp.IdentityType == 1 { // IMSI
		imsi, err := DecodeIMSI(rawID)
		if err != nil {
			return nil, fmt.Errorf("decoding IMSI from Identity Response: %w", err)
		}
		resp.IMSI = imsi
	}
	return resp, nil
}

// EncodeAttachAccept encodes a NAS ATTACH ACCEPT message (TS 24.301 §8.2.1).
// It embeds an Activate Default EPS Bearer Context Request as the ESM container.
// plmn is the serving PLMN, tac is the tracking area code, bearerID is the
// EPS bearer ID (typically 5), apn is the access point name, and pdn is the
// allocated IPv4 address for the UE.
func EncodeAttachAccept(plmn [3]byte, tac uint16, bearerID uint8, apn string, pdn net.IP) ([]byte, error) {
	pdn4 := pdn.To4()
	if pdn4 == nil {
		return nil, fmt.Errorf("EncodeAttachAccept: only IPv4 PDN addresses supported")
	}

	// NAS plain EMM header
	msg := make([]byte, 0, 64)
	msg = append(msg, uint8(SecurityHeaderPlainNAS<<4)|uint8(EPSMobilityManagement))
	msg = append(msg, uint8(MsgTypeAttachAccept))

	// EPS attach result (3 bits) in low nibble; high nibble spare
	// 1 = EPS only attach
	msg = append(msg, 0x01)

	// T3412 timer value: 1 hour = timer unit 001 (1 hr) + value 1 → 0x21
	msg = append(msg, 0x21)

	// TAI list (LV): one TAI, type 0 (list of TACs, one PLMN), 1 element
	taiList := encodeTAIList(plmn, tac)
	msg = append(msg, uint8(len(taiList)))
	msg = append(msg, taiList...)

	// ESM message container (LV-E: 2-byte big-endian length + value)
	esm := encodeActivateDefaultBearerContextRequest(bearerID, apn, pdn4)
	esmLenBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(esmLenBytes, uint16(len(esm)))
	msg = append(msg, esmLenBytes...)
	msg = append(msg, esm...)

	return msg, nil
}

// encodeTAIList encodes a TAI list with a single TAI (type 0: one PLMN, one TAC).
// TS 24.301 §9.9.3.33: type 00, n-1=0 elements, PLMN, TAC
func encodeTAIList(plmn [3]byte, tac uint16) []byte {
	b := make([]byte, 6)
	b[0] = 0x00 // type=00 (list of TACs, one PLMN), num-1=0 (one element)
	copy(b[1:4], plmn[:])
	binary.BigEndian.PutUint16(b[4:6], tac)
	return b
}

// encodeActivateDefaultBearerContextRequest encodes an ESM Activate Default EPS
// Bearer Context Request (TS 24.301 §8.3.6). Minimum fields for IPv4 bearer.
func encodeActivateDefaultBearerContextRequest(bearerID uint8, apn string, pdn net.IP) []byte {
	msg := make([]byte, 0, 24)

	// ESM header: [bearerID|PD=0x02][PTI=0x01][msg_type=0xC1]
	msg = append(msg, (bearerID<<4)|uint8(EPSSessionManagement))
	msg = append(msg, 0x01)   // PTI = 1 (arbitrary, network-initiated)
	msg = append(msg, 0xC1)   // Activate Default EPS Bearer Context Request

	// EPS QoS (mandatory LV): QCI=9 (internet, non-GBR default)
	msg = append(msg, 0x01, 0x09) // length=1, QCI=9

	// Access Point Name (mandatory LV): DNS label format
	apnEncoded := encodeAPN(apn)
	msg = append(msg, uint8(len(apnEncoded)))
	msg = append(msg, apnEncoded...)

	// PDN Address (mandatory LV): type=IPv4(0x01) + 4-byte address
	pdn4 := pdn.To4()
	msg = append(msg, 0x05, 0x01) // length=5, type=IPv4
	msg = append(msg, pdn4...)

	return msg
}

// DetachRequest represents a UE-initiated NAS Detach Request (TS 24.301 §8.2.11).
type DetachRequest struct {
	DetachType uint8 // 3 bits: 001=EPS detach, 010=IMSI detach, 011=combined
	SwitchOff  bool  // true if UE is powering off (no Detach Accept expected)
}

// DecodeDetachRequest decodes a NAS Detach Request body (after the header).
func DecodeDetachRequest(data []byte) (*DetachRequest, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("Detach Request too short: %d bytes", len(data))
	}
	req := &DetachRequest{}
	// Byte 0: detachType (low 3 bits) | switchOff (bit 3) | spare
	req.DetachType = data[0] & 0x07
	req.SwitchOff = (data[0]>>3)&0x01 == 1
	return req, nil
}

// EncodeDetachAccept encodes a NAS Detach Accept (TS 24.301 §8.2.12).
func EncodeDetachAccept() []byte {
	return []byte{
		uint8(SecurityHeaderPlainNAS<<4) | uint8(EPSMobilityManagement),
		uint8(MsgTypeDetachAccept),
	}
}

// EncodeEMMInformation encodes a NAS EMM INFORMATION message (TS 24.301 §8.2.13).
// It carries the network full name, short name, and local time to the UE. All IEs
// are optional; we send full name, short name, and universal time/local time zone.
// networkName should be the human-readable operator name (e.g., "QCore").
func EncodeEMMInformation(networkName string) []byte {
	msg := make([]byte, 0, 32)

	// NAS EMM plain header
	msg = append(msg, uint8(SecurityHeaderPlainNAS<<4)|uint8(EPSMobilityManagement))
	msg = append(msg, uint8(MsgTypeEMMInformation))

	// Full Name for Network (IEI=0x43, TLV): TS 24.301 §9.9.3.24
	// Value: [coding=UCS2(0x90)|spare_CI=0] + UCS-2 encoded name
	if networkName != "" {
		ucs2 := encodeUCS2(networkName)
		nameVal := make([]byte, 1+len(ucs2))
		nameVal[0] = 0x90 // coding scheme = UCS2 (0b1001_0000), CI=0
		copy(nameVal[1:], ucs2)
		msg = append(msg, 0x43, uint8(len(nameVal)))
		msg = append(msg, nameVal...)
	}

	// Universal Time and Local Time Zone (IEI=0x46, TV, 7 bytes): TS 24.301 §9.9.3.32
	// Format: YY MM DD HH mm SS TZ — each byte is 2 BCD digits (digit1<<4|digit0)
	now := time.Now().UTC()
	bcd := func(v int) byte { return byte((v/10)<<4 | v%10) }
	msg = append(msg, 0x46)
	msg = append(msg,
		bcd(now.Year()%100),
		bcd(int(now.Month())),
		bcd(now.Day()),
		bcd(now.Hour()),
		bcd(now.Minute()),
		bcd(now.Second()),
		0x00, // time zone: UTC+0 (encoded as quarter-hours offset, 0=UTC)
	)

	return msg
}

// encodeUCS2 encodes a UTF-8 string as UCS-2 big-endian.
func encodeUCS2(s string) []byte {
	runes := []rune(s)
	// Convert runes to UTF-16, then take only the BMP code points (UCS-2 subset).
	utf16Runes := utf16.Encode(runes)
	out := make([]byte, 2*len(utf16Runes))
	for i, r := range utf16Runes {
		out[2*i] = byte(r >> 8)
		out[2*i+1] = byte(r)
	}
	return out
}

// encodeAPN encodes an APN string as DNS labels per 3GPP TS 23.003 §9.1.
// e.g., "internet" → [0x08, 'i', 'n', 't', 'e', 'r', 'n', 'e', 't']
func encodeAPN(apn string) []byte {
	if apn == "" {
		return []byte{0x00}
	}
	out := make([]byte, 0, len(apn)+2)
	start := 0
	for i := 0; i <= len(apn); i++ {
		if i == len(apn) || apn[i] == '.' {
			label := apn[start:i]
			out = append(out, uint8(len(label)))
			out = append(out, []byte(label)...)
			start = i + 1
		}
	}
	return out
}
