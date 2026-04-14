package nas

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAESCMAC_RFC4493(t *testing.T) {
	// RFC 4493 test vectors
	key, _ := hex.DecodeString("2b7e151628aed2a6abf7158809cf4f3c")

	tests := []struct {
		name    string
		msg     string
		want    string
	}{
		{
			name: "empty message",
			msg:  "",
			want: "bb1d6929e95937287fa37d129b756746",
		},
		{
			name: "16 bytes",
			msg:  "6bc1bee22e409f96e93d7e117393172a",
			want: "070a16b46b4d4144f79bdd9dd04a287c",
		},
		{
			name: "40 bytes",
			msg:  "6bc1bee22e409f96e93d7e117393172aae2d8a571e03ac9c9eb76fac45af8e5130c81c46a35ce411",
			want: "dfa66747de9ae63030ca32611497c827",
		},
		{
			name: "64 bytes",
			msg:  "6bc1bee22e409f96e93d7e117393172aae2d8a571e03ac9c9eb76fac45af8e5130c81c46a35ce411e5fbc1191a0a52eff69f2445df4f9b17ad2b417be66c3710",
			want: "51f0bebf7e3b9d92fc49741779363cfe",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg, _ := hex.DecodeString(tc.msg)
			mac, err := AESCMAC(key, msg)
			require.NoError(t, err)
			assert.Equal(t, tc.want, hex.EncodeToString(mac))
		})
	}
}

func TestDeriveKNASenc(t *testing.T) {
	// Use a known KASME (32 bytes)
	kasme := make([]byte, 32)
	for i := range kasme {
		kasme[i] = byte(i)
	}

	key, err := DeriveKNASenc(kasme, 2) // EEA2
	require.NoError(t, err)
	assert.Len(t, key, 16)

	// Verify deterministic
	key2, err := DeriveKNASenc(kasme, 2)
	require.NoError(t, err)
	assert.Equal(t, key, key2)

	// Different algorithm produces different key
	key3, err := DeriveKNASenc(kasme, 0) // EEA0
	require.NoError(t, err)
	assert.NotEqual(t, key, key3)
}

func TestDeriveKNASint(t *testing.T) {
	kasme := make([]byte, 32)
	for i := range kasme {
		kasme[i] = byte(i)
	}

	key, err := DeriveKNASint(kasme, 2) // EIA2
	require.NoError(t, err)
	assert.Len(t, key, 16)

	// int key should differ from enc key
	encKey, _ := DeriveKNASenc(kasme, 2)
	assert.NotEqual(t, key, encKey)
}

func TestDeriveKeNB(t *testing.T) {
	kasme := make([]byte, 32)
	for i := range kasme {
		kasme[i] = byte(i)
	}

	key, err := DeriveKeNB(kasme, 0)
	require.NoError(t, err)
	assert.Len(t, key, 32) // KeNB is 256 bits

	// Different NAS COUNT produces different key
	key2, err := DeriveKeNB(kasme, 1)
	require.NoError(t, err)
	assert.NotEqual(t, key, key2)
}

func TestNASIntegrityProtect(t *testing.T) {
	kNASint := make([]byte, 16)
	for i := range kNASint {
		kNASint[i] = byte(i + 1)
	}

	message := []byte("hello NAS world")
	mac, err := NASIntegrityProtect(kNASint, 0, 0, 0, message)
	require.NoError(t, err)
	assert.Len(t, mac, 4)

	// Same inputs produce same MAC
	mac2, err := NASIntegrityProtect(kNASint, 0, 0, 0, message)
	require.NoError(t, err)
	assert.Equal(t, mac, mac2)

	// Different count produces different MAC
	mac3, err := NASIntegrityProtect(kNASint, 1, 0, 0, message)
	require.NoError(t, err)
	assert.NotEqual(t, mac, mac3)
}

func TestKASMELengthValidation(t *testing.T) {
	shortKASME := make([]byte, 16) // too short

	_, err := DeriveKNASenc(shortKASME, 0)
	assert.Error(t, err)

	_, err = DeriveKNASint(shortKASME, 0)
	assert.Error(t, err)

	_, err = DeriveKeNB(shortKASME, 0)
	assert.Error(t, err)
}
