package simulator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEncodeMSINBCD verifies the BCD packing used to build null-scheme SUCI
// MSIN bytes. Format: low nibble = earlier digit, high nibble = later digit.
// Odd-length strings are padded with 0xF in the final high nibble.
func TestEncodeMSINBCD(t *testing.T) {
	cases := []struct {
		msin string
		want []byte
	}{
		// 10-digit MSIN from IMSI 001010000000001 (MCC=001, MNC=01).
		// Digits: 0,0,0,0,0,0,0,0,0,1
		// Pairs (lo|hi): 00, 00, 00, 00, 0|1 → 0x00, 0x00, 0x00, 0x00, 0x10
		{"0000000001", []byte{0x00, 0x00, 0x00, 0x00, 0x10}},

		// All-nines MSIN (unprovisioned_imsi scenario with 10-digit MSIN).
		// Digits: 9,9,9,9,9,9,9,9,9,9
		// Pairs (lo|hi): 9|9 → 0x99 × 5
		{"9999999999", []byte{0x99, 0x99, 0x99, 0x99, 0x99}},

		// Odd-length MSIN (7 digits) — last nibble padded with 0xF.
		// Digits: 1,2,3,4,5,6,7
		// Bytes: 0x21, 0x43, 0x65, 0xF7
		{"1234567", []byte{0x21, 0x43, 0x65, 0xF7}},

		// Empty MSIN edge case.
		{"", []byte{}},

		// Single digit — padded: 0xF5
		{"5", []byte{0xF5}},

		// Two digits — 0x21
		{"12", []byte{0x21}},
	}

	for _, tc := range cases {
		t.Run(tc.msin, func(t *testing.T) {
			got := encodeMSINBCD(tc.msin)
			if len(tc.want) == 0 {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tc.want, got, "encodeMSINBCD(%q)", tc.msin)
			}
		})
	}
}
