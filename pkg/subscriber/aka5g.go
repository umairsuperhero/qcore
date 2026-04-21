package subscriber

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// 5G-AKA derivations per 3GPP TS 33.501 Annex A. Builds on top of the
// Milenage primitives in milenage.go: Milenage still produces RAND, AUTN,
// RES, CK, IK (all unchanged from 4G), but 5G wraps those with two extra
// KDF steps so the vector is bound to the serving-network-name and to a
// fresh long-term key (KAUSF) that lives at the AUSF.
//
//   RES* / XRES* (Annex A.4) — SN-Name-bound transform of RES
//   KAUSF        (Annex A.2) — long-term key derived from CK||IK and
//                              SQN⊕AK, scoped to the serving network
//
// QCore's pkg/subscriber now generates both the 4G EPS-AKA vector and
// the 5G vector from the same Milenage core. This file is only the 5G
// side; the 4G side stays in milenage.go.

// AuthVector5G is the 5G authentication vector the UDM returns from
// Nudm_UEAU generate-auth-data. All fields are hex-encoded.
//
//	RAND     — 128-bit challenge (same RAND the UE sees)
//	AUTN     — 128-bit network authentication token
//	XRESStar — 128-bit (not 64!) expected response, SN-bound
//	KAUSF    — 256-bit long-term key anchored at the AUSF
type AuthVector5G struct {
	RAND     string `json:"rand"`
	AUTN     string `json:"autn"`
	XRESStar string `json:"xresStar"`
	KAUSF    string `json:"kausf"`
}

// DeriveKAUSF — TS 33.501 Annex A.2.
//
//	FC = 0x6A
//	P0 = SN name (UTF-8), L0 = len(P0)
//	P1 = SQN XOR AK (6 bytes), L1 = 0x0006
//	Key = CK || IK
//	KAUSF = HMAC-SHA-256(Key, FC || P0 || L0 || P1 || L1)
func DeriveKAUSF(ck, ik [16]byte, snName string, sqnXorAk [6]byte) [32]byte {
	var key [32]byte
	copy(key[0:16], ck[:])
	copy(key[16:32], ik[:])

	sn := []byte(snName)
	s := make([]byte, 0, 1+len(sn)+2+6+2)
	s = append(s, 0x6A)
	s = append(s, sn...)
	s = append(s, byte(len(sn)>>8), byte(len(sn)&0xFF))
	s = append(s, sqnXorAk[:]...)
	s = append(s, 0x00, 0x06)

	mac := hmac.New(sha256.New, key[:])
	mac.Write(s)
	var out [32]byte
	copy(out[:], mac.Sum(nil))
	return out
}

// DeriveKSEAF — TS 33.501 Annex A.6. AUSF derives KSEAF from KAUSF and
// hands it to the SEAF (AMF) on successful auth. This is the anchor key
// for the rest of the NAS/AS key hierarchy.
//
//	FC = 0x6C
//	P0 = SN name (UTF-8), L0 = len(P0)
//	Key = KAUSF (32 bytes)
//	KSEAF = HMAC-SHA-256(KAUSF, FC || P0 || L0)
func DeriveKSEAF(kausf [32]byte, snName string) [32]byte {
	sn := []byte(snName)
	s := make([]byte, 0, 1+len(sn)+2)
	s = append(s, 0x6C)
	s = append(s, sn...)
	s = append(s, byte(len(sn)>>8), byte(len(sn)&0xFF))

	mac := hmac.New(sha256.New, kausf[:])
	mac.Write(s)
	var out [32]byte
	copy(out[:], mac.Sum(nil))
	return out
}

// DeriveHXRESStar — TS 33.501 Annex A.5. AUSF-side compression of the
// XRES* it got from UDM, to hand to AMF without leaking the full value.
//
//	HXRES* = SHA-256(RAND || XRES*)[16:32]   -- the low-order 128 bits
//
// AMF compares HXRES*(RAND, RES*) from the UE against this value.
func DeriveHXRESStar(randVal, xresStar [16]byte) [16]byte {
	h := sha256.New()
	h.Write(randVal[:])
	h.Write(xresStar[:])
	full := h.Sum(nil)
	var out [16]byte
	copy(out[:], full[16:32])
	return out
}

// DeriveRESStar — TS 33.501 Annex A.4. Same construction on both sides:
// the UE calls it with its own RES and gets RES*; the home network calls
// it with XRES and gets XRES*.
//
//	FC = 0x6B
//	P0 = SN name (UTF-8), L0 = len(P0)
//	P1 = RAND (16 bytes), L1 = 0x0010
//	P2 = RES or XRES (8 bytes), L2 = 0x0008
//	Key = CK || IK
//	OUT = HMAC-SHA-256(Key, FC || P0 || L0 || P1 || L1 || P2 || L2)
//	RES* = OUT[16:32]  -- the low-order 128 bits
func DeriveRESStar(ck, ik [16]byte, snName string, randVal [16]byte, res [8]byte) [16]byte {
	var key [32]byte
	copy(key[0:16], ck[:])
	copy(key[16:32], ik[:])

	sn := []byte(snName)
	s := make([]byte, 0, 1+len(sn)+2+16+2+8+2)
	s = append(s, 0x6B)
	s = append(s, sn...)
	s = append(s, byte(len(sn)>>8), byte(len(sn)&0xFF))
	s = append(s, randVal[:]...)
	s = append(s, 0x00, 0x10)
	s = append(s, res[:]...)
	s = append(s, 0x00, 0x08)

	mac := hmac.New(sha256.New, key[:])
	mac.Write(s)
	full := mac.Sum(nil)
	var out [16]byte
	copy(out[:], full[16:32])
	return out
}

// Generate5GAuthVector produces a fresh 5G-AKA vector for (supi, SN-Name).
// Fresh RAND is drawn from crypto/rand; use Generate5GAuthVectorWithRAND
// for deterministic tests.
func Generate5GAuthVector(k, opc [16]byte, sqn [6]byte, amf [2]byte, snName string) (*AuthVector5G, error) {
	var randVal [16]byte
	if _, err := rand.Read(randVal[:]); err != nil {
		return nil, fmt.Errorf("generating RAND: %w", err)
	}
	return Generate5GAuthVectorWithRAND(k, opc, randVal, sqn, amf, snName)
}

// Generate5GAuthVectorWithRAND is the deterministic variant used by tests.
func Generate5GAuthVectorWithRAND(k, opc, randVal [16]byte, sqn [6]byte, amf [2]byte, snName string) (*AuthVector5G, error) {
	res, ck, ik, ak, err := F2345(k, opc, randVal)
	if err != nil {
		return nil, fmt.Errorf("computing F2345: %w", err)
	}

	macA, _, err := F1(k, opc, randVal, sqn, amf)
	if err != nil {
		return nil, fmt.Errorf("computing F1: %w", err)
	}

	var autn [16]byte
	var sqnXorAk [6]byte
	for i := 0; i < 6; i++ {
		sqnXorAk[i] = sqn[i] ^ ak[i]
		autn[i] = sqnXorAk[i]
	}
	copy(autn[6:8], amf[:])
	copy(autn[8:16], macA[:])

	xresStar := DeriveRESStar(ck, ik, snName, randVal, res)
	kausf := DeriveKAUSF(ck, ik, snName, sqnXorAk)

	return &AuthVector5G{
		RAND:     hex.EncodeToString(randVal[:]),
		AUTN:     hex.EncodeToString(autn[:]),
		XRESStar: hex.EncodeToString(xresStar[:]),
		KAUSF:    hex.EncodeToString(kausf[:]),
	}, nil
}
