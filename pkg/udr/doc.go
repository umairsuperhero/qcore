// Package udr is QCore's 5G Unified Data Repository.
//
// Per 3GPP TS 23.501 §6.2.8, the UDR is the stateful storage layer for
// subscription data, policy data, and application data. Other NFs (most
// notably the UDM) read and write via the Nudr_DataRepository SBI (TS
// 29.504 / 29.505) rather than touching the database directly. This
// keeps the data schema behind a single wire contract.
//
// Scope for v0.5 — just enough to demonstrate the layering seam:
//
//	GET /nudr-dr/v2/subscription-data/{ueId}/{servingPlmnId}/provisioned-data/am-data
//	    → AccessAndMobilitySubscriptionData
//
// Architectural note — QCore today has a single subscriber library
// (pkg/subscriber) that both pkg/udm and pkg/udr read from through the
// same SubscriberStore interface. In a strict 3GPP deployment UDM would
// only ever read via UDR; for QCore's single-binary dev posture the
// extra network hop is waste, so UDM keeps direct access and UDR is a
// parallel SBI face on the same store. When pkg/udr gains its own
// storage (distinct schema, its own PostgreSQL tables) the flip to
// UDM-over-UDR will be a client-swap inside pkg/udm, not a refactor.
//
// Intentionally deferred:
//   - Policy data repository (TS 29.519) — waits on PCF
//   - Application data repository — waits on v1.0 scope decision
//   - Authentication subscription data endpoint — waits on pkg/ausf
package udr
