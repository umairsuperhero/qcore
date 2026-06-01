// Package events defines the structured event schema emitted by every QCore
// network function. The envelope is protocol-agnostic; Payload carries
// protocol-specific decoded detail. This is the substrate consumed by the
// live observability UI (Phase B) and the AI diagnostic layer (Phase C).
package events

import (
	"context"
	"time"
)

// JourneyIDHeader is the HTTP header used to propagate a journey ID across
// network function boundaries. Every inter-NF request from the MME outward
// carries this header so downstream NFs can tag their own events with the
// same ID without doing any identity translation.
const JourneyIDHeader = "X-QCore-Journey-ID"

type journeyCtxKey struct{}

// WithJourneyID embeds the journey ID into the context.
func WithJourneyID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, journeyCtxKey{}, id)
}

// JourneyIDFromContext extracts the journey ID from the context.
func JourneyIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(journeyCtxKey{}).(string)
	return id
}

// Category classifies what kind of event occurred.
type Category string

const (
	SignalingRx     Category = "signaling_rx"     // message received from a peer
	SignalingTx     Category = "signaling_tx"     // message sent to a peer
	StateTransition Category = "state_transition" // NF internal state machine change
	ConfigChange    Category = "config_change"    // configuration loaded or validated
	ErrorEvent      Category = "error"            // failure with a cause code
)

// Severity indicates operational importance.
type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

