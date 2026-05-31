package diag

import (
	"fmt"
	"strings"

	"github.com/qcore-project/qcore/pkg/events"
)

// RegistrationResult is the output of DiagnoseRegistration. The same struct
// pattern as NGSetupResult — OK=true means the registration succeeded;
// OK=false means a specific, diagnosable failure occurred.
type RegistrationResult struct {
	OK bool

	// Machine-readable cause tag. The frontend maps this to an icon and colour;
	// the human text comes from Explanation + Fix fields.
	Cause string

	Explanation  string // plain English: what happened and why
	FixUESide    string // what the device/SIM operator must change
	FixQCoreSide string // what the QCore operator can change as an alternative
}

// Registration failure cause tags. These constants are the contract between
// the AMF (which sets them on RegistrationFailurePayload.Cause) and the
// diagnostic layer (which matches on them). Keep them in sync.
const (
	// CauseUnknownSubscriber — the SUPI/IMSI was not found in the subscriber
	// store. Error comes from UDM returning 404 for the SUPI lookup.
	CauseUnknownSubscriber = "unknown_subscriber"

	// CauseAuthMACFailure — the UE rejected the network's auth challenge.
	// The UE sent Authentication Failure with cause 0x14 (MAC failure), meaning
	// the AMF's AUTN failed the UE's Milenage check. Root cause: wrong Ki or
	// OPc configured in QCore for this IMSI.
	CauseAuthMACFailure = "auth_mac_failure"

	// CauseAuthSQNFailure — the UE rejected the challenge due to sequence number
	// out of sync (cause 0x15). The SQN in QCore's subscriber record is ahead of
	// the UE's SIM. Root cause: SQN counter desync, often after a factory reset.
	CauseAuthSQNFailure = "auth_sqn_failure"

	// CauseSliceNotSubscribed — the UE requested a slice (S-NSSAI) that is not
	// in the subscriber's subscription data.
	CauseSliceNotSubscribed = "slice_not_subscribed"

	// CauseSliceNotSupported — the UE requested a slice that the AMF supports but
	// is not in the PLMNSupportList for this connection. Distinct from
	// CauseSliceNotSubscribed: the slice exists in QCore, but not for this UE.
	CauseSliceNotSupported = "slice_not_supported"

	// CauseDNNNotConfigured — the UE requested a DNN (Data Network Name, e.g.
	// "internet") that the SMF is not configured to serve.
	CauseDNNNotConfigured = "dnn_not_configured"

	// CauseSMFReject — the SMF returned an HTTP error for the PDU session create
	// request. Covers IP pool exhaustion, internal SMF error, and SMF unreachable.
	CauseSMFReject = "smf_reject"

	// CauseIPPoolExhausted — the SMF cannot allocate a UE IP address because the
	// pool is full. Surfaced as a specific subcase of CauseSMFReject.
	CauseIPPoolExhausted = "ip_pool_exhausted"

	// CauseSUCIDecodeFailure — the AMF could not decode the UE's mobile identity.
	// Either the SUCI is malformed or it uses a protection scheme QCore does not
	// support (only null-scheme / scheme 0 is supported today).
	CauseSUCIDecodeFailure = "suci_decode_failure"
)

// DiagnoseRegistration inspects a journey's event trace and returns a
// RegistrationResult. It looks for a RegistrationFailurePayload event — if it
// finds one, it produces the matching rule. If the trace has no failure event
// it returns OK=true. If the trace is empty it returns a sentinel result.
//
// Rules are evaluated in order of specificity. The function never guesses;
// if no rule matches it returns OK=true (assume success unless proven otherwise).
func DiagnoseRegistration(trace []events.Event) RegistrationResult {
	for _, ev := range trace {
		switch p := ev.Payload.(type) {
		case events.RegistrationFailurePayload:
			return applyRegistrationRule(p.Cause, p.SUPI, p.SUCI, p.Detail)
		case events.AuthenticationFailurePayload:
			// UE-originated failure — treated as MAC or SQN failure.
			cause := CauseAuthMACFailure
			if p.CauseName == "sqn_failure" {
				cause = CauseAuthSQNFailure
			}
			return applyRegistrationRule(cause, p.SUPI, "", "")
		case events.SUCIDecodeFailurePayload:
			return applyRegistrationRule(CauseSUCIDecodeFailure, "", p.RawIdentity, p.Reason)
		case events.PDUSessionResultPayload:
			if !p.Success {
				return applyRegistrationRule(p.Cause, p.SUPI, "", p.Detail)
			}
		}
	}
	return RegistrationResult{OK: true}
}

