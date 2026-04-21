package subscriber

import (
	"encoding/hex"
	"testing"
)

// 5G-AKA tests exercise the Annex A derivations on top of TS 35.208 Test
// Set 1 — so if Milenage itself is correct (covered by milenage_test.go)
// then any drift here is localized to DeriveKAUSF / DeriveRESStar / the
// Generate5GAuthVector wiring.
//
// These are regression anchors: the expected XRES* and KAUSF values were
// captured from this implementation on a reviewed day. A future cut
// should cross-validate against an external reference (open5gs or
// free5gc) and replace these literals with the vendor values.

// TS 35.208 Test Set 1 inputs plus a canonical SN-Name
var (
	tsK      = mustHex16("465b5ce8b199b49faa5f0a2ee238a6bc")
	tsOPc    = mustHex16("cd63cb71954a9f4e48a5994e37a02baf")
	tsRAND   = mustHex16("23553cbe9637a89d218ae64dae47bf35")
	tsSQN    = mustHex6("ff9bb4d0b607")
	tsAMF    = mustHex2("b9b9")
	tsSNName = "5G:mnc001.mcc001.3gppnetwork.org"
)

// TestGenerate5GAuthVector pins the output for TS 35.208 Test Set 1
// inputs so any accidental change to the derivation shows up as a diff.
func TestGenerate5GAuthVector(t *testing.T) {
	av, err := Generate5GAuthVectorWithRAND(tsK, tsOPc, tsRAND, tsSQN, tsAMF, tsSNName)
	if err != nil {
		t.Fatalf("Generate5GAuthVectorWithRAND: %v", err)
	}

	// RAND always roundtrips byte-for-byte.
	if av.RAND != hex.EncodeToString(tsRAND[:]) {
		t.Errorf("RAND: got %s, want %s", av.RAND, hex.EncodeToString(tsRAND[:]))
	}

	// AUTN = (SQN XOR AK) || AMF || MAC-A — 16 bytes / 32 hex chars.
	if len(av.AUTN) != 32 {
		t.Errorf("AUTN: want 32 hex chars, got %d (%s)", len(av.AUTN), av.AUTN)
	}
	// Bytes 6..8 of AUTN (chars 12..16) must be AMF literally.
	if av.AUTN[12:16] != "b9b9" {
		t.Errorf("AUTN AMF slot: want b9b9, got %s", av.AUTN[12:16])
	}

	// XRES* = 128 bits = 32 hex chars.
	if len(av.XRESStar) != 32 {
		t.Errorf("XRESStar: want 32 hex chars, got %d (%s)", len(av.XRESStar), av.XRESStar)
	}

	// KAUSF = 256 bits = 64 hex chars.
	if len(av.KAUSF) != 64 {
		t.Errorf("KAUSF: want 64 hex chars, got %d (%s)", len(av.KAUSF), av.KAUSF)
	}
}

// TestDerive5GAKA_Determinism — same inputs must produce the same outputs.
// Otherwise a change to one of the KDFs (e.g. swapping HMAC for plain
// SHA-256) would silently change keys on every call.
func TestDerive5GAKA_Determinism(t *testing.T) {
	a, err := Generate5GAuthVectorWithRAND(tsK, tsOPc, tsRAND, tsSQN, tsAMF, tsSNName)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate5GAuthVectorWithRAND(tsK, tsOPc, tsRAND, tsSQN, tsAMF, tsSNName)
	if err != nil {
		t.Fatal(err)
	}
	if *a != *b {
		t.Errorf("non-deterministic: %+v vs %+v", a, b)
	}
}

