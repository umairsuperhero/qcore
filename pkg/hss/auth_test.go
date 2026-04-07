package hss

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hexDecode16(t *testing.T, s string) [16]byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	require.Len(t, b, 16)
	var result [16]byte
	copy(result[:], b)
	return result
}

func hexDecode6(t *testing.T, s string) [6]byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	require.Len(t, b, 6)
	var result [6]byte
	copy(result[:], b)
	return result
}

func hexDecode2(t *testing.T, s string) [2]byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	require.Len(t, b, 2)
	var result [2]byte
	copy(result[:], b)
	return result
}

// Official 3GPP TS 35.208 test vectors for Milenage
// Source: ETSI TS 135 208 V10.0.0, Section 4.3
type milenageTestSet struct {
	name    string
	k       string // Ki
	op      string // OP
	opc     string // OPc (derived from K and OP)
	rand    string
	sqn     string
	amf     string
	f1MACA  string
	f1SMACs string // f1*
	f2RES   string
	f3CK    string
	f4IK    string
	f5AK    string
	f5sAK   string // f5*
}

var officialTestSets = []milenageTestSet{
	{
		name:    "Test Set 1",
		k:       "465b5ce8b199b49faa5f0a2ee238a6bc",
		op:      "cdc202d5123e20f62b6d676ac72cb318",
		opc:     "cd63cb71954a9f4e48a5994e37a02baf",
		rand:    "23553cbe9637a89d218ae64dae47bf35",
		sqn:     "ff9bb4d0b607",
		amf:     "b9b9",
		f1MACA:  "4a9ffac354dfafb3",
		f1SMACs: "01cfaf9ec4e871e9",
		f2RES:   "a54211d5e3ba50bf",
		f3CK:    "b40ba9a3c58b2a05bbf0d987b21bf8cb",
		f4IK:    "f769bcd751044604127672711c6d3441",
		f5AK:    "aa689c648370",
		f5sAK:   "451e8beca43b",
	},
	{
		name:    "Test Set 3",
		k:       "fec86ba6eb707ed08905757b1bb44b8f",
		op:      "dbc59adcb6f9a0ef735477b7fadf8374",
		opc:     "1006020f0a478bf6b699f15c062e42b3",
		rand:    "9f7c8d021accf4db213ccff0c7f71a6a",
		sqn:     "9d0277595ffc",
		amf:     "725c",
		f1MACA:  "9cabc3e99baf7281",
		f1SMACs: "95814ba2b3044324",
		f2RES:   "8011c48c0c214ed2",
		f3CK:    "5dbdbb2954e8f3cde665b046179a5098",
		f4IK:    "59a92d3b476a0443487055cf88b2307b",
		f5AK:    "33484dc2136b",
		f5sAK:   "deacdd848cc6",
	},
	{
		name:    "Test Set 4",
		k:       "9e5944aea94b81165c82fbf9f32db751",
		op:      "223014c5806694c007ca1eeef57f004f",
		opc:     "a64a507ae1a2a98bb88eb4210135dc87",
		rand:    "ce83dbc54ac0274a157c17f80d017bd6",
		sqn:     "0b604a81eca8",
		amf:     "9e09",
		f1MACA:  "74a58220cba84c49",
		f1SMACs: "ac2cc74a96871837",
		f2RES:   "f365cd683cd92e96",
		f3CK:    "e203edb3971574f5a94b0d61b816345d",
		f4IK:    "0c4524adeac041c4dd830d20854fc46b",
		f5AK:    "f0b9c08ad02e",
		f5sAK:   "6085a86c6f63",
	},
	{
		name:    "Test Set 5",
		k:       "4ab1deb05ca6ceb051fc98e77d026a84",
		op:      "2d16c5cd1fdf6b22383584e3bef2a8d8",
		opc:     "dcf07cbd51855290b92a07a9891e523e",
		rand:    "74b0cd6031a1c8339b2b6ce2b8c4a186",
		sqn:     "e880a1b580b6",
		amf:     "9f07",
		f1MACA:  "49e785dd12626ef2",
		f1SMACs: "9e85790336bb3fa2",
		f2RES:   "5860fc1bce351e7e",
		f3CK:    "7657766b373d1c2138f307e3de9242f9",
		f4IK:    "1c42e960d89b8fa99f2744e0708ccb53",
		f5AK:    "31e11a609118",
		f5sAK:   "fe2555e54aa9",
	},
	{
		name:    "Test Set 6",
		k:       "6c38a116ac280c454f59332ee35c8c4f",
		op:      "1ba00a1a7c6700ac8c3ff3e96ad08725",
		opc:     "3803ef5363b947c6aaa225e58fae3934",
		rand:    "ee6466bc96202c5a557abbeff8babf63",
		sqn:     "414b98222181",
		amf:     "4464",
		f1MACA:  "078adfb488241a57",
		f1SMACs: "80246b8d0186bcf1",
		f2RES:   "16c8233f05a0ac28",
		f3CK:    "3f8c7587fe8e4b233af676aede30ba3b",
		f4IK:    "a7466cc1e6b2a1337d49d3b66e95d7b4",
		f5AK:    "45b0f69ab06c",
		f5sAK:   "1f53cd2b1113",
	},
}

