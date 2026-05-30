package ident

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPLMNGoldenVectors verifies EncodePLMN against values independently
// derived from 3GPP TS 24.008 §10.5.1.13 and cross-checked against the
// default UERANSIM / open5GS test configurations. Internal round-trip tests
// are necessary but not sufficient — these vectors catch byte-ordering bugs
// that a round-trip test cannot detect.
//
// TS 24.008 §10.5.1.13 wire layout reminder:
//   Byte 0: MCC digit 2 (high) | MCC digit 1 (low)
//   Byte 1: MNC digit 3 (high) | MCC digit 3 (low)   [MNC3=0xF for 2-digit MNC]
//   Byte 2: MNC digit 2 (high) | MNC digit 1 (low)
func TestPLMNGoldenVectors(t *testing.T) {
	cases := []struct {
		name string
		mcc  string
		mnc  string
		want [3]byte
	}{
		// Vector 1: UERANSIM / 3GPP default test network (2-digit MNC).
		// MCC=001 MNC=01 → byte0: 00|00=0x00; byte1: F|1=0xF1; byte2: 1|0=0x10
		{
			name: "MCC=001 MNC=01 (UERANSIM default, 2-digit)",
			mcc:  "001", mnc: "01",
			want: [3]byte{0x00, 0xF1, 0x10},
		},
		// Vector 2: 3-digit MNC test case.
		// MCC=001 MNC=001 → byte0: 0x00; byte1: MNC3=1, MCC3=1 → 0x11; byte2: MNC2=0, MNC1=0 → 0x00
		{
			name: "MCC=001 MNC=001 (3-digit MNC)",
			mcc:  "001", mnc: "001",
			want: [3]byte{0x00, 0x11, 0x00},
		},
		// Vector 3: Deutsche Telekom Germany (2-digit MNC).
		// MCC=262 MNC=01 → byte0: 6|2=0x62; byte1: F|2=0xF2; byte2: 1|0=0x10
		{
			name: "MCC=262 MNC=01 (Deutsche Telekom DE, 2-digit)",
			mcc:  "262", mnc: "01",
			want: [3]byte{0x62, 0xF2, 0x10},
		},
		// Vector 4: AT&T USA (3-digit MNC).
		// MCC=310 MNC=410 → byte0: 1|3=0x13; byte1: MNC3=0, MCC3=0 → 0x00; byte2: MNC2=1, MNC1=4 → 0x14
		{
			name: "MCC=310 MNC=410 (AT&T USA, 3-digit)",
			mcc:  "310", mnc: "410",
			want: [3]byte{0x13, 0x00, 0x14},
		},
		// Vector 5: Free Mobile France (2-digit MNC).
		// MCC=208 MNC=15 → byte0: 0|2=0x02; byte1: F|8=0xF8; byte2: 5|1=0x51
		{
			name: "MCC=208 MNC=15 (Free Mobile FR, 2-digit)",
			mcc:  "208", mnc: "15",
			want: [3]byte{0x02, 0xF8, 0x51},
		},
		// Vector 6: Vodafone India (2-digit MNC).
		// MCC=404 MNC=20: MNC1=2, MNC2=0 → byte2: MNC2(high)|MNC1(low) = 0<<4|2 = 0x02
		// (Note: 0x02 not 0x20 — low nibble is MNC1=2, high nibble is MNC2=0)
		{
			name: "MCC=404 MNC=20 (Vodafone India, 2-digit)",
			mcc:  "404", mnc: "20",
			want: [3]byte{0x04, 0xF4, 0x02},
		},
		// Vector 7: UK test (3-digit MNC, per TS 24.008 §10.5.1.13 example region).
		// MCC=234 MNC=015 → byte0: 3|2=0x32; byte1: MNC3=5, MCC3=4 → 0x54; byte2: MNC2=1, MNC1=0 → 0x10
		{
			name: "MCC=234 MNC=015 (3-digit MNC example)",
			mcc:  "234", mnc: "015",
			want: [3]byte{0x32, 0x54, 0x10},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EncodePLMN(tc.mcc, tc.mnc)
			assert.Equal(t, tc.want, got, "EncodePLMN(%q, %q)", tc.mcc, tc.mnc)
		})
	}
}

// TestPLMNRoundTrip verifies that DecodePLMN(EncodePLMN(mcc, mnc)) == (mcc, mnc)
// for both 2-digit and 3-digit MNCs.
func TestPLMNRoundTrip(t *testing.T) {
	cases := []struct{ mcc, mnc string }{
		{"001", "01"},
		{"001", "001"},
		{"262", "01"},
		{"310", "410"},
		{"208", "15"},
		{"404", "20"},
		{"234", "015"},
	}
	for _, tc := range cases {
		t.Run(tc.mcc+"/"+tc.mnc, func(t *testing.T) {
			enc := EncodePLMN(tc.mcc, tc.mnc)
			gotMCC, gotMNC := DecodePLMN(enc)
			assert.Equal(t, tc.mcc, gotMCC, "MCC")
			assert.Equal(t, tc.mnc, gotMNC, "MNC")
		})
	}
}

// TestParsePLMNString verifies the combined-string parser against the same golden vectors.
func TestParsePLMNString(t *testing.T) {
	cases := []struct {
		in   string
		want [3]byte
	}{
		{"00101", [3]byte{0x00, 0xF1, 0x10}},
		{"001001", [3]byte{0x00, 0x11, 0x00}},
		{"26201", [3]byte{0x62, 0xF2, 0x10}},
		{"310410", [3]byte{0x13, 0x00, 0x14}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParsePLMNString(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestParsePLMNString_Errors verifies invalid inputs are rejected.
func TestParsePLMNString_Errors(t *testing.T) {
	for _, bad := range []string{"001", "0010", "0010101", "00X01", ""} {
		_, err := ParsePLMNString(bad)
		assert.Error(t, err, "expected error for %q", bad)
	}
}
