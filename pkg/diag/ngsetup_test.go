package diag

import (
	"errors"
	"strings"
	"testing"

	"github.com/qcore-project/qcore/pkg/ident"
	"github.com/qcore-project/qcore/pkg/ngap"
)

// buildReq is a minimal-but-valid NGSetupRequest for the given MCC/MNC, TAC, and SST.
func buildReq(mcc, mnc string, tac [3]byte, sst uint8) *ngap.NGSetupRequest {
	plmn := ngap.PLMN(ident.EncodePLMN(mcc, mnc))
	return &ngap.NGSetupRequest{
		GlobalRANNodeID: ngap.GlobalGNBID{PLMN: plmn, GNBID: 0x001, GNBIDBits: 22},
		RANNodeName:     "test-gnb",
		SupportedTAList: []ngap.SupportedTA{
			{
				TAC: tac,
				BroadcastPLMN: []ngap.BroadcastPLMNItem{
					{
						PLMN:   plmn,
						SNSSAI: []ngap.SNSSAI{{SST: sst}},
					},
				},
			},
		},
	}
}

// amfConfig001_01 is a representative AMF config for PLMN 001/01, TAC 0x000001, SST 1.
func amfConfig001_01() AMFConfig {
	return AMFConfig{
		SupportedPLMNs:   []ngap.PLMN{ngap.PLMN(ident.EncodePLMN("001", "01"))},
		SupportedTACs:    [][3]byte{{0x00, 0x00, 0x01}},
		SupportedSNSSAIs: []ngap.SNSSAI{{SST: 1}},
	}
}

// TestDiagnoseNGSetup_HappyPath verifies a matching gNB returns OK.
func TestDiagnoseNGSetup_HappyPath(t *testing.T) {
	req := buildReq("001", "01", [3]byte{0x00, 0x00, 0x01}, 1)
	result := DiagnoseNGSetup(req, amfConfig001_01())
	if !result.OK {
		t.Fatalf("expected OK, got cause=%q explain=%q", result.Cause, result.Explanation)
	}
}

// TestDiagnoseNGSetup_PLMNMismatch verifies PLMN mismatch detection.
// The gNB sends PLMN 310/260 (AT&T); the AMF is configured for 001/01.
func TestDiagnoseNGSetup_PLMNMismatch(t *testing.T) {
	req := buildReq("310", "260", [3]byte{0x00, 0x00, 0x01}, 1)
	result := DiagnoseNGSetup(req, amfConfig001_01())
	if result.OK {
		t.Fatal("expected failure, got OK")
	}
	if result.Cause != "plmn_mismatch" {
		t.Errorf("cause: want plmn_mismatch, got %q", result.Cause)
	}
	// Explanation must mention both the gNB PLMN and the AMF PLMN.
	if !strings.Contains(result.Explanation, "310") {
		t.Errorf("explanation should mention gNB MCC 310: %s", result.Explanation)
	}
	if !strings.Contains(result.Explanation, "001") {
		t.Errorf("explanation should mention AMF MCC 001: %s", result.Explanation)
	}
	// Both fix fields must be non-empty.
	if result.FixGNBSide == "" || result.FixQCoreSide == "" {
		t.Error("both fix fields must be non-empty")
	}
	// gNB fix must say what to change on the gNB.
	if !strings.Contains(result.FixGNBSide, "001") {
		t.Errorf("gNB fix should tell operator to set MCC/MNC to 001/01: %s", result.FixGNBSide)
	}
}

// TestDiagnoseNGSetup_TACMismatch verifies TAC mismatch detection.
// The gNB sends TAC 0x000099; the AMF only accepts 0x000001.
func TestDiagnoseNGSetup_TACMismatch(t *testing.T) {
	req := buildReq("001", "01", [3]byte{0x00, 0x00, 0x99}, 1)
	result := DiagnoseNGSetup(req, amfConfig001_01())
	if result.OK {
		t.Fatal("expected failure, got OK")
	}
	if result.Cause != "tac_mismatch" {
		t.Errorf("cause: want tac_mismatch, got %q", result.Cause)
	}
	if !strings.Contains(result.Explanation, "0000") { // TAC hex prefix
		t.Errorf("explanation should include TAC hex: %s", result.Explanation)
	}
	if result.FixGNBSide == "" || result.FixQCoreSide == "" {
		t.Error("both fix fields must be non-empty")
	}
}

