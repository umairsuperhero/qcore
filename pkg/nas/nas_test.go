package nas

import (
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
