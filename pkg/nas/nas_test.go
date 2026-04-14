package nas

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIMSIRoundTrip(t *testing.T) {
	tests := []struct {
		imsi string
	}{
		{"001010000000001"},
		{"310260000000001"},
		{"123456789012345"},
	}

	for _, tc := range tests {
		t.Run(tc.imsi, func(t *testing.T) {
			encoded, err := EncodeIMSI(tc.imsi)
			require.NoError(t, err)

			decoded, err := DecodeIMSI(encoded)
			require.NoError(t, err)
			assert.Equal(t, tc.imsi, decoded)
		})
	}
}

func TestParseHeaderPlainNAS(t *testing.T) {
	// Plain NAS EMM Attach Request
	data := []byte{0x07, 0x41} // PD=EMM(0x07), security=plain(0), type=AttachRequest(0x41)
	h, off, err := ParseHeader(data)
	require.NoError(t, err)

	assert.Equal(t, SecurityHeaderPlainNAS, h.SecurityHeader)
	assert.Equal(t, EPSMobilityManagement, h.Protocol)
	assert.Equal(t, MsgTypeAttachRequest, h.MessageType)
	assert.Equal(t, 2, off)
}

func TestDecodeAttachRequest(t *testing.T) {
	// Build a minimal Attach Request body (after header)
	imsi := "001010000000001"
	encodedIMSI, err := EncodeIMSI(imsi)
	require.NoError(t, err)

	body := make([]byte, 0, 32)
	// NAS key set ID (high nibble=0x07 native) | attach type (low nibble=1 EPS attach)
	body = append(body, 0x71) // KSI=7 (no key), attach type=1

	// EPS mobile identity (LV)
	body = append(body, uint8(len(encodedIMSI)))
	body = append(body, encodedIMSI...)

	// UE network capability (LV) — 2 bytes for simplicity
	body = append(body, 0x02, 0xE0, 0xE0)

	// ESM message container (LV-E) — empty
	body = append(body, 0x00, 0x00)

	req, err := DecodeAttachRequest(body)
	require.NoError(t, err)

	assert.Equal(t, uint8(1), req.AttachType)
	assert.Equal(t, uint8(7), req.NASKeySetIdentifier)
	assert.Equal(t, imsi, req.IMSI)
	assert.Equal(t, []byte{0xE0, 0xE0}, req.UENetworkCapability)
}

