package ai

import (
	"testing"

	"github.com/qcore-project/qcore/pkg/diag"
	"github.com/qcore-project/qcore/pkg/events"
)

// TestCatalogCoverage asserts the catalog has at least the 9 §9.1 cause
// categories. This prevents silent shrinkage — if a rule is removed the test
// fails and someone has to intentionally lower the threshold.
func TestCatalogCoverage(t *testing.T) {
	c := NewCatalog()
	const minRules = 9
	if c.RuleCount() < minRules {
		t.Errorf("catalog has %d rules, want at least %d (charter §9.1 requires ≥9 cause categories)",
			c.RuleCount(), minRules)
	}
}

// syntheticTrace builds a minimal event trace containing a single payload.
func syntheticTrace(payload interface{}) []events.Event {
	return []events.Event{{
		NF:       "amf",
		Category: events.ErrorEvent,
		Severity: events.SeverityError,
		Protocol: "nas5g",
		Message:  "test event",
		Payload:  payload,
	}}
}

// table-driven test: one row per catalog rule.
var catalogRuleTests = []struct {
	name     string // human label
	ruleID   string // must match rule.id in catalog.go
	trace    []events.Event
	wantRootCauseContains string // key phrase that must appear in RootCause
}{
	// ── 5G typed-payload rules ───────────────────────────────────────────────
	{
		name:   "5G unknown subscriber",
		ruleID: "5g_unknown_subscriber",
		trace: syntheticTrace(events.RegistrationFailurePayload{
			SUPI:  "imsi-001019999999999",
			Cause: diag.CauseUnknownSubscriber,
		}),
		wantRootCauseContains: "database",
	},
	{
		name:   "5G auth MAC failure via RegistrationFailurePayload",
		ruleID: "5g_auth_mac_failure",
		trace: syntheticTrace(events.RegistrationFailurePayload{
			SUPI:  "imsi-001010000000001",
			Cause: diag.CauseAuthMACFailure,
		}),
		wantRootCauseContains: "Ki",
	},
	{
		name:   "5G auth MAC failure via AuthenticationFailurePayload",
		ruleID: "5g_auth_mac_failure",
		trace: syntheticTrace(events.AuthenticationFailurePayload{
			SUPI:      "imsi-001010000000001",
			Cause5GMM: 0x14,
			CauseName: "mac_failure",
		}),
		wantRootCauseContains: "Ki",
	},
	{
		name:   "5G SQN failure via AuthenticationFailurePayload",
		ruleID: "5g_auth_sqn_failure",
		trace: syntheticTrace(events.AuthenticationFailurePayload{
			SUPI:      "imsi-001010000000001",
			Cause5GMM: 0x15,
			CauseName: "sqn_failure",
		}),
		wantRootCauseContains: "SQN",
	},
	{
		name:   "5G SUCI decode failure via SUCIDecodeFailurePayload",
		ruleID: "5g_suci_decode_failure",
		trace: syntheticTrace(events.SUCIDecodeFailurePayload{
			RawIdentity: "deadbeef",
			Reason:      "unsupported_protection_scheme",
			Scheme:      1,
		}),
		wantRootCauseContains: "protection scheme",
	},
	{
		name:   "5G slice not subscribed",
		ruleID: "5g_slice_mismatch",
		trace: syntheticTrace(events.RegistrationFailurePayload{
			SUPI:  "imsi-001010000000001",
			Cause: diag.CauseSliceNotSubscribed,
		}),
		wantRootCauseContains: "slice",
	},
	{
		name:   "5G slice not supported (AMF config)",
		ruleID: "5g_slice_mismatch",
		trace: syntheticTrace(events.RegistrationFailurePayload{
			SUPI:  "imsi-001010000000001",
			Cause: diag.CauseSliceNotSupported,
		}),
		wantRootCauseContains: "slice",
	},
	{
		name:   "5G DNN not configured",
		ruleID: "5g_dnn_not_configured",
		trace: syntheticTrace(events.RegistrationFailurePayload{
			SUPI:   "imsi-001010000000001",
			Cause:  diag.CauseDNNNotConfigured,
			Detail: "DNN 'enterprise' not found",
		}),
		wantRootCauseContains: "DNN",
	},
	{
		name:   "5G PDU session: SMF reject",
		ruleID: "5g_pdu_session_failure",
		trace: syntheticTrace(events.PDUSessionResultPayload{
			SUPI:    "imsi-001010000000001",
			Success: false,
			Cause:   diag.CauseSMFReject,
			Detail:  "SMF returned HTTP 500",
		}),
		wantRootCauseContains: "smf_reject",
	},
	{
		name:   "5G PDU session: IP pool exhausted",
		ruleID: "5g_pdu_session_failure",
		trace: syntheticTrace(events.PDUSessionResultPayload{
			SUPI:    "imsi-001010000000001",
			Success: false,
			Cause:   diag.CauseIPPoolExhausted,
		}),
		wantRootCauseContains: "ip_pool_exhausted",
	},
	{
		name:   "5G NG Setup failure (PLMN mismatch)",
		ruleID: "5g_ng_setup_failure",
		trace: syntheticTrace(events.NGSetupPayload{
			GNBAssocID:     "gnb-1",
			Success:        false,
			FailureCause:   "plmn_mismatch",
			FailureExplain: "gNB PLMN 310/260 not in AMF supported list",
			FixGNBSide:     "Set MCC=001, MNC=01",
			FixQCoreSide:   "Add 310/260 to AMF config",
		}),
		wantRootCauseContains: "plmn_mismatch",
	},

	// ── 4G typed-payload rules ───────────────────────────────────────────────
	{
		name:   "4G Ki/OPc mismatch (RES mismatch)",
		ruleID: "4g_auth_res_mismatch",
		trace: syntheticTrace(events.AuthResponsePayload{
			IMSI:    "001010000000001",
			Success: false,
			Cause:   "res_mismatch",
		}),
		wantRootCauseContains: "OPc",
	},
	{
		name:   "4G unprovisioned IMSI (HSS error)",
		ruleID: "4g_hss_error",
		trace: syntheticTrace(events.ErrorPayload{
			Code:    "hss_error",
			Message: "subscriber not found",
		}),
		wantRootCauseContains: "HSS",
	},
	{
		name:   "4G S1 Setup failure (PLMN/TAC mismatch)",
		ruleID: "4g_s1_setup_failure",
		trace: syntheticTrace(events.S1SetupPayload{
			ENBName: "srsRAN",
			ENBID:   0xABCDE,
			Success: false,
		}),
		wantRootCauseContains: "S1 Setup rejected",
	},
	{
		name:   "4G data-plane failure (session created but no completion)",
		ruleID: "4g_data_plane_failure",
		trace: []events.Event{
			{Payload: events.AttachRequestPayload{IMSI: "001010000000001", AttachType: 1}},
			{Payload: events.SessionCreatePayload{IMSI: "001010000000001", APN: "internet", UEIP: "10.45.0.1"}},
			// no AttachCompletePayload — simulates data-plane stall
		},
		wantRootCauseContains: "SPGW",
	},

	// ── String-matching catch-all rules ─────────────────────────────────────
	{
		name:   "NGAP transport error",
		ruleID: "ngap_transport_error",
		trace: []events.Event{{
			Category: events.ErrorEvent,
			Message:  "could not decode PDU: buffer too short",
		}},
		wantRootCauseContains: "NGAP",
	},
	{
		name:   "NAS security algorithm mismatch",
		ruleID: "nas_security_mismatch",
		trace: []events.Event{{
			Category: events.ErrorEvent,
			Message:  "Security Mode Reject: algorithm negotiation failed",
		}},
		wantRootCauseContains: "algorithm",
	},
	{
		name:   "No connection (connection refused)",
		ruleID: "no_connection",
		trace: []events.Event{{
			Category: events.ErrorEvent,
			Message:  "dial tcp: connection refused",
		}},
		wantRootCauseContains: "connect",
	},
}

