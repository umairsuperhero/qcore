// Package common holds 3GPP TS 29.571 CommonData schema types that are
// shared across multiple 5G SBI interfaces. 29.571 is the 3GPP specifier
// for cross-NF data shapes (AMBR, NSSAI, S-NSSAI, PlmnId, ...); Nudm_SDM
// and Nudr_DataRepository both reference it, so putting copies in
// pkg/udm and pkg/udr would just be duplication waiting to drift.
//
// Scope is deliberately narrow: only types QCore actually populates
// today. Fields get added as downstream NFs start consuming them —
// 29.571 has dozens of types we don't use yet.
package common

// AccessAndMobilitySubscriptionData — TS 29.571 / TS 29.519 §5.2.2.3.
// Returned by both Nudm_SDM `GET /am-data` and Nudr_DR `.../am-data`.
// UDM reads this from UDR and re-serves it verbatim to AMF.
type AccessAndMobilitySubscriptionData struct {
	Gpsis            []string `json:"gpsis,omitempty"`
	SubscribedUeAmbr *AmbrRm  `json:"subscribedUeAmbr,omitempty"`
	Nssai            *Nssai   `json:"nssai,omitempty"`
	RatRestrictions  []string `json:"ratRestrictions,omitempty"`
}

// AmbrRm — TS 29.571 §5.4.2.6. Aggregate Maximum Bit Rate, removable
// variant. Strings like "1 Gbps" per the spec.
type AmbrRm struct {
	Uplink   string `json:"uplink"`
	Downlink string `json:"downlink"`
}

// Nssai — TS 29.571 §5.4.4.1. Network Slice Selection Assistance
// Information.
type Nssai struct {
	DefaultSingleNssais []Snssai `json:"defaultSingleNssais,omitempty"`
	SingleNssais        []Snssai `json:"singleNssais,omitempty"`
}

// Snssai — TS 29.571 §5.4.4.2. Single NSSAI: SST (0–255) and optional
// SD (6 hex digits).
type Snssai struct {
	Sst int    `json:"sst"`
	Sd  string `json:"sd,omitempty"`
}

// AuthenticationSubscription — TS 29.503 §6.3.6.2.2. Served by Nudr
// under /authentication-data/authentication-subscription and consumed
// by Nudm UEAU to run Milenage. TS 29.505 §5.2.2.3 re-uses the shape.
//
// Spec expects EncPermanentKey and EncOpcKey to be ciphertext wrapped
// with a protection-parameter key. QCore v0.5 doesn't yet do
// encryption-at-rest, so those fields carry the plaintext 32-char hex
// strings of K/OPc. Callers must not assume encryption until this
// package wires a KMS.
type AuthenticationSubscription struct {
	// AuthenticationMethod: "5G_AKA" or "EAP_AKA_PRIME".
	AuthenticationMethod string `json:"authenticationMethod"`

	// EncPermanentKey — long-term key K (Ki). 32 hex chars (plaintext in v0.5).
	EncPermanentKey string `json:"encPermanentKey,omitempty"`

	// EncOpcKey — operator variant OPc. 32 hex chars (plaintext in v0.5).
	EncOpcKey string `json:"encOpcKey,omitempty"`

	// AuthenticationManagementField — 4 hex chars.
	AuthenticationManagementField string `json:"authenticationManagementField,omitempty"`

	// AlgorithmId: "milenage" (QCore only supports milenage today).
	AlgorithmId string `json:"algorithmId,omitempty"`

	// SequenceNumber carries SQN state for replay protection.
	SequenceNumber *SequenceNumber `json:"sequenceNumber,omitempty"`

	// Supi is echoed back for the caller's convenience.
	Supi string `json:"supi,omitempty"`
}

// SequenceNumber — TS 29.503 §6.3.6.2.2. Simplified: QCore carries only
// the SQN hex string today, using the "GENERAL" scheme. Full spec also
// carries lastIndexes / indLength / difSign for SQN-list scheme; we'll
// add those when a second auth vector in flight actually needs them.
type SequenceNumber struct {
	SqnScheme string `json:"sqnScheme,omitempty"` // "GENERAL" | "NON_TIME_BASED" | "TIME_BASED"
	Sqn       string `json:"sqn,omitempty"`       // 12 hex chars
}
