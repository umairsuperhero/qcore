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
- `pkg/sbi/common` — shared TS 29.571 types (AccessAndMobilitySubscriptionData, AmbrRm, Nssai, Snssai) lifted out of pkg/udm + pkg/udr so cross-NF shapes don't drift.
- `pkg/udm` Nudm_SDM `GET /nudm-sdm/v2/{supi}/am-data` shipped. Refactored behind an `AmDataSource` interface: `NewStoreSource` wraps pkg/subscriber for direct mode; `NewUDRSource` wraps `pkg/udr.Client` for network mode — the UDM→UDR layering flip is a constructor-arg change.
- `pkg/udr` Nudr_DataRepository `GET /nudr-dr/v2/subscription-data/{ueId}/{servingPlmnId}/provisioned-data/am-data` shipped. Has a client (`pkg/udr.Client`) with typed errors (`ErrNotFound`, `ErrBadUeID`). End-to-end UDM→UDR chain test (`TestUDM_over_UDR_chain`) exercises both NFs over h2c loopback.
- 5G-AKA in `pkg/subscriber`: `DeriveKAUSF` (TS 33.501 Annex A.2), `DeriveRESStar` (Annex A.4), `Generate5GAuthVector`. Same Milenage core as 4G EPS-AKA, SQN shared per-subscriber.
- `pkg/udm` Nudm_UEAU `POST /nudm-ueau/v1/{supi}/security-information/generate-auth-data` shipped. `AuthSource` seam parallels `AmDataSource`; attach with `WithAuthSource`. Returns `AuthenticationInfoResult` with `Av5gHeAka`. Resync (AUTS) returns 501 until Milenage reverse is wired.

### Remaining
- `pkg/udm` — still to do: `Nudm_UECM` serving-AMF registration (needs pkg/amf).
- `pkg/udr` — still to do: authentication-subscription endpoint (waits on pkg/ausf).
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
| `pkg/sbi/common` | Shared TS 29.571 types | Shipping |
| `pkg/sbi/nrf` | NRF types + in-memory client for dev/tests | Phase 0 sketch (v0.5) |
| `pkg/udm` | Nudm SBI face (SDM + UEAU) + AmDataSource / AuthSource seams | Shipping (SDM am-data, UEAU 5G-AKA); UECM pending |
| `pkg/udr` | Nudr SBI face + client | Shipping (DR am-data); auth-subscription pending |
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
