package diag

import (
	"strings"
	"testing"

	"github.com/qcore-project/qcore/pkg/events"
)

// --- unit tests per rule ---

// TestDiagnoseRegistration_UnknownSubscriber verifies the unprovisioned-IMSI rule.
// The key distinguishing factor from auth_mac_failure: 404 from UDM propagated
// through AUSF → AMF detects "not found" in the error string.
func TestDiagnoseRegistration_UnknownSubscriber(t *testing.T) {
	trace := []events.Event{{
		Payload: events.RegistrationFailurePayload{
			SUCI:  "imsi-001019999999999",
			Cause: CauseUnknownSubscriber,
		},
	}}
	res := DiagnoseRegistration(trace)
	if res.OK {
		t.Fatal("expected failure, got OK")
	}
	if res.Cause != CauseUnknownSubscriber {
		t.Errorf("cause: got %q, want %q", res.Cause, CauseUnknownSubscriber)
	}
	if !strings.Contains(res.Explanation, "not found") {
		t.Errorf("explanation should mention 'not found': %s", res.Explanation)
	}
	if !strings.Contains(res.FixQCoreSide, "dashboard") {
		t.Errorf("QCore fix should mention dashboard: %s", res.FixQCoreSide)
	}
	// The fix for unknown subscriber is QCore-side provisioning — there is nothing
	// the UE can change if the IMSI simply doesn't exist in the database.
	if res.FixUESide == "" {
		t.Error("FixUESide must not be empty")
	}
	if res.FixQCoreSide == "" {
		t.Error("FixQCoreSide must not be empty")
	}
}

// TestDiagnoseRegistration_AuthMACFailure verifies Ki/OPc mismatch detection.
// Distinguished from unknown_subscriber: the subscriber IS found (no 404), but
// the Milenage derivation fails because Ki or OPc is wrong.
func TestDiagnoseRegistration_AuthMACFailure(t *testing.T) {
	trace := []events.Event{{
		Payload: events.RegistrationFailurePayload{
			SUPI:  "imsi-001010000000001",
			Cause: CauseAuthMACFailure,
		},
	}}
	res := DiagnoseRegistration(trace)
	if res.OK {
		t.Fatal("expected failure, got OK")
	}
	if res.Cause != CauseAuthMACFailure {
		t.Errorf("cause: got %q, want %q", res.Cause, CauseAuthMACFailure)
	}
	// Explanation must mention Ki/OPc — the concrete thing to fix.
	if !strings.Contains(strings.ToLower(res.Explanation), "ki") {
		t.Errorf("explanation should mention Ki: %s", res.Explanation)
	}
	if !strings.Contains(strings.ToLower(res.Explanation), "opc") {
		t.Errorf("explanation should mention OPc: %s", res.Explanation)
	}
	// FixUESide must mention OPc, not just Ki (the SIM may have correct Ki but
	// wrong OPc derivation).
	if !strings.Contains(strings.ToLower(res.FixUESide), "ki") {
		t.Errorf("UE fix should mention Ki: %s", res.FixUESide)
	}
	if !strings.Contains(strings.ToLower(res.FixQCoreSide), "dashboard") {
		t.Errorf("QCore fix should mention dashboard: %s", res.FixQCoreSide)
	}
}

// TestDiagnoseRegistration_AuthMACFailure_ViaAuthFailurePayload verifies that
// an AuthenticationFailurePayload (UE-originated) also maps correctly.
func TestDiagnoseRegistration_AuthMACFailure_ViaAuthFailurePayload(t *testing.T) {
	trace := []events.Event{{
		Payload: events.AuthenticationFailurePayload{
			SUPI:      "imsi-001010000000001",
			Cause5GMM: 0x14, // MAC failure
			CauseName: "mac_failure",
		},
	}}
	res := DiagnoseRegistration(trace)
	if res.OK {
		t.Fatal("expected failure, got OK")
	}
	if res.Cause != CauseAuthMACFailure {
		t.Errorf("cause: got %q, want %q", res.Cause, CauseAuthMACFailure)
	}
}

// TestDiagnoseRegistration_AuthSQNFailure verifies SQN-out-of-sync detection.
// Different fix from MAC failure: reset SQN, not Ki/OPc.
func TestDiagnoseRegistration_AuthSQNFailure(t *testing.T) {
	trace := []events.Event{{
		Payload: events.AuthenticationFailurePayload{
			SUPI:      "imsi-001010000000001",
			Cause5GMM: 0x15, // SQN failure
			CauseName: "sqn_failure",
		},
	}}
	res := DiagnoseRegistration(trace)
	if res.OK {
		t.Fatal("expected failure, got OK")
	}
	if res.Cause != CauseAuthSQNFailure {
		t.Errorf("cause: got %q, want %q", res.Cause, CauseAuthSQNFailure)
	}
	// The fix must mention resetting the SQN — not Ki/OPc.
	if !strings.Contains(strings.ToLower(res.FixQCoreSide), "sqn") {
		t.Errorf("QCore fix must mention SQN reset: %s", res.FixQCoreSide)
	}
	if strings.Contains(strings.ToLower(res.FixQCoreSide), "ki") {
		t.Errorf("QCore fix must NOT mention Ki for SQN failure: %s", res.FixQCoreSide)
	}
}

