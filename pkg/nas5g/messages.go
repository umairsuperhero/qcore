package nas5g

import (
	"encoding/binary"
	"fmt"
)

// --- 5GS Mobile Identity helpers -------------------------------------------

// Encode5GGUTI encodes a 5G-GUTI mobile identity IE value (without TL).
// TS 24.501 §9.11.3.4 — type=5G-GUTI (0x02)
// Layout: octet1=0xF2 | AMFRegionID | AMFSetID(high) | AMFSetID(low,4b)+AMFPointer(4b... wait
//
// Actually per spec:
//   Octet 3: spare(1) + odd/even(1) + type(3 bits), but for non-SUCI formats:
//   Identity type is in bits 3-1 of octet 1 (after length). Let me use the correct layout.
//
// TS 24.501 Table 9.11.3.4.1:
//   Octet 1 (after L): spare(4) | identity type(3) | odd/even(1) -- NO, that's for IMEI
//
// For 5G-GUTI (Table 9.11.3.4.3):
//   Octet 1: Spare(b8-b5) = 1111 | identity-type = 010 | Odd/even = 0  => 0xF2
//   Octet 2-4: MCC+MNC (BCD, 3 bytes)
//   Octet 5: AMF Region ID
//   Octet 6-7: AMF Set ID (10 bits, bits 15-6) | AMF Pointer (6 bits, bits 5-0)
//   Octet 8-11: 5G-TMSI (4 bytes)
func Encode5GGUTI(g GUTI5G) []byte {
	b := make([]byte, 11)
	b[0] = 0xF2 // spare(1111) + type(010) + odd/even(0)
	copy(b[1:4], g.PLMN[:])
	b[4] = g.AMFRegionID
	// AMFSetID(10b) | AMFPointer(6b) packed into 2 bytes
	b[5] = uint8(g.AMFSetID >> 2)
	b[6] = uint8((g.AMFSetID&0x03)<<6) | (g.AMFPointer & 0x3F)
	copy(b[7:11], g.TMSI5G[:])
	return b
}

// Decode5GGUTI decodes a 5G-GUTI from the IE value (without length byte).
func Decode5GGUTI(data []byte) (GUTI5G, error) {
	if len(data) < 11 {
		return GUTI5G{}, fmt.Errorf("nas5g: 5G-GUTI too short: %d", len(data))
	}
	g := GUTI5G{}
	copy(g.PLMN[:], data[1:4])
	g.AMFRegionID = data[4]
	g.AMFSetID = uint16(data[5])<<2 | uint16(data[6]>>6)
	g.AMFPointer = data[6] & 0x3F
	copy(g.TMSI5G[:], data[7:11])
	return g, nil
}

// EncodeSUCI encodes a SUCI (null scheme) mobile identity IE value.
// TS 24.501 §9.11.3.4, Table 9.11.3.4.7 (null-scheme SUCI)
// Octet 1: 0xF1 (spare=1111, type=001, odd/even=0)
// Octet 2-4: PLMN (BCD)
// Octet 5-6: Routing Indicator (BCD, 2 bytes)
// Octet 7: Protection Scheme ID (bits 3-0)
// Octet 8: Home Network Public Key ID
// Octet 9+: MSIN (BCD)
func EncodeSUCI(s SUCI) []byte {
	b := make([]byte, 8+len(s.MSIN))
	b[0] = 0xF1 // spare(1111) + type(001) + odd/even(0)
	copy(b[1:4], s.PLMN[:])
	copy(b[4:6], s.RoutingIndicator[:])
	b[6] = s.ProtectionScheme & 0x0F
	b[7] = s.HomeNetworkPKID
	copy(b[8:], s.MSIN)
	return b
}

// DecodeSUCI decodes a SUCI (null scheme) from the IE value (without length byte).
func DecodeSUCI(data []byte) (SUCI, error) {
	if len(data) < 8 {
		return SUCI{}, fmt.Errorf("nas5g: SUCI too short: %d", len(data))
	}
	s := SUCI{}
	copy(s.PLMN[:], data[1:4])
	copy(s.RoutingIndicator[:], data[4:6])
	s.ProtectionScheme = data[6] & 0x0F
	s.HomeNetworkPKID = data[7]
	if len(data) > 8 {
		s.MSIN = make([]byte, len(data)-8)
		copy(s.MSIN, data[8:])
	}
	return s, nil
}

// --- NSSAI helpers ---------------------------------------------------------