// TestDiagnoseNGSetup_SliceMismatch verifies S-NSSAI mismatch detection.
// The gNB offers only SST=2 (URLLC); the AMF supports only SST=1 (eMBB).
func TestDiagnoseNGSetup_SliceMismatch(t *testing.T) {
	req := buildReq("001", "01", [3]byte{0x00, 0x00, 0x01}, 2 /* URLLC */)
	result := DiagnoseNGSetup(req, amfConfig001_01())
	if result.OK {
		t.Fatal("expected failure, got OK")
	}
	if result.Cause != "slice_mismatch" {
		t.Errorf("cause: want slice_mismatch, got %q", result.Cause)
	}
	if !strings.Contains(result.Explanation, "SST=2") {
		t.Errorf("explanation should mention gNB SST: %s", result.Explanation)
	}
	if !strings.Contains(result.Explanation, "SST=1") {
		t.Errorf("explanation should mention AMF SST: %s", result.Explanation)
	}
	if result.FixGNBSide == "" || result.FixQCoreSide == "" {
		t.Error("both fix fields must be non-empty")
	}
}

// TestDiagnoseNGSetupMalformed verifies the malformed-PDU rule.
func TestDiagnoseNGSetupMalformed(t *testing.T) {
	result := DiagnoseNGSetupMalformed(errors.New("buffer too short"))
	if result.OK {
		t.Fatal("expected failure, got OK")
	}
	if result.Cause != "malformed_request" {
		t.Errorf("cause: want malformed_request, got %q", result.Cause)
	}
	if !strings.Contains(result.Explanation, "buffer too short") {
		t.Errorf("explanation should include the decode error: %s", result.Explanation)
	}
	if result.FixGNBSide == "" || result.FixQCoreSide == "" {
		t.Error("both fix fields must be non-empty")
	}
}

// TestDiagnoseNoConnection verifies the no-connection-attempt rule.
func TestDiagnoseNoConnection(t *testing.T) {
	result := DiagnoseNoConnection("192.168.1.50:38412", "001/01")
	if result.OK {
		t.Fatal("expected failure, got OK")
	}
	if result.Cause != "no_connection" {
		t.Errorf("cause: want no_connection, got %q", result.Cause)
	}
	if !strings.Contains(result.Explanation, "192.168.1.50") {
		t.Errorf("explanation should include the AMF address: %s", result.Explanation)
	}
	if !strings.Contains(result.FixGNBSide, "38412") {
		t.Errorf("gNB fix should mention port 38412: %s", result.FixGNBSide)
	}
}

// TestDiagnoseNGSetup_AnyTACAccepted verifies that an empty SupportedTACs list
// means "accept any TAC" (so the TAC check is a no-op).
func TestDiagnoseNGSetup_AnyTACAccepted(t *testing.T) {
	cfg := AMFConfig{
		SupportedPLMNs:   []ngap.PLMN{ngap.PLMN(ident.EncodePLMN("001", "01"))},
		SupportedTACs:    nil, // empty = accept all
		SupportedSNSSAIs: []ngap.SNSSAI{{SST: 1}},
	}
	req := buildReq("001", "01", [3]byte{0xFF, 0xFF, 0xFF}, 1) // exotic TAC
	result := DiagnoseNGSetup(req, cfg)
	if !result.OK {
		t.Fatalf("empty SupportedTACs should accept any TAC; got cause=%q", result.Cause)
	}
}

