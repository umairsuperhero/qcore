package nas5g

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Header -----------------------------------------------------------------

func TestHeaderRoundTrip(t *testing.T) {
	h := Header{EPD5GMM, SecurityHeaderPlainNAS, MsgTypeRegistrationRequest}
	enc := EncodeHeader(h)
	assert.Equal(t, []byte{0x7E, 0x00, 0x41}, enc)

	dec, err := DecodeHeader(enc)
	require.NoError(t, err)
	assert.Equal(t, h, dec)
}

func TestSecurityProtectedHeaderRoundTrip(t *testing.T) {
	h := SecurityProtectedHeader{
		EPD:                EPD5GMM,
		SecurityHeaderType: SecurityHeaderIntegrityProtected,
		MAC:                [4]byte{0x01, 0x02, 0x03, 0x04},
		SequenceNumber:     0x05,
	}
	enc := EncodeSecurityProtectedHeader(h)
	assert.Len(t, enc, 7)
	assert.Equal(t, uint8(0x7E), enc[0])
	assert.Equal(t, uint8(0x01), enc[1])
	assert.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, enc[2:6])
	assert.Equal(t, uint8(0x05), enc[6])

	dec, err := DecodeSecurityProtectedHeader(enc)
	require.NoError(t, err)
	assert.Equal(t, h, dec)
}

// --- Mobile Identity --------------------------------------------------------

func TestGUTI5GRoundTrip(t *testing.T) {
	g := GUTI5G{
		PLMN:        [3]byte{0x00, 0x01, 0xF1}, // MCC=001, MNC=01
		AMFRegionID: 0xAB,
		AMFSetID:    0x3FF, // max 10-bit
		AMFPointer:  0x3F,  // max 6-bit
		TMSI5G:      [4]byte{0x11, 0x22, 0x33, 0x44},
	}
	enc := Encode5GGUTI(g)
	assert.Len(t, enc, 11)
	assert.Equal(t, uint8(0xF2), enc[0])
	assert.Equal(t, [3]byte{0x00, 0x01, 0xF1}, [3]byte(enc[1:4]))
	assert.Equal(t, uint8(0xAB), enc[4])
	// AMFSetID=0x3FF: enc[5]=(0x3FF>>2)=0xFF, enc[6]=(0x03<<6)|(0x3F)=0xFF
	assert.Equal(t, uint8(0xFF), enc[5])
	assert.Equal(t, uint8(0xFF), enc[6])

	dec, err := Decode5GGUTI(enc)
	require.NoError(t, err)
	assert.Equal(t, g, dec)
}

func TestSUCIRoundTrip(t *testing.T) {
	s := SUCI{
		PLMN:             [3]byte{0x00, 0x01, 0xF1},
		RoutingIndicator: [2]byte{0xFF, 0xFF},
		ProtectionScheme: 0x00,
		HomeNetworkPKID:  0x00,
		MSIN:             []byte{0x12, 0x34, 0x56, 0x78, 0x90},
	}
	enc := EncodeSUCI(s)
	assert.Equal(t, uint8(0xF1), enc[0])

	dec, err := DecodeSUCI(enc)
	require.NoError(t, err)
	assert.Equal(t, s.PLMN, dec.PLMN)
	assert.Equal(t, s.ProtectionScheme, dec.ProtectionScheme)
	assert.Equal(t, s.MSIN, dec.MSIN)
}

// --- Registration Request ---------------------------------------------------