// encodeNSSAIEntry encodes a single SNSSAI (SST only, or SST+SD).
func encodeNSSAIEntry(e NSSAIEntry) []byte {
	if e.SDPresent {
		return []byte{4, e.SST, e.SD[0], e.SD[1], e.SD[2]} // L=4, SST, SD(3)
	}
	return []byte{1, e.SST} // L=1, SST
}

// EncodeRequestedNSSAI encodes the Requested NSSAI IE value (without T).
// TS 24.501 §9.11.3.37
func EncodeRequestedNSSAI(entries []NSSAIEntry) []byte {
	var body []byte
	for _, e := range entries {
		body = append(body, encodeNSSAIEntry(e)...)
	}
	// TLV: type is added by caller; we return L + V
	out := make([]byte, 1+len(body))
	out[0] = uint8(len(body))
	copy(out[1:], body)
	return out
}

// --- Registration Request --------------------------------------------------

// RegistrationRequest is a 5GMM Registration Request (TS 24.501 §8.2.6)
type RegistrationRequest struct {
	// Mandatory
	RegistrationType RegistrationTypeValue
	FollowOnRequest  bool
	NASKeySetID      uint8 // 4 bits (ngKSI)
	MobileIdentity   []byte // encoded identity IE value (SUCI or 5G-GUTI)
	// Optional
	UESecurityCapability []byte // if present, raw 2-4 bytes (TS 24.501 §9.11.3.54)
	RequestedNSSAI       []NSSAIEntry
	UEStatus             *uint8 // S1 mode, N1 mode bits
	LastVisitedTAI       []byte // 6-byte TAI (PLMN+TAC)
	MICO                 bool
}

// EncodeRegistrationRequest encodes a Registration Request NAS PDU.
func EncodeRegistrationRequest(req *RegistrationRequest) []byte {
	// Header: EPD=7E, SecurityHeader=00, MessageType=41
	body := EncodeHeader(Header{EPD5GMM, SecurityHeaderPlainNAS, MsgTypeRegistrationRequest})

	// Octet 4: 5GS registration type (bits 3-1) + follow-on request (bit 4) | ngKSI (bits 8-5)
	regTypeByte := uint8(req.RegistrationType) & 0x07
	if req.FollowOnRequest {
		regTypeByte |= FollowOnRequestBit
	}
	ngKSIByte := (req.NASKeySetID & 0x07) << 4
	body = append(body, ngKSIByte|regTypeByte)

	// Mobile Identity (LV-E)
	l := len(req.MobileIdentity)
	body = append(body, uint8(l>>8), uint8(l))
	body = append(body, req.MobileIdentity...)

	// Optional IEs (TLV)
	if len(req.UESecurityCapability) > 0 {
		body = append(body, 0x2E) // IEI
		body = append(body, uint8(len(req.UESecurityCapability)))
		body = append(body, req.UESecurityCapability...)
	}

	if len(req.RequestedNSSAI) > 0 {
		body = append(body, 0x2F) // IEI
		body = append(body, EncodeRequestedNSSAI(req.RequestedNSSAI)...)
	}

	return body
}

// DecodeRegistrationRequest decodes a Registration Request from bytes after the header.
func DecodeRegistrationRequest(data []byte) (*RegistrationRequest, error) {
	// data starts at octet 4 (after EPD+SecHdr+MsgType)
	if len(data) < 1 {
		return nil, fmt.Errorf("nas5g: RegistrationRequest body too short")
	}
	req := &RegistrationRequest{}

	// Octet 4
	b := data[0]
	req.NASKeySetID = (b >> 4) & 0x07
	req.RegistrationType = RegistrationTypeValue(b & 0x07)
	req.FollowOnRequest = (b & FollowOnRequestBit) != 0
	data = data[1:]

	// Mobile Identity (LV-E)
	if len(data) < 2 {
		return nil, fmt.Errorf("nas5g: missing mobile identity length")
	}
	mobileIDLen := int(binary.BigEndian.Uint16(data[:2]))
	data = data[2:]
	if len(data) < mobileIDLen {
		return nil, fmt.Errorf("nas5g: mobile identity truncated")
	}
	req.MobileIdentity = make([]byte, mobileIDLen)
	copy(req.MobileIdentity, data[:mobileIDLen])
	data = data[mobileIDLen:]

	// Optional IEs
	for len(data) >= 2 {
		iei := data[0]
		l := int(data[1])
		data = data[2:]
		if len(data) < l {
			return nil, fmt.Errorf("nas5g: IE 0x%02X truncated", iei)
		}
		v := data[:l]
		data = data[l:]
		switch iei {
		case 0x2E:
			req.UESecurityCapability = make([]byte, l)
			copy(req.UESecurityCapability, v)
		case 0x2F:
			// Requested NSSAI — decode entries
			req.RequestedNSSAI = decodeNSSAIEntries(v)
		}
	}
	return req, nil
}