// Event is the canonical record emitted by every network function. The
// envelope is protocol-agnostic; Payload carries protocol-specific detail.
// All fields are JSON-serialisable so the collector can store and forward
// them without schema knowledge.
type Event struct {
	ID        string      `json:"id"`
	JourneyID string      `json:"journey_id,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	NF        string      `json:"nf"`
	Category  Category    `json:"category"`
	Severity  Severity    `json:"severity"`
	Protocol  string      `json:"protocol,omitempty"` // "s1ap" | "nas4g" | "gtp" | "sbi" | …
	Message   string      `json:"message"`
	Payload   interface{} `json:"payload,omitempty"`
}

// --- 4G EPC payload types ---

// S1SetupPayload carries eNodeB identification from S1 Setup.
type S1SetupPayload struct {
	ENBName string `json:"enb_name,omitempty"`
	ENBID   uint32 `json:"enb_id"`
	PLMN    string `json:"plmn,omitempty"`
	Success bool   `json:"success"`
}

// AttachRequestPayload carries NAS Attach Request fields.
type AttachRequestPayload struct {
	IMSI       string `json:"imsi,omitempty"`
	AttachType int    `json:"attach_type"`
	TAC        uint16 `json:"tac"`
}

// AuthRequestPayload carries the outbound NAS Authentication Request.
// RAND is truncated to the first 8 hex chars to avoid leaking the full vector.
type AuthRequestPayload struct {
	IMSI    string `json:"imsi"`
	RANDHex string `json:"rand_prefix"` // first 16 chars of RAND hex (8 bytes)
}

// AuthResponsePayload records the outcome of a NAS Authentication Response.
type AuthResponsePayload struct {
	IMSI    string `json:"imsi"`
	Success bool   `json:"success"`
	Cause   string `json:"cause,omitempty"`
}

// SecurityModePayload carries NAS Security Mode Command/Complete detail.
type SecurityModePayload struct {
	IMSI      string `json:"imsi"`
	CipherAlg uint8  `json:"cipher_alg"`
	IntegAlg  uint8  `json:"integ_alg"`
}

// SessionCreatePayload carries S11 Create Session fields.
type SessionCreatePayload struct {
	IMSI    string `json:"imsi"`
	APN     string `json:"apn,omitempty"`
	UEIP    string `json:"ue_ip,omitempty"`
	SGWTEID uint32 `json:"sgw_teid,omitempty"`
}

// SessionDeletePayload records a session teardown.
type SessionDeletePayload struct {
	IMSI string `json:"imsi"`
}

// AttachCompletePayload records a UE reaching EMMRegistered.
type AttachCompletePayload struct {
	IMSI    string `json:"imsi"`
	UEIP    string `json:"ue_ip"`
	SGWTEID uint32 `json:"sgw_teid"`
}

// StateTransitionPayload records an NF state machine change.
type StateTransitionPayload struct {
	IMSI string `json:"imsi,omitempty"`
	From string `json:"from"`
	To   string `json:"to"`
}

// AuthVectorPayload records HSS auth vector generation.
type AuthVectorPayload struct {
	IMSI string `json:"imsi"`
}

// ErrorPayload carries failure detail with a cause code.
type ErrorPayload struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Cause   string `json:"cause,omitempty"`
}

// ModifyBearerPayload records S11 Modify Bearer (eNB TEID learned).
type ModifyBearerPayload struct {
	IMSI    string `json:"imsi"`
	ENBAddr string `json:"enb_addr,omitempty"`
	ENBTEID uint32 `json:"enb_teid,omitempty"`
}

// --- 5G SA payload types ---

// --- NG Setup lifecycle payloads ---
// Each payload corresponds to one step the hero screen (docs/ui-ux-design.md §2)
// needs in order to show "Waiting → Connected / Failed" with cause and fix.

// GNBConnectedPayload fires the moment a TCP/SCTP association is accepted —
// before any NGAP is exchanged. Feeds the "Waiting for gNB…" → in-progress state.
type GNBConnectedPayload struct {
	GNBAssocID string `json:"gnb_assoc_id"` // stable ID for this connection
	SourceIP   string `json:"source_ip"`    // remote IP:port
}

// NGSetupReceivedPayload fires after DecodeNGSetupRequest succeeds — the raw
// fields the gNB advertised, decoded with the canonical ident codec.
type NGSetupReceivedPayload struct {
	GNBAssocID  string     `json:"gnb_assoc_id"`
	GNBName     string     `json:"gnb_name,omitempty"`
	GNBID       uint64     `json:"gnb_id"`
	GNBMCC      string     `json:"gnb_mcc"` // decoded from GlobalRANNodeID PLMN
	GNBMNC      string     `json:"gnb_mnc"`
	OfferedTACs []string   `json:"offered_tacs"` // hex TAC strings
	OfferedSNSSAIs []SNSSAIDesc `json:"offered_snssais"`
}

// SNSSAIDesc is a displayable slice identifier.
type SNSSAIDesc struct {
	SST uint8  `json:"sst"`
	SD  string `json:"sd,omitempty"` // hex, omitted when absent
}

// NGSetupPayload carries gNB identification from NG Setup.
// Kept for backward-compat with existing dashboard/collector consumers.
type NGSetupPayload struct {
	GNBAssocID string `json:"gnb_assoc_id"`
	GNBName    string `json:"gnb_name,omitempty"`
	GNBID      uint64 `json:"gnb_id"`
	PLMN       string `json:"plmn,omitempty"` // "MCC/MNC" human-readable
	TAC        string `json:"tac,omitempty"`  // hex of first TAC
	Success    bool   `json:"success"`
	// On failure:
	FailureCause    string `json:"failure_cause,omitempty"`    // e.g. "plmn_mismatch"
	FailureExplain  string `json:"failure_explain,omitempty"`  // plain English
	FixGNBSide      string `json:"fix_gnb_side,omitempty"`
	FixQCoreSide    string `json:"fix_qcore_side,omitempty"`
}

// RegistrationRequestPayload carries NAS 5G Registration Request fields.
type RegistrationRequestPayload struct {
	SUCI         string `json:"suci,omitempty"`
	RegType      int    `json:"reg_type"`
	RequestedNSSAI []string `json:"requested_nssai,omitempty"`
}

// AuthRequestPayload5G carries the outbound 5G NAS Authentication Request.
type AuthRequestPayload5G struct {
	SUPI    string `json:"supi"`
	RANDHex string `json:"rand_prefix"` // first 16 chars of RAND hex (8 bytes)
}

// SecurityModeCommandPayload5G carries 5G NAS Security Mode Command/Complete detail.
type SecurityModeCommandPayload5G struct {
	SUPI      string `json:"supi"`
	CipherAlg uint8  `json:"cipher_alg"`
	IntegAlg  uint8  `json:"integ_alg"`
}

// RegistrationAcceptPayload records a UE reaching 5GMM-REGISTERED.
type RegistrationAcceptPayload struct {
	SUPI string `json:"supi"`
	GUTI string `json:"guti,omitempty"`
}

// PDUSessionEstablishmentPayload carries Nsmf_PDUSession Create/Modify fields.
type PDUSessionEstablishmentPayload struct {
	SUPI         string `json:"supi"`
	PDUSessionID uint8  `json:"pdu_session_id"`
	DNN          string `json:"dnn,omitempty"`
	UEIP         string `json:"ue_ip,omitempty"`
	UPFTEID      uint32 `json:"upf_teid,omitempty"`
}

// PFCPAssociationPayload carries N4 Association Setup detail.
type PFCPAssociationPayload struct {
	NodeID  string `json:"node_id"`
	Success bool   `json:"success"`
}

// PFCPSessionEstablishmentPayload carries N4 Session Establishment detail.
type PFCPSessionEstablishmentPayload struct {
	FSEID   uint64 `json:"fseid"`
	UPFTEID uint32 `json:"upf_teid,omitempty"`
}

// --- NRF lifecycle payload types ---

// NFRegisteredPayload records an NF successfully registering with the NRF.
type NFRegisteredPayload struct {
	NFType       string `json:"nf_type"`
	NFInstanceID string `json:"nf_instance_id"`
	NRFURL       string `json:"nrf_url"`
}

// NFDiscoveredPayload records an NF discovering a peer via the NRF.
type NFDiscoveredPayload struct {
	RequesterType string `json:"requester_type"`
	TargetType    string `json:"target_type"`
	ServiceName   string `json:"service_name,omitempty"`
	ResolvedURL   string `json:"resolved_url"`
	Fallback      bool   `json:"fallback,omitempty"` // true when NRF was unreachable
}

// --- Registration + PDU session diagnostic payload types ---
// These payloads feed the diag layer rules and the frontend live trace view.
// Every registration failure MUST emit a RegistrationFailurePayload so the
// diag layer can reason over the trace without inspecting raw error strings.

// RegistrationFailurePayload is emitted whenever a 5G registration attempt
// fails for any reason. Cause is a machine-readable tag; the diag layer maps
// it to plain-language explanation + fix. This is the SINGLE failure event
// the frontend traces against — do not emit generic error events for reg failures.
type RegistrationFailurePayload struct {
	SUPI  string `json:"supi,omitempty"`  // resolved SUPI if available, else empty
	SUCI  string `json:"suci,omitempty"`  // raw SUCI string if SUPI not resolved
	Cause string `json:"cause"`           // machine tag: see pkg/diag/registration.go for the full list
	// Detail carries the raw error string for debugging; not shown to end users.
	Detail string `json:"detail,omitempty"`
}

// AuthenticationFailurePayload is emitted when the UE sends an Authentication
// Failure (MAC failure, SQN failure, etc.) — distinct from a network-side reject.
type AuthenticationFailurePayload struct {
	SUPI      string `json:"supi,omitempty"`
	Cause5GMM uint8  `json:"cause_5gmm"` // TS 24.501 §9.11.3.2 cause value
	CauseName string `json:"cause_name"` // e.g. "mac_failure", "sqn_failure"
}

// SUCIDecodeFailurePayload is emitted when the AMF cannot decode the UE's
// mobile identity (malformed SUCI, unsupported protection scheme, etc.).
type SUCIDecodeFailurePayload struct {
	RawIdentity string `json:"raw_identity_hex"` // hex of the raw IE bytes
	Reason      string `json:"reason"`           // e.g. "unsupported_protection_scheme"
	Scheme      uint8  `json:"scheme,omitempty"` // protection scheme byte (0=null, 1+=ECIES)
}

// PDUSessionResultPayload is emitted when the SMF responds to a PDU session
// creation request — whether success or failure.
type PDUSessionResultPayload struct {
	SUPI         string `json:"supi"`
	PDUSessionID uint8  `json:"pdu_session_id"`
	DNN          string `json:"dnn,omitempty"`
	Success      bool   `json:"success"`
	Cause        string `json:"cause,omitempty"` // machine tag on failure: "smf_reject", "ip_pool_exhausted", …
	Detail       string `json:"detail,omitempty"`
}

// --- 5G NF instrumentation payload types (C1 / T7) ---

type AUSFAuthRequestPayload struct {
	SupiOrSuci         string `json:"supi_or_suci"`
	ServingNetworkName string `json:"serving_network_name,omitempty"`
}
type AUSFAuthVectorPayload struct {
	SUPI string `json:"supi"`
}
type AUSFAuthFailurePayload struct {
	SupiOrSuci string `json:"supi_or_suci"`
	Reason     string `json:"reason"`
}
type AUSFAuthResultPayload struct {
	SUPI    string `json:"supi"`
	Success bool   `json:"success"`
}
type UDMAuthDataPayload struct {
	SupiOrSuci string `json:"supi_or_suci"`
	SUPI       string `json:"supi,omitempty"`
	Success    bool   `json:"success"`
	Reason     string `json:"reason,omitempty"`
}
type SMFSessionPayload struct {
	SUPI         string `json:"supi"`
	PDUSessionID int    `json:"pdu_session_id"`
	DNN          string `json:"dnn,omitempty"`
	UEIP         string `json:"ue_ip,omitempty"`
	Success      bool   `json:"success"`
	Cause        string `json:"cause,omitempty"`
}
type UPFPFCPAssocPayload struct {
	RemoteAddr string `json:"remote_addr"`
	Success    bool   `json:"success"`
}
type UPFPFCPSessionPayload struct {
	SEID      uint64 `json:"seid"`
	LocalTEID uint32 `json:"local_teid"`
	Success   bool   `json:"success"`
}
