package suci

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/qcore-project/qcore/pkg/ident"
)

const (
	SchemeNull     uint8 = 0
	SchemeProfileA uint8 = 1
	SchemeProfileB uint8 = 2

	macLen      = 8
	encKeyLen   = 16
	icbLen      = 16
	macKeyLen   = 32
	totalKeyLen = encKeyLen + icbLen + macKeyLen
)

var (
	ErrMalformed       = errors.New("suci: malformed mobile identity")
	ErrUnsupported     = errors.New("suci: unsupported protection scheme")
	ErrUnknownKey      = errors.New("suci: unknown home network public key id")
	ErrMACVerification = errors.New("suci: MAC verification failed")
)

// MobileIdentity is the IMSI-based SUCI mobile identity IE value from
// TS 24.501 §9.11.3.4 without the LV-E length bytes.
type MobileIdentity struct {
	PLMN             [3]byte
	MCC              string
	MNC              string
	RoutingIndicator [2]byte
	ProtectionScheme uint8
	HomeNetworkPKID  uint8
	SchemeOutput     []byte
}

// HomeNetworkPrivateKey is a configured SIDF de-concealment key selected by
// protection scheme and Home Network Public Key ID.
type HomeNetworkPrivateKey struct {
	ID         uint8
	Scheme     uint8
	PrivateKey *ecdh.PrivateKey
}

// Resolver de-conceals suci-<hex> identifiers to imsi-<digits>.
type Resolver struct {
	keys map[uint16]*ecdh.PrivateKey
}

// ParseMobileIdentity parses an IMSI-based SUCI mobile identity IE value.
func ParseMobileIdentity(data []byte) (MobileIdentity, error) {
	if len(data) < 8 {
		return MobileIdentity{}, fmt.Errorf("%w: SUCI too short: %d", ErrMalformed, len(data))
	}
	if data[0]&0x07 != 0x01 {
		return MobileIdentity{}, fmt.Errorf("%w: identity type 0x%x is not SUCI", ErrMalformed, data[0]&0x07)
	}
	id := MobileIdentity{
		ProtectionScheme: data[6] & 0x0f,
		HomeNetworkPKID:  data[7],
	}
	copy(id.PLMN[:], data[1:4])
	copy(id.RoutingIndicator[:], data[4:6])
	id.MCC, id.MNC = ident.DecodePLMN(id.PLMN)
	if len(data) > 8 {
		id.SchemeOutput = append([]byte(nil), data[8:]...)
	}
	return id, nil
}

// ParsePrivateKeyHex decodes a configured HN private key for an ECIES profile.
func ParsePrivateKeyHex(scheme uint8, privateKeyHex string) (*ecdh.PrivateKey, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(privateKeyHex))
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	switch scheme {
	case SchemeProfileA:
		if len(raw) != 32 {
			return nil, fmt.Errorf("profile A private key must be 32 bytes, got %d", len(raw))
		}
		return ecdh.X25519().NewPrivateKey(raw)
	case SchemeProfileB:
		if len(raw) != 32 {
			return nil, fmt.Errorf("profile B private key must be 32 bytes, got %d", len(raw))
		}
		return ecdh.P256().NewPrivateKey(raw)
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnsupported, scheme)
	}
}

// NewResolver builds a SIDF key resolver.
func NewResolver(keys []HomeNetworkPrivateKey) (*Resolver, error) {
	r := &Resolver{keys: make(map[uint16]*ecdh.PrivateKey, len(keys))}
	for _, key := range keys {
		if key.PrivateKey == nil {
			return nil, fmt.Errorf("SIDF key id %d scheme %d has nil private key", key.ID, key.Scheme)
		}
		switch key.Scheme {
		case SchemeProfileA, SchemeProfileB:
		default:
			return nil, fmt.Errorf("%w: %d", ErrUnsupported, key.Scheme)
		}
		k := keyMapKey(key.Scheme, key.ID)
		if _, exists := r.keys[k]; exists {
			return nil, fmt.Errorf("duplicate SIDF key for scheme %d id %d", key.Scheme, key.ID)
		}
		r.keys[k] = key.PrivateKey
	}
	return r, nil
}

// ResolveSUCIHex resolves a suci-<hex> string or returns imsi- inputs unchanged.
func (r *Resolver) ResolveSUCIHex(supiOrSuci string) (string, error) {
	if strings.HasPrefix(supiOrSuci, "imsi-") {
		return supiOrSuci, nil
	}
	if !strings.HasPrefix(supiOrSuci, "suci-") {
		return "", fmt.Errorf("%w: expected imsi- or suci- identifier", ErrMalformed)
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(supiOrSuci, "suci-"))
	if err != nil {
		return "", fmt.Errorf("%w: SUCI hex decode: %v", ErrMalformed, err)
	}
	id, err := ParseMobileIdentity(raw)
	if err != nil {
		return "", err
	}
	return r.ResolveMobileIdentity(id)
}

