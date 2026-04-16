package nas

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// NAS key derivation per 3GPP TS 33.401 Annex A.
// All KDF functions use HMAC-SHA-256 with KASME as key and a specific S parameter.
// However, 3GPP actually specifies a simpler KDF based on AES for NAS keys:
// KDF(key, FC, P0, L0, P1, L1, ...) where FC is the function code.

// DeriveKNASenc derives the NAS encryption key from KASME.
// TS 33.401 Annex A.7: FC=0x15, P0=algorithm type distinguisher (0x01 for NAS-enc),
// P1=algorithm identity (0=EEA0, 1=EEA1, 2=EEA2)
func DeriveKNASenc(kasme []byte, algID uint8) ([]byte, error) {
	return deriveNASKey(kasme, 0x01, algID)
}

// DeriveKNASint derives the NAS integrity key from KASME.
// TS 33.401 Annex A.7: FC=0x15, P0=algorithm type distinguisher (0x02 for NAS-int),
// P1=algorithm identity (0=EIA0, 1=EIA1, 2=EIA2)
func DeriveKNASint(kasme []byte, algID uint8) ([]byte, error) {
	return deriveNASKey(kasme, 0x02, algID)
}

// DeriveKeNB derives the eNodeB key from KASME and uplink NAS COUNT.
// TS 33.401 Annex A.3: FC=0x11, P0=uplink NAS COUNT (4 bytes)
func DeriveKeNB(kasme []byte, ulNASCount uint32) ([]byte, error) {
	if len(kasme) != 32 {
		return nil, fmt.Errorf("KASME must be 32 bytes, got %d", len(kasme))
	}

	// S = FC || P0 || L0
	s := make([]byte, 0, 7)
	s = append(s, 0x11) // FC
	countBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(countBytes, ulNASCount)
	s = append(s, countBytes...) // P0
	s = append(s, 0x00, 0x04)   // L0 = 4

	return kdf(kasme, s), nil
}

// deriveNASKey implements the common NAS key derivation.
func deriveNASKey(kasme []byte, algTypeDist, algID uint8) ([]byte, error) {
	if len(kasme) != 32 {
		return nil, fmt.Errorf("KASME must be 32 bytes, got %d", len(kasme))
	}

	// S = FC || P0 || L0 || P1 || L1
	// FC = 0x15
	// P0 = algorithm type distinguisher (1 byte), L0 = 0x0001
	// P1 = algorithm identity (1 byte), L1 = 0x0001
	s := make([]byte, 0, 7)
	s = append(s, 0x15)               // FC
	s = append(s, algTypeDist)         // P0
	s = append(s, 0x00, 0x01)         // L0
	s = append(s, algID)              // P1
	s = append(s, 0x00, 0x01)         // L1

	derived := kdf(kasme, s)
	// Return last 16 bytes as the key
	return derived[16:], nil
}

// kdf implements the 3GPP KDF (Key Derivation Function) per TS 33.220 Annex B.
// output = HMAC-SHA-256(key, S)
func kdf(key, s []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(s)
	return mac.Sum(nil)
}

// AES-CMAC implementation for EIA2 (NAS integrity protection).
// Per NIST SP 800-38B / RFC 4493.

// AESCMAC computes AES-CMAC over the given message with the given key.
func AESCMAC(key, message []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("AES-CMAC key error: %w", err)
	}

	// Generate subkeys
	k1, k2 := generateSubkeys(block)

	// Number of blocks
	n := (len(message) + aes.BlockSize - 1) / aes.BlockSize
	if n == 0 {
		n = 1
	}

	var lastBlock [aes.BlockSize]byte
	isComplete := len(message) > 0 && len(message)%aes.BlockSize == 0

	if isComplete {
		// XOR last block with K1
		copy(lastBlock[:], message[len(message)-aes.BlockSize:])
		xorBlock(&lastBlock, k1)
	} else {
		// Pad and XOR with K2
		remaining := len(message) % aes.BlockSize
		if remaining == 0 && len(message) == 0 {
			remaining = 0
		}
		start := (n - 1) * aes.BlockSize
		if start < len(message) {
			copy(lastBlock[:], message[start:])
		}
		lastBlock[len(message)-start] = 0x80 // padding
		xorBlock(&lastBlock, k2)
	}

	// CBC-MAC
	var x [aes.BlockSize]byte
	for i := 0; i < n-1; i++ {
		var blk [aes.BlockSize]byte
		copy(blk[:], message[i*aes.BlockSize:])
		xorBlock(&x, blk)
		block.Encrypt(x[:], x[:])
	}
	xorBlock(&x, lastBlock)
	block.Encrypt(x[:], x[:])

	return x[:], nil
}

func generateSubkeys(block cipher.Block) (k1, k2 [aes.BlockSize]byte) {
	var zero [aes.BlockSize]byte
	var l [aes.BlockSize]byte
	block.Encrypt(l[:], zero[:])

	k1 = shiftLeft(l)
	if l[0]&0x80 != 0 {
		k1[aes.BlockSize-1] ^= 0x87 // Rb for AES-128
	}

	k2 = shiftLeft(k1)
	if k1[0]&0x80 != 0 {
		k2[aes.BlockSize-1] ^= 0x87
	}

	return
}