func decodeNSSAIEntries(data []byte) []NSSAIEntry {
	var entries []NSSAIEntry
	for len(data) >= 2 {
		l := int(data[0])
		data = data[1:]
		if len(data) < l {
			break
		}
		e := NSSAIEntry{SST: data[0]}
		if l >= 4 {
			e.SDPresent = true
			copy(e.SD[:], data[1:4])
		}
		entries = append(entries, e)
		data = data[l:]
	}
	return entries
}

// --- Authentication Request ------------------------------------------------

// AuthenticationRequest carries RAND + AUTN to the UE.
// TS 24.501 §8.2.1
type AuthenticationRequest struct {
	NASKeySetID uint8    // ngKSI (bits 3-0)
	ABBA        []byte   // at least 2 bytes (TS 24.501 §9.11.3.10)
	RAND        [16]byte
	AUTN        [16]byte
}

// EncodeAuthenticationRequest encodes an Authentication Request NAS PDU.
func EncodeAuthenticationRequest(req *AuthenticationRequest) []byte {
	body := EncodeHeader(Header{EPD5GMM, SecurityHeaderPlainNAS, MsgTypeAuthenticationRequest})

	// Octet 4: spare(4) | ngKSI(3) | spare(1) — per TS 24.501 Table 8.2.1.1
	body = append(body, (req.NASKeySetID&0x07)<<4)

	// ABBA (LV): TS 24.501 §9.11.3.10
	abba := req.ABBA
	if len(abba) == 0 {
		abba = []byte{0x00, 0x00} // default: no additional parameters
	}
	body = append(body, uint8(len(abba)))
	body = append(body, abba...)

	// Authentication parameter RAND (type 3): IEI + fixed 16-byte value.
	body = append(body, 0x21)
	body = append(body, req.RAND[:]...)

	// Authentication parameter AUTN (type 4): IEI + length + 16-byte value.
	body = append(body, 0x20, 0x10)
	body = append(body, req.AUTN[:]...)

	return body
}

// DecodeAuthenticationRequest decodes an Authentication Request from bytes after the 3-byte header.
func DecodeAuthenticationRequest(data []byte) (*AuthenticationRequest, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("nas5g: AuthRequest too short")
	}
	req := &AuthenticationRequest{}
	req.NASKeySetID = (data[0] >> 4) & 0x07
	data = data[1:]

	// ABBA (LV)
	if len(data) < 1 {
		return nil, fmt.Errorf("nas5g: missing ABBA")
	}
	abbaLen := int(data[0])
	data = data[1:]
	if len(data) < abbaLen {
		return nil, fmt.Errorf("nas5g: ABBA truncated")
	}
	req.ABBA = make([]byte, abbaLen)
	copy(req.ABBA, data[:abbaLen])
	data = data[abbaLen:]

	// Optional IEs: RAND (type 3, fixed 16 bytes) and AUTN (type 4, LV).
	for len(data) >= 1 {
		iei := data[0]
		data = data[1:]
		switch iei {
		case 0x21:
			if len(data) < 16 {
				return nil, fmt.Errorf("nas5g: RAND truncated")
			}
			copy(req.RAND[:], data[:16])
			data = data[16:]
		case 0x20:
			if len(data) < 1 {
				return nil, fmt.Errorf("nas5g: missing AUTN length")
			}
			l := int(data[0])
			data = data[1:]
			if l != 16 {
				return nil, fmt.Errorf("nas5g: AUTN length %d, want 16", l)
			}
			if len(data) < l {
				return nil, fmt.Errorf("nas5g: AUTN truncated")
			}
			copy(req.AUTN[:], data[:l])
			data = data[l:]
		default:
			return nil, fmt.Errorf("nas5g: unknown AuthRequest IE 0x%02X", iei)
		}
	}
	return req, nil
}

// --- Authentication Response -----------------------------------------------

// AuthenticationResponse carries RES* from UE to network.
// TS 24.501 §8.2.2
type AuthenticationResponse struct {
	ResStar []byte // 16 bytes (TS 24.501 §9.11.3.9)
}

// EncodeAuthenticationResponse encodes an Authentication Response NAS PDU.
func EncodeAuthenticationResponse(resp *AuthenticationResponse) []byte {
	body := EncodeHeader(Header{EPD5GMM, SecurityHeaderPlainNAS, MsgTypeAuthenticationResponse})
	// EAP message or RES*: IEI=0x2D, LV
	body = append(body, 0x2D, uint8(len(resp.ResStar)))
	body = append(body, resp.ResStar...)
	return body
}