// TestCatalogRules runs every row in catalogRuleTests and asserts:
//   1. Analyze returns Matched=true for the synthetic trace.
//   2. AnalyzeWithID returns the expected rule ID.
//   3. RootCause contains the expected phrase.
//   4. Explanation and Fix are non-empty.
func TestCatalogRules(t *testing.T) {
	c := NewCatalog()
	for _, tc := range catalogRuleTests {
		t.Run(tc.name, func(t *testing.T) {
			result, id := c.AnalyzeWithID(tc.trace)
			if !result.Matched {
				t.Fatalf("Analyze: expected Matched=true, got no match (ruleID=%q)", tc.ruleID)
			}
			if id != tc.ruleID {
				t.Errorf("rule ID: got %q, want %q", id, tc.ruleID)
			}
			if tc.wantRootCauseContains != "" {
				if !containsIgnoreCase(result.RootCause, tc.wantRootCauseContains) {
					t.Errorf("RootCause %q should contain %q", result.RootCause, tc.wantRootCauseContains)
				}
			}
			if result.Explanation == "" {
				t.Error("Explanation must not be empty")
			}
			if result.Fix == "" {
				t.Error("Fix must not be empty")
			}
		})
	}
}

// TestCatalog_EmptyTrace verifies an empty trace returns Matched=false.
func TestCatalog_EmptyTrace(t *testing.T) {
	c := NewCatalog()
	result := c.Analyze(nil)
	if result.Matched {
		t.Fatal("empty trace should not match any rule")
	}
}

