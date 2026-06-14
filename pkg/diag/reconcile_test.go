package diag

import (
	"strings"
	"testing"
)

const (
	testIMSI = "001010000000001"
	testKi   = "465b5ce8b199b49faa5f0a2ee238a6bc"
	testOPc  = "cd63cb71954a9f4e48a5994e37a02baf"
	testOP   = "cdc202d5123e20f62b6d676ac72cb318"
)

func testCore() ReconcileCore {
	return ReconcileCore{
		PLMN:               "00101",
		TAC:                1,
		SNSSAIs:            []SNSSAI{{SST: 1, SD: "000001"}},
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
		DefaultDNN:         "internet",
		Subscriber: &SubscriberCredentials{
			IMSI: testIMSI,
			Ki:   testKi,
			OPc:  testOPc,
			APN:  "internet",
		},
	}
}

func matchingInput() ReconcileInput {
	tac := uint16(1)
	return ReconcileInput{
		RANPLMN:              "00101",
		UEPLMN:               "00101",
		TAC:                  &tac,
		SNSSAIs:              []SNSSAI{{SST: 1, SD: "000001"}},
		ServingNetworkName:   "5G:mnc001.mcc001.3gppnetwork.org",
		IMSI:                 testIMSI,
		Ki:                   testKi,
		OPc:                  testOPc,
		OPType:               "OPC",
		SUCIProtectionScheme: "0",
		DNN:                  "internet",
	}
}

func TestReconcileConfig_AllMatch(t *testing.T) {
	got := ReconcileConfig(matchingInput(), testCore())
	if !got.OK {
		t.Fatalf("expected OK, got %+v", got.Mismatches)
	}
	if len(got.Mismatches) != 0 {
		t.Fatalf("mismatches = %d, want 0", len(got.Mismatches))
	}
	if !strings.Contains(got.Summary, "match") {
		t.Fatalf("summary should be positive, got %q", got.Summary)
	}
}

func TestReconcileConfig_MismatchCases(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(*ReconcileInput)
		wantField string
		wantCause string
		wantFix   string
	}{
		{
			name: "PLMN mismatch",
			mutate: func(in *ReconcileInput) {
				in.RANPLMN = "00102"
			},
			wantField: "gnb.plmn",
			wantCause: "plmn_mismatch",
			wantFix:   "001/01",
		},
		{
			name: "TAC mismatch",
			mutate: func(in *ReconcileInput) {
				tac := uint16(99)
				in.TAC = &tac
			},
			wantField: "gnb.tac",
			wantCause: "tac_mismatch",
			wantFix:   "tac",
		},
		{
			name: "SST mismatch",
			mutate: func(in *ReconcileInput) {
				in.SNSSAIs = []SNSSAI{{SST: 2, SD: "000001"}}
			},
			wantField: "s-nssai",
			wantCause: "slice_mismatch",
			wantFix:   "SST=1",
		},
		{
			name: "SD mismatch",
			mutate: func(in *ReconcileInput) {
				in.SNSSAIs = []SNSSAI{{SST: 1, SD: "ffffff"}}
			},
			wantField: "s-nssai",
			wantCause: "slice_mismatch",
			wantFix:   "000001",
		},
		{
			name: "Serving network name mismatch",
			mutate: func(in *ReconcileInput) {
				in.ServingNetworkName = "5G:mnc260.mcc310.3gppnetwork.org"
			},
			wantField: "serving_network_name",
			wantCause: CauseServingNetworkMismatch,
			wantFix:   "5G:mnc001.mcc001.3gppnetwork.org",
		},
		{
			name: "Ki mismatch",
			mutate: func(in *ReconcileInput) {
				in.Ki = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			},
			wantField: "ue.key",
			wantCause: CauseKiMismatch,
			wantFix:   "Ki",
		},
		{
			name: "OPc mismatch",
			mutate: func(in *ReconcileInput) {
				in.OPc = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			},
			wantField: "ue.opc",
			wantCause: CauseOPcMismatch,
			wantFix:   "OPc",
		},
		{
			name: "DNN mismatch",
			mutate: func(in *ReconcileInput) {
				in.DNN = "enterprise"
			},
			wantField: "ue.dnn",
			wantCause: CauseDNNMismatch,
			wantFix:   "internet",
		},
		{
			name: "SUCI scheme mismatch",
			mutate: func(in *ReconcileInput) {
				in.SUCIProtectionScheme = "1"
			},
			wantField: "ue.suci_protection_scheme",
			wantCause: CauseSUCIDecodeFailure,
			wantFix:   "null-scheme",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := matchingInput()
			tc.mutate(&in)

			got := ReconcileConfig(in, testCore())
			if got.OK {
				t.Fatal("expected mismatch, got OK")
			}
			var found *ConfigMismatch
			for i := range got.Mismatches {
				if got.Mismatches[i].Field == tc.wantField {
					found = &got.Mismatches[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("field %q not found in mismatches: %+v", tc.wantField, got.Mismatches)
			}
			if found.Cause != tc.wantCause {
				t.Fatalf("cause = %q, want %q", found.Cause, tc.wantCause)
			}
			if !strings.Contains(found.Fix, tc.wantFix) {
				t.Fatalf("fix %q should contain %q", found.Fix, tc.wantFix)
			}
		})
	}
}

