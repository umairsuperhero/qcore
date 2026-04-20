# QCore — Claude Code Context

## What is QCore
Open-source 4G/5G core network in Go. GitHub: https://github.com/umairsuperhero/qcore  
**North star:** "The open-source core network that's actually easy to use" — DX is the differentiator over open5GS/Magma/free5GC.  
**Long-term ambition:** Beat commercial cores (Attocore, Druid) on 4G + 5G with best-in-class web UI, remote troubleshooting, local AI hotfixes, and a clear monetization layer.

## Strategic Direction
**5G SA is the primary track.** 4G EPC is supported legacy. 3GPP 5G SBA is HTTP/2 + OpenAPI — natively aligned with QCore's REST-first instincts. Private-5G (enterprise/campus/industrial) is where monetization lives. See `docs/rfc/0001-5g-sba-pivot.md` and `docs/ARCHITECTURE.md`.

## Shipped (working, tested)
- **Phase 1 — HSS:** Milenage verified vs 3GPP TS 35.208. REST API, PostgreSQL, Docker Compose, Prometheus metrics, CLI.
- **Phase 2 — MME:** Full S1AP/NAS attach — SCTP/TCP, PER codec, NAS security, KDF, AES-CMAC, GUTI attach, Detach, Paging. `TestEndToEndAttachOverWire` green.
- **Phase 3 — SPGW + GTP-U:** GTP-U v1 codec, collapsed SPGW, UE IP + TEID pools, S11-over-HTTP/JSON, Linux TUN egress (`//go:build linux`), 10 Prometheus metrics. `TestEndToEndUserPlane` green.

## Current Task — v0.5 Subscriber Plane Refactor
**Goal:** Unified subscriber library usable by both the legacy 4G admin REST API and the 5G SBI faces (UDM/UDR/AUSF).  
**Milestone:** A subscriber added via CLI is queryable via both the admin REST API (legacy) and N8/Nudr (5G SBI).

### Done
- `pkg/subscriber` extracted from the old `pkg/hss`: model + validation + DB ops, Milenage (`F2345`, `GenerateOPc`, `GenerateAuthVector`), SQN management. Verified against 3GPP TS 35.208.
- `pkg/subscriber/admin` hosts the operator-facing REST API (CRUD, CSV import/export, on-demand auth vectors). Consumed by `cmd/hss`.
- `pkg/hss` retired. `cmd/hss` now imports `pkg/subscriber` + `pkg/subscriber/admin` directly. The `hss` binary name is kept for operator familiarity.
- `pkg/sbi` Phase 0 sketch: HTTP/2 (h2/h2c) server, client, RFC 7807 ProblemDetails, RequestID/AccessLog/Recover middleware, round-trip test green.
- `pkg/sbi/nrf` Phase 0 sketch: NFProfile/NFService types, in-memory Client for single-process dev + tests.

### Remaining
- `pkg/udm` — first cut landed: `Nudm_SDM` `GET /nudm-sdm/v2/{supi}/am-data` returns AccessAndMobilitySubscriptionData over pkg/sbi, round-trip tested in h2c. Still to do: `Nudm_UEAU` generate-auth-data (needs 5G-AKA derivation from TS 33.501 Annex A — current pkg/subscriber Milenage yields a 4G EPS-AKA vector, not 5G) and `Nudm_UECM` serving-AMF registration.
- `pkg/udr` — first cut landed: `Nudr_DataRepository` `GET /nudr-dr/v2/subscription-data/{ueId}/{servingPlmnId}/provisioned-data/am-data` over pkg/sbi, round-trip tested in h2c. Still to do: authentication-subscription endpoint (waits on pkg/ausf) and wiring pkg/udm's SubscriberStore to a UDRClient (network-mode layering) as a later cut.
- `pkg/ausf` — Nausf SBI face (authentication), calls UDM UEAU.
- Tests for `pkg/subscriber/admin` (moved but untested).

## Roadmap (after v0.5)
- **v0.6** — NGAP + 5G-NAS + AMF + NRF. Milestone: UERANSIM 5G UE completes REGISTRATION.
- **v0.7** — PFCP + SMF + UPF (dual-mode). Milestone: UERANSIM 5G PDU session + ping.
- **v0.8** — Dashboard (Next.js): subscriber CRUD, service topology, live attach flow visualiser.
- **v0.9** — 4G legacy refactor on unified subscriber plane.
- **v1.0** — OTel tracing, mTLS, Helm chart, conformance suite.

## Package Map
| Package | Purpose | Status |
|---------|---------|--------|
| `pkg/subscriber` | Subscriber storage + Milenage (unified 4G/5G) | Shipping |
| `pkg/subscriber/admin` | Operator-facing REST API (CRUD, CSV, auth vectors) | Shipping |
| `pkg/mme` | MME + S1AP + S6a client + S11 client | Shipping |
| `pkg/s1ap` | S1AP PER codec | Shipping → PER will extract to `pkg/asn1per` |
| `pkg/nas` | NAS codec + security | Shipping → will split nas4g/nas5g |
| `pkg/sctp` | SCTP + TCP fallback | Shipping |
| `pkg/gtp` | GTP-U v1 codec | Shipping |
| `pkg/spgw` | SPGW + TUN egress + metrics | Shipping → will evolve to `pkg/upf` |
| `pkg/sbi` | HTTP/2 + RFC 7807 SBI framework | Phase 0 sketch (v0.5) |
| `pkg/sbi/nrf` | NRF types + in-memory client for dev/tests | Phase 0 sketch (v0.5) |
| `pkg/udm` | Nudm SBI face | Shipping (SDM am-data); UEAU + UECM pending |
| `pkg/udr` | Nudr SBI face | Shipping (DR am-data); auth-subscription + UDM→UDR wiring pending |
| `pkg/ausf` | Nausf SBI face | Planned (v0.5) |
| `pkg/nrf` | NRF service (Nnrf over SBI) | Planned (v0.6) |
| `pkg/amf` | AMF core | Planned (v0.6) |

## Tech Stack
- **Go 1.23**, logrus, cobra+viper, gorm, gorilla/mux, prometheus/client_golang, golang.org/x/sys
- **PostgreSQL 16**, Docker Compose
- **Next.js + TypeScript** (dashboard, v0.8)
- Go module: `github.com/qcore-project/qcore`

## Dev Environment
- Windows machine (XPS-15), working via WSL Ubuntu-24.04
- Project path: `/mnt/c/Users/XPS-15/Documents/Software/qcore` (WSL)
- Docker Desktop must be running for integration tests

## Common Commands
```bash
# Build everything
go build ./...

# Run all tests
go test ./...

# Run HSS self-tests (crypto verification)
go run ./cmd/hss test

# Start HSS server
go run ./cmd/hss start

# Start with Docker (full stack)
docker compose up
```

## Code Conventions
- Error wrapping: `fmt.Errorf("context: %w", err)`
- Table-driven tests with testify/assert
- Structured logging via `pkg/logger` (never import logrus directly)
- Config via `pkg/config` (viper, YAML + env vars with QCORE_ prefix)
- Linux-only code uses `//go:build linux` build tags
- All crypto verified against 3GPP TS 35.208 test vectors

## Design Principles
1. **DX first** — prefer HTTP/JSON over obscure binary protocols where standards allow
2. **Test vectors** — all crypto must be verified against 3GPP TS 35.208 test sets
3. **No magic** — errors are explicit, logs are structured, metrics are Prometheus
4. **Honest feedback** — push back on tech debt and short-term thinking