// ResolveMobileIdentity returns imsi-<MCC><MNC><MSIN> for null-scheme or
// ECIES-protected IMSI-based SUCI identities.
func (r *Resolver) ResolveMobileIdentity(id MobileIdentity) (string, error) {
	var msinBCD []byte
	switch id.ProtectionScheme {
	case SchemeNull:
		msinBCD = id.SchemeOutput
	case SchemeProfileA, SchemeProfileB:
		if r == nil {
			return "", fmt.Errorf("%w: scheme %d id %d", ErrUnknownKey, id.ProtectionScheme, id.HomeNetworkPKID)
		}
		key := r.keys[keyMapKey(id.ProtectionScheme, id.HomeNetworkPKID)]
		if key == nil {
			return "", fmt.Errorf("%w: scheme %d id %d", ErrUnknownKey, id.ProtectionScheme, id.HomeNetworkPKID)
		}
		var err error
		msinBCD, err = Deconceal(id.ProtectionScheme, key, id.SchemeOutput)
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("%w: %d", ErrUnsupported, id.ProtectionScheme)
	}
	msin, err := DecodeMSINBCD(msinBCD)
	if err != nil {
		return "", err
	}
	return "imsi-" + id.MCC + id.MNC + msin, nil
}

// DecodeMSINBCD decodes the BCD MSIN plaintext block used by TS 33.501 Annex C.
func DecodeMSINBCD(msinBCD []byte) (string, error) {
	if len(msinBCD) == 0 {
		return "", fmt.Errorf("%w: empty MSIN", ErrMalformed)
	}
	var b strings.Builder
	for i, octet := range msinBCD {
		lo := octet & 0x0f
		hi := (octet >> 4) & 0x0f
		if lo > 9 {
			return "", fmt.Errorf("%w: invalid low BCD nibble 0x%x at byte %d", ErrMalformed, lo, i)
		}
		b.WriteByte('0' + lo)
		if hi == 0x0f {
			continue
		}
		if hi > 9 {
			return "", fmt.Errorf("%w: invalid high BCD nibble 0x%x at byte %d", ErrMalformed, hi, i)
		}
		b.WriteByte('0' + hi)
	}
	return b.String(), nil
}

// Deconceal recovers the BCD-encoded MSIN from an ECIES Profile A/B scheme output.
func Deconceal(scheme uint8, hnPriv *ecdh.PrivateKey, schemeOutput []byte) ([]byte, error) {
	if hnPriv == nil {
		return nil, errors.New("suci: nil home network private key")
	}

	var (
		curve       ecdh.Curve
		ephForKDF   []byte
		ephForECDH  []byte
		ciphertext  []byte
		receivedMAC []byte
	)
	switch scheme {
	case SchemeProfileA:
		if len(schemeOutput) <= 32+macLen {
			return nil, fmt.Errorf("%w: profile A scheme output too short: %d", ErrMalformed, len(schemeOutput))
		}
		curve = ecdh.X25519()
		ephForKDF = schemeOutput[:32]
		ephForECDH = ephForKDF
		ciphertext = schemeOutput[32 : len(schemeOutput)-macLen]
		receivedMAC = schemeOutput[len(schemeOutput)-macLen:]
	case SchemeProfileB:
		if len(schemeOutput) <= 33+macLen {
			return nil, fmt.Errorf("%w: profile B scheme output too short: %d", ErrMalformed, len(schemeOutput))
		}
		curve = ecdh.P256()
		ephForKDF = schemeOutput[:33]
		var err error
		ephForECDH, err = p256PublicForECDH(ephForKDF)
		if err != nil {
			return nil, err
		}
		ciphertext = schemeOutput[33 : len(schemeOutput)-macLen]
		receivedMAC = schemeOutput[len(schemeOutput)-macLen:]
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnsupported, scheme)
	}

	ephPub, err := curve.NewPublicKey(ephForECDH)
	if err != nil {
		return nil, fmt.Errorf("parse ephemeral public key: %w", err)
	}
	shared, err := hnPriv.ECDH(ephPub)
	if err != nil {
		return nil, fmt.Errorf("derive shared key: %w", err)
	}

	keyingData := x963KDF(shared, ephForKDF, totalKeyLen)
	encKey := keyingData[:encKeyLen]
	icb := keyingData[encKeyLen : encKeyLen+icbLen]
	macKey := keyingData[encKeyLen+icbLen:]

	mac := hmac.New(sha256.New, macKey)
	_, _ = mac.Write(ciphertext)
	expectedMAC := mac.Sum(nil)[:macLen]
	if !hmac.Equal(expectedMAC, receivedMAC) {
		return nil, ErrMACVerification
	}

	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCTR(block, icb).XORKeyStream(plaintext, ciphertext)
	return plaintext, nil
}

func x963KDF(shared, sharedInfo []byte, length int) []byte {
	var out []byte
	var counter [4]byte
	for i := uint32(1); len(out) < length; i++ {
		binary.BigEndian.PutUint32(counter[:], i)
		h := sha256.New()
		_, _ = h.Write(shared)
		_, _ = h.Write(counter[:])
		_, _ = h.Write(sharedInfo)
		out = append(out, h.Sum(nil)...)
	}
	return out[:length]
}

func p256PublicForECDH(public []byte) ([]byte, error) {
	switch len(public) {
	case 33:
		x, y := elliptic.UnmarshalCompressed(elliptic.P256(), public)
		if x == nil {
			return nil, fmt.Errorf("%w: invalid compressed P-256 public key", ErrMalformed)
		}
		return elliptic.Marshal(elliptic.P256(), x, y), nil
	case 65:
		x, _ := elliptic.Unmarshal(elliptic.P256(), public)
		if x == nil {
			return nil, fmt.Errorf("%w: invalid uncompressed P-256 public key", ErrMalformed)
		}
		return public, nil
	default:
		return nil, fmt.Errorf("%w: P-256 public key length %d", ErrMalformed, len(public))
	}
}

func keyMapKey(scheme, id uint8) uint16 {
	return uint16(scheme)<<8 | uint16(id)
}
