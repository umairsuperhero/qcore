// Package sbi provides shared plumbing for 3GPP 5G Service-Based Interfaces.
//
// Per 3GPP TS 29.500 §6, every 5G control-plane Network Function (AMF, SMF,
// AUSF, UDM, UDR, NRF, PCF, ...) exposes its API over HTTP/2 using REST
// conventions, JSON bodies, and RFC 7807 Problem Details for errors. QCore
// builds every 5G NF on top of this package so we get a single consistent
// dev + production posture:
//
//	- HTTP/2 native (h2 over TLS in prod; h2c plaintext in dev for loopback DX)
//	- Structured ProblemDetails responses
//	- Request-ID correlation across services
//	- Panic recovery + access logging middleware
//	- NRF-based service discovery (see pkg/sbi/nrf)
//	- OpenAPI validation hook (wire a spec into Server.OpenAPIValidator to turn on)
//
// This package is deliberately thin — it does NOT define 5G wire types
// (AuthenticationInfoRequest, SmContextCreateData, etc.); those live in each
// NF's pkg and are generated from 3GPP YAML specs when we get there.
//
// Status: sketch. Phase 0 deliverable per docs/rfc/0001-5g-sba-pivot.md.
// Expect shape to evolve as the first real NF (UDM/UDR, planned for v0.5)
// lands on it.
package sbi
