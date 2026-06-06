package amf

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/qcore-project/qcore/pkg/nas"
)

// 5G NAS key derivation per 3GPP TS 33.501 Annex A.
// kdf is HMAC-SHA-256(key, S) where S = FC || params.

func kdf(key, s []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(s)
	return mac.Sum(nil)
}

// DeriveKAMF derives KAMF from KSEAF. TS 33.501 Annex A.7.
//
//	FC = 0x6D
//	P0 = SUPI (UTF-8 string), L0 = len(SUPI)
//	P1 = ABBA parameter, L1 = len(ABBA)
func DeriveKAMF(kseaf []byte, supi string, abba []byte) ([]byte, error) {
	if len(kseaf) != 32 {
		return nil, fmt.Errorf("amf: KSEAF must be 32 bytes, got %d", len(kseaf))
	}
	supiBytes := []byte(supi)
	s := make([]byte, 0, 1+len(supiBytes)+2+len(abba)+2)
	s = append(s, 0x6D) // FC
	s = append(s, supiBytes...)
	s = appendLen(s, len(supiBytes))
	s = append(s, abba...)
	s = appendLen(s, len(abba))
	return kdf(kseaf, s), nil
}

// DeriveKNASint5G derives the 5G NAS integrity key from KAMF.
// TS 33.501 Annex A.8: FC=0x69, P0=alg-type=0x02 (NAS-int), P1=alg-ID.
func DeriveKNASint5G(kamf []byte, algID uint8) ([]byte, error) {
	return deriveNASKey5G(kamf, 0x02, algID)
}

// DeriveKNASenc5G derives the 5G NAS encryption key from KAMF.
// TS 33.501 Annex A.8: FC=0x69, P0=alg-type=0x01 (NAS-enc), P1=alg-ID.
func DeriveKNASenc5G(kamf []byte, algID uint8) ([]byte, error) {
	return deriveNASKey5G(kamf, 0x01, algID)
}

func deriveNASKey5G(kamf []byte, algType, algID uint8) ([]byte, error) {
	if len(kamf) != 32 {
		return nil, fmt.Errorf("amf: KAMF must be 32 bytes, got %d", len(kamf))
	}
	// S = FC(1) || P0(1) || L0(2) || P1(1) || L1(2)
	s := []byte{0x69, algType, 0x00, 0x01, algID, 0x00, 0x01}
	derived := kdf(kamf, s)
	return derived[16:], nil // last 16 bytes = 128-bit NAS key
}

// DeriveKgNB derives KgNB from KAMF. TS 33.501 Annex A.9.
//
//	FC = 0x6E
//	P0 = uplink NAS COUNT (4 bytes), L0 = 0x0004
//	P1 = access type (1 byte: 0x01 = 3GPP access), L1 = 0x0001
func DeriveKgNB(kamf []byte, ulNASCount uint32) ([]byte, error) {
	if len(kamf) != 32 {
		return nil, fmt.Errorf("amf: KAMF must be 32 bytes, got %d", len(kamf))
	}
	countBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(countBytes, ulNASCount)
	s := make([]byte, 0, 10)
	s = append(s, 0x6E) // FC
	s = append(s, countBytes...)
	s = appendLen(s, 4)
	s = append(s, 0x01) // 3GPP access
	s = appendLen(s, 1)
	return kdf(kamf, s), nil
}

func appendLen(s []byte, l int) []byte {
	return append(s, uint8(l>>8), uint8(l))
}

// WrapNAS5G wraps a plain 5G NAS PDU with integrity protection.
// Uses NIA2 (AES-CMAC) with the given KNASint.
// secHdrType: SecurityHeaderIntegrityProtectedNewCtx (0x03) for SMC,
//             SecurityHeaderIntegrityProtected (0x01) for subsequent messages.
func WrapNAS5G(kNASint []byte, dlCount uint32, secHdrType nas5gSecHdr, plainNAS []byte) ([]byte, error) {
	sn := uint8(dlCount & 0xFF)

	// MAC input: [SN][plain NAS] — same pattern as 4G (TS 24.501 §4.4.4)
	macInput := make([]byte, 1+len(plainNAS))
	macInput[0] = sn
	copy(macInput[1:], plainNAS)

	// direction=1 (downlink), bearer=1 (NAS bearer per TS 33.501)
	mac, err := nas.NASIntegrityProtect(kNASint, dlCount, 1, 1, macInput)
	if err != nil {
		return nil, fmt.Errorf("amf: NAS MAC: %w", err)
	}

	// 5G NAS security header: EPD(0x7E) | SecHdr(1B) | MAC(4B) | SN(1B) | plainNAS
	result := make([]byte, 0, 7+len(plainNAS))
	result = append(result, 0x7E)         // EPD = 5GMM
	result = append(result, uint8(secHdrType))
	result = append(result, mac...)       // 4-byte MAC
	result = append(result, sn)
	result = append(result, plainNAS...)
	return result, nil
}

// nas5gSecHdr is the security header type for 5G NAS.
type nas5gSecHdr uint8

const (
	SecHdrIntegrityProtected       nas5gSecHdr = 0x01
	SecHdrIntegrityProtectedNewCtx nas5gSecHdr = 0x03
)

// VerifyNAS5GUplink verifies the MAC on an uplink integrity-protected 5G NAS PDU.
// rawNAS starts at byte 0 of the security-protected NAS PDU.
// Returns the inner plain NAS PDU on success.
func VerifyNAS5GUplink(kNASint []byte, ulCount uint32, rawNAS []byte) ([]byte, error) {
	if len(rawNAS) < 7 {
		return nil, fmt.Errorf("amf: security-protected NAS too short: %d", len(rawNAS))
	}
	// [EPD=0x7E][SecHdr][MAC 4B][SN][inner NAS...]
	receivedMAC := rawNAS[2:6]
	sn := rawNAS[6]
	innerNAS := rawNAS[7:]

	macInput := make([]byte, 1+len(innerNAS))
	macInput[0] = sn
	copy(macInput[1:], innerNAS)

	// direction=0 (uplink), bearer=1
	expectedMAC, err := nas.NASIntegrityProtect(kNASint, ulCount, 1, 0, macInput)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 4; i++ {
		if receivedMAC[i] != expectedMAC[i] {
			return nil, fmt.Errorf("amf: NAS MAC mismatch")
		}
	}
	return innerNAS, nil
}
