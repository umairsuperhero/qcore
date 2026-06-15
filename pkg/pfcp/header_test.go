package pfcp

import (
	"bytes"
	"testing"
)

func TestEncodeDecodeHeader(t *testing.T) {
	tests := []struct {
		name string
		h    Header
	}{
		{
			name: "Heartbeat Request (No SEID)",
			h: Header{
				Version:        1,
				SEIDPresent:    false,
				MessageType:    MsgTypeHeartbeatRequest,
				MessageLength:  12, // 4 (seq) + 8 (payload)
				SequenceNumber: 12345,
			},
		},
		{
			name: "Session Establishment Request (With SEID)",
			h: Header{
				Version:        1,
				SEIDPresent:    true,
				MessageType:    MsgTypeSessionEstablishmentRequest,
				MessageLength:  20, // 4 (seq) + 8 (SEID) + 8 (payload)
				SEID:           0x1122334455667788,
				SequenceNumber: 54321,
			},
		},
		{
			name: "With Message Priority",
			h: Header{
				Version:         1,
				SEIDPresent:     true,
				MessagePriority: true,
				MessageType:     MsgTypeSessionModificationRequest,
				MessageLength:   20,
				SEID:            0x8877665544332211,
				SequenceNumber:  999,
				Priority:        5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := EncodeHeader(tt.h)

			decoded, offset, err := DecodeHeader(b)
			if err != nil {
				t.Fatalf("DecodeHeader failed: %v", err)
			}

			expectedOffset := 8
			if tt.h.SEIDPresent {
				expectedOffset = 16
			}
			if offset != expectedOffset {
				t.Errorf("Expected offset %d, got %d", expectedOffset, offset)
			}

			if decoded != tt.h {
				t.Errorf("Decoded header doesn't match original.\nGot:  %+v\nWant: %+v", decoded, tt.h)
			}

			// Verify exact bytes for the first case
			if tt.name == "Heartbeat Request (No SEID)" {
				expectedBytes := []byte{
					0x20,       // Version 1, Spare 0, FO 0, MP 0, S 0
					0x01,       // MsgTypeHeartbeatRequest
					0x00, 0x0c, // Length 12
					0x00, 0x30, 0x39, // Seq 12345
					0x00, // Spare + MP
				}
				if !bytes.Equal(b, expectedBytes) {
					t.Errorf("Byte mismatch.\nGot:  %x\nWant: %x", b, expectedBytes)
				}
			}
		})
	}
}

func TestDecodeHeaderErrors(t *testing.T) {
	// Too small
	_, _, err := DecodeHeader([]byte{0x20, 0x01})
	if err != ErrBufferTooSmall {
		t.Errorf("Expected ErrBufferTooSmall, got %v", err)
	}

	// Invalid version
	_, _, err = DecodeHeader([]byte{
		0x40, // Version 2
		0x01,
		0x00, 0x0c,
		0x00, 0x30, 0x39,
		0x00,
	})
	if err != ErrInvalidVersion {
		t.Errorf("Expected ErrInvalidVersion, got %v", err)
	}

	// Truncated with SEID
	b := []byte{
		0x21,       // Version 1, SEID present
		0x32,       // Session Estab Req
		0x00, 0x14, // Length 20
		0x11, 0x22, 0x33, 0x44, // Only 4 bytes of SEID
	}
	_, _, err = DecodeHeader(b)
	if err != ErrBufferTooSmall {
		t.Errorf("Expected ErrBufferTooSmall, got %v", err)
	}
}
