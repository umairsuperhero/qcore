package subscriber

import (
	"crypto/aes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Milenage rotation constants (3GPP TS 35.206 Section 3)
const (
	r1 = 64
	r2 = 0
	r3 = 32
	r4 = 64
	r5 = 96
)

// Milenage XOR constants (3GPP TS 35.206 Section 3)
var (
	c1 = [16]byte{}
	c2 = [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	c3 = [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
	c4 = [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4}
	c5 = [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8}
)

type AuthVector struct {
	RAND  string `json:"rand"`
	XRES  string `json:"xres"`
	AUTN  string `json:"autn"`
	KASME string `json:"kasme"`
}

// GenerateOPc computes OPc from OP and K per 3GPP TS 35.206.
// OPc = AES_K(OP) XOR OP
func GenerateOPc(k, op [16]byte) ([16]byte, error) {
	cipher, err := aes.NewCipher(k[:])
	if err != nil {
		return [16]byte{}, fmt.Errorf("creating AES cipher: %w", err)
	}
	var encrypted [16]byte
	cipher.Encrypt(encrypted[:], op[:])
	return xor128(encrypted, op), nil
}

// F1 computes network authentication code MAC-A and resync MAC-S.
// Per 3GPP TS 35.206 Section 4.1.
func F1(k, opc, randVal [16]byte, sqn [6]byte, amf [2]byte) (macA [8]byte, macS [8]byte, err error) {
	cipher, err := aes.NewCipher(k[:])
	if err != nil {
		return macA, macS, fmt.Errorf("creating AES cipher: %w", err)
	}

	// temp = AES_K(RAND XOR OPc)
	var temp [16]byte
	rijndaelInput := xor128(randVal, opc)
	cipher.Encrypt(temp[:], rijndaelInput[:])

	// Build in1: SQN || AMF || SQN || AMF
	var in1 [16]byte
	copy(in1[0:6], sqn[:])
	copy(in1[6:8], amf[:])
	copy(in1[8:14], sqn[:])
	copy(in1[14:16], amf[:])

	// out = AES_K(rotate(in1 XOR OPc, r1) XOR c1 XOR temp) XOR OPc
	x := xor128(in1, opc)
	x = rotate(x, r1)
	x = xor128(x, c1)
	x = xor128(x, temp)

	var out [16]byte
	cipher.Encrypt(out[:], x[:])
	out = xor128(out, opc)

	copy(macA[:], out[0:8])
	copy(macS[:], out[8:16])
	return macA, macS, nil
}

// F2345 computes RES, CK, IK, AK in parallel.
// Per 3GPP TS 35.206 Section 4.1.
func F2345(k, opc, randVal [16]byte) (res [8]byte, ck, ik [16]byte, ak [6]byte, err error) {
	cipher, err := aes.NewCipher(k[:])
	if err != nil {
		return res, ck, ik, ak, fmt.Errorf("creating AES cipher: %w", err)
	}

	// temp = AES_K(RAND XOR OPc)
	var temp [16]byte
	rijndaelInput := xor128(randVal, opc)
	cipher.Encrypt(temp[:], rijndaelInput[:])

	// f2 and f5: out = AES_K(rotate(temp XOR OPc, r2) XOR c2) XOR OPc
	x := xor128(temp, opc)
	x = rotate(x, r2)
	x = xor128(x, c2)
	var out [16]byte
	cipher.Encrypt(out[:], x[:])
	out = xor128(out, opc)
	copy(res[:], out[8:16]) // f2: RES = last 8 bytes
	copy(ak[:], out[0:6])   // f5: AK = first 6 bytes

	// f3 (CK): out = AES_K(rotate(temp XOR OPc, r3) XOR c3) XOR OPc
	x = xor128(temp, opc)
	x = rotate(x, r3)
	x = xor128(x, c3)
	cipher.Encrypt(ck[:], x[:])
	ck = xor128(ck, opc)

	// f4 (IK): out = AES_K(rotate(temp XOR OPc, r4) XOR c4) XOR OPc
	x = xor128(temp, opc)
	x = rotate(x, r4)
	x = xor128(x, c4)
	cipher.Encrypt(ik[:], x[:])
	ik = xor128(ik, opc)

	return res, ck, ik, ak, nil
}

// F5Star computes AK for resynchronization.
// Per 3GPP TS 35.206 Section 4.1.
func F5Star(k, opc, randVal [16]byte) (ak [6]byte, err error) {
	cipher, err := aes.NewCipher(k[:])
	if err != nil {
		return ak, fmt.Errorf("creating AES cipher: %w", err)
	}

	var temp [16]byte
	rijndaelInput := xor128(randVal, opc)
	cipher.Encrypt(temp[:], rijndaelInput[:])

	x := xor128(temp, opc)
	x = rotate(x, r5)
	x = xor128(x, c5)

	var out [16]byte
	cipher.Encrypt(out[:], x[:])
	out = xor128(out, opc)
	copy(ak[:], out[0:6])

	return ak, nil
}

// DeriveKASME derives the access security management entity key per 3GPP TS 33.401 Annex A.2.
// Key = CK || IK
// S = FC(0x10) || SN_ID(3 bytes) || 0x0003 || SQN_XOR_AK(6 bytes) || 0x0006
// KASME = HMAC-SHA-256(Key, S)
func DeriveKASME(ck, ik [16]byte, plmnID [3]byte, sqn [6]byte, ak [6]byte) [32]byte {
	// Key = CK || IK
	var key [32]byte
	copy(key[0:16], ck[:])
	copy(key[16:32], ik[:])

	// SQN XOR AK
	var sqnXorAK [6]byte
	for i := 0; i < 6; i++ {
		sqnXorAK[i] = sqn[i] ^ ak[i]
	}

	// S = FC || P0 || L0 || P1 || L1
	s := make([]byte, 0, 14)
	s = append(s, 0x10)           // FC
	s = append(s, plmnID[:]...)   // P0: SN ID (3 bytes)
	s = append(s, 0x00, 0x03)     // L0: length of SN ID
	s = append(s, sqnXorAK[:]...) // P1: SQN XOR AK (6 bytes)
	s = append(s, 0x00, 0x06)     // L1: length of SQN XOR AK

	mac := hmac.New(sha256.New, key[:])
	mac.Write(s)
	var kasme [32]byte
	copy(kasme[:], mac.Sum(nil))
	return kasme
}

// GenerateAuthVector generates a complete LTE authentication vector.
func GenerateAuthVector(k, opc [16]byte, sqn [6]byte, amf [2]byte, plmnID [3]byte) (*AuthVector, error) {
	// Generate random RAND
	var randVal [16]byte
	if _, err := rand.Read(randVal[:]); err != nil {
		return nil, fmt.Errorf("generating RAND: %w", err)
	}

	return GenerateAuthVectorWithRAND(k, opc, randVal, sqn, amf, plmnID)
}

// GenerateAuthVectorWithRAND generates an auth vector with a specific RAND (for testing).
func GenerateAuthVectorWithRAND(k, opc, randVal [16]byte, sqn [6]byte, amf [2]byte, plmnID [3]byte) (*AuthVector, error) {
	// Compute RES, CK, IK, AK
	res, ck, ik, ak, err := F2345(k, opc, randVal)
	if err != nil {
		return nil, fmt.Errorf("computing F2345: %w", err)
	}

	// Compute MAC-A
	macA, _, err := F1(k, opc, randVal, sqn, amf)
	if err != nil {
		return nil, fmt.Errorf("computing F1: %w", err)
	}

	// Construct AUTN = (SQN XOR AK) || AMF || MAC-A
	var autn [16]byte
	for i := 0; i < 6; i++ {
		autn[i] = sqn[i] ^ ak[i]
	}
	copy(autn[6:8], amf[:])
	copy(autn[8:16], macA[:])

	// Derive KASME
	kasme := DeriveKASME(ck, ik, plmnID, sqn, ak)

	return &AuthVector{
		RAND:  hex.EncodeToString(randVal[:]),
		XRES:  hex.EncodeToString(res[:]),
		AUTN:  hex.EncodeToString(autn[:]),
		KASME: hex.EncodeToString(kasme[:]),
	}, nil
}

// xor128 XORs two 128-bit (16-byte) blocks.
func xor128(a, b [16]byte) [16]byte {
	var result [16]byte
	for i := 0; i < 16; i++ {
		result[i] = a[i] ^ b[i]
	}
	return result
}

// rotate performs cyclic left rotation of a 128-bit block by n bits.
func rotate(input [16]byte, n int) [16]byte {
	if n == 0 {
		return input
	}
	n = n % 128
	byteShift := n / 8
	bitShift := uint(n % 8)

	var output [16]byte
	for i := 0; i < 16; i++ {
		srcIdx := (i + byteShift) % 16
		nextIdx := (i + byteShift + 1) % 16
		if bitShift == 0 {
			output[i] = input[srcIdx]
		} else {
			output[i] = (input[srcIdx] << bitShift) | (input[nextIdx] >> (8 - bitShift))
		}
	}
	return output
}
