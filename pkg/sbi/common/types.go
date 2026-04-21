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
