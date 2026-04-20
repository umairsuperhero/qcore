// Package udm is QCore's 5G Unified Data Management function.
//
// Per 3GPP TS 23.501 §6.2.7, the UDM is the UE's authoritative subscription
// owner in the 5G control plane. It exposes several services over SBI
// (TS 29.503):
//
//	Nudm_SDM   — Subscription Data Management (AMF reads AM/SM data here)
//	Nudm_UEAU  — UE Authentication (AUSF asks for a 5G auth vector)
//	Nudm_UECM  — UE Context Management (serving AMF/SMF registration)
//	Nudm_EE    — Event Exposure (not in v0.5 scope)
//	Nudm_PP    — Parameter Provisioning (not in v0.5 scope)
//
// This package is the first real NF to land on pkg/sbi. Scope for v0.5:
//
//	GET  /nudm-sdm/v2/{supi}/am-data   — AccessAndMobilitySubscriptionData
//
// That single endpoint is enough to hit the v0.5 milestone: a subscriber
// added via the admin REST API is queryable over 5G SBI. UEAU and UECM
// are stubbed for v0.6 when AMF + AUSF land — UEAU specifically needs the
// 5G-AKA derivation from TS 33.501 Annex A, which the current Milenage in
// pkg/subscriber doesn't yet produce (it yields a 4G EPS-AKA vector with
// KASME, not a 5G vector with KAUSF + RES*).
//
// UDM here intentionally talks to pkg/subscriber directly through a small
// SubscriberStore interface rather than to a pkg/udr client. In a strict
// 3GPP deployment UDM-over-UDR is the correct layering; for QCore's
// single-binary dev posture it would be wasted indirection. When pkg/udr
// lands as a real NF, UDM will get a network-mode that routes through it.
package udm