// DecodeAuthenticationResponse decodes an Authentication Response from bytes after the header.
func DecodeAuthenticationResponse(data []byte) (*AuthenticationResponse, error) {
	resp := &AuthenticationResponse{}
	for len(data) >= 2 {
		iei := data[0]
		l := int(data[1])
		data = data[2:]
		if len(data) < l {
			return nil, fmt.Errorf("nas5g: IE 0x%02X truncated", iei)
		}
		v := data[:l]
		data = data[l:]
		if iei == 0x2D {
			resp.ResStar = make([]byte, l)
			copy(resp.ResStar, v)
		}
	}
	return resp, nil
}

// --- Security Mode Command -------------------------------------------------

// SecurityModeCommand is sent by the AMF to activate NAS security.
// TS 24.501 §8.2.22
type SecurityModeCommand struct {
	NASSecAlgos        uint8  // ciphering(bits 7-4) | integrity(bits 3-0)
	NASKeySetID        uint8  // ngKSI (bits 3-0)
	ReplayedUESecCaps  []byte // UE 5G security capability
	IMEISV_Requested   bool
	EPSNASSecAlgos     *uint8 // optional
}

// EncodeSecurityModeCommand encodes a Security Mode Command NAS PDU.
func EncodeSecurityModeCommand(cmd *SecurityModeCommand) []byte {
	body := EncodeHeader(Header{EPD5GMM, SecurityHeaderPlainNAS, MsgTypeSecurityModeCommand})

	// Selected NAS security algorithms (1 byte)
	body = append(body, cmd.NASSecAlgos)

	// ngKSI: spare(4) | value(3) | spare(1) -- octet 5
	body = append(body, (cmd.NASKeySetID&0x07)<<4)

	// Replayed UE security capabilities (LV)
	body = append(body, uint8(len(cmd.ReplayedUESecCaps)))
	body = append(body, cmd.ReplayedUESecCaps...)

	// IMEISV request (optional, T=0x0E, TV: 1 byte value in low nibble)
	if cmd.IMEISV_Requested {
		body = append(body, 0xE1) // IEI 0xE, value=request IMEISV
	}

	return body
}

// DecodeSecurityModeCommand decodes a Security Mode Command from bytes after the header.
func DecodeSecurityModeCommand(data []byte) (*SecurityModeCommand, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("nas5g: SecurityModeCommand too short")
	}
	cmd := &SecurityModeCommand{}
	cmd.NASSecAlgos = data[0]
	cmd.NASKeySetID = (data[1] >> 4) & 0x07
	data = data[2:]

	// Replayed UE security capabilities (LV)
	if len(data) < 1 {
		return nil, fmt.Errorf("nas5g: missing replayed UE sec caps")
	}
	capLen := int(data[0])
	data = data[1:]
	if len(data) < capLen {
		return nil, fmt.Errorf("nas5g: UE sec caps truncated")
	}
	cmd.ReplayedUESecCaps = make([]byte, capLen)
	copy(cmd.ReplayedUESecCaps, data[:capLen])
	data = data[capLen:]

	// Optional IEs
	for len(data) >= 1 {
		iei := data[0]
		data = data[1:]
		switch iei {
		default:
			if iei>>4 == 0x0E {
				cmd.IMEISV_Requested = (iei & 0x0F) == 0x01
				continue
			}
			// skip unknown optional TV/TLV
			if len(data) >= 1 {
				data = data[1:] // skip value byte (TV format assumed for unknowns)
			}
		}
	}
	return cmd, nil
}

// --- Security Mode Complete ------------------------------------------------

// SecurityModeComplete is sent by the UE to confirm NAS security activation.
// TS 24.501 §8.2.23
type SecurityModeComplete struct {
	IMEISV   []byte // optional: IMEISV (TLV, IEI=0x77)
	NASPDUContainer []byte // optional: replayed NAS message (IEI=0x71)
}

// EncodeSecurityModeComplete encodes a Security Mode Complete NAS PDU.
func EncodeSecurityModeComplete(msg *SecurityModeComplete) []byte {
	body := EncodeHeader(Header{EPD5GMM, SecurityHeaderPlainNAS, MsgTypeSecurityModeComplete})
	if len(msg.IMEISV) > 0 {
		body = append(body, 0x77, uint8(len(msg.IMEISV)))
		body = append(body, msg.IMEISV...)
	}
	if len(msg.NASPDUContainer) > 0 {
		body = append(body, 0x71)
		body = appendBigLenTLV(body, msg.NASPDUContainer)
	}
	return body
}