func TestRegistrationRequestRoundTrip(t *testing.T) {
	suci := EncodeSUCI(SUCI{
		PLMN:             [3]byte{0x00, 0x01, 0xF1},
		RoutingIndicator: [2]byte{0xFF, 0xFF},
		MSIN:             []byte{0x00, 0x00, 0x00, 0x00, 0x01},
	})

	req := &RegistrationRequest{
		RegistrationType: RegistrationTypeInitialRegistration,
		FollowOnRequest:  true,
		NASKeySetID:      0x07,
		MobileIdentity:   suci,
		UESecurityCapability: []byte{0xE0, 0x00, 0xC0, 0x00},
		RequestedNSSAI: []NSSAIEntry{
			{SST: 0x01},
			{SST: 0x02, SD: [3]byte{0x01, 0x02, 0x03}, SDPresent: true},
		},
	}

	enc := EncodeRegistrationRequest(req)
	assert.Equal(t, uint8(0x7E), enc[0]) // EPD
	assert.Equal(t, uint8(0x00), enc[1]) // SecurityHeader
	assert.Equal(t, uint8(0x41), enc[2]) // MsgType

	// Decode the full PDU
	msg, err := Decode(enc)
	require.NoError(t, err)
	require.NotNil(t, msg.RegistrationRequest)

	got := msg.RegistrationRequest
	assert.Equal(t, req.RegistrationType, got.RegistrationType)
	assert.Equal(t, req.FollowOnRequest, got.FollowOnRequest)
	assert.Equal(t, req.NASKeySetID, got.NASKeySetID)
	assert.Equal(t, req.MobileIdentity, got.MobileIdentity)
	assert.Equal(t, req.UESecurityCapability, got.UESecurityCapability)
	require.Len(t, got.RequestedNSSAI, 2)
	assert.Equal(t, uint8(0x01), got.RequestedNSSAI[0].SST)
	assert.Equal(t, uint8(0x02), got.RequestedNSSAI[1].SST)
	assert.True(t, got.RequestedNSSAI[1].SDPresent)
	assert.Equal(t, [3]byte{0x01, 0x02, 0x03}, got.RequestedNSSAI[1].SD)
}

func TestRegistrationRequest_NoOptionals(t *testing.T) {
	req := &RegistrationRequest{
		RegistrationType: RegistrationTypePeriodicUpdating,
		NASKeySetID:      0x00,
		MobileIdentity:   []byte{0xF2, 0x00, 0x01, 0xF1, 0x01, 0x00, 0x00, 0x11, 0x22, 0x33, 0x44},
	}
	enc := EncodeRegistrationRequest(req)
	msg, err := Decode(enc)
	require.NoError(t, err)
	assert.Equal(t, req.RegistrationType, msg.RegistrationRequest.RegistrationType)
	assert.Nil(t, msg.RegistrationRequest.UESecurityCapability)
}

// --- Authentication Request -------------------------------------------------