// TestDiagnoseRegistration_SliceNotSubscribed verifies slice-not-in-subscription detection.
func TestDiagnoseRegistration_SliceNotSubscribed(t *testing.T) {
	trace := []events.Event{{
		Payload: events.RegistrationFailurePayload{
			SUPI:  "imsi-001010000000001",
			Cause: CauseSliceNotSubscribed,
		},
	}}
	res := DiagnoseRegistration(trace)
	if res.OK {
		t.Fatal("expected failure, got OK")
	}
	if res.Cause != CauseSliceNotSubscribed {
		t.Errorf("cause: got %q, want %q", res.Cause, CauseSliceNotSubscribed)
	}
	if !strings.Contains(strings.ToLower(res.Explanation), "subscription") {
		t.Errorf("explanation should mention subscription: %s", res.Explanation)
	}
	if !strings.Contains(strings.ToLower(res.FixUESide), "sst") {
		t.Errorf("UE fix should mention SST: %s", res.FixUESide)
	}
}

// TestDiagnoseRegistration_SliceNotSupported verifies slice-not-in-AMF-config detection.
// Distinguish from SliceNotSubscribed: here the slice isn't in the AMF's support list.
func TestDiagnoseRegistration_SliceNotSupported(t *testing.T) {
	trace := []events.Event{{
		Payload: events.RegistrationFailurePayload{
			SUPI:  "imsi-001010000000001",
			Cause: CauseSliceNotSupported,
		},
	}}
	res := DiagnoseRegistration(trace)
	if res.OK {
		t.Fatal("expected failure, got OK")
	}
	if res.Cause != CauseSliceNotSupported {
		t.Errorf("cause: got %q, want %q", res.Cause, CauseSliceNotSupported)
	}
	// The QCore fix is AMF config, not UDM subscription.
	if !strings.Contains(strings.ToLower(res.FixQCoreSide), "amf") {
		t.Errorf("QCore fix should mention AMF config: %s", res.FixQCoreSide)
	}
}

// TestDiagnoseRegistration_DNNNotConfigured verifies DNN mismatch detection.
func TestDiagnoseRegistration_DNNNotConfigured(t *testing.T) {
	trace := []events.Event{{
		Payload: events.RegistrationFailurePayload{
			SUPI:   "imsi-001010000000001",
			Cause:  CauseDNNNotConfigured,
			Detail: "DNN 'corporate' not found",
		},
	}}
	res := DiagnoseRegistration(trace)
	if res.OK {
		t.Fatal("expected failure, got OK")
	}
	if res.Cause != CauseDNNNotConfigured {
		t.Errorf("cause: got %q, want %q", res.Cause, CauseDNNNotConfigured)
	}
	// UE fix must mention the default DNN 'internet'.
	if !strings.Contains(res.FixUESide, "internet") {
		t.Errorf("UE fix should mention 'internet' DNN: %s", res.FixUESide)
	}
}

// TestDiagnoseRegistration_SMFReject verifies SMF rejection detection.
func TestDiagnoseRegistration_SMFReject(t *testing.T) {
	trace := []events.Event{{
		Payload: events.PDUSessionResultPayload{
			SUPI:         "imsi-001010000000001",
			PDUSessionID: 1,
			Success:      false,
			Cause:        CauseSMFReject,
			Detail:       "SMF returned HTTP 500",
		},
	}}
	res := DiagnoseRegistration(trace)
	if res.OK {
		t.Fatal("expected failure, got OK")
	}
	if res.Cause != CauseSMFReject {
		t.Errorf("cause: got %q, want %q", res.Cause, CauseSMFReject)
	}
	if !strings.Contains(strings.ToLower(res.FixQCoreSide), "smf") {
		t.Errorf("QCore fix should mention SMF: %s", res.FixQCoreSide)
	}
}