// appendBigLenTLV appends a value with 2-byte length (for IEIs that use extended length).
func appendBigLenTLV(dst []byte, value []byte) []byte {
	l := make([]byte, 2)
	binary.BigEndian.PutUint16(l, uint16(len(value)))
	dst = append(dst, l...)
	return append(dst, value...)
}

// DecodeSecurityModeComplete decodes a Security Mode Complete from bytes after the header.
func DecodeSecurityModeComplete(data []byte) (*SecurityModeComplete, error) {
	msg := &SecurityModeComplete{}
	for len(data) >= 2 {
		iei := data[0]
		data = data[1:]

		switch iei {
		case 0x77: // IMEISV — 1-byte length TLV
			l := int(data[0])
			data = data[1:]
			if len(data) < l {
				return nil, fmt.Errorf("nas5g: IMEISV IE truncated")
			}
			msg.IMEISV = make([]byte, l)
			copy(msg.IMEISV, data[:l])
			data = data[l:]
		case 0x71: // NAS message container — 2-byte length (TS 24.501 §9.11.3.33a)
			if len(data) < 2 {
				return nil, fmt.Errorf("nas5g: NAS PDU container length truncated")
			}
			l := int(binary.BigEndian.Uint16(data[:2]))
			data = data[2:]
			if len(data) < l {
				return nil, fmt.Errorf("nas5g: NAS PDU container truncated")
			}
			msg.NASPDUContainer = make([]byte, l)
			copy(msg.NASPDUContainer, data[:l])
			data = data[l:]
		default:
			// unknown TLV: skip using 1-byte length
			if len(data) < 1 {
				break
			}
			l := int(data[0])
			data = data[1:]
			if len(data) >= l {
				data = data[l:]
			} else {
				data = nil
			}
		}
	}
	return msg, nil
}

// --- Registration Accept ---------------------------------------------------

// RegistrationAccept is sent by the AMF to complete registration.
// TS 24.501 §8.2.7
type RegistrationAccept struct {
	RegistrationResult uint8 // bits 1-0: 3GPP access; bits 2: non-3GPP
	AssignedGUTI       *GUTI5G
	AllowedNSSAI       []NSSAIEntry
	TAIList            []byte // raw encoding for now
}

// EncodeRegistrationAccept encodes a Registration Accept NAS PDU.
func EncodeRegistrationAccept(msg *RegistrationAccept) []byte {
	body := EncodeHeader(Header{EPD5GMM, SecurityHeaderPlainNAS, MsgTypeRegistrationAccept})

	// 5GS registration result (LV, 1 byte value)
	body = append(body, 0x01, msg.RegistrationResult&0x07)

	// Assigned 5G-GUTI (optional, TLV-E, IEI=0x77)
	if msg.AssignedGUTI != nil {
		gutiBytes := Encode5GGUTI(*msg.AssignedGUTI)
		// 5GS mobile identity is a type-6 IE, so its length is two octets.
		body = append(body, 0x77, uint8(len(gutiBytes)>>8), uint8(len(gutiBytes)))
		body = append(body, gutiBytes...)
	}

	// Allowed NSSAI (optional, TLV, IEI=0x15)
	if len(msg.AllowedNSSAI) > 0 {
		body = append(body, 0x15)
		nssaiVal := EncodeRequestedNSSAI(msg.AllowedNSSAI)
		body = append(body, nssaiVal...)
	}

	// TAI list (optional, TLV, IEI=0x54)
	if len(msg.TAIList) > 0 {
		body = append(body, 0x54, uint8(len(msg.TAIList)))
		body = append(body, msg.TAIList...)
	}

	return body
}

// DecodeRegistrationAccept decodes a Registration Accept from bytes after the header.
func DecodeRegistrationAccept(data []byte) (*RegistrationAccept, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("nas5g: RegistrationAccept too short")
	}
	msg := &RegistrationAccept{}

	// 5GS registration result (LV)
	resLen := int(data[0])
	data = data[1:]
	if len(data) < resLen {
		return nil, fmt.Errorf("nas5g: registration result truncated")
	}
	if resLen >= 1 {
		msg.RegistrationResult = data[0] & 0x07
	}
	data = data[resLen:]

	// Optional TLV IEs. Most local IEs are type-4 (one-octet length), but
	// assigned 5G-GUTI / 5GS mobile identity (IEI 0x77) is type-6.
	for len(data) > 0 {
		iei := data[0]
		data = data[1:]
		if iei == 0x77 {
			if len(data) < 2 {
				return nil, fmt.Errorf("nas5g: IE 0x%02X length truncated", iei)
			}
			l := int(data[0])<<8 | int(data[1])
			data = data[2:]
			if len(data) < l {
				return nil, fmt.Errorf("nas5g: IE 0x%02X truncated", iei)
			}
			v := data[:l]
			data = data[l:]
			g, err := Decode5GGUTI(v)
			if err == nil {
				msg.AssignedGUTI = &g
			}
			continue
		}
		if len(data) < 1 {
			return nil, fmt.Errorf("nas5g: IE 0x%02X length truncated", iei)
		}
		l := int(data[0])
		data = data[1:]
		if len(data) < l {
			return nil, fmt.Errorf("nas5g: IE 0x%02X truncated", iei)
		}
		v := data[:l]
		data = data[l:]
		switch iei {
		case 0x15:
			msg.AllowedNSSAI = decodeNSSAIEntries(v)
		case 0x54:
			msg.TAIList = make([]byte, l)
			copy(msg.TAIList, v)
		}
	}
	return msg, nil
}