func shiftLeft(input [aes.BlockSize]byte) [aes.BlockSize]byte {
	var output [aes.BlockSize]byte
	for i := 0; i < aes.BlockSize-1; i++ {
		output[i] = (input[i] << 1) | (input[i+1] >> 7)
	}
	output[aes.BlockSize-1] = input[aes.BlockSize-1] << 1
	return output
}

func xorBlock(dst *[aes.BlockSize]byte, src [aes.BlockSize]byte) {
	for i := range dst {
		dst[i] ^= src[i]
	}
}

// WrapNASWithIntegrity adds a NAS security header with MAC to a plain NAS PDU.
// Use SecurityHeaderIntegrityProtectedNewCtx (3) for SECURITY MODE COMMAND
// (first message with new security context) and SecurityHeaderIntegrityProtectedCiphered (2)
// for subsequent messages once the security context is active.
// With EEA0 (null cipher), ciphering is a no-op so only integrity protection applies.
func WrapNASWithIntegrity(kNASint []byte, count uint32, secHeaderType SecurityHeaderType, plainNAS []byte) ([]byte, error) {
	sn := uint8(count & 0xFF) // Sequence Number = NAS COUNT mod 256

	// MAC input is [SN][plain NAS] per TS 24.301 §4.4.4.3
	macInput := make([]byte, 1+len(plainNAS))
	macInput[0] = sn
	copy(macInput[1:], plainNAS)

	// direction = 1 (downlink), bearer = 0 (NAS)
	mac, err := NASIntegrityProtect(kNASint, count, 0, 1, macInput)
	if err != nil {
		return nil, fmt.Errorf("computing NAS MAC: %w", err)
	}

	// Build: [sec_type|PD][MAC 4 bytes][SN][plain NAS...]
	result := make([]byte, 0, 6+len(plainNAS))
	result = append(result, uint8(secHeaderType<<4)|uint8(EPSMobilityManagement))
	result = append(result, mac...)
	result = append(result, sn)
	result = append(result, plainNAS...)
	return result, nil
}

// NASIntegrityProtect computes NAS integrity (EIA2 = AES-CMAC based).
// Input: integrity key, count, bearer, direction, message
// Output: 4-byte MAC
func NASIntegrityProtect(kNASint []byte, count uint32, bearer uint8, direction uint8, message []byte) ([]byte, error) {
	// Build the input per TS 33.401 Annex B.2:
	// COUNT (4 bytes) || BEARER (5 bits) || DIRECTION (1 bit) || 0..0 (26 bits) || MESSAGE
	header := make([]byte, 8)
	binary.BigEndian.PutUint32(header[0:4], count)
	header[4] = (bearer << 3) | (direction << 2) // bearer(5 bits) | direction(1 bit) | spare(2 bits)
	// bytes 5-7 are zero (remaining spare bits)

	input := append(header, message...)

	mac, err := AESCMAC(kNASint, input)
	if err != nil {
		return nil, err
	}

	// Return first 4 bytes as NAS-MAC
	return mac[:4], nil
}

// VerifyNASUplinkIntegrity verifies the MAC on an uplink NAS PDU that carries
// a security header. rawNAS is the complete security-protected NAS PDU (starting
// with the security header byte). ulCount is the expected uplink NAS COUNT.
//
// Wire format (TS 24.301 §4.4.4): [sec_hdr|PD][MAC 4B][SN][inner NAS...]
// MAC is computed over [SN][inner NAS] with COUNT, bearer=0, direction=0 (UL).
func VerifyNASUplinkIntegrity(kNASint []byte, ulCount uint32, rawNAS []byte) (bool, error) {
	if len(rawNAS) < 6 {
		return false, fmt.Errorf("security-protected NAS too short: %d bytes", len(rawNAS))
	}
	// Byte 0: security header | PD
	// Bytes 1-4: received MAC
	// Byte 5: SN (NAS COUNT low byte)
	// Bytes 6+: inner NAS PDU
	receivedMAC := rawNAS[1:5]
	sn := rawNAS[5]
	innerNAS := rawNAS[6:]

	// The MAC covers [SN || inner NAS]
	macInput := make([]byte, 1+len(innerNAS))
	macInput[0] = sn
	copy(macInput[1:], innerNAS)

	expectedMAC, err := NASIntegrityProtect(kNASint, ulCount, 0, 0, macInput) // direction=0 (uplink)
	if err != nil {
		return false, fmt.Errorf("computing expected UL MAC: %w", err)
	}

	if len(receivedMAC) != 4 || len(expectedMAC) != 4 {
		return false, fmt.Errorf("MAC length mismatch")
	}
	match := true
	for i := 0; i < 4; i++ {
		if receivedMAC[i] != expectedMAC[i] {
			match = false
		}
	}
	return match, nil
}
