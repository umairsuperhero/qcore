# QCore Wiki

> Living reference. Refreshed at the end of each build session, on every milestone,
> and on the recurring audit cadence (see `docs/audit-v1.0.md` §7).
> Last updated: 2026-06-08
>
> **Authoritative docs:** `docs/experience-charter.md` (vision + scope) · `CLAUDE.md` (build order) · `docs/audit-v1.0.md` (living baseline audit + long-term decisions D-1…D-4) · `docs/v1-gap-closure-plan.md` (executable plan to reach v1 — tracks A–E, sequenced, per-task acceptance criteria)

---

## Table of Contents

1. [What QCore Is](#1-what-qcore-is)
2. [Project Status](#2-project-status)
3. [Architecture](#3-architecture)
4. [Package Map](#4-package-map)
5. [5G SA Track](#5-5g-sa-track)
6. [4G EPC — How It Works](#6-4g-epc--how-it-works)
7. [Event Model (Phase A)](#7-event-model-phase-a)
8. [Dashboard (Phase B)](#8-dashboard-phase-b)
9. [Phase C — Diagnostic AI](#9-phase-c--diagnostic-ai)
10. [Running QCore Locally](#10-running-qcore-locally)
11. [Key Design Decisions](#11-key-design-decisions)
12. [Glossary](#12-glossary)

---

## 1. What QCore Is

QCore is a **development and test environment** for cellular networks — not a production 5G core competing on protocol features.

**Primary user:** the RAN/device developer who needs a core to test against.

**How QCore wins:** fast start (< 5 min to first attach), deep observability (every signaling message is a structured, queryable event), and AI that explains failures in plain language — not log dumps.

QCore is **not** trying to be open5GS or free5GC. Those optimize for spec coverage. QCore optimizes for developer joy inside a well-defined subset of the spec.

---

## 2. Project Status

| Track | What | Status |
|-------|------|--------|
| 4G EPC | HSS, MME (S1AP, NAS, Milenage, KASME), SPGW (GTP-U, S11, Linux TUN egress). End-to-end attach + uplink verified. | ✅ Shipped |
| Phase A — Event model | `pkg/events` structured schema, journey-ID correlation, HTTP emitter. `cmd/qcore-collector` SSE stream + journey store. 4G and 5G NFs instrumented (5G via C1/T7). | ✅ Shipped |
| Phase B — Golden Path | `make up` one-command launch. Dashboard (port 3000): health view, subscriber management, live event trace, RAN-connect config panel. Built-in simulator with error-injection scenarios. | ✅ Shipped |
| 5G SA control plane | AMF/AUSF/UDM/UDR/NRF with binary entrypoints, Dockerfiles, compose entries. Registration flow passes in the in-process E2E test. UERANSIM T10 now reaches PDU session establishment on cloud Linux; the SMC-integrity, InitialContextSetup APER, Registration Accept IE-length, protected NAS routing, and PDU Session Establishment Accept blockers are validated fixed against real UERANSIM. | ✅ Works in E2E / 🔭 T10 data-plane pending |
| 5G SA user plane | `pkg/pfcp` codec, `pkg/smf` + `cmd/smf`, `pkg/upf` + `cmd/upf`. Builds, unit-tested, exercised by the E2E test (Registration → PDU session → GTP-U tunnel). | ✅ Builds + E2E |
| Phase C — Diagnostic AI (catalog) | `pkg/ai/catalog.go`: 13 typed symptom→cause rules across ≥9 cause categories (4G + 5G) + optional Gemini escalation; wired to the dashboard diagnose endpoint. | ✅ Shipped (catalog) |
| Phase C — Diagnostic AI (offline SLM) | `pkg/ai` local provider + `make up-ai` llama.cpp sidecar (baked Qwen2.5-1.5B GGUF); catalog runs first, SLM handles misses over the same grounded prompt. Code merged + unit-tested green; live model-serve (real GGUF pull / air-gapped) not yet validated. | ✅ Code merged / 🔭 live-serve pending |
| **Interop hardening (I1–I4 / D-1…D-4)** | D-1 PLMN codec (`pkg/ident`) · D-2 NRF register/discover · D-3 real SUCI + genuine unprovisioned-IMSI reject · D-4 N11 AMF→SMF (E2E no longer fakes the SMF call). All merged to main. | ✅ Complete |
| Dashboard experience layer | gNB-connection hero screen (Gate 1) + animated live signaling-trace view with progressive disclosure. | ✅ Shipped |
| 5G telemetry (T7 / C1) | Journey-correlated events across AMF/AUSF/UDM/SMF/UPF; one correlated trace per 5G registration (`TestC1_RegistrationEventTrace`, PR #25). | ✅ Shipped |
| 5G simulator UX + dashboard 5G mode (T8/T9 / C2/C3) | Error injection on the real-SUCI 5G sim; protocol selector, 5G sim controls, UDR view. | 🔭 Next |
| UERANSIM real-RAN validation (T10) | Native SCTP + NGSetup + InitialUEMessage + Authentication Request/Response + Security Mode Control + InitialContextSetup + Registration Complete now reproduced with UERANSIM. AMF decodes the UE's PDU Session Establishment Request, SMF returns `201` for Create SM Context, AMF sends a protected PDU Session Establishment Accept, and UERANSIM reports PDU session establishment success. Current gap: no external UE→UPF→peer data-plane packet or ping claim exists. | 🔭 Data-plane pending |
| Phase D — Workflow adoption | Scenario authoring, CI hooks, Learning Mode. | 🔭 Planned |

> **Verification note:** as of 2026-06-06 every package compiles, `go vet` is clean,
> `go test ./...` and `go test -race ./...` pass (verified in `golang:1.23`), and the
> dashboard front-end builds and type-checks with `npm run build` + `tsc --noEmit`.
> "Works in E2E" /
> "Builds + E2E" mean the automated test passes — **not** that a real external gNB/UE has
> been validated. Interop hardening (I1–I4) is now complete; the remaining real-RAN gate
> is **T10 (UERANSIM)**, which depends on C1 (5G telemetry).

---

## 3. Architecture

### 4G (today — shipped)

```
+-----------+         +----------+         +----------+
|  eNodeB   |---S1--->|   MME    |---S6a-->|   HSS    |
+-----------+         +----------+         +----------+
      |                    |
      |               S11 (HTTP/JSON)
      |                    |
      |               +----v-----+
      +---- S1-U ---->|  SPGW    |--SGi--> Internet
           (GTP-U)    +----------+

All NFs emit structured events to:

+------------+      SSE      +------------+
| Collector  |<----------+-->| Dashboard  |  http://localhost:3000
+------------+              +------------+
```

### 5G SA (target — in progress)

```
gNB ──N2 NGAP──▶ AMF ──SBI──▶ AUSF ──SBI──▶ UDM ──SBI──▶ UDR
                  │                            │
                  └──SBI──▶ SMF ──N4 PFCP──▶ UPF ──N3 GTP-U──▶ gNB
                              │
                         NRF (discovery)
```

> **D-2 — done.** NRF-based registration/discovery is now wired: every NF registers via
> `Nnrf_NFManagement` on boot and discovers peers via `Nnrf_NFDiscovery`, with static
> config/URLs retained as the deterministic zero-config fallback. See `docs/audit-v1.0.md`.

Both 4G and 5G share `pkg/subscriber` as a unified subscriber store — provisioning a subscriber once works for both protocols.

---

## 4. Package Map

### Core infrastructure

| Package | What it does |
|---------|-------------|
| `pkg/events` | Structured event schema, journey-ID correlation, HTTP emitter. The substrate Phase C AI reasons over. |
| `pkg/collector` | Collects events from all NFs; exposes SSE stream + journey store. Binary: `cmd/qcore-collector`. |
| `pkg/config` | Viper-based config loading with input-time validation (`config.Validate()`). |
| `pkg/logger` | Structured JSON logging with per-UE correlation IDs. |
| `pkg/metrics` | Prometheus metrics (one endpoint per NF). |
| `pkg/subscriber` | Unified subscriber store — Milenage, 5G-AKA, SUCI/SUPI, KASME/K_AUSF/K_SEAF, SQN management, PostgreSQL persistence. Shared by all NFs; no network exposure. |
| `pkg/database` | PostgreSQL connection + migrations. |

### 4G EPC (shipped)

| Package | What it does |
|---------|-------------|
| `pkg/hss` | Subscriber management + S6a facade over `pkg/subscriber`. Binary: `cmd/hss`. |
| `pkg/mme` | S1AP listener, NAS attach, authentication, security mode, bearer setup. Binary: `cmd/mme`. |
| `pkg/spgw` | GTP-U user plane, S11 (HTTP/JSON), IP pool, TUN egress (Linux). Binary: `cmd/spgw`. |
| `pkg/s1ap` | S1AP ASN.1 PER codec. |
| `pkg/nas` | 4G NAS message framing, security, Milenage key derivation. |
| `pkg/gtp` | GTP-U v1 codec. Shared by SPGW (4G) and future UPF (5G). |
| `pkg/simulator` | Built-in S1AP/NAS simulator with error-injection scenarios (wrong Ki, wrong PLMN, unprovisioned IMSI, wrong MME address). |

### 5G NFs (build + E2E green; 5G telemetry T7/C1 still pending)

| Package | State | What it does |
|---------|-------|-------------|
| `pkg/amf` | `cmd/amf` | NGAP listener, registration flow, key derivation, **N11 AMF→SMF (D-4)**. Tests green. |
| `pkg/ausf` | `cmd/ausf` | Full 5G-AKA create + confirm (TS 33.501). Tests green. |
| `pkg/udm` | `cmd/udm` | Nudm_SDM + Nudm_UEAU + UECM. Tests green. |
| `pkg/udr` | `cmd/udr` | AM-data + authentication-subscription GET. Tests green. |
| `pkg/nrf` | `cmd/nrf` | NF registration + discovery (Nnrf_NFManagement/NFDiscovery, D-2). Tests green. |
| `pkg/smf` | `cmd/smf` | Session management, IPAM, PFCP client to UPF, `Nsmf_PDUSession` SM-context endpoint, driven by the AMF over N11. |
| `pkg/upf` | `cmd/upf` | PFCP N4 server, GTP-U N3 (uplink/downlink), TUN egress N6 (Linux; dummy egress elsewhere). |
| `pkg/pfcp` | Builds, unit-tested | PFCP/N4 binary codec — header, IEs, Association + Session Establish/Modify/Delete. |

### 5G protocol codecs

| Package | State | What it does |
|---------|-------|-------------|
| `pkg/ngap` | ~2600 LOC, shipped | NGAP ASN.1 PER codec. Used by AMF. |
| `pkg/nas5g` | ~1200 LOC, shipped | 5G-NAS message codec. Used by AMF. |
| `pkg/sctp` | Native (Linux) + TCP fallback | Transport for NGAP. Native kernel SCTP on Linux (`sctp_linux.go`); TCP fallback on macOS/other with a dev-mode warning. |
| `pkg/sbi` | Shipped | HTTP/2 server + client, RFC 7807 ProblemDetails, NRF register/discover client (Nnrf_NFManagement/NFDiscovery, D-2). |

### Dashboard

| Package | What it does |
|---------|-------------|
| `pkg/dashboard` | Go backend: health aggregation, SSE proxy, RAN config snippets, simulator control, static asset serving. Binary: `cmd/dashboard`. |

---

## 5. 5G SA Track

Critical path **T1 → T7 is landed in code and green in the in-process E2E test, and the
Interop-Hardening track (I1–I4 / D-1…D-4) is complete.** What remains for a *credible,
real-RAN-ready* 5G core is **T8/T9 (C2/C3) → T10 (UERANSIM)**. Long-term decisions behind
I1–I4 are recorded in `docs/audit-v1.0.md` §4 (D-1…D-4).

| Step | What | State |
|------|------|-------|
| T1 | Binary entrypoints (`cmd/amf|ausf|udm|udr|nrf`), Dockerfiles, compose, config | ✅ Done |
| T2 | `pkg/pfcp` codec (header, IEs, Association + Session Establish/Modify/Delete) | ✅ Done |
| T3 | `pkg/smf` + `cmd/smf` (IPAM, PFCP client, `Nsmf_PDUSession` SM-context) | ✅ Done |
| T4 | `pkg/upf` + `cmd/upf` (PFCP N4, GTP-U N3, TUN egress N6) | ✅ Done |
| T5 | In-process E2E: Registration → PDU session → GTP-U tunnel | ✅ Passes |
| T6 | Native SCTP (Linux kernel; TCP fallback + warning elsewhere) | ✅ Done |
| T7 | Phase-A event instrumentation for 5G NFs (C1) — one correlated trace per registration | ✅ Done |

### Interop hardening — ✅ complete (was required before T10 / "real-RAN ready")

| Step | Decision (see audit §4) | State |
|------|-------------------------|-------|
| I1 | **D-1** Single standards-correct PLMN codec + golden vectors (`pkg/ident`) | ✅ Done |
| I2 | **D-3** Real SUCI (null-scheme first) + wire simulator IMSI; genuine unprovisioned-IMSI reject | ✅ Done |
| I3 | **D-2** NRF register/discover + Phase-A discovery events; static fallback retained | ✅ Done |
| I4 | **D-4** N11 AMF→SMF; dropped the test's direct-SMF shortcut | ✅ Done |

### Then: T8–T10 (T7 done)
T7: ✅ Phase A event instrumentation for 5G NFs (C1) — done; Phase C can now reason over 5G traces.
T8: 5G simulator UX (NGAP+NAS-5G, 5G-AKA, error injection — builds on I2's real SUCI).
T9: Dashboard 5G mode (protocol selector, 5G sim buttons, UDR subscriber view).
T10: UERANSIM compatibility verification (real gNB+UE in sidecar) — partially reproduced as of 2026-06-08; SMC integrity, InitialContextSetup APER, Registration Accept IE-length, protected UL NAS routing, UL NAS Transport IE-shape, compose SMF URL, and PDU Session Establishment Accept blockers are validated fixed. Current gap: prove UE→UPF→peer data-plane/ping.

---

## 6. 4G EPC — How It Works

### Attach flow (high level)

1. eNodeB sends S1 Setup Request over TCP (SCTP in production) → **MME**
2. UE sends Attach Request (in S1AP InitialUEMessage) → MME
3. MME sends Authentication-Information-Request to **HSS** (S6a / HTTP-JSON facade)
4. HSS computes Milenage authentication vector (RAND, AUTN, XRES, KASME), returns to MME
5. MME sends Authentication Request to UE; UE responds with RES
6. MME compares RES vs XRES — auth success
7. MME sends Security Mode Command (NAS security activated, KASME derived)
8. MME tells **SPGW** to create a bearer (S11/HTTP)
9. SPGW allocates IP from pool, creates GTP-U tunnel, returns S1-U TEID
10. MME sends Attach Accept + Activate Default Bearer
11. UE is attached — data flows UE ↔ eNodeB ↔ SPGW (GTP-U) ↔ TUN interface

All steps 1–11 emit structured events to the Collector. Journey ID threads through every event for a single UE's attach.

### Error-injection scenarios (built-in simulator)

| Scenario | What happens |
|----------|-------------|
| `wrong_ki` | Milenage auth vectors don't match → MME sends Authentication Reject |
| `wrong_plmn` | PLMN in Attach Request rejected by MME |
| `unprovisioned_imsi` | HSS returns Unknown Subscriber error |
| `wrong_mme_address` | Simulator cannot connect — connection error |

---

## 7. Event Model (Phase A)

Every signaling message, NF state transition, and config change is emitted as a structured Go event (`pkg/events`). Events are:

- **Protocol-agnostic** — same schema for 4G and 5G
- **Correlated** — each event carries a Journey ID that threads a single UE's session across all NFs
- **Streamable** — the Collector exposes an SSE endpoint; the dashboard subscribes live
- **Queryable** — the Collector stores events in an in-memory journey store

Event types (4G, shipped): `S1SetupPayload`, `AttachRequestPayload`, `AuthRequestPayload`, `AuthResponsePayload`, `SecurityModePayload`, `AttachAcceptPayload`, `BearerCreatePayload`.

5G event types (T7, pending): `NGSetupPayload`, `RegistrationRequestPayload`, `AuthRequest5GPayload`, `SecurityModeCommand5GPayload`, `RegistrationAcceptPayload`, `PDUSessionEstablishmentPayload`, `PFCPAssociationPayload`, `PFCPSessionEstablishmentPayload`.

---

## 8. Dashboard (Phase B)

URL: `http://localhost:3000`

| View | What it shows |
|------|---------------|
| Health | Aggregated NF status (green/red per NF) + system-wide health |
| Subscribers | CRUD table; add/edit/delete subscribers; CSV import/export |
| Live Trace | Real-time SSE event stream; per-journey trace timeline |
| RAN Connect | Config snippets for eNodeB (4G) / gNB (5G) pointing at QCore |
| Simulator | Start/stop; error-injection buttons |

Backend: `pkg/dashboard` (Go, port 3000). Proxies subscriber API from HSS (port 8080). Proxies event SSE from Collector.

---

## 9. Phase C — Diagnostic AI

**Shipped.**

Layers:
1. **Structured diagnostic knowledge layer** — curated symptom→cause catalog tied to event types
2. **AI Level 1 (Explain)** — cause-code decoding, signaling narration in plain English
3. **AI Level 2 (Diagnose)** — root-cause analysis + proposed fix, reasoning over the Phase A event trace. **This is the flagship.**

Model strategy: embedded SLM for bounded/common cases; optional bring-your-own-key frontier-model escalation for hard cases. AI reliability comes from the structured diagnostic layer + ground-truth telemetry — not parametric knowledge.

TTRC targets (from charter §4): < 2 min to root cause for common failures.

---

## 10. Running QCore Locally

```bash
# Prerequisites: Go 1.23+, Docker

# Start the full stack (builds images, starts all NFs + collector + dashboard)
make up

# Open the dashboard
open http://localhost:3000

# Run all tests (with race detector)
make test

# Build all binaries to bin/
make build

# Stop the stack
make down
```

### End-to-end tests

```bash
go test -v -run TestEndToEndAttachOverWire ./pkg/mme/
go test -v -run TestEndToEndUserPlane ./pkg/mme/
go test -v -run TestSimulatorHappyPath ./pkg/simulator/
```

### Simulator via curl

```bash
# Start a happy-path attach
curl -X POST http://localhost:3000/api/simulator/start

# Error injection
curl -X POST http://localhost:3000/api/simulator/inject/wrong_ki
curl -X POST http://localhost:3000/api/simulator/inject/wrong_plmn
curl -X POST http://localhost:3000/api/simulator/inject/unprovisioned_imsi
curl -X POST http://localhost:3000/api/simulator/inject/wrong_mme_address
```

### Add a subscriber manually

```bash
curl -X POST http://localhost:8080/api/v1/subscribers \
  -H "Content-Type: application/json" \
  -d '{
    "imsi": "001010000000001",
    "ki":   "465b5ce8b199b49faa5f0a2ee238a6bc",
    "opc":  "cd63cb71954a9f4e48a5994e37a02baf",
    "amf":  "8000"
  }'
```

The demo subscriber (3GPP TS 35.208 Test Set 1) is seeded automatically on first `make up`.

---

## 11. Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| Pure Go, no C dependencies | Static binary, no kernel modules, cross-platform builds, simple CI |
| Single subscriber store (`pkg/subscriber`) shared by 4G and 5G | Provisioning a subscriber once works for both protocols; no dual-write bugs |
| `make up` as the primary dev entry point | TTFC (time to first attach) < 5 min is a charter requirement |
| Dashboard is source of truth; YAML is an export path | Users should not need to hand-edit config files |
| Validate configuration at input time, not at runtime | Every config error names its cause and its fix before anything starts |
| Event model before AI | The AI is only as good as the telemetry it reasons over — substrate first |
| SMF + UPF as separate packages from SPGW | 5G session management (PFCP) is different enough from 4G (HTTP/S11) to warrant separate code; GTP-U is shared via `pkg/gtp` |
| TCP fallback for SCTP in dev | Real gNBs speak NGAP over SCTP; macOS has no native SCTP. Linux uses native kernel SCTP; macOS/other keep TCP fallback with a dev-mode warning |
| **D-1: one standards-correct identifier codec** | The wedge is real-RAN interop. Two private PLMN encoders that agree only with each other pass CI and fail the first real device. Consolidate PLMN/TAC/S-NSSAI/GUAMI on a TS-correct codec validated by golden vectors from real stacks |
| **D-2: NRF is the discovery backbone, static config is the fallback** | SBA discovery is the standards-correct design *and* a diagnostic-AI observability surface ("SMF never registered"). But zero-config fast-start must never be gated on a discovery race — so discovery is layered, not mandatory |
| **D-3: real SUCI, null-scheme first** | A simulator scenario that silently doesn't test what it claims erodes trust in the simulator — and the simulator's credibility is the product. Null-scheme covers UERANSIM test defaults; ECIES A/B layer in behind the same interface later |
| **D-4: implement N11 (AMF→SMF)** | The data plane is half of "test against a core." A 5G core that can't carry a PDU session through the real control flow isn't credible by 2030 |

---

## 12. Glossary

| Term | Meaning |
|------|---------|
| AMF | Access and Mobility Management Function (5G) |
| AUSF | Authentication Server Function (5G) |
| EPC | Evolved Packet Core (4G) |
| GTP-U | GPRS Tunneling Protocol — User plane |
| HSS | Home Subscriber Server (4G) |
| MME | Mobility Management Entity (4G) |
| NAS | Non-Access Stratum (UE ↔ MME/AMF signaling) |
| NGAP | Next Generation Application Protocol (gNB ↔ AMF, 5G) |
| NRF | Network Repository Function (5G service discovery) |
| PFCP | Packet Forwarding Control Protocol (SMF ↔ UPF, 5G) |
| S1AP | S1 Application Protocol (eNodeB ↔ MME, 4G) |
| SA | Standalone (5G mode without LTE anchor) |
| SBI | Service-Based Interface (HTTP/2+JSON between 5G NFs) |
| SCTP | Stream Control Transmission Protocol (transport for S1AP/NGAP) |
| SMF | Session Management Function (5G) |
| SPGW | Serving/PDN Gateway (4G combined SGW+PGW) |
| SUCI | Subscription Concealed Identifier (5G) |
| SUPI | Subscription Permanent Identifier (5G equivalent of IMSI) |
| TTFC | Time To First Call/Connect — charter performance target |
| TTRC | Time To Root Cause — Phase C AI performance target |
| UDM | Unified Data Management (5G) |
| UDR | Unified Data Repository (5G) |
| UPF | User Plane Function (5G, evolves from SPGW) |
| UERANSIM | Open-source gNB+UE simulator used for 5G testing |