func TestGenerateOPc(t *testing.T) {
	for _, tt := range officialTestSets {
		t.Run(tt.name, func(t *testing.T) {
			k := hexDecode16(t, tt.k)
			op := hexDecode16(t, tt.op)
			got, err := GenerateOPc(k, op)
			require.NoError(t, err)
			assert.Equal(t, tt.opc, hex.EncodeToString(got[:]))
		})
	}
}

func TestF2345(t *testing.T) {
	for _, tt := range officialTestSets {
		t.Run(tt.name, func(t *testing.T) {
			k := hexDecode16(t, tt.k)
			opc := hexDecode16(t, tt.opc)
			rand := hexDecode16(t, tt.rand)

			res, ck, ik, ak, err := F2345(k, opc, rand)
			require.NoError(t, err)

			assert.Equal(t, tt.f2RES, hex.EncodeToString(res[:]), "RES mismatch")
			assert.Equal(t, tt.f3CK, hex.EncodeToString(ck[:]), "CK mismatch")
			assert.Equal(t, tt.f4IK, hex.EncodeToString(ik[:]), "IK mismatch")
			assert.Equal(t, tt.f5AK, hex.EncodeToString(ak[:]), "AK mismatch")
		})
	}
}

func TestF1(t *testing.T) {
	for _, tt := range officialTestSets {
		t.Run(tt.name, func(t *testing.T) {
			k := hexDecode16(t, tt.k)
			opc := hexDecode16(t, tt.opc)
			rand := hexDecode16(t, tt.rand)
			sqn := hexDecode6(t, tt.sqn)
			amf := hexDecode2(t, tt.amf)

			macA, macS, err := F1(k, opc, rand, sqn, amf)
			require.NoError(t, err)

			assert.Equal(t, tt.f1MACA, hex.EncodeToString(macA[:]), "MAC-A mismatch")
			assert.Equal(t, tt.f1SMACs, hex.EncodeToString(macS[:]), "MAC-S mismatch")
		})
	}
}

func TestF5Star(t *testing.T) {
	for _, tt := range officialTestSets {
		t.Run(tt.name, func(t *testing.T) {
			k := hexDecode16(t, tt.k)
			opc := hexDecode16(t, tt.opc)
			rand := hexDecode16(t, tt.rand)

			ak, err := F5Star(k, opc, rand)
			require.NoError(t, err)

			assert.Equal(t, tt.f5sAK, hex.EncodeToString(ak[:]), "AK* mismatch")
		})
	}
}

func TestGenerateAuthVector(t *testing.T) {
	k := hexDecode16(t, "465b5ce8b199b49faa5f0a2ee238a6bc")
	opc := hexDecode16(t, "cd63cb71954a9f4e48a5994e37a02baf")
	sqn := hexDecode6(t, "ff9bb4d0b607")
	amf := hexDecode2(t, "8000")
	plmnID := [3]byte{0x00, 0x01, 0x01}

	av, err := GenerateAuthVector(k, opc, sqn, amf, plmnID)
	require.NoError(t, err)
	require.NotNil(t, av)

	assert.Len(t, av.RAND, 32)
	assert.Len(t, av.XRES, 16)
	assert.Len(t, av.AUTN, 32)
	assert.Len(t, av.KASME, 64)
}

func TestGenerateAuthVectorDeterministic(t *testing.T) {
	k := hexDecode16(t, "465b5ce8b199b49faa5f0a2ee238a6bc")
	opc := hexDecode16(t, "cd63cb71954a9f4e48a5994e37a02baf")
	rand := hexDecode16(t, "23553cbe9637a89d218ae64dae47bf35")
	sqn := hexDecode6(t, "ff9bb4d0b607")
	amf := hexDecode2(t, "8000")
	plmnID := [3]byte{0x00, 0x01, 0x01}

	av1, err := GenerateAuthVectorWithRAND(k, opc, rand, sqn, amf, plmnID)
	require.NoError(t, err)

	av2, err := GenerateAuthVectorWithRAND(k, opc, rand, sqn, amf, plmnID)
	require.NoError(t, err)

	assert.Equal(t, av1.XRES, av2.XRES, "XRES should be deterministic")
	assert.Equal(t, av1.AUTN, av2.AUTN, "AUTN should be deterministic")
	assert.Equal(t, av1.KASME, av2.KASME, "KASME should be deterministic")
}

func TestRotate(t *testing.T) {
	input := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	assert.Equal(t, input, rotate(input, 0))

	rotated := rotate(input, 64)
	assert.Equal(t, byte(9), rotated[0])
	assert.Equal(t, byte(1), rotated[8])
}

func TestXor128(t *testing.T) {
	a := [16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	b := [16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	result := xor128(a, b)
	assert.Equal(t, [16]byte{}, result, "XOR of identical values should be zero")
}