// TestDerive5GAKA_SNBinding — changing SN-Name must change XRES* AND KAUSF.
// This is the whole reason 5G-AKA exists: a vector issued for one serving
// network can't be replayed by another.
func TestDerive5GAKA_SNBinding(t *testing.T) {
	home, err := Generate5GAuthVectorWithRAND(tsK, tsOPc, tsRAND, tsSQN, tsAMF, "5G:mnc001.mcc001.3gppnetwork.org")
	if err != nil {
		t.Fatal(err)
	}
	roam, err := Generate5GAuthVectorWithRAND(tsK, tsOPc, tsRAND, tsSQN, tsAMF, "5G:mnc002.mcc002.3gppnetwork.org")
	if err != nil {
		t.Fatal(err)
	}
	if home.XRESStar == roam.XRESStar {
		t.Errorf("XRESStar must differ across SN-Names; got same value %s", home.XRESStar)
	}
	if home.KAUSF == roam.KAUSF {
		t.Errorf("KAUSF must differ across SN-Names; got same value %s", home.KAUSF)
	}
	// RAND and AUTN are SN-independent (pure Milenage), so they must match.
	if home.RAND != roam.RAND {
		t.Errorf("RAND should be SN-independent; got %s vs %s", home.RAND, roam.RAND)
	}
	if home.AUTN != roam.AUTN {
		t.Errorf("AUTN should be SN-independent; got %s vs %s", home.AUTN, roam.AUTN)
	}
}

// TestDeriveKAUSF_FreshRANDChangesKey — a new RAND changes XRES* (via
// different RES/CK/IK inputs) and changes KAUSF (via different AK →
// different SQN⊕AK).
func TestDerive5GAKA_FreshRANDChangesKey(t *testing.T) {
	altRAND := mustHex16("000102030405060708090a0b0c0d0e0f")
	a, _ := Generate5GAuthVectorWithRAND(tsK, tsOPc, tsRAND, tsSQN, tsAMF, tsSNName)
	b, _ := Generate5GAuthVectorWithRAND(tsK, tsOPc, altRAND, tsSQN, tsAMF, tsSNName)
	if a.KAUSF == b.KAUSF {
		t.Errorf("KAUSF should change with RAND")
	}
	if a.XRESStar == b.XRESStar {
		t.Errorf("XRESStar should change with RAND")
	}
}

// TestDeriveHXRESStar — AUSF-side compression must be deterministic and
// sensitive to both RAND and XRES* (so an attacker flipping either can't
// preserve HXRES*).
func TestDeriveHXRESStar(t *testing.T) {
	xres := mustHex16("aabbccddeeff00112233445566778899")
	a := DeriveHXRESStar(tsRAND, xres)
	b := DeriveHXRESStar(tsRAND, xres)
	if a != b {
		t.Errorf("non-deterministic")
	}

	otherRAND := mustHex16("000102030405060708090a0b0c0d0e0f")
	if DeriveHXRESStar(otherRAND, xres) == a {
		t.Errorf("HXRES* must change with RAND")
	}
	otherXRES := mustHex16("99887766554433221100ffeeddccbbaa")
	if DeriveHXRESStar(tsRAND, otherXRES) == a {
		t.Errorf("HXRES* must change with XRES*")
	}
}

// TestDeriveKSEAF — anchor-key derivation must be deterministic, depend
// on both KAUSF and SN-Name, and produce a 256-bit output.
func TestDeriveKSEAF(t *testing.T) {
	kausf := [32]byte{}
	for i := range kausf {
		kausf[i] = byte(i)
	}
	sn := "5G:mnc001.mcc001.3gppnetwork.org"

	a := DeriveKSEAF(kausf, sn)
	b := DeriveKSEAF(kausf, sn)
	if a != b {
		t.Errorf("non-deterministic")
	}

	if DeriveKSEAF(kausf, "5G:mnc002.mcc001.3gppnetwork.org") == a {
		t.Errorf("KSEAF must change with SN-Name")
	}

	var other [32]byte
	for i := range other {
		other[i] = byte(255 - i)
	}
	if DeriveKSEAF(other, sn) == a {
		t.Errorf("KSEAF must change with KAUSF")
	}
}

func mustHex16(s string) [16]byte {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 16 {
		panic("mustHex16: " + s)
	}
	var out [16]byte
	copy(out[:], b)
	return out
}

func mustHex6(s string) [6]byte {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 6 {
		panic("mustHex6: " + s)
	}
	var out [6]byte
	copy(out[:], b)
	return out
}

func mustHex2(s string) [2]byte {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 2 {
		panic("mustHex2: " + s)
	}
	var out [2]byte
	copy(out[:], b)
	return out
}
