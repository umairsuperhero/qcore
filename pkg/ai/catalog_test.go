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
	const minRules = 28
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

func syntheticInteropTrace(code, message string) []events.Event {
	return []events.Event{{
		NF:       "amf",
		Category: events.ErrorEvent,
		Severity: events.SeverityError,
		Protocol: "ngap",
		Message:  message,
		Payload: events.ErrorPayload{
			Code:    code,
			Message: message,
		},
	}}
}

// table-driven test: one row per catalog rule.
var catalogRuleTests = []struct {
	name                  string // human label
	ruleID                string // must match rule.id in catalog.go
	trace                 []events.Event
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
		wantRootCauseContains: "SIDF",
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

	// ── Real UERANSIM T10 interop findings ──────────────────────────────────
	{
		name:   "T10 DownlinkNASTransport APER transfer-syntax-error",
		ruleID: "t10_downlink_nas_transport_aper",
		trace: syntheticInteropTrace(
			"t10_downlink_nas_transport_aper",
			"UERANSIM rejected DownlinkNASTransport with APER transfer-syntax-error",
		),
		wantRootCauseContains: "DownlinkNASTransport",
	},
	{
		name:   "T10 SMC integrity failure from K_AMF SUPI prefix",
		ruleID: "t10_smc_kamf_supi_prefix",
		trace: syntheticInteropTrace(
			"t10_smc_kamf_supi_prefix",
			"Security Mode Command integrity check failed",
		),
		wantRootCauseContains: "K_AMF",
	},
	{
		name:   "T10 InitialContextSetupRequest APER transfer-syntax-error",
		ruleID: "t10_initial_context_setup_aper",
		trace: syntheticInteropTrace(
			"t10_initial_context_setup_aper",
			"gNB reports APER transfer-syntax-error on InitialContextSetupRequest",
		),
		wantRootCauseContains: "InitialContextSetupRequest",
	},
	{
		name:   "T10 Registration Accept assigned GUTI TLV-E length",
		ruleID: "t10_registration_accept_guti_tlve",
		trace: syntheticInteropTrace(
			"t10_registration_accept_guti_tlve",
			"Registration Accept failed around assigned 5G-GUTI; UE signal lost",
		),
		wantRootCauseContains: "5G-GUTI",
	},
	{
		name:   "T10 UL NAS Transport UERANSIM IE shape",
		ruleID: "t10_ul_nas_transport_shape",
		trace: syntheticInteropTrace(
			"t10_ul_nas_transport_shape",
			"decode UL NAS Transport failed for protected PDU Session Establishment Request",
		),
		wantRootCauseContains: "UL NAS Transport",
	},
	{
		name:   "T10 AMF container uses localhost SMF URL",
		ruleID: "t10_smf_url_localhost",
		trace: syntheticInteropTrace(
			"t10_smf_url_localhost",
			"AMF failed to reach SMF at http://localhost:8002 from inside Docker",
		),
		wantRootCauseContains: "localhost:8002",
	},
	{
		name:   "T10 PDU Session Establishment Accept missing",
		ruleID: "t10_pdu_session_accept_missing",
		trace: syntheticInteropTrace(
			"t10_pdu_session_accept_missing",
			"PDU Session Establishment Accept not received by UERANSIM after SMF 201",
		),
		wantRootCauseContains: "mandatory",
	},
	{
		name:   "T10 PDU session established but no data-plane ping",
		ruleID: "t10_data_plane_n2_n3_gap",
		trace: syntheticInteropTrace(
			"t10_data_plane_n2_n3_gap",
			"PDU session established but ping failed; no external data-plane packet proven",
		),
		wantRootCauseContains: "N2/N3",
	},
	{
		name:   "T10 UPF Linux TUN unavailable",
		ruleID: "t10_upf_tun_unavailable",
		trace: syntheticInteropTrace(
			"t10_upf_tun_unavailable",
			"Failed to create Linux TUN egress. Falling back to DummyEgress. /dev/net/tun unavailable",
		),
		wantRootCauseContains: "TUN",
	},

	// ── String-matching catch-all rules ─────────────────────────────────────
	{
		name:   "Simulator wrong Ki scenario",
		ruleID: "simulator_wrong_ki",
		trace: []events.Event{{
			Category: events.ErrorEvent,
			Message:  "Scenario wrong_ki failed at security_mode_command: read timed out after 3s",
		}},
		wantRootCauseContains: "Ki",
	},
	{
		name:   "Simulator wrong PLMN scenario",
		ruleID: "simulator_wrong_plmn",
		trace: []events.Event{{
			Category: events.ErrorEvent,
			Message:  "Scenario wrong_plmn failed at ng_setup: expected NG Setup Response",
		}},
		wantRootCauseContains: "PLMN",
	},
	{
		name:   "Simulator unprovisioned IMSI scenario",
		ruleID: "simulator_unprovisioned_imsi",
		trace: []events.Event{{
			Category: events.ErrorEvent,
			Message:  "Scenario unprovisioned_imsi failed at auth_request: AMF sent Registration Reject",
		}},
		wantRootCauseContains: "Unknown subscriber",
	},
	{
		name:   "Simulator wrong MME address scenario",
		ruleID: "simulator_wrong_mme_address",
		trace: []events.Event{{
			Category: events.ErrorEvent,
			Message:  "Scenario wrong_mme_address failed at connect: dial tcp: connection refused",
		}},
		wantRootCauseContains: "endpoint",
	},
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
//  1. Analyze returns Matched=true for the synthetic trace.
//  2. AnalyzeWithID returns the expected rule ID.
//  3. RootCause contains the expected phrase.
//  4. Explanation and Fix are non-empty.
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