// TestDiagnoseNGSetup_AnySliceAccepted verifies that an empty SupportedSNSSAIs list
// means "accept any slice."
func TestDiagnoseNGSetup_AnySliceAccepted(t *testing.T) {
	cfg := AMFConfig{
		SupportedPLMNs:   []ngap.PLMN{ngap.PLMN(ident.EncodePLMN("001", "01"))},
		SupportedTACs:    [][3]byte{{0x00, 0x00, 0x01}},
		SupportedSNSSAIs: nil, // empty = accept all
	}
	req := buildReq("001", "01", [3]byte{0x00, 0x00, 0x01}, 42) // unusual SST
	result := DiagnoseNGSetup(req, cfg)
	if !result.OK {
		t.Fatalf("empty SupportedSNSSAIs should accept any slice; got cause=%q", result.Cause)
	}
}

// TestNGSetupPLMNCodec_RealGNBEncoding verifies that the rule engine uses
// pkg/ident (A1 canonical codec) to decode real-world gNB PLMN bytes, not an
// internal round-trip. Golden vectors captured from UERANSIM default config and
// Deutsche Telekom field encoding.
func TestNGSetupPLMNCodec_RealGNBEncoding(t *testing.T) {
	cases := []struct {
		name    string
		rawPLMN [3]byte // bytes as a real gNB would send on the wire
		wantMCC string
		wantMNC string
	}{
		{
			// UERANSIM default: MCC=001, MNC=01
			// TS 24.008 §10.5.1.13: byte0=MCC2|MCC1=0x00, byte1=MNC3|MCC3=0xF1, byte2=MNC2|MNC1=0x10
			name:    "UERANSIM default 001/01",
			rawPLMN: [3]byte{0x00, 0xF1, 0x10},
			wantMCC: "001", wantMNC: "01",
		},
		{
			// Deutsche Telekom Germany: MCC=262, MNC=01
			// byte0=0x62, byte1=0xF2, byte2=0x10
			name:    "Deutsche Telekom 262/01",
			rawPLMN: [3]byte{0x62, 0xF2, 0x10},
			wantMCC: "262", wantMNC: "01",
		},
		{
			// AT&T USA: MCC=310, MNC=410 (3-digit MNC)
			// byte0=0x13, byte1=0x00, byte2=0x14
			name:    "AT&T USA 310/410",
			rawPLMN: [3]byte{0x13, 0x00, 0x14},
			wantMCC: "310", wantMNC: "410",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Build a request carrying raw gNB PLMN bytes.
			req := &ngap.NGSetupRequest{
				GlobalRANNodeID: ngap.GlobalGNBID{
					PLMN:      ngap.PLMN(tc.rawPLMN),
					GNBID:     0x001,
					GNBIDBits: 22,
				},
				SupportedTAList: []ngap.SupportedTA{{
					TAC: [3]byte{0x00, 0x00, 0x01},
					BroadcastPLMN: []ngap.BroadcastPLMNItem{{
						PLMN:   ngap.PLMN(tc.rawPLMN),
						SNSSAI: []ngap.SNSSAI{{SST: 1}},
					}},
				}},
			}

			// AMF configured for the same PLMN — should succeed.
			amfPLMN := ngap.PLMN(ident.EncodePLMN(tc.wantMCC, tc.wantMNC))
			cfg := AMFConfig{
				SupportedPLMNs:   []ngap.PLMN{amfPLMN},
				SupportedSNSSAIs: []ngap.SNSSAI{{SST: 1}},
			}
			result := DiagnoseNGSetup(req, cfg)
			if !result.OK {
				t.Errorf("PLMN decode mismatch for %s: cause=%q\n"+
					"  This indicates the rule engine is not using the ident codec correctly.\n"+
					"  Raw bytes: %X, want MCC=%s MNC=%s",
					tc.name, result.Cause, tc.rawPLMN, tc.wantMCC, tc.wantMNC)
			}

			// Also directly verify ident.DecodePLMN gives us the right strings.
			mcc, mnc := ident.DecodePLMN(tc.rawPLMN)
			if mcc != tc.wantMCC || mnc != tc.wantMNC {
				t.Errorf("ident.DecodePLMN(%X) = %s/%s, want %s/%s",
					tc.rawPLMN, mcc, mnc, tc.wantMCC, tc.wantMNC)
			}
		})
	}
}
