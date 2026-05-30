// Package ident provides canonical encoders/decoders for 3GPP identifiers.
// All implementations follow the cited 3GPP spec section and are validated by
// golden test vectors — not just internal round-trips.
package ident

import "fmt"

// EncodePLMN encodes an MCC and MNC string pair into the 3-byte PLMN identity
// format defined in 3GPP TS 24.008 §10.5.1.13 and TS 23.003.
//
// Wire layout (nibble = 4 bits):
//
//	Byte 0: MCC digit 2 (high) | MCC digit 1 (low)
//	Byte 1: MNC digit 3 (high) | MCC digit 3 (low)
//	Byte 2: MNC digit 2 (high) | MNC digit 1 (low)
//
// For a 2-digit MNC, MNC digit 3 is set to 0xF (the standard filler sentinel).
//
// Golden vector: MCC="001", MNC="01"  →  [0x00, 0xF1, 0x10]
// Golden vector: MCC="001", MNC="001" →  [0x00, 0x11, 0x00]
func EncodePLMN(mcc, mnc string) [3]byte {
	mccD := [3]uint8{}
	for i := 0; i < 3 && i < len(mcc); i++ {
		mccD[i] = mcc[i] - '0' // MCC digit 1,2,3
	}
	// Initialise MNC digits to 0xF (filler); overwrite with actual digits.
	mncD := [3]uint8{0xF, 0xF, 0xF}
	for i := 0; i < len(mnc) && i < 3; i++ {
		mncD[i] = mnc[i] - '0' // MNC digit 1,2,3
	}
	return [3]byte{
		(mccD[1] << 4) | mccD[0], // byte 0: MCC2 | MCC1
		(mncD[2] << 4) | mccD[2], // byte 1: MNC3 | MCC3  (MNC3=0xF for 2-digit)
		(mncD[1] << 4) | mncD[0], // byte 2: MNC2 | MNC1
	}
}

// DecodePLMN reverses EncodePLMN. Returns a 3-digit MCC string and a 2- or
// 3-digit MNC string (2-digit when MNC digit 3 == 0xF).
func DecodePLMN(b [3]byte) (mcc, mnc string) {
	mcc1 := b[0] & 0x0F
	mcc2 := (b[0] >> 4) & 0x0F
	mcc3 := b[1] & 0x0F
	mnc3 := (b[1] >> 4) & 0x0F
	mnc1 := b[2] & 0x0F
	mnc2 := (b[2] >> 4) & 0x0F

	mcc = fmt.Sprintf("%d%d%d", mcc1, mcc2, mcc3)
	if mnc3 == 0xF {
		mnc = fmt.Sprintf("%d%d", mnc1, mnc2)
	} else {
		mnc = fmt.Sprintf("%d%d%d", mnc1, mnc2, mnc3)
	}
	return
}

// ParsePLMNString parses a 5- or 6-digit PLMN string (MCC+MNC concatenated,
// e.g. "00101" for MCC=001/MNC=01, or "001001" for MCC=001/MNC=001) into the
// 3-byte BCD representation defined by TS 24.008 §10.5.1.13.
func ParsePLMNString(plmn string) ([3]byte, error) {
	if len(plmn) != 5 && len(plmn) != 6 {
		return [3]byte{}, fmt.Errorf("ident: PLMN must be 5 or 6 digits (MCC+MNC), got %q", plmn)
	}
	for _, ch := range plmn {
		if ch < '0' || ch > '9' {
			return [3]byte{}, fmt.Errorf("ident: PLMN must contain only digits, got %q", plmn)
		}
	}
	return EncodePLMN(plmn[:3], plmn[3:]), nil
}