// TestDiagnoseRegistration_IPPoolExhausted verifies IP pool exhaustion detection.
func TestDiagnoseRegistration_IPPoolExhausted(t *testing.T) {
	trace := []events.Event{{
		Payload: events.PDUSessionResultPayload{
			SUPI:         "imsi-001010000000001",
			PDUSessionID: 1,
			Success:      false,
			Cause:        CauseIPPoolExhausted,
		},
	}}
	res := DiagnoseRegistration(trace)
	if res.OK {
		t.Fatal("expected failure, got OK")
	}
	if res.Cause != CauseIPPoolExhausted {
		t.Errorf("cause: got %q, want %q", res.Cause, CauseIPPoolExhausted)
	}
	// Fix must mention IP pool or CIDR.
	if !strings.Contains(strings.ToLower(res.FixQCoreSide), "pool") {
		t.Errorf("QCore fix should mention IP pool: %s", res.FixQCoreSide)
	}
}

// TestDiagnoseRegistration_SUCIDecodeFailure verifies unsupported protection scheme detection.
func TestDiagnoseRegistration_SUCIDecodeFailure(t *testing.T) {
	trace := []events.Event{{
		Payload: events.SUCIDecodeFailurePayload{
			RawIdentity: "01f1100000010002deadbeef",
			Reason:      "unsupported_protection_scheme",
			Scheme:      1,
		},
	}}
	res := DiagnoseRegistration(trace)
	if res.OK {
		t.Fatal("expected failure, got OK")
	}
	if res.Cause != CauseSUCIDecodeFailure {
		t.Errorf("cause: got %q, want %q", res.Cause, CauseSUCIDecodeFailure)
	}
	// Must mention null-scheme as the fix.
	if !strings.Contains(strings.ToLower(res.FixUESide), "null") {
		t.Errorf("UE fix should mention null-scheme: %s", res.FixUESide)
	}
	if !strings.Contains(strings.ToLower(res.FixUESide), "0") {
		t.Errorf("UE fix should mention scheme 0: %s", res.FixUESide)
	}
}

// TestDiagnoseRegistration_Success verifies a trace with no failure event returns OK.
func TestDiagnoseRegistration_Success(t *testing.T) {
	trace := []events.Event{
		{Payload: events.RegistrationRequestPayload{SUCI: "imsi-001010000000001"}},
		{Payload: events.AuthRequestPayload5G{SUPI: "imsi-001010000000001"}},
		{Payload: events.AuthResponsePayload{IMSI: "imsi-001010000000001", Success: true}},
		{Payload: events.RegistrationAcceptPayload{SUPI: "imsi-001010000000001"}},
	}
	res := DiagnoseRegistration(trace)
	if !res.OK {
		t.Fatalf("expected OK for clean trace, got cause=%q", res.Cause)
	}
}

// TestDiagnoseRegistration_EmptyTrace verifies an empty trace returns OK.
func TestDiagnoseRegistration_EmptyTrace(t *testing.T) {
	res := DiagnoseRegistration(nil)
	if !res.OK {
		t.Fatalf("empty trace should return OK, got cause=%q", res.Cause)
	}
}

// TestDiagnoseRegistrationFailure_Direct verifies the inline path used by the AMF.
func TestDiagnoseRegistrationFailure_Direct(t *testing.T) {
	res := DiagnoseRegistrationFailure(CauseUnknownSubscriber, "imsi-001019999999999", "", "")
	if res.OK {
		t.Fatal("expected failure, got OK")
	}
	if res.Cause != CauseUnknownSubscriber {
		t.Errorf("got %q, want %q", res.Cause, CauseUnknownSubscriber)
	}
}

// TestDistinction_UnknownVsMACFailure is the critical test: these two failures
// have different root causes and MUST produce different explanations and fixes.
// Unknown subscriber → provision in QCore. MAC failure → check Ki/OPc on SIM.
func TestDistinction_UnknownVsMACFailure(t *testing.T) {
	unknown := DiagnoseRegistrationFailure(CauseUnknownSubscriber, "imsi-001019999999999", "", "")
	mac := DiagnoseRegistrationFailure(CauseAuthMACFailure, "imsi-001010000000001", "", "")

	if unknown.Cause == mac.Cause {
		t.Error("unknown_subscriber and auth_mac_failure must have different Cause tags")
	}
	if unknown.FixQCoreSide == mac.FixQCoreSide {
		t.Error("fixes must differ: unknown subscriber requires provisioning, MAC failure requires Ki/OPc correction")
	}
	// Unknown subscriber fix is about adding the subscriber; MAC failure fix is about Ki/OPc.
	if !strings.Contains(strings.ToLower(unknown.FixQCoreSide), "add") &&
		!strings.Contains(strings.ToLower(unknown.FixQCoreSide), "provision") &&
		!strings.Contains(strings.ToLower(unknown.FixQCoreSide), "subscriber") {
		t.Errorf("unknown subscriber QCore fix should be about provisioning: %s", unknown.FixQCoreSide)
	}
	if !strings.Contains(strings.ToLower(mac.FixQCoreSide), "ki") {
		t.Errorf("MAC failure QCore fix should mention Ki: %s", mac.FixQCoreSide)
	}
}