// DiagnoseRegistrationFailure is the inline path — called at the point of
// failure before any event is emitted, so the AMF can get the human-readable
// strings to embed in the RegistrationFailurePayload event.
func DiagnoseRegistrationFailure(cause, supi, suci, detail string) RegistrationResult {
	return applyRegistrationRule(cause, supi, suci, detail)
}

func applyRegistrationRule(cause, supi, suci, detail string) RegistrationResult {
	identity := supi
	if identity == "" {
		identity = suci
	}
	if identity == "" {
		identity = "(unknown)"
	}

	switch cause {

	case CauseUnknownSubscriber:
		return RegistrationResult{
			Cause: cause,
			Explanation: fmt.Sprintf(
				"The subscriber %q was not found in QCore's database. "+
					"The SUPI/IMSI arrived at the core and was decoded correctly, "+
					"but no matching subscriber record exists in the UDR/HSS.",
				identity,
			),
			FixUESide: "Verify the SIM is correctly provisioned. " +
				"Confirm the IMSI printed on the SIM card or configured in the device " +
				"matches the SUPI that QCore is reporting in this failure.",
			FixQCoreSide: fmt.Sprintf(
				"Add the subscriber %q via the QCore dashboard (Subscribers → Add) "+
					"or via the API: POST /api/v1/subscribers. "+
					"Provide the correct Ki, OPc, and AMF values that match the SIM card.",
				identity,
			),
		}

	case CauseAuthMACFailure:
		return RegistrationResult{
			Cause: cause,
			Explanation: fmt.Sprintf(
				"The UE rejected QCore's authentication challenge with a MAC failure. "+
					"The network sent an AUTN value derived from SUPI %q using the "+
					"Ki and OPc stored in QCore. The UE's SIM ran the same derivation "+
					"with its own Ki/OPc and the result did not match. "+
					"This is a cryptographic mismatch — the credentials stored in QCore "+
					"do not match those programmed into the SIM.",
				identity,
			),
			FixUESide: "Verify the SIM is programmed with the correct Ki and OPc values. " +
				"If using a test SIM or soft-SIM, compare the Ki/OPc byte-for-byte with " +
				"what QCore has stored. Note: OP ≠ OPc — OPc is derived from OP using the Ki.",
			FixQCoreSide: fmt.Sprintf(
				"Check the Ki and OPc stored for subscriber %q in the QCore dashboard "+
					"(Subscribers → Edit). Correct values must match the SIM exactly. "+
					"If the SIM uses OP (not OPc), convert: OPc = AES_Ki(OP) XOR OP.",
				identity,
			),
		}

	case CauseAuthSQNFailure:
		return RegistrationResult{
			Cause: cause,
			Explanation: fmt.Sprintf(
				"The UE rejected QCore's authentication challenge with a sequence number (SQN) "+
					"failure for SUPI %q. The SQN counter in QCore's database is ahead of the "+
					"counter stored in the UE's SIM. This typically happens after a SIM or "+
					"subscriber record reset without resetting both sides.",
				identity,
			),
			FixUESide: "If using a soft-SIM or programmable SIM, reset the SIM's SQN counter " +
				"to a value lower than QCore's current SQN, then retry. " +
				"On physical SIMs this may require re-programming the SIM.",
			FixQCoreSide: fmt.Sprintf(
				"Reset the SQN for subscriber %q to 000000000000 in the QCore dashboard "+
					"(Subscribers → Edit → Reset SQN). "+
					"The next authentication attempt will resynchronise the counters.",
				identity,
			),
		}

	case CauseSliceNotSubscribed:
		return RegistrationResult{
			Cause: cause,
			Explanation: fmt.Sprintf(
				"The UE %q requested a network slice (S-NSSAI) that is not in its "+
					"subscription data. The AMF looked up the UE's subscription and the "+
					"requested SST/SD was not listed as an allowed slice for this subscriber.",
				identity,
			),
			FixUESide: "Check which slices the UE is requesting. On UERANSIM, edit the " +
				"defaultNssai and requestedNssai fields in ue.yaml to use SST=1 (eMBB). " +
				"On a real device, the APN/slice profile must match the subscription.",
			FixQCoreSide: fmt.Sprintf(
				"Add the required S-NSSAI to the subscription for %q via the API, or "+
					"configure the UE to request a slice that is already provisioned (SST=1).",
				identity,
			),
		}

	case CauseSliceNotSupported:
		return RegistrationResult{
			Cause: cause,
			Explanation: fmt.Sprintf(
				"The UE %q requested a slice that the AMF does not offer. The UE's "+
					"requested S-NSSAI is not in the AMF's PLMNSupportList. "+
					"This is different from 'not subscribed' — the slice itself is not "+
					"configured in QCore, regardless of what the subscriber is allowed.",
				identity,
			),
			FixUESide: "Set the UE to request SST=1 (eMBB), which is QCore's default. " +
				"On UERANSIM: set sst: 1 in defaultNssai in ue.yaml.",
			FixQCoreSide: "Add the required SST to the PLMNSupportList in the AMF config " +
				"(amf.plmn_support_list → snssai in config.yaml) and restart the AMF.",
		}

	case CauseDNNNotConfigured:
		return RegistrationResult{
			Cause: cause,
			Explanation: fmt.Sprintf(
				"The UE %q requested a DNN (Data Network Name) that the SMF is not "+
					"configured to serve. The PDU session was rejected because the DNN "+
					"does not match any configured data network.",
				identity,
			),
			FixUESide: "Set the UE's APN/DNN to 'internet' (the default QCore DNN). " +
				"On UERANSIM: set defaultDnn: internet in ue.yaml. " +
				"On Android: set the APN name to 'internet' in mobile network settings.",
			FixQCoreSide: fmt.Sprintf(
				"Add the UE's requested DNN to the SMF configuration, or instruct the "+
					"UE operator to use the default DNN 'internet'. "+
					"Detail: %s", strings.TrimSpace(detail),
			),
		}

	case CauseSMFReject:
		return RegistrationResult{
			Cause: cause,
			Explanation: fmt.Sprintf(
				"The SMF rejected the PDU session creation request for UE %q. "+
					"The AMF forwarded the UE's PDU Session Establishment Request to the SMF, "+
					"but the SMF returned an error. Detail: %s",
				identity, strings.TrimSpace(detail),
			),
			FixUESide: "Retry the PDU session. If the error persists, check whether " +
				"the network is currently experiencing load or has been recently restarted.",
			FixQCoreSide: "Check the SMF logs for the specific rejection reason. " +
				"Verify the SMF is running and connected to the UPF. " +
				"If the UPF is unreachable, start it with 'make up' or 'make up-5g'.",
		}

	case CauseIPPoolExhausted:
		return RegistrationResult{
			Cause: cause,
			Explanation: fmt.Sprintf(
				"The SMF could not allocate an IP address for UE %q because the "+
					"configured IP pool is exhausted. All addresses in the UE IP pool "+
					"are currently in use.",
				identity,
			),
			FixUESide: "Wait for another UE's session to expire, or contact the QCore operator.",
			FixQCoreSide: "Increase the UE IP pool size in the SMF configuration " +
				"(smf.ip_pool_cidr). The default is 10.45.0.0/16 which supports ~65,000 UEs. " +
				"If the pool is a smaller test subnet, expand it or restart the SMF to " +
				"release stale sessions.",
		}

	case CauseSUCIDecodeFailure:
		scheme := ""
		if detail != "" {
			scheme = ": " + detail
		}
		return RegistrationResult{
			Cause: cause,
			Explanation: fmt.Sprintf(
				"QCore could not decode the UE's mobile identity (SUCI)%s. "+
					"Raw identity: %s. "+
					"QCore currently supports only null-scheme SUCI (protection scheme 0). "+
					"ECIES-protected SUCI (schemes 1 and 2) is not yet implemented.",
				scheme, identity,
			),
			FixUESide: "Configure the UE or SIM to use null-scheme SUCI (protection scheme 0). " +
				"On UERANSIM: set protectionScheme: 0 in ue.yaml. " +
				"On a real SIM, the protection scheme is programmed at SIM personalisation time.",
			FixQCoreSide: "SUCI with ECIES protection (schemes 1+2) is a planned feature. " +
				"For now, all test SIMs must be provisioned with null-scheme SUCI. " +
				"See docs/v1-gap-closure-plan.md item D-3 for the ECIES roadmap.",
		}

	default:
		// Unknown or unrecognised cause — return a generic result so we never
		// crash or return empty fields. The caller can always emit OK=false.
		return RegistrationResult{
			Cause: cause,
			Explanation: fmt.Sprintf(
				"Registration failed for UE %q with an unrecognised cause: %q. "+
					"Check the QCore logs for more detail.",
				identity, cause,
			),
			FixUESide:    "Check the device configuration and retry.",
			FixQCoreSide: "Check the AMF and SMF logs for the specific error.",
		}
	}
}
