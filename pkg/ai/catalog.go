package ai

import (
	"strings"

	"github.com/qcore-project/qcore/pkg/diag"
	"github.com/qcore-project/qcore/pkg/events"
)

// DiagnosticResult represents a known symptom mapped to a root cause and fix.
// The AI engine calls the catalog first; if no rule matches it escalates to
// the LLM. Rules must be precise — wrong output here is a confident liar.
type DiagnosticResult struct {
	Matched     bool
	Explanation string
	RootCause   string
	Fix         string
}

// rule is one entry in the catalog. id is a stable machine tag used by tests
// to assert that a specific rule fired (not just that *something* matched).
type rule struct {
	id      string
	matches func(trace []events.Event) (DiagnosticResult, bool)
}

// Catalog is the structured diagnostic knowledge layer.
// It uses typed event payload matching, not fragile string matching.
// Rules cover the charter §9.1 cause categories for both 4G and 5G.
// Order matters: more-specific rules run before catch-all ones.
var catalog = []rule{

	// ── 5G typed-payload rules (from pkg/diag/registration.go cause tags) ─────
	// These match on structured RegistrationFailurePayload.Cause — no string guessing.

	{
		id: "5g_unknown_subscriber",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				if p, ok := ev.Payload.(events.RegistrationFailurePayload); ok &&
					p.Cause == diag.CauseUnknownSubscriber {
					res := diag.DiagnoseRegistrationFailure(p.Cause, p.SUPI, p.SUCI, p.Detail)
					return DiagnosticResult{
						Matched:     true,
						Explanation: res.Explanation,
						RootCause:   "The SUPI/IMSI was not found in QCore's subscriber database.",
						Fix:         res.FixQCoreSide + " On the device/SIM side: " + res.FixUESide,
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	{
		id: "5g_auth_mac_failure",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				if p, ok := ev.Payload.(events.AuthenticationFailurePayload); ok &&
					p.CauseName == "mac_failure" {
					res := diag.DiagnoseRegistrationFailure(diag.CauseAuthMACFailure, p.SUPI, "", "")
					return DiagnosticResult{
						Matched:     true,
						Explanation: res.Explanation,
						RootCause:   "Cryptographic mismatch: the Ki or OPc stored in QCore does not match the SIM.",
						Fix:         res.FixQCoreSide + " On the SIM/device side: " + res.FixUESide,
					}, true
				}
				if p, ok := ev.Payload.(events.RegistrationFailurePayload); ok &&
					p.Cause == diag.CauseAuthMACFailure {
					res := diag.DiagnoseRegistrationFailure(p.Cause, p.SUPI, "", "")
					return DiagnosticResult{
						Matched:     true,
						Explanation: res.Explanation,
						RootCause:   "Cryptographic mismatch: the Ki or OPc stored in QCore does not match the SIM.",
						Fix:         res.FixQCoreSide + " On the SIM/device side: " + res.FixUESide,
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	{
		id: "5g_auth_sqn_failure",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				if p, ok := ev.Payload.(events.AuthenticationFailurePayload); ok &&
					p.CauseName == "sqn_failure" {
					res := diag.DiagnoseRegistrationFailure(diag.CauseAuthSQNFailure, p.SUPI, "", "")
					return DiagnosticResult{
						Matched:     true,
						Explanation: res.Explanation,
						RootCause:   "SQN counter desync between the SIM and QCore's subscriber record.",
						Fix:         res.FixQCoreSide + " On the SIM/device side: " + res.FixUESide,
					}, true
				}
				if p, ok := ev.Payload.(events.RegistrationFailurePayload); ok &&
					p.Cause == diag.CauseAuthSQNFailure {
					res := diag.DiagnoseRegistrationFailure(p.Cause, p.SUPI, "", "")
					return DiagnosticResult{
						Matched:     true,
						Explanation: res.Explanation,
						RootCause:   "SQN counter desync between the SIM and QCore's subscriber record.",
						Fix:         res.FixQCoreSide,
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	{
		id: "5g_suci_decode_failure",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				if p, ok := ev.Payload.(events.SUCIDecodeFailurePayload); ok {
					res := diag.DiagnoseRegistrationFailure(diag.CauseSUCIDecodeFailure, "", p.RawIdentity, p.Reason)
					return DiagnosticResult{
						Matched:     true,
						Explanation: res.Explanation,
						RootCause:   "Malformed SUCI, unsupported protection scheme, missing SIDF key, or SUCI MAC verification failure.",
						Fix:         res.FixUESide + " QCore side: " + res.FixQCoreSide,
					}, true
				}
				if p, ok := ev.Payload.(events.RegistrationFailurePayload); ok &&
					p.Cause == diag.CauseSUCIDecodeFailure {
					res := diag.DiagnoseRegistrationFailure(p.Cause, "", p.SUCI, p.Detail)
					return DiagnosticResult{
						Matched:     true,
						Explanation: res.Explanation,
						RootCause:   "Malformed SUCI, unsupported protection scheme, missing SIDF key, or SUCI MAC verification failure.",
						Fix:         res.FixUESide,
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	{
		id: "5g_slice_mismatch",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				if p, ok := ev.Payload.(events.RegistrationFailurePayload); ok &&
					(p.Cause == diag.CauseSliceNotSubscribed || p.Cause == diag.CauseSliceNotSupported) {
					res := diag.DiagnoseRegistrationFailure(p.Cause, p.SUPI, "", p.Detail)
					return DiagnosticResult{
						Matched:     true,
						Explanation: res.Explanation,
						RootCause:   "The requested S-NSSAI (network slice) is not available for this UE.",
						Fix:         res.FixUESide + " QCore side: " + res.FixQCoreSide,
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	{
		id: "5g_dnn_not_configured",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				if p, ok := ev.Payload.(events.RegistrationFailurePayload); ok &&
					p.Cause == diag.CauseDNNNotConfigured {
					res := diag.DiagnoseRegistrationFailure(p.Cause, p.SUPI, "", p.Detail)
					return DiagnosticResult{
						Matched:     true,
						Explanation: res.Explanation,
						RootCause:   "The requested DNN (Data Network Name / APN) is not served by QCore's SMF.",
						Fix:         res.FixUESide,
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	{
		id: "5g_pdu_session_failure",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				if p, ok := ev.Payload.(events.PDUSessionResultPayload); ok && !p.Success {
					cause := p.Cause
					if cause == "" {
						cause = diag.CauseSMFReject
					}
					res := diag.DiagnoseRegistrationFailure(cause, p.SUPI, "", p.Detail)
					return DiagnosticResult{
						Matched:     true,
						Explanation: res.Explanation,
						RootCause:   "PDU session establishment failed: " + cause,
						Fix:         res.FixQCoreSide,
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	// 5G NGSetup failures — match on typed NGSetupPayload from pkg/amf/ngap.go
	{
		id: "5g_ng_setup_failure",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				if p, ok := ev.Payload.(events.NGSetupPayload); ok && !p.Success && p.FailureCause != "" {
					return DiagnosticResult{
						Matched:     true,
						Explanation: p.FailureExplain,
						RootCause:   "NG Setup rejected: " + p.FailureCause,
						Fix:         p.FixGNBSide + " Alternatively (QCore side): " + p.FixQCoreSide,
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	// ── 4G typed-payload rules ─────────────────────────────────────────────────

	// 4G: Ki/OPc mismatch — AuthResponsePayload with Success=false and Cause="res_mismatch"
	{
		id: "4g_auth_res_mismatch",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				if p, ok := ev.Payload.(events.AuthResponsePayload); ok &&
					!p.Success && p.Cause == "res_mismatch" {
					return DiagnosticResult{
						Matched:     true,
						Explanation: "The UE's XRES did not match the expected value from the HSS. The Milenage derivation failed.",
						RootCause:   "Ki or OPc mismatch between the SIM and QCore's HSS for IMSI " + p.IMSI + ".",
						Fix: "Verify the Ki and OPc in the HSS match the SIM exactly. " +
							"Note: OP ≠ OPc — OPc is derived via AES_Ki(OP) XOR OP. " +
							"Use the QCore dashboard (Subscribers → Edit) to correct the values.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	// 4G: Unprovisioned IMSI — HSS returns error for the auth-vector request
	{
		id: "4g_hss_error",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				if p, ok := ev.Payload.(events.ErrorPayload); ok && p.Code == "hss_error" {
					return DiagnosticResult{
						Matched: true,
						Explanation: "The MME could not fetch authentication vectors from the HSS. " +
							"The subscriber may not be provisioned, or the HSS is unreachable.",
						RootCause: "HSS error: " + p.Message,
						Fix: "Check that the subscriber is provisioned in QCore (dashboard → Subscribers). " +
							"If the subscriber exists, check the HSS API is reachable at its configured port.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	// 4G: S1 Setup rejected — PLMN or TAC mismatch, or no supported GUMMEIs
	{
		id: "4g_s1_setup_failure",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				if p, ok := ev.Payload.(events.S1SetupPayload); ok && !p.Success {
					return DiagnosticResult{
						Matched: true,
						Explanation: "The eNodeB's S1 Setup Request was rejected by QCore's MME. " +
							"The most common cause is a PLMN mismatch between the eNB and the MME.",
						RootCause: "S1 Setup rejected for eNB " + p.ENBName + " (ID=" + uint32Hex(p.ENBID) + ").",
						Fix: "Verify the eNB's MCC/MNC matches QCore's PLMN (default: 001/01). " +
							"In srsRAN: check mcc/mnc in enb.conf. " +
							"In a real eNB: update the PLMN in the cell configuration.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	// 4G: Data-plane routing failure — no SPGW session or TUN egress error
	// Matched when we see an AttachRequest but no corresponding AttachComplete.
	{
		id: "4g_data_plane_failure",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			hasAttachReq := false
			hasAttachComplete := false
			hasSessionCreate := false
			for _, ev := range trace {
				switch ev.Payload.(type) {
				case events.AttachRequestPayload:
					hasAttachReq = true
				case events.AttachCompletePayload:
					hasAttachComplete = true
				case events.SessionCreatePayload:
					hasSessionCreate = true
				}
			}
			// If we see an attach request and a session create but no completion,
			// the data plane likely failed to come up.
			if hasAttachReq && hasSessionCreate && !hasAttachComplete {
				return DiagnosticResult{
					Matched: true,
					Explanation: "The UE attached and a bearer was requested, but the session never completed. " +
						"The SPGW may have failed to allocate an IP address or set up the GTP-U tunnel.",
					RootCause: "Data plane setup failure: SPGW could not complete the bearer establishment.",
					Fix: "Check that the SPGW is running and the UE IP pool (spgw.ue_pool) has available addresses. " +
						"If using TUN egress, verify the qcore0 interface is up (ip link show qcore0).",
				}, true
			}
			return DiagnosticResult{}, false
		},
	},

	// ── Real interop findings from UERANSIM T10 ───────────────────────────────
	// These are specific, evidence-backed failure modes from docs/3gpp-tracking.md.
	// Match stable ErrorPayload code/cause tags when present, with narrow message
	// fallbacks for captured external logs.

	{
		id: "t10_downlink_nas_transport_aper",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				msg := eventText(ev)
				if errorPayloadHasAny(ev, "t10_downlink_nas_transport_aper", "downlink_nas_transport_aper") ||
					(strings.Contains(msg, "downlinknastransport") &&
						(strings.Contains(msg, "aper") || strings.Contains(msg, "transfer-syntax-error"))) {
					return DiagnosticResult{
						Matched:     true,
						Explanation: "The gNB rejected QCore's DownlinkNASTransport as an invalid APER-encoded NGAP PDU.",
						RootCause:   "NGAP DownlinkNASTransport APER encoding mismatch: UE-NGAP-ID or NAS-PDU value encoding is malformed on the wire.",
						Fix: "Capture the rejected NGAP hex and validate the DownlinkNASTransport IE container against UERANSIM. " +
							"Check AMF/RAN UE NGAP ID constrained integer encoding, criticality, and NAS-PDU OCTET STRING length/alignment. " +
							"Use the external-corpus fixture path rather than relying on QCore round trips.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	{
		id: "t10_smc_kamf_supi_prefix",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				msg := eventText(ev)
				if errorPayloadHasAny(ev, "t10_smc_kamf_supi_prefix", "kamf_supi_prefix", "smc_integrity_failure") ||
					(strings.Contains(msg, "security mode command") &&
						(strings.Contains(msg, "integrity check failed") || strings.Contains(msg, "mac failed"))) {
					return DiagnosticResult{
						Matched:     true,
						Explanation: "The UE rejected the Security Mode Command because the NAS integrity MAC did not verify.",
						RootCause:   "K_AMF/K_NASint mismatch: QCore likely derived K_AMF using the SBI SUPI string with the `imsi-` prefix instead of the bare IMSI digits required by TS 33.501 Annex A.7.",
						Fix: "Derive K_AMF with bare IMSI digits only, while keeping `imsi-<digits>` for SBI and telemetry. " +
							"Then verify K_NASint uses NIA2, ABBA is 0x0000, bearer is 1, and the NAS downlink count matches the protected SMC.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	{
		id: "t10_initial_context_setup_aper",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				msg := eventText(ev)
				if errorPayloadHasAny(ev, "t10_initial_context_setup_aper", "initial_context_setup_aper") ||
					(strings.Contains(msg, "initialcontextsetuprequest") &&
						(strings.Contains(msg, "aper") || strings.Contains(msg, "transfer-syntax-error"))) {
					return DiagnosticResult{
						Matched:     true,
						Explanation: "The gNB rejected QCore's InitialContextSetupRequest before the UE could receive Registration Accept.",
						RootCause:   "InitialContextSetupRequest APER encoding mismatch, especially UEAggregateMaximumBitRate or UESecurityCapabilities extension-marker/bit-string encoding.",
						Fix: "Validate the rejected InitialContextSetupRequest hex against UERANSIM. " +
							"Check BitRate extensible constrained integer encoding and all UE security-capability BIT STRING extension markers.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	{
		id: "t10_registration_accept_guti_tlve",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				msg := eventText(ev)
				if errorPayloadHasAny(ev, "t10_registration_accept_guti_tlve", "guti_tlve_length") ||
					(strings.Contains(msg, "registration accept") &&
						(strings.Contains(msg, "guti") || strings.Contains(msg, "t3510") || strings.Contains(msg, "ue signal lost"))) {
					return DiagnosticResult{
						Matched:     true,
						Explanation: "Registration reached Initial Context Setup, but the UE failed around Registration Accept / assigned 5G-GUTI handling.",
						RootCause:   "Assigned 5G-GUTI IEI 0x77 in Registration Accept is malformed; UERANSIM expects IE6/TLV-E with a two-byte length field.",
						Fix: "Encode Registration Accept assigned 5G-GUTI as IEI 0x77 followed by a two-byte length and the 5G-GUTI value. " +
							"Pin the plain NAS bytes with an external UERANSIM fixture.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	{
		id: "t10_ul_nas_transport_shape",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				msg := eventText(ev)
				if errorPayloadHasAny(ev, "t10_ul_nas_transport_shape", "ul_nas_transport_shape") ||
					(strings.Contains(msg, "ul nas transport") &&
						(strings.Contains(msg, "decode") || strings.Contains(msg, "pdu session"))) {
					return DiagnosticResult{
						Matched:     true,
						Explanation: "The UE sent a protected UL NAS Transport for PDU session setup, but the AMF could not process the decoded NAS payload.",
						RootCause:   "UL NAS Transport parsing mismatch: the AMF must route the decrypted inner NAS payload and accept UERANSIM's low-nibble payload-container type plus IE1/IE3 optional fields.",
						Fix: "Unwrap protected NAS before dispatching to UL NAS Transport handling. " +
							"Decode payload-container type from the low nibble and parse PDU Session ID as IE3 (`0x12 value`) plus request type as IE1.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	{
		id: "t10_smf_url_localhost",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				msg := eventText(ev)
				if errorPayloadHasAny(ev, "t10_smf_url_localhost", "smf_url_localhost") ||
					(strings.Contains(msg, "smf") && strings.Contains(msg, "localhost:8002")) {
					return DiagnosticResult{
						Matched:     true,
						Explanation: "The AMF tried to create a PDU session but contacted a container-local SMF address.",
						RootCause:   "Docker networking misconfiguration: `localhost:8002` inside the AMF container points back to the AMF container, not the SMF service.",
						Fix:         "Set `QCORE_AMF_SMF_URL=http://smf:8002` in the 5G compose profile, or use the SMF service DNS name for any containerized AMF deployment.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	{
		id: "t10_pdu_session_accept_missing",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				msg := eventText(ev)
				if errorPayloadHasAny(ev, "t10_pdu_session_accept_missing", "pdu_session_accept_missing") ||
					(strings.Contains(msg, "pdu session establishment") &&
						(strings.Contains(msg, "accept") || strings.Contains(msg, "session establishment")) &&
						(strings.Contains(msg, "missing") || strings.Contains(msg, "not received") || strings.Contains(msg, "dropped"))) {
					return DiagnosticResult{
						Matched:     true,
						Explanation: "The SMF created the PDU session, but the UE did not receive or accept the 5GSM PDU Session Establishment Accept.",
						RootCause:   "AMF did not relay a spec-complete protected DL NAS Transport with mandatory 5GSM Accept IEs.",
						Fix: "Send a protected DL NAS Transport carrying PDU Session Establishment Accept with selected PDU session type, SSC mode, Authorized QoS Rules, Session-AMBR, and PDU address. " +
							"Pin the NAS bytes with a UERANSIM golden fixture.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	{
		id: "t10_data_plane_n2_n3_gap",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				msg := eventText(ev)
				if errorPayloadHasAny(ev, "t10_data_plane_n2_n3_gap", "data_plane_n2_n3_gap") ||
					(strings.Contains(msg, "pdu session") &&
						(strings.Contains(msg, "ping") || strings.Contains(msg, "data-plane") || strings.Contains(msg, "data plane")) &&
						(strings.Contains(msg, "failed") || strings.Contains(msg, "no external") || strings.Contains(msg, "no packet"))) {
					return DiagnosticResult{
						Matched:     true,
						Explanation: "PDU session signaling completed, but no UE packet was proven through the UPF.",
						RootCause:   "Missing N2/N3 data-plane completion: AMF must send NGAP PDU Session Resource Setup, SMF must PFCP-modify the UPF with the gNB N3 tunnel, and UPF must route through real TUN/NAT.",
						Fix: "Verify PDU Session Resource Setup Request/Response, PFCP Session Modification with gNB N3 IP/TEID, and UPF Linux TUN/NAT configuration. " +
							"Do not call T10 complete until `ping -I uesimtun0` succeeds.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	{
		id: "t10_upf_tun_unavailable",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				msg := eventText(ev)
				if errorPayloadHasAny(ev, "t10_upf_tun_unavailable", "upf_tun_unavailable") ||
					(strings.Contains(msg, "tun") &&
						(strings.Contains(msg, "/dev/net/tun") || strings.Contains(msg, "dummyegress") || strings.Contains(msg, "cap_net_admin") || strings.Contains(msg, "tun egress"))) {
					return DiagnosticResult{
						Matched:     true,
						Explanation: "The UPF could not use a real Linux TUN interface, so user-plane packets cannot prove external data flow.",
						RootCause:   "UPF runtime lacks `/dev/net/tun`, NET_ADMIN/CAP_NET_ADMIN, or Linux TUN support and fell back to dummy egress.",
						Fix:         "Run the UPF on Linux with `/dev/net/tun` mounted and NET_ADMIN enabled, assign the UE subnet to the TUN interface, enable IP forwarding, and configure NAT/MASQUERADE.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	// ── Catch-all string-matching rules (legacy; prefer typed rules above) ─────
	// These fire for cases where the NF did not emit a structured failure payload.

	// Built-in simulator failure injection: wrong Ki/OPc.
	{
		id: "simulator_wrong_ki",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				msg := strings.ToLower(ev.Message)
				if ev.Category == events.ErrorEvent && strings.Contains(msg, "wrong_ki") {
					res := diag.DiagnoseRegistrationFailure(diag.CauseAuthMACFailure, "", "", "built-in simulator wrong_ki scenario")
					return DiagnosticResult{
						Matched:     true,
						Explanation: res.Explanation,
						RootCause:   "Ki/OPc mismatch: the UE-side credentials do not match the subscriber credentials stored in QCore.",
						Fix:         res.FixUESide + " " + res.FixQCoreSide,
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	// Built-in simulator failure injection: unsupported PLMN.
	{
		id: "simulator_wrong_plmn",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				msg := strings.ToLower(ev.Message)
				if ev.Category == events.ErrorEvent && strings.Contains(msg, "wrong_plmn") {
					return DiagnosticResult{
						Matched:     true,
						Explanation: "The built-in simulator intentionally advertised a PLMN that does not match QCore's configured network.",
						RootCause:   "PLMN mismatch: the RAN is broadcasting or presenting a PLMN that QCore is not configured to serve.",
						Fix: "Set the gNB/eNB PLMN to match QCore's configured MCC/MNC, or update QCore's PLMN if the RAN value is correct. " +
							"For the Docker demo, use the default 001/01 PLMN.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	// Built-in simulator failure injection: unprovisioned subscriber.
	{
		id: "simulator_unprovisioned_imsi",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				msg := strings.ToLower(ev.Message)
				if ev.Category == events.ErrorEvent && strings.Contains(msg, "unprovisioned_imsi") {
					res := diag.DiagnoseRegistrationFailure(diag.CauseUnknownSubscriber, "", "", "built-in simulator unprovisioned_imsi scenario")
					return DiagnosticResult{
						Matched:     true,
						Explanation: res.Explanation,
						RootCause:   "Unknown subscriber: the UE identity is not provisioned in QCore's subscriber database.",
						Fix:         res.FixUESide + " " + res.FixQCoreSide,
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	// Built-in simulator failure injection: wrong core signaling address.
	{
		id: "simulator_wrong_mme_address",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				msg := strings.ToLower(ev.Message)
				if ev.Category == events.ErrorEvent && strings.Contains(msg, "wrong_mme_address") {
					return DiagnosticResult{
						Matched:     true,
						Explanation: "The built-in simulator intentionally dialed an address where QCore is not listening.",
						RootCause:   "Wrong core signaling endpoint: the RAN/UE simulator is pointed at the wrong MME S1AP or AMF NGAP address.",
						Fix:         "Set the simulator or RAN to QCore's advertised endpoint from the dashboard RAN config: S1AP 36412 for 4G or NGAP 38412 for 5G.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	// SCTP/NGAP transport issue — malformed PDU or connection error at the NGAP layer
	{
		id: "ngap_transport_error",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				msg := strings.ToLower(ev.Message)
				if ev.Category == events.ErrorEvent &&
					(strings.Contains(msg, "decode pdu") ||
						strings.Contains(msg, "ngap") && strings.Contains(msg, "malformed") ||
						strings.Contains(msg, "sctp") && strings.Contains(msg, "error")) {
					return DiagnosticResult{
						Matched:     true,
						Explanation: "An NGAP PDU could not be decoded or the SCTP transport reported an error.",
						RootCause:   "Malformed NGAP message or SCTP connection issue between gNB/eNB and QCore.",
						Fix: "Check the gNB/eNB NGAP library version and ensure it matches QCore's expected format. " +
							"If using UERANSIM, confirm you are on a supported version. " +
							"For SCTP errors, check kernel SCTP support and firewall rules on port 38412.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	// NAS algorithm / security capability mismatch
	{
		id: "nas_security_mismatch",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				msg := strings.ToLower(ev.Message)
				if ev.Category == events.ErrorEvent &&
					(strings.Contains(msg, "algorithm") ||
						strings.Contains(msg, "security mode reject") ||
						strings.Contains(msg, "ue security cap")) {
					return DiagnosticResult{
						Matched:     true,
						Explanation: "The UE and QCore could not agree on NAS security algorithms.",
						RootCause:   "NAS algorithm mismatch: QCore requires NIA2+NEA0 but the UE did not offer a compatible combination.",
						Fix: "Configure the UE to offer at least NIA2 (5G integrity) and NEA0 (null cipher). " +
							"In UERANSIM ue.yaml: set integrityAlgorithms: [NIA2] and encryptionAlgorithms: [NEA0]. " +
							"In a real UE: update the UE security capabilities in the modem configuration.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},

	// No connection attempt — heard nothing from gNB/UE after setup
	{
		id: "no_connection",
		matches: func(trace []events.Event) (DiagnosticResult, bool) {
			for _, ev := range trace {
				msg := strings.ToLower(ev.Message)
				if ev.Category == events.ErrorEvent &&
					(strings.Contains(msg, "connection refused") ||
						strings.Contains(msg, "no gno") ||
						strings.Contains(msg, "timeout") ||
						strings.Contains(msg, "no connection")) {
					return DiagnosticResult{
						Matched:     true,
						Explanation: "No gNB or eNB has connected to QCore's NGAP/S1AP listener.",
						RootCause:   "The base station has not attempted to connect, or the connection was refused.",
						Fix: "Verify the gNB/eNB is configured with QCore's IP address and port (38412 for NGAP, 36412 for S1AP). " +
							"Check that no firewall blocks SCTP/TCP on those ports. " +
							"In UERANSIM: set amfConfigs[0].address in gnb.yaml to QCore's IP.",
					}, true
				}
			}
			return DiagnosticResult{}, false
		},
	},
}

// Catalog is the structured diagnostic knowledge layer.
type Catalog struct{}

// NewCatalog returns a new diagnostic catalog.
func NewCatalog() *Catalog { return &Catalog{} }

// RuleCount returns the number of rules in the catalog. Tests assert this is
// at least a minimum value to prevent silent shrinkage.
func (c *Catalog) RuleCount() int { return len(catalog) }

// Analyze runs the catalog rules over the event trace and returns the first
// matching DiagnosticResult. Rules are tried in order; the first match wins.
// Typed payload rules run before string-matching catch-alls.
func (c *Catalog) Analyze(trace []events.Event) DiagnosticResult {
	if len(trace) == 0 {
		return DiagnosticResult{}
	}
	for _, r := range catalog {
		if result, ok := r.matches(trace); ok {
			return result
		}
	}
	return DiagnosticResult{}
}

// AnalyzeWithID runs the catalog and returns the rule ID that matched.
// Used by tests to assert a specific rule fired.
func (c *Catalog) AnalyzeWithID(trace []events.Event) (DiagnosticResult, string) {
	for _, r := range catalog {
		if result, ok := r.matches(trace); ok {
			return result, r.id
		}
	}
	return DiagnosticResult{}, ""
}

// --- helpers ---

func uint32Hex(v uint32) string {
	const hex = "0123456789abcdef"
	b := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		b[i] = hex[v&0xF]
		v >>= 4
	}
	return string(b)
}

func eventText(ev events.Event) string {
	parts := []string{ev.Message}
	if p, ok := ev.Payload.(events.ErrorPayload); ok {
		parts = append(parts, p.Code, p.Cause, p.Message)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func errorPayloadHasAny(ev events.Event, values ...string) bool {
	p, ok := ev.Payload.(events.ErrorPayload)
	if !ok {
		return false
	}
	code := strings.ToLower(p.Code)
	cause := strings.ToLower(p.Cause)
	msg := strings.ToLower(p.Message)
	for _, v := range values {
		needle := strings.ToLower(v)
		if code == needle || cause == needle || strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}