// --- Registration Complete -------------------------------------------------

// RegistrationComplete is sent by the UE to acknowledge RA and confirm 5G-GUTI allocation.
// TS 24.501 §8.2.8 — no mandatory IEs beyond the header.
type RegistrationComplete struct{}

// EncodeRegistrationComplete encodes a Registration Complete NAS PDU.
func EncodeRegistrationComplete() []byte {
	return EncodeHeader(Header{EPD5GMM, SecurityHeaderPlainNAS, MsgTypeRegistrationComplete})
}

// EncodeRegistrationReject encodes a plain (unprotected) Registration Reject
// NAS PDU. TS 24.501 §8.2.4: mandatory IE is the 5GMM cause (1 byte).
func EncodeRegistrationReject(cause Cause5GMM) []byte {
	hdr := EncodeHeader(Header{EPD5GMM, SecurityHeaderPlainNAS, MsgTypeRegistrationReject})
	return append(hdr, uint8(cause))
}

// --- Top-level Decode ------------------------------------------------------

// Message is a discriminated union of all decoded 5G NAS messages.
// AuthenticationFailure is sent by the UE when the network's authentication
// challenge fails the UE's own checks (e.g. MAC failure = wrong Ki/OPc on
// the network side, or SQN out of range). TS 24.501 §8.2.12.
type AuthenticationFailure struct {
	Cause Cause5GMM // mandatory: 0x14 = MAC failure, 0x15 = SQN failure
}

// EncodeAuthenticationFailure encodes a NAS Authentication Failure PDU.
func EncodeAuthenticationFailure(cause Cause5GMM) []byte {
	hdr := EncodeHeader(Header{EPD5GMM, SecurityHeaderPlainNAS, MsgTypeAuthenticationFailure})
	return append(hdr, uint8(cause))
}

type Message struct {
	Header Header

	RegistrationRequest    *RegistrationRequest
	RegistrationAccept     *RegistrationAccept
	AuthenticationRequest  *AuthenticationRequest
	AuthenticationResponse *AuthenticationResponse
	AuthenticationFailure  *AuthenticationFailure
	SecurityModeCommand    *SecurityModeCommand
	SecurityModeComplete   *SecurityModeComplete
}

// Decode decodes a plain (unprotected) 5G NAS PDU.
// For integrity/ciphered PDUs call DecodeSecurityProtectedHeader first, then
// strip the 7-byte outer header and call Decode on the inner plain NAS PDU.
func Decode(data []byte) (*Message, error) {
	hdr, err := DecodeHeader(data)
	if err != nil {
		return nil, err
	}
	msg := &Message{Header: hdr}
	body := data[3:] // skip 3-byte header

	switch hdr.MessageType {
	case MsgTypeRegistrationRequest:
		msg.RegistrationRequest, err = DecodeRegistrationRequest(body)
	case MsgTypeRegistrationAccept:
		msg.RegistrationAccept, err = DecodeRegistrationAccept(body)
	case MsgTypeAuthenticationRequest:
		msg.AuthenticationRequest, err = DecodeAuthenticationRequest(body)
	case MsgTypeAuthenticationResponse:
		msg.AuthenticationResponse, err = DecodeAuthenticationResponse(body)
	case MsgTypeAuthenticationFailure:
		af := &AuthenticationFailure{}
		if len(body) >= 1 {
			af.Cause = Cause5GMM(body[0])
		}
		msg.AuthenticationFailure = af
	case MsgTypeSecurityModeCommand:
		msg.SecurityModeCommand, err = DecodeSecurityModeCommand(body)
	case MsgTypeSecurityModeComplete:
		msg.SecurityModeComplete, err = DecodeSecurityModeComplete(body)
	case MsgTypeRegistrationComplete:
		// no body
	case MsgTypeULNASTransport, MsgTypeDLNASTransport,
		MsgTypeRegistrationReject, MsgTypeAuthenticationReject:
		// handled by the AMF dispatcher before calling Decode; silently accepted here.
	default:
		return nil, fmt.Errorf("nas5g: unsupported message type %s", hdr.MessageType)
	}
	if err != nil {
		return nil, fmt.Errorf("nas5g: %s: %w", hdr.MessageType, err)
	}
	return msg, nil
}