// TestCatalog_UnmatchedTrace verifies a benign trace returns Matched=false.
func TestCatalog_UnmatchedTrace(t *testing.T) {
	c := NewCatalog()
	trace := []events.Event{
		{Payload: events.RegistrationRequestPayload{SUCI: "imsi-001010000000001"}},
		{Payload: events.RegistrationAcceptPayload{SUPI: "imsi-001010000000001"}},
	}
	result := c.Analyze(trace)
	if result.Matched {
		t.Fatalf("happy-path trace should not match any failure rule; matched rule returned: %q", result.Explanation)
	}
}

// TestCatalog_DistinguishUnknownVsMACFailure ensures the two most-confused
// 5G failures fire different rules and produce different RootCause strings.
func TestCatalog_DistinguishUnknownVsMACFailure(t *testing.T) {
	c := NewCatalog()

	unknownTrace := syntheticTrace(events.RegistrationFailurePayload{
		Cause: diag.CauseUnknownSubscriber,
	})
	macTrace := syntheticTrace(events.RegistrationFailurePayload{
		Cause: diag.CauseAuthMACFailure,
	})

	unknownRes, unknownID := c.AnalyzeWithID(unknownTrace)
	macRes, macID := c.AnalyzeWithID(macTrace)

	if unknownID == macID {
		t.Errorf("unknown_subscriber and auth_mac_failure must fire different rules; both fired %q", unknownID)
	}
	if unknownRes.RootCause == macRes.RootCause {
		t.Error("RootCause must differ between unknown subscriber and MAC failure")
	}
	if unknownRes.Fix == macRes.Fix {
		t.Error("Fix must differ between unknown subscriber and MAC failure")
	}
}

// containsIgnoreCase reports whether s contains substr, case-insensitively.
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > 0 && containsIgnoreCaseSlow(s, substr))
}

func containsIgnoreCaseSlow(s, substr string) bool {
	sl := len(substr)
	for i := 0; i <= len(s)-sl; i++ {
		match := true
		for j := 0; j < sl; j++ {
			a, b := s[i+j], substr[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
