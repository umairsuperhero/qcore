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

// NGSetupPayload carries gNB identification from NG Setup.
type NGSetupPayload struct {
	GNBName string `json:"gnb_name,omitempty"`
	GNBID   uint64 `json:"gnb_id"`
	PLMN    string `json:"plmn,omitempty"`
	Success bool   `json:"success"`
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
