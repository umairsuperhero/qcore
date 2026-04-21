// Package ausf is QCore's 5G Authentication Server Function.
//
// Per 3GPP TS 23.501 §6.2.4, the AUSF sits between the AMF/SEAF and the
// home network's UDM. It is the only NF that speaks to both sides of the
// 5G-AKA transaction:
//
//	AMF ── POST /nausf-auth/v1/ue-authentications ──▶ AUSF
//	                                                   │
//	                            ┌──────────────────────┘
//	                            ▼
//	   POST /nudm-ueau/v1/{supi}/security-information/generate-auth-data ──▶ UDM
//
// The UDM returns an Av5gHeAka (RAND, AUTN, XRES*, KAUSF). AUSF keeps
// KAUSF and XRES* for itself, compresses XRES* to HXRES* per TS 33.501
// Annex A.5, and hands {RAND, AUTN, HXRES*} (Av5gAka) to the AMF. The
// AMF drives the UE challenge; the UE sends RES* back; AMF forwards it:
//
//	AMF ── PUT /nausf-auth/v1/ue-authentications/{ctx}/5g-aka-confirmation ──▶ AUSF
//
// AUSF compares RES* to the stored XRES* and — on match — derives KSEAF
// per TS 33.501 Annex A.6 and returns it.
//
// Scope for v0.5:
//
//	POST /nausf-auth/v1/ue-authentications              — Av5gAka creation
//	PUT  /nausf-auth/v1/ue-authentications/{ctx}/5g-aka-confirmation
//	                                                    — RES* → KSEAF
//
// The auth-context store is in-memory. That's fine for single-instance
// dev and CI; v1.0 will want a shared store (Redis/etcd) so AMF can hit
// any AUSF replica on the confirmation leg.
//
// Intentionally deferred:
//   - EAP-AKA' (TS 33.501 §6.1.3.1) — waits on non-3GPP access
//   - SUCI concealment/deconcealment (TS 33.501 §6.12) — SUPI-only today
//   - UDM selection via NRF — single static UDM per AUSF for now
package ausf