// EncodeTAIList encodes a single-PLMN TAI list (TS 24.501 §9.11.3.9a).
// type=0x00 (one PLMN, non-consecutive TACs), count-1 in bits 4-0 of first byte.
func EncodeTAIList(plmn [3]byte, tacs [][3]byte) []byte {
	b := make([]byte, 0, 1+3+3*len(tacs))
	b = append(b, uint8(len(tacs)-1)&0x1F) // type=0 (bits 7-5), count-1 (bits 4-0)
	b = append(b, plmn[:]...)
	for _, tac := range tacs {
		b = append(b, tac[:]...)
	}
	return b
}

// ─── 5GSM (PDU session layer) ────────────────────────────────────────────────
//
// TS 24.501 §9.4 defines the 5G Session Management (5GSM) protocol header:
//   Octet 1: Extended Protocol Discriminator (EPD) = 0x2E
//   Octet 2: PDU Session ID
//   Octet 3: Procedure Transaction Identity (PTI)
//   Octet 4: Message Type

// EncodePDUSessionEstablishmentRequest encodes a minimal 5GSM PDU Session
// Establishment Request (TS 24.501 §8.3.1). Only the mandatory header is
// encoded; optional IEs (PDU type, SSC mode, etc.) are omitted for this
// minimal implementation.
func EncodePDUSessionEstablishmentRequest(pduSessionID, pti uint8) []byte {
	return []byte{uint8(EPD5GSM), pduSessionID, pti, uint8(MsgTypePDUSessionEstablishmentRequest)}
}

// EncodePDUSessionEstablishmentAccept encodes a 5GSM PDU Session Establishment
// Accept (TS 24.501 §8.3.2) for a default IPv4, SSC-mode-1 session. It carries
// the mandatory IEs a real UE (UERANSIM) requires — Selected PDU session type +
// SSC mode, Authorized QoS rules (one default match-all rule), and Session-AMBR —
// plus the optional PDU address IE conveying the SMF-assigned UE IPv4.
//
// ueIPv4 is the address the SMF allocated; pass the zero value to omit the PDU
// address IE. Bytes are pinned by TestEncodePDUSessionEstablishmentAcceptGolden.
func EncodePDUSessionEstablishmentAccept(pduSessionID, pti uint8, ueIPv4 [4]byte) []byte {
	b := []byte{
		uint8(EPD5GSM),                              // Extended protocol discriminator (0x2E)
		pduSessionID,                                // PDU session ID
		pti,                                         // Procedure transaction identity
		uint8(MsgTypePDUSessionEstablishmentAccept), // Message type (0xC2)
		0x11,                                        // Selected PDU session type IPv4(1) | Selected SSC mode 1 (<<4)
	}

	// Authorized QoS rules (IE 9.11.4.13, LV-E): one default QoS rule —
	// identifier 1, "create new QoS rule", DQR=1, one bidirectional match-all
	// packet filter, precedence 255, QFI 1. This is the free5GC default shape
	// that UERANSIM accepts.
	qosRule := []byte{
		0x01,       // QoS rule identifier = 1
		0x00, 0x06, // Length of QoS rule = 6
		0x31, // rule op=CreateNewQoSRule(001) | DQR=1 | number of packet filters=1
		0x31, // packet filter: direction=bidirectional(11) | identifier=1
		0x01, // packet filter contents length = 1
		0x01, // match-all packet filter component (type 0x01, no value)
		0xFF, // QoS rule precedence = 255
		0x01, // QoS flow identifier (QFI) = 1
	}
	b = append(b, 0x00, uint8(len(qosRule))) // Authorized QoS rules length (2 octets, LV-E)
	b = append(b, qosRule...)

	// Session-AMBR (IE 9.11.4.14, LV): unit = 1 Mbps (0x06), DL = UL = 1000 (~1 Gbps).
	sessionAMBR := []byte{0x06, 0x03, 0xE8, 0x06, 0x03, 0xE8}
	b = append(b, uint8(len(sessionAMBR)))
	b = append(b, sessionAMBR...)

	// PDU address (IE 9.11.4.10, TLV, IEI 0x29), IPv4.
	if ueIPv4 != ([4]byte{}) {
		b = append(b, 0x29, 0x05, 0x01) // IEI, length=5, PDU session type=IPv4(1)
		b = append(b, ueIPv4[:]...)
	}
	return b
}