func TestAuthenticationRequestRoundTrip(t *testing.T) {
	req := &AuthenticationRequest{
		NASKeySetID: 0x01,
		ABBA:        []byte{0x00, 0x00},
		RAND:        [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		AUTN:        [16]byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
	}
	enc := EncodeAuthenticationRequest(req)
	assert.Equal(t, uint8(0x56), enc[2]) // MsgTypeAuthenticationRequest

	msg, err := Decode(enc)
	require.NoError(t, err)
	require.NotNil(t, msg.AuthenticationRequest)

	got := msg.AuthenticationRequest
	assert.Equal(t, req.NASKeySetID, got.NASKeySetID)
	assert.Equal(t, req.RAND, got.RAND)
	assert.Equal(t, req.AUTN, got.AUTN)
}

func TestAuthenticationRequestUERANSIMGolden(t *testing.T) {
	req := &AuthenticationRequest{
		NASKeySetID: 0x01,
		ABBA:        []byte{0x00, 0x00},
		RAND:        [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		AUTN:        [16]byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
	}

	assert.Equal(t,
		"7e005610020000210102030405060708090a0b0c0d0e0f102010100f0e0d0c0b0a090807060504030201",
		hex.EncodeToString(EncodeAuthenticationRequest(req)),
	)
}

// --- Authentication Response ------------------------------------------------

func TestAuthenticationResponseRoundTrip(t *testing.T) {
	resp := &AuthenticationResponse{
		ResStar: bytes.Repeat([]byte{0xAB}, 16),
	}
	enc := EncodeAuthenticationResponse(resp)
	assert.Equal(t, uint8(0x57), enc[2])

	msg, err := Decode(enc)
	require.NoError(t, err)
	require.NotNil(t, msg.AuthenticationResponse)
	assert.Equal(t, resp.ResStar, msg.AuthenticationResponse.ResStar)
}

// --- Security Mode Command --------------------------------------------------

func TestSecurityModeCommandRoundTrip(t *testing.T) {
	cmd := &SecurityModeCommand{
		NASSecAlgos:       0x02, // NIA2 | NEA0
		NASKeySetID:       0x01,
		ReplayedUESecCaps: []byte{0xE0, 0x00, 0xC0, 0x00},
		IMEISV_Requested:  true,
	}
	enc := EncodeSecurityModeCommand(cmd)
	assert.Equal(t, uint8(0x5D), enc[2])
	assert.Equal(t, []byte{0x7e, 0x00, 0x5d, 0x02, 0x10, 0x04, 0xe0, 0x00, 0xc0, 0x00, 0xe1}, enc)

	msg, err := Decode(enc)
	require.NoError(t, err)
	require.NotNil(t, msg.SecurityModeCommand)

	got := msg.SecurityModeCommand
	assert.Equal(t, cmd.NASSecAlgos, got.NASSecAlgos)
	assert.Equal(t, cmd.NASKeySetID, got.NASKeySetID)
	assert.Equal(t, cmd.ReplayedUESecCaps, got.ReplayedUESecCaps)
	assert.True(t, got.IMEISV_Requested)
}

// --- Security Mode Complete -------------------------------------------------

func TestSecurityModeCompleteRoundTrip(t *testing.T) {
	msg := &SecurityModeComplete{
		NASPDUContainer: []byte{0x7E, 0x00, 0x43}, // Registration Complete inner
	}
	enc := EncodeSecurityModeComplete(msg)
	assert.Equal(t, uint8(0x5E), enc[2])

	decoded, err := Decode(enc)
	require.NoError(t, err)
	require.NotNil(t, decoded.SecurityModeComplete)
	assert.Equal(t, msg.NASPDUContainer, decoded.SecurityModeComplete.NASPDUContainer)
}

// --- Registration Accept ----------------------------------------------------

func TestRegistrationAcceptRoundTrip(t *testing.T) {
	guti := GUTI5G{
		PLMN:        [3]byte{0x00, 0x01, 0xF1},
		AMFRegionID: 0x01,
		AMFSetID:    0x01,
		AMFPointer:  0x00,
		TMSI5G:      [4]byte{0x00, 0x00, 0x00, 0x01},
	}
	msg := &RegistrationAccept{
		RegistrationResult: 0x01, // 3GPP access
		AssignedGUTI:       &guti,
		AllowedNSSAI:       []NSSAIEntry{{SST: 0x01}},
	}
	enc := EncodeRegistrationAccept(msg)
	assert.Equal(t, uint8(0x42), enc[2])

	decoded, err := Decode(enc)
	require.NoError(t, err)
	require.NotNil(t, decoded.RegistrationAccept)

	got := decoded.RegistrationAccept
	assert.Equal(t, uint8(0x01), got.RegistrationResult)
	require.NotNil(t, got.AssignedGUTI)
	assert.Equal(t, guti.AMFRegionID, got.AssignedGUTI.AMFRegionID)
	assert.Equal(t, guti.TMSI5G, got.AssignedGUTI.TMSI5G)
	require.Len(t, got.AllowedNSSAI, 1)
	assert.Equal(t, uint8(0x01), got.AllowedNSSAI[0].SST)
}

func TestRegistrationAcceptUERANSIMMobileIdentityLengthGolden(t *testing.T) {
	guti := GUTI5G{
		PLMN:        [3]byte{0x00, 0xF1, 0x10},
		AMFRegionID: 0x01,
		AMFSetID:    0x01,
		AMFPointer:  0x00,
		TMSI5G:      [4]byte{0x00, 0x00, 0x00, 0x01},
	}
	msg := &RegistrationAccept{
		RegistrationResult: 0x01,
		AssignedGUTI:       &guti,
		AllowedNSSAI:       []NSSAIEntry{{SST: 0x01}},
	}

	encoded := EncodeRegistrationAccept(msg)
	assert.Equal(t,
		"7e0042010177000bf200f1100100400000000115020101",
		hex.EncodeToString(encoded),
	)

	// Captured before this fix from UERANSIM run 27070827795. UERANSIM aborted
	// with "Bad constructed NAS message" because IEI 0x77 used a one-octet
	// length (0x0b) even though 5GS mobile identity is an IE6 two-octet length.
	rejected, err := hex.DecodeString("7e00420101770bf200f1100100400000000115020101")
	require.NoError(t, err)
	_, err = Decode(rejected)
	require.Error(t, err)
}

// --- Registration Complete --------------------------------------------------

func TestRegistrationCompleteRoundTrip(t *testing.T) {
	enc := EncodeRegistrationComplete()
	assert.Equal(t, []byte{0x7E, 0x00, 0x43}, enc)

	msg, err := Decode(enc)
	require.NoError(t, err)
	assert.Equal(t, MsgTypeRegistrationComplete, msg.Header.MessageType)
}

// --- UL NAS Transport -------------------------------------------------------

func TestULNASTransportUERANSIMFixture(t *testing.T) {
	// Captured after successful UERANSIM registration in run 27073020885.
	// Body shape: payload container type IE1 = 0x01, payload container IE4,
	// PDU session ID IE3 = 0x12 0x01, then additional optional IEs.
	raw, err := hex.DecodeString("7e00670100152e0101c1ffff91a12801007b000780000a00000d00120181220401000001250908696e7465726e6574")
	require.NoError(t, err)
	msg, err := Decode(raw)
	require.NoError(t, err)
	require.Equal(t, MsgTypeULNASTransport, msg.Header.MessageType)

	ul, err := DecodeULNASTransport(raw[3:])
	require.NoError(t, err)
	assert.Equal(t, uint8(1), ul.PDUSessionID)
	assert.Equal(t, "2e0101c1", hex.EncodeToString(ul.PayloadContainer[:4]))
}

// --- TAI List ---------------------------------------------------------------

func TestEncodeTAIList(t *testing.T) {
	plmn := [3]byte{0x00, 0x01, 0xF1}
	tacs := [][3]byte{{0x00, 0x00, 0x01}}
	enc := EncodeTAIList(plmn, tacs)
	// byte 0: type=0, count=0 (1 TAC)
	assert.Equal(t, uint8(0x00), enc[0])
	assert.Equal(t, plmn[:], enc[1:4])
	assert.Equal(t, tacs[0][:], enc[4:7])
}

// --- PDU Session Establishment Accept / DL NAS Transport --------------------

// TestEncodePDUSessionEstablishmentAcceptGolden pins the exact 5GSM Accept bytes
// a real UE (UERANSIM) parses — its NAS-SM decoder is strict and crashes on a
// malformed mandatory IE, so the QoS-rules / Session-AMBR shape must not drift.
func TestEncodePDUSessionEstablishmentAcceptGolden(t *testing.T) {
	got := EncodePDUSessionEstablishmentAccept(0x05, 0x01, [4]byte{10, 45, 0, 2})
	// 2E,psi,pti,C2 | sel-type/ssc(11) | QoS rules LV-E(00 09 …) |
	// Session-AMBR LV(06 …) | PDU address TLV(29 05 01 + IPv4).
	want := "2e0501c211000901000631310101ff01060603e80603e82905010a2d0002"
	assert.Equal(t, want, hex.EncodeToString(got))

	assert.Equal(t, uint8(0x2E), got[0], "5GSM EPD")
	assert.Equal(t, uint8(0xC2), got[3], "PDU Session Establishment Accept message type")
	assert.Equal(t, uint8(0x11), got[4], "selected PDU session type IPv4 + SSC mode 1")
	assert.True(t, bytes.Contains(got, []byte{0x29, 0x05, 0x01, 10, 45, 0, 2}),
		"PDU address IE carries the assigned UE IPv4")
}

// TestEncodePDUSessionEstablishmentAcceptNoIP omits the PDU address IE when no
// IPv4 was allocated (zero value).
func TestEncodePDUSessionEstablishmentAcceptNoIP(t *testing.T) {
	got := EncodePDUSessionEstablishmentAccept(0x05, 0x01, [4]byte{})
	assert.False(t, bytes.Contains(got, []byte{0x29}), "no PDU address IE without an IP")
	assert.Equal(t, uint8(0xC2), got[3])
}

// TestEncodeDLNASTransportShape checks the 5GMM DL NAS Transport wrapper carries
// the N1 SM container with an LV-E length and the PDU session ID IE.
func TestEncodeDLNASTransportShape(t *testing.T) {
	n1 := []byte{0x2E, 0x05, 0x01, 0xC2} // stand-in 5GSM container
	got := EncodeDLNASTransport(0x05, n1)

	assert.Equal(t, []byte{0x7E, 0x00, 0x68}, got[:3], "5GMM DL NAS Transport header")
	assert.Equal(t, uint8(0x01), got[3], "payload container type = N1 SM info")
	assert.Equal(t, []byte{0x00, uint8(len(n1))}, got[4:6], "payload container LV-E length")
	assert.Equal(t, n1, got[6:6+len(n1)], "embedded N1 SM container")
	assert.Equal(t, []byte{0x12, 0x05}, got[len(got)-2:], "trailing PDU session ID IE")
}