func TestReconcileConfig_OPDerivesOPc(t *testing.T) {
	in := matchingInput()
	in.OPc = ""
	in.OP = testOP
	in.OPType = "OP"

	got := ReconcileConfig(in, testCore())
	if !got.OK {
		t.Fatalf("OP should derive matching OPc, got %+v", got.Mismatches)
	}

	in.OP = "00000000000000000000000000000000"
	got = ReconcileConfig(in, testCore())
	if got.OK {
		t.Fatal("wrong OP should produce OPc mismatch")
	}
	if got.Mismatches[0].Cause != CauseOPcMismatch {
		t.Fatalf("cause = %q, want %q", got.Mismatches[0].Cause, CauseOPcMismatch)
	}
}

func TestReconcileConfig_UnknownSubscriber(t *testing.T) {
	core := testCore()
	core.Subscriber = nil
	in := matchingInput()

	got := ReconcileConfig(in, core)
	if got.OK {
		t.Fatal("expected unknown subscriber mismatch")
	}
	if got.Mismatches[0].Cause != CauseUnknownSubscriber {
		t.Fatalf("cause = %q, want %q", got.Mismatches[0].Cause, CauseUnknownSubscriber)
	}
}

func TestParseReconcileInput_UERANSIMYAMLWrapper(t *testing.T) {
	body := []byte(`gnb_yaml: |
  mcc: '001'
  mnc: '01'
  tac: 1
  slices:
    - sst: 1
      sd: '000001'
ue_yaml: |
  supi: 'imsi-001010000000001'
  mcc: '001'
  mnc: '01'
  key: '465b5ce8b199b49faa5f0a2ee238a6bc'
  op: 'cd63cb71954a9f4e48a5994e37a02baf'
  opType: 'OPC'
  sessions:
    - apn: 'internet'
      slice:
        sst: 1
        sd: '000001'
  protectionScheme: 0
`)
	got, err := ParseReconcileInput(body)
	if err != nil {
		t.Fatalf("ParseReconcileInput returned error: %v", err)
	}
	if got.RANPLMN != "00101" || got.UEPLMN != "00101" {
		t.Fatalf("PLMN parse mismatch: %+v", got)
	}
	if got.TAC == nil || *got.TAC != 1 {
		t.Fatalf("TAC parse mismatch: %+v", got.TAC)
	}
	if got.IMSI != testIMSI || got.Ki != testKi || got.OPc != testOPc {
		t.Fatalf("UE credential parse mismatch: %+v", got)
	}
	if got.DNN != "internet" {
		t.Fatalf("DNN = %q, want internet", got.DNN)
	}
	if len(got.SNSSAIs) == 0 || got.SNSSAIs[0].SST != 1 || got.SNSSAIs[0].SD != "000001" {
		t.Fatalf("SNSSAI parse mismatch: %+v", got.SNSSAIs)
	}
}

func TestParseReconcileInput_JSONFields(t *testing.T) {
	body := []byte(`{
  "plmn": "00101",
  "tac": 1,
  "snssai": {"sst": 1, "sd": "000001"},
  "imsi": "001010000000001",
  "ki": "465b5ce8b199b49faa5f0a2ee238a6bc",
  "opc": "cd63cb71954a9f4e48a5994e37a02baf",
  "dnn": "internet"
}`)
	got, err := ParseReconcileInput(body)
	if err != nil {
		t.Fatalf("ParseReconcileInput returned error: %v", err)
	}
	if got.RANPLMN != "00101" || got.UEPLMN != "00101" {
		t.Fatalf("PLMN parse mismatch: %+v", got)
	}
	if got.IMSI != testIMSI {
		t.Fatalf("IMSI = %q, want %q", got.IMSI, testIMSI)
	}
}

func TestParseReconcileInput_Malformed(t *testing.T) {
	if _, err := ParseReconcileInput([]byte("mcc: [")); err == nil {
		t.Fatal("expected malformed YAML error")
	}
}