// ULNASTransportPayload holds decoded fields from a UL NAS Transport message.
type ULNASTransportPayload struct {
	PDUSessionID     uint8
	PayloadContainer []byte // 5GSM container (N1 SM info)
}

// EncodeULNASTransport encodes an unprotected 5GMM UL NAS Transport message
// (TS 24.501 §8.2.17) carrying an N1 SM (5GSM) container.
//
// Mandatory IEs encoded: payload container type (N1 SM, 0x01), payload
// container (the 5GSM bytes), PDU session ID.
func EncodeULNASTransport(pduSessionID uint8, n1SmContainer []byte) []byte {
	b := EncodeHeader(Header{EPD5GMM, SecurityHeaderPlainNAS, MsgTypeULNASTransport})

	// Payload container type (IE1): IEI is 0, value lives in the low nibble.
	b = append(b, 0x01)

	// Payload container length (2 bytes big-endian) + container
	clen := len(n1SmContainer)
	b = append(b, uint8(clen>>8), uint8(clen))
	b = append(b, n1SmContainer...)

	// Optional IE: PDU Session ID is an IE3 (IEI + one-octet value).
	b = append(b, 0x12, pduSessionID)
	return b
}

// EncodeDLNASTransport encodes an unprotected 5GMM DL NAS Transport message
// (TS 24.501 §8.2.11) carrying an N1 SM (5GSM) container down to the UE — the
// mirror of EncodeULNASTransport. The caller integrity-protects the result via
// the NAS security context before handing it to NGAP DownlinkNASTransport.
//
// Mandatory IEs encoded: payload container type (N1 SM, 0x01), payload
// container (LV-E: 2-octet length + the 5GSM bytes), PDU session ID (IE3).
func EncodeDLNASTransport(pduSessionID uint8, n1SmContainer []byte) []byte {
	b := EncodeHeader(Header{EPD5GMM, SecurityHeaderPlainNAS, MsgTypeDLNASTransport})

	// Payload container type (IE1): IEI is 0, value (N1 SM = 1) in the low nibble.
	b = append(b, 0x01)

	// Payload container (LV-E): 2-byte big-endian length + container.
	clen := len(n1SmContainer)
	b = append(b, uint8(clen>>8), uint8(clen))
	b = append(b, n1SmContainer...)

	// PDU Session ID (IE3: IEI 0x12 + one-octet value).
	b = append(b, 0x12, pduSessionID)
	return b
}

// DecodeULNASTransport decodes the payload of a UL NAS Transport message
// (after the 3-byte NAS header). Returns the N1 SM container and the PDU
// session ID IEI value if present.
func DecodeULNASTransport(body []byte) (*ULNASTransportPayload, error) {
	if len(body) < 4 {
		return nil, fmt.Errorf("nas5g: UL NAS Transport body too short: %d", len(body))
	}
	// Payload container type (IE1): IEI is 0, value lives in the low nibble.
	containerType := body[0] & 0x0F
	if containerType != 0x01 { // 0x01 = N1 SM info
		return nil, fmt.Errorf("nas5g: unexpected payload container type 0x%02X", containerType)
	}
	// Payload container length (2 bytes big-endian)
	clen := int(body[1])<<8 | int(body[2])
	if len(body) < 3+clen {
		return nil, fmt.Errorf("nas5g: UL NAS Transport truncated: need %d body bytes, have %d", 3+clen, len(body))
	}
	payload := &ULNASTransportPayload{
		PayloadContainer: body[3 : 3+clen],
	}
	// Scan optional IEs after the container. We only need PDU Session ID for the
	// AMF→SMF request; leave the rest as skipped payload.
	pos := 3 + clen
	for pos < len(body) {
		iei := body[pos]
		pos++
		switch iei {
		case 0x12: // PDU Session ID, IE3
			if pos < len(body) {
				payload.PDUSessionID = body[pos]
				pos++
			}
		default:
			// Optional half-octet IEIs are encoded as IEI in the high nibble and
			// value in the low nibble, with no following length.
			if iei>>4 == 0x08 {
				continue
			}
			// Unknown IE — try to skip (1-byte length follows for most IEs)
			if pos < len(body) {
				pos += int(body[pos]) + 1
			}
		}
	}
	return payload, nil
}