func TestAuthenticationRequestEncode(t *testing.T) {
	req := &AuthenticationRequest{
		NASKeySetIdentifier: 0,
		RAND:                [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		AUTN:                [16]byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
	}

	encoded, err := EncodeAuthenticationRequest(req)
	require.NoError(t, err)

	// Verify header
	assert.Equal(t, uint8(0x07), encoded[0]) // plain NAS + EMM
	assert.Equal(t, uint8(0x52), encoded[1]) // Authentication Request
	assert.Equal(t, uint8(0x00), encoded[2]) // KSI=0

	// RAND at offset 3
	assert.Equal(t, req.RAND[:], encoded[3:19])

	// AUTN length + value at offset 19
	assert.Equal(t, uint8(16), encoded[19])
	assert.Equal(t, req.AUTN[:], encoded[20:36])
}

func TestAuthenticationResponseDecode(t *testing.T) {
	// RES = 8 bytes
	body := []byte{0x08, 0xA5, 0x42, 0x11, 0xD5, 0xE3, 0xBA, 0x50, 0xBF}
	resp, err := DecodeAuthenticationResponse(body)
	require.NoError(t, err)

	assert.Len(t, resp.RES, 8)
	assert.Equal(t, []byte{0xA5, 0x42, 0x11, 0xD5, 0xE3, 0xBA, 0x50, 0xBF}, resp.RES)
}

func TestSecurityModeCommandEncode(t *testing.T) {
	cmd := &SecurityModeCommand{
		SelectedNASSecAlg:   0x20, // EEA0 + EIA2
		NASKeySetIdentifier: 0,
		ReplayedUESecCap:    []byte{0xE0, 0xE0},
	}

	encoded, err := EncodeSecurityModeCommand(cmd)
	require.NoError(t, err)

	assert.Equal(t, uint8(0x07), encoded[0]) // plain NAS + EMM
	assert.Equal(t, uint8(0x5D), encoded[1]) // Security Mode Command
	assert.Equal(t, uint8(0x20), encoded[2]) // selected algorithms
	assert.Equal(t, uint8(0x00), encoded[3]) // KSI
	assert.Equal(t, uint8(0x02), encoded[4]) // UE sec cap length
	assert.Equal(t, []byte{0xE0, 0xE0}, encoded[5:7])
}

func TestVerifyAuthResponse(t *testing.T) {
	res := []byte{0xA5, 0x42, 0x11, 0xD5, 0xE3, 0xBA, 0x50, 0xBF}
	xres := []byte{0xA5, 0x42, 0x11, 0xD5, 0xE3, 0xBA, 0x50, 0xBF}
	assert.True(t, VerifyAuthResponse(res, xres))

	badRes := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	assert.False(t, VerifyAuthResponse(badRes, xres))

	assert.False(t, VerifyAuthResponse([]byte{0x01}, xres)) // different length
}

func TestMessageTypeStrings(t *testing.T) {
	assert.Equal(t, "AttachRequest", MsgTypeAttachRequest.String())
	assert.Equal(t, "AuthenticationRequest", MsgTypeAuthenticationRequest.String())
	assert.Equal(t, "SecurityModeCommand", MsgTypeSecurityModeCommand.String())
}

// --- ParseHeader with security-protected NAS ---

func TestParseHeaderSecurityProtected(t *testing.T) {
	// Build a security-protected NAS wrapping a SECURITY MODE COMPLETE.
	// Layout: [sec_type|PD] [MAC 4 bytes] [SN] [inner plain NAS: PD msg_type]
	inner := []byte{
		uint8(SecurityHeaderPlainNAS<<4) | uint8(EPSMobilityManagement), // 0x07
		uint8(MsgTypeSecurityModeComplete),                               // 0x5E
	}
	pdu := make([]byte, 0, 8)
	pdu = append(pdu, uint8(SecurityHeaderIntegrityProtectedNewCtx<<4)|uint8(EPSMobilityManagement)) // 0x37
	pdu = append(pdu, 0x01, 0x02, 0x03, 0x04) // MAC (fake)
	pdu = append(pdu, 0x00)                    // SN
	pdu = append(pdu, inner...)

	h, off, err := ParseHeader(pdu)
	require.NoError(t, err)
	assert.Equal(t, MsgTypeSecurityModeComplete, h.MessageType)
	assert.Equal(t, EPSMobilityManagement, h.Protocol)
	assert.Equal(t, SecurityHeaderIntegrityProtectedNewCtx, h.SecurityHeader)
	// Body starts after: 1 (outer) + 4 (MAC) + 1 (SN) + 2 (inner header) = 8
	assert.Equal(t, 8, off)
}

// --- EncodeAttachAccept ---

func TestEncodeAttachAccept(t *testing.T) {
	plmn := [3]byte{0x00, 0xF1, 0x10}
	pdn := net.ParseIP("10.45.0.2")
	require.NotNil(t, pdn)

	msg, err := EncodeAttachAccept(plmn, 0x0001, 5, "internet", pdn)
	require.NoError(t, err)
	require.Greater(t, len(msg), 10)

	// Header
	assert.Equal(t, uint8(0x07), msg[0]) // plain NAS + EMM PD
	assert.Equal(t, uint8(MsgTypeAttachAccept), msg[1])

	// EPS attach result = 1 (EPS only)
	assert.Equal(t, uint8(0x01), msg[2])

	// T3412 timer = 0x21 (1 hour)
	assert.Equal(t, uint8(0x21), msg[3])
}

func TestEncodeAttachAccept_IPv6Rejected(t *testing.T) {
	plmn := [3]byte{0x00, 0xF1, 0x10}
	pdn := net.ParseIP("2001:db8::1") // IPv6 should be rejected
	_, err := EncodeAttachAccept(plmn, 0x0001, 5, "internet", pdn)
	assert.Error(t, err)
}

func TestEncodeAPN(t *testing.T) {
	// "internet" → [0x08][i n t e r n e t]
	out := encodeAPN("internet")
	assert.Equal(t, []byte{0x08, 'i', 'n', 't', 'e', 'r', 'n', 'e', 't'}, out)

	// "foo.bar" → [0x03 f o o][0x03 b a r]
	out2 := encodeAPN("foo.bar")
	assert.Equal(t, []byte{0x03, 'f', 'o', 'o', 0x03, 'b', 'a', 'r'}, out2)
}

// --- WrapNASWithIntegrity ---

func TestWrapNASWithIntegrity(t *testing.T) {
	kNASint := make([]byte, 16)
	for i := range kNASint {
		kNASint[i] = byte(i + 1)
	}
	inner := []byte{0x07, 0x5D, 0x02, 0x00, 0x02, 0xE0, 0xE0} // SEC MODE CMD

	wrapped, err := WrapNASWithIntegrity(kNASint, 0, SecurityHeaderIntegrityProtectedNewCtx, inner)
	require.NoError(t, err)

	// Check outer header byte
	assert.Equal(t, uint8(SecurityHeaderIntegrityProtectedNewCtx<<4)|uint8(EPSMobilityManagement), wrapped[0])
	// MAC = 4 bytes at [1:5]
	assert.Len(t, wrapped[1:5], 4)
	// SN = 0 at [5]
	assert.Equal(t, uint8(0), wrapped[5])
	// Inner NAS starts at [6]
	assert.Equal(t, inner, wrapped[6:])
}

func TestWrapNASWithIntegrity_DeterministicMAC(t *testing.T) {
	kNASint := make([]byte, 16)
	for i := range kNASint {
		kNASint[i] = byte(i + 5)
	}
	inner := []byte{0x07, 0x42, 0x01} // Fake ATTACH ACCEPT

	w1, err := WrapNASWithIntegrity(kNASint, 1, SecurityHeaderIntegrityProtectedCiphered, inner)
	require.NoError(t, err)
	w2, err := WrapNASWithIntegrity(kNASint, 1, SecurityHeaderIntegrityProtectedCiphered, inner)
	require.NoError(t, err)
	assert.Equal(t, w1, w2, "same inputs must produce same MAC")

	// Different count → different MAC
	w3, err := WrapNASWithIntegrity(kNASint, 2, SecurityHeaderIntegrityProtectedCiphered, inner)
	require.NoError(t, err)
	assert.NotEqual(t, w1[1:5], w3[1:5], "different count must produce different MAC")
}

func TestWrapNASWithIntegrity_ParseableByParseHeader(t *testing.T) {
	kNASint := make([]byte, 16)
	for i := range kNASint {
		kNASint[i] = byte(i + 1)
	}
	inner := []byte{
		uint8(SecurityHeaderPlainNAS<<4) | uint8(EPSMobilityManagement),
		uint8(MsgTypeSecurityModeComplete),
	}

	wrapped, err := WrapNASWithIntegrity(kNASint, 0, SecurityHeaderIntegrityProtectedNewCtx, inner)
	require.NoError(t, err)

	h, off, err := ParseHeader(wrapped)
	require.NoError(t, err)
	assert.Equal(t, MsgTypeSecurityModeComplete, h.MessageType)
	assert.Equal(t, 8, off) // body offset past both headers
}
