# QCore Architecture

This is the **target** architecture QCore is migrating toward, per
[RFC 0001](rfc/0001-5g-sba-pivot.md). Today's `master` is partway along
this path — see the "Current state" section at the end for what is
actually shipped.

---

## 1. Top-level picture

QCore is two product tracks sharing one foundation:

```
                                   ┌──────────────────────────────┐
                                   │       Web Dashboard          │
                                   │    (Next.js + TypeScript)    │
                                   └──────────────┬───────────────┘
                                                  │
                                                  │ HTTP (from SBI
                                                  │  + event SSE)
                                                  ▼
                          ┌─────────────────────────────────────────────┐
                          │          Service-Based Plane (5G)           │
                          │   AMF · SMF · AUSF · UDM · UDR · NRF · PCF  │
                          │           all HTTP/2 + OpenAPI              │
                          └───┬──────────────────────────┬──────────────┘
                              │                          │
                         N2 (NGAP)                  N4 (PFCP)
                              │                          │
                              ▼                          ▼
                       ┌────────────┐              ┌─────────┐
                       │    gNB     │──── N3 ─────▶│   UPF   │──── N6 ───▶ Internet
                       │   (5G RAN) │    (GTP-U)   └─────────┘
                       └────────────┘                   ▲
                                                        │
                            ┌──────── UPF also serves ──┘
                            │         as 4G SPGW via
                            │         "legacy mode"
                            │
                  ┌─────────┴─────────────┐
                  │    EPC Plane (4G)     │
                  │   HSS · MME · SGW/PGW │
                  │  (kept as legacy)     │
                  └───────────────────────┘
                              │
                              │ S6a (Diameter facade)
                              ▼
                      ┌──────────────────┐
                      │ pkg/subscriber   │◀── UDM/UDR (5G) also talk here
                      │ (unified store)  │
                      └──────────────────┘
```

Both tracks draw from a **unified subscriber plane** (one IMSI-keyed
source of truth for Milenage keys, profile, and session state). Both
tracks share the **UPF** (the 4G legacy mode is a configuration flag on
the same user-plane binary, not a separate product).

---

## 2. 5G Service-Based Plane (primary track)

Each network function (NF) is its own Go binary, its own HTTP/2 service,
and self-registers with the NRF on startup.

### 2.1 NF catalogue

| NF | Responsibility | Spec surface | Talks to |
|----|----------------|--------------|----------|
| **NRF** | Service discovery | Nnrf_NFManagement, Nnrf_NFDiscovery | All NFs |
| **AMF** | Registration, connection management, mobility | NGAP (N2), 5G-NAS (N1), Namf | gNB, AUSF, SMF, UDM |
| **AUSF** | Authentication | Nausf_UEAuthentication | UDM |
| **UDM** | Subscription data (read-mostly) | Nudm_UEAuthentication, Nudm_SubscriberDataMgmt | UDR, AUSF, AMF, SMF |
| **UDR** | Subscription data (storage) | Nudr_DataRepository | UDM (only) |
| **SMF** | Session management | PFCP (N4), Nsmf | UPF, UDM, PCF |
| **UPF** | User plane — GTP-U, IP pool, egress | N3 GTP-U, N4 PFCP, N6 IP | SMF, gNB |
| **PCF** *(post-v1.0)* | Policy | Npcf | SMF, AMF |

### 2.2 Inter-NF wire protocols

- **SBI (everywhere except N2/N3/N4/N6):** HTTP/2 + JSON, OpenAPI-described.
- **N2 (AMF ↔ gNB):** NGAP over SCTP. ASN.1 PER encoding (shared with
  legacy S1AP via `pkg/asn1per`).
- **N1 (UE ↔ AMF, carried in NGAP):** 5G-NAS. Shares framing + security
  primitives with 4G-NAS via `pkg/nas/core`.
- **N3 (gNB ↔ UPF):** GTP-U v1. Unchanged from 4G. `pkg/gtp` reused.
- **N4 (SMF ↔ UPF):** PFCP. Binary. New codec (`pkg/pfcp`).
- **N6 (UPF ↔ internet):** Plain IP. `pkg/spgw` TUN egress reused as-is.

### 2.3 Where each NF lives in the repo

```
pkg/
├── sbi/           # HTTP/2 server, OpenAPI validation, NRF client (shared)
├── asn1per/       # ALIGNED PER codec (shared: S1AP, NGAP)
├── nas/
│   ├── core/      # Framing, security primitives (shared: 4G-NAS, 5G-NAS)
│   ├── nas4g/     # 4G procedures (was pkg/nas)
│   └── nas5g/     # 5G-NAS procedures
├── subscriber/    # Unified subscriber store + Milenage + SUCI (was pkg/hss)
├── nrf/           # Service discovery
├── amf/           # 5G AMF
├── ausf/          # 5G Authentication Server Function
├── udm/           # 5G Unified Data Management
├── udr/           # 5G Unified Data Repository
├── smf/           # 5G Session Management Function
├── upf/           # User Plane Function (was pkg/spgw, now dual-mode 4G+5G)
├── pfcp/          # PFCP (N4) codec + client + server framework
├── ngap/          # NGAP (N2) codec (uses asn1per)
├── s1ap/          # S1AP (legacy 4G) (uses asn1per)
├── hss/           # S6a facade over pkg/subscriber (legacy 4G)
├── mme/           # Legacy 4G MME
└── gtp/           # GTP-U v1 (shared: SGW legacy, UPF 5G)

cmd/
├── qcore/         # The single-binary experience: "qcore up" starts it all
├── qcore-amf/     # per-NF binaries for production deployments
├── qcore-smf/
├── qcore-upf/
├── ... (etc per NF)
└── qcore-hss/     # Legacy per-function binaries stay where they are

web/               # Next.js dashboard (new)
```

`cmd/qcore/` is the single "batteries-included" binary for development
(`qcore up` starts all NFs in a supervised process tree). Production
deployments use the per-NF binaries behind Helm.

---

## 3. Unified subscriber plane

The single piece of code that both 4G and 5G read subscriber data from.

```
                   ┌───────────────────────────┐
                   │     pkg/subscriber        │
                   │ ─ Milenage (TS 35.206)    │
                   │ ─ 5G-AKA auth vectors     │
                   │ ─ SUCI/SUPI crypto        │
                   │ ─ KASME + K_AUSF + K_SEAF │
                   │ ─ Subscriber CRUD         │
                   │ ─ SQ N management         │
                   │ ─ PostgreSQL persistence  │
                   └─────┬───────┬───────┬─────┘
                         │       │       │
             Go import   │       │       │ Go import
             (no wire)   │       │       │ (no wire)
                         ▼       ▼       ▼
                  ┌──────────┐ ┌────┐ ┌────┐
                  │   hss    │ │udm │ │udr │
                  │  (S6a    │ │(5G │ │(5G │
                  │ facade)  │ │SBI)│ │SBI)│
                  └────┬─────┘ └─┬──┘ └─┬──┘
                       │         │      │
                  Diameter     HTTP/2  HTTP/2
                       │         │      │
                       ▼         ▼      ▼
                     4G MME   5G NFs  5G UDM
```

`pkg/subscriber` has **no network exposure** — it's a Go library. The
`hss`, `udm`, and `udr` services each compose it and expose a different
protocol face. All mutations still flow through one code path.

Why this matters: provisioning a subscriber via CLI, API, or dashboard
is a single operation. 4G sessions see it. 5G sessions see it. No
dual-write bugs.

---

## 4. User plane (UPF)

The UPF is the evolution of the current `pkg/spgw`. It is dual-mode:

- **5G mode (default):** PFCP N4 control, GTP-U N3, TUN egress N6.
- **Legacy 4G mode:** HTTP-S11 control (our existing wire format), GTP-U
  S1-U, same TUN egress.

This lets 4G and 5G share **one** user-plane binary in a deployment —
important because the datapath is the expensive-to-optimise piece and we
don't want to maintain two.

```
            ┌───────────── UPF ─────────────┐
            │                                │
N4 PFCP ───▶│  control-plane                │
            │     - session create/modify    │
            │     - URR, QER, FAR, PDR       │
 HTTP ─────▶│     (legacy S11 dual-wired)    │
            │                                │
            │  user-plane (shared)           │
            │     - IP pool                  │
            │     - TEID pool                │
            │     - session store (N-way)    │
N3 GTP-U ◀─▶│     - GTP-U codec              │
            │     - Egress adapter           │
N6 IP   ◀─▶│        ├── LogEgress           │
            │        └── TUNEgress (Linux)   │
            └────────────────────────────────┘
```

PDRs/FARs/QERs/URRs are PFCP concepts but map cleanly onto our existing
`Bearer` struct with a few more fields. Legacy 4G bearers become a
degenerate case of 5G PDU session rules.

---

## 5. Cross-cutting concerns

### 5.1 Configuration

- One YAML file per binary, same Viper-based loading as today.
- `config.Validate()` method on every config struct with friendly error
  messages (missing field `X`; `ue_pool` is not a valid CIDR; `nrf_url`
  unreachable).
- Environment variables with `QCORE_` prefix override every value.
- `qcore up` (the dev binary) reads one top-level config and plumbs
  per-NF subtrees.

### 5.2 Observability

- **Logs:** structured JSON, one correlation ID per UE, via `pkg/logger`.
- **Metrics:** Prometheus, one `/metrics` endpoint per NF, standard RED
  metrics + 3GPP KPIs (attach/registration success rate, auth failure
  rate, PFCP latency, UPF throughput).
- **Traces:** OpenTelemetry, one span per SBI call, one span per NGAP
  procedure, one span per PFCP request. Exemplars tie logs → metrics →
  traces.
- **Events:** AMF publishes an SSE stream of NAS procedures (started,
  auth-ok, security-ok, registered). Dashboard subscribes directly.

### 5.3 Inter-service auth

- **Dev mode (default):** plaintext HTTP/2 with a big console warning.
- **Production (Helm):** mTLS with an internal CA. Auto-rotation. Every
  SBI call is authenticated.
- **External APIs (dashboard, SDK, CLI → NFs):** TLS + token auth. RBAC
  at the NF boundary.

### 5.4 Persistence

- **Subscriber data:** PostgreSQL (unchanged from today). Schema lives in
  `pkg/subscriber/migrations/`.
- **Session state (5G PDU, 4G bearers):** in-memory with periodic
  snapshots to disk for crash recovery. No external dependency for dev.
  Production can mount persistent volume for the snapshot directory.
- **NRF state:** in-memory + PostgreSQL mirror for restart recovery.

### 5.5 Packaging

- **Dev:** `docker compose up` gives the full stack.
- **Single-binary dev:** `qcore up` (new) runs every NF as a goroutine
  in one process — fastest local startup, one log stream.
- **Production:** one container per NF. Helm chart with topology
  presets (dev, ha, edge).

---

## 6. Dashboard

Next.js 14 + TypeScript + Tailwind + shadcn/ui.

### 6.1 Pages (priority order)

1. **Subscribers** — searchable table, CRUD, CSV import. Points at the
   subscriber plane via REST.
2. **Live attach flow** — timeline of NAS/NGAP messages per UE. Subscribes
   to AMF's SSE stream. This is the "wow" demo.
3. **Topology** — live graph of all NFs, health, throughput. Reads from
   NRF + Prometheus.
4. **Sessions** — active PDU sessions / bearers. Reads from SMF + UPF.
5. **Observability** — embedded Grafana dashboards.
6. **Doctor / Diagnostics** — live output of `qcore doctor` with one-click
   fixes.
7. **Settings** — PLMN, certs, backup/restore, audit log.

### 6.2 Command palette (⌘K)

Jump to any subscriber, NF, session, or metric. Every UI action prints
its CLI equivalent so the user learns `qcore ...` organically.

### 6.3 Monetisation seam

Open-source dashboard covers everything above. Paid enterprise
dashboard adds SSO, advanced RBAC, audit trails, multi-tenant org
management, SLA reports. **The OSS/paid boundary lives at the dashboard
repo level**, not scattered through protocol code. This keeps QCore
core fully open-source.

---

## 7. Current state vs target

Today's `master` has:

| Target component | What exists today | Distance to target |
|---|---|---|
| `pkg/subscriber` | **shipped** — extracted from old `pkg/hss` in v0.5 (model, Milenage, SQN, service). 5G-AKA derivations (TS 33.501 Annex A.2 KAUSF / A.4 RES*) landed on top of the same Milenage core; `Generate5GAuthVector` shares the per-subscriber SQN counter with the 4G path. | Add AUTS reverse-Milenage for SQN resync; cross-validate 5G-AKA against external reference |
| `pkg/subscriber/admin` | **shipped** — REST CRUD + CSV + auth-vectors, consumed by `cmd/hss`; now behind a `Store` interface + `HealthCheckFunc` seam with `api_test.go` exercising every handler via `httptest` + fake store | Reused as-is |
| `pkg/asn1per` | embedded in `pkg/s1ap` | Extract |
| `pkg/nas/core` | embedded in `pkg/nas` | Extract |
| `pkg/hss` (S6a facade) | **retired** in v0.5 — no Diameter code exists yet; `cmd/hss` now talks REST via `pkg/subscriber/admin` | Reintroduce as real Diameter facade in Phase 5 (post-v1.0) |
| `pkg/mme` | shipped | Reused as-is for legacy track |
| `pkg/s1ap` | shipped | Refactor to use `pkg/asn1per` |
| `pkg/gtp` | shipped | Reused as-is in UPF |
| `pkg/spgw` | shipped | Evolve into `pkg/upf` + PFCP front |
| `pkg/sbi` | **Phase 0 sketch shipped** (v0.5) — h2/h2c server + client, RFC 7807 ProblemDetails, middleware (RequestID/AccessLog/Recover) | Harden + wire OpenAPI validation as real NFs mount |
| `pkg/sbi/nrf` | **Phase 0 sketch shipped** (v0.5) — NFProfile types + in-memory Client for dev/tests; HTTP Client stub for real NRF | Expand when real NRF server lands |
| `pkg/nrf` (server) | does not exist | Build (Nnrf over SBI) |
| `pkg/amf` | does not exist | Build |
| `pkg/ngap` | does not exist | Build (on `pkg/asn1per`) |
| `pkg/nas5g` | does not exist | Build (on `pkg/nas/core`) |
| `pkg/ausf` | **shipped** (v0.5) — `Nausf_UEAuthentication` full 5G-AKA flow: `POST /nausf-auth/v1/ue-authentications` calls UDM UEAU, compresses XRES* → HXRES* per TS 33.501 Annex A.5, returns Av5gAka to AMF; `PUT .../{ctx}/5g-aka-confirmation` compares RES* constant-time and derives KSEAF per Annex A.6. In-memory `authCtx` store keyed by ctx-id. `TestAUSF_EndToEnd` / `TestAUSF_ConfirmationFailure` / `TestAUSF_UnknownSUPI` exercise the full create + confirm loop over h2c loopback. | Shared ctx store (Redis/etcd) for multi-AUSF HA; EAP-AKA' for non-3GPP access; SUCI deconcealment |
| `pkg/udm` | **shipped** (v0.5) — `Nudm_SDM` am-data over `pkg/sbi`, plus `Nudm_UEAU` generate-auth-data. `AmDataSource` and `AuthSource` seams separate SDM and UEAU backends. `NewStoreSource`/`NewStoreAuthSource` for direct mode, `NewUDRSource` for network mode. UDM→UDR SDM chain covered by `TestUDM_over_UDR_chain`; UEAU covered by `TestUDM_UEAU_GenerateAuthData` on h2c loopback. | Add `Nudm_UECM` (needs AMF) and AUTS-based SQN resync |
| `pkg/udr` | **shipped** (v0.5) — `Nudr_DataRepository` am-data over `pkg/sbi`, plus `pkg/udr.Client` with typed errors (`ErrNotFound`, `ErrBadUeID`) | Add authentication-subscription endpoint (for AUSF); give UDR its own storage schema so direct-mode can retire |
| `pkg/sbi/common` | **shipped** (v0.5) — shared TS 29.571 types (AccessAndMobilitySubscriptionData, AmbrRm, Nssai, Snssai) consumed by both `pkg/udm` and `pkg/udr` | Add types as downstream NFs start consuming them |
| `pkg/smf` | does not exist | Build |
| `pkg/pfcp` | does not exist | Build |
| Dashboard | does not exist | Build (Next.js) |
| Linux CI with NET_ADMIN | exists (ci.yml Linux integration job) | Extend as more NFs land |

Refactor work ≈ 1 Phase of effort.
Net new 5G protocol code ≈ 3 Phases.
Dashboard in parallel ≈ 2 Phases, overlapping with net new work.

No timelines promised. See the RFC for the milestone-driven sequencing.

---

## 8. What this architecture is not

- **Not a microservices framework.** NFs are services because the spec
  says so, not because we love microservices. Deploy-as-monolith is a
  first-class mode via `qcore up`.
- **Not a Kubernetes-native rewrite.** K8s is a deployment target, not
  a prerequisite. Binaries run standalone, in containers, and in K8s.
- **Not trying to be open5GS or free5GC.** Those projects optimise for
  spec coverage. We optimise for developer joy inside a well-defined
  subset of the spec.
- **Not a carrier-grade replacement for Ericsson.** Enterprise and
  private 5G are the v1.0 market. Carrier scale is v2.

---

## 9. Changelog

- **2026-04-16** — Initial draft alongside RFC 0001.
- **2026-04-19** — v0.5 progress: `pkg/hss` retired; `pkg/subscriber` + `pkg/subscriber/admin` shipped; `pkg/sbi` + `pkg/sbi/nrf` Phase 0 sketches shipped; first two 5G NF cuts shipped and round-trip tested — `pkg/udm` (Nudm_SDM am-data) and `pkg/udr` (Nudr_DataRepository am-data). §7 state-vs-target table updated. Target architecture (§§1-6) unchanged — still the same destination.
- **2026-04-20** — UDM→UDR layering seam landed: `pkg/sbi/common` extracted for shared TS 29.571 types; `pkg/udm.AmDataSource` interface splits direct-mode (`NewStoreSource`) from network-mode (`NewUDRSource` over `pkg/udr.Client`); `TestUDM_over_UDR_chain` exercises the full chain over h2c loopback. Mode is a constructor-arg change — no refactor needed to flip UDM from reading `pkg/subscriber` directly to reading through UDR.
- **2026-04-20** — 5G-AKA and Nudm_UEAU shipped. `pkg/subscriber` gained `DeriveKAUSF` / `DeriveRESStar` / `Generate5GAuthVector` per TS 33.501 Annex A, sharing the Milenage core and SQN state with the 4G EPS-AKA path. `pkg/udm` gained `POST /nudm-ueau/v1/{supi}/security-information/generate-auth-data` behind a parallel `AuthSource` seam (`WithAuthSource`). AUSF can now get a 5G-AKA vector from UDM — unblocks the upcoming `pkg/ausf`.
- **2026-04-21** — `pkg/subscriber/admin` taken off the untested list. Refactored behind a narrow `Store` interface (covering the eight `*subscriber.Service` methods the REST handlers actually call) plus a `HealthCheckFunc` seam so `cmd/hss` passes a `db.PingContext` closure. `api_test.go` exercises every handler via `httptest` + fake store, including CSV round-trip, the error-string → HTTP-status mapping, and the recovery middleware catching a handler panic. No new DB dependency for tests.
- **2026-04-20** — `pkg/ausf` landed — the full 5G-AKA create + confirm flow. `pkg/subscriber` gained `DeriveHXRESStar` (Annex A.5) and `DeriveKSEAF` (Annex A.6). `pkg/udm` gained a consumer `Client` (used by AUSF). `pkg/ausf` exposes `POST /nausf-auth/v1/ue-authentications` (fetch Av5gHeAka from UDM, compress XRES* → HXRES*, stash auth-ctx, return Av5gAka + Location to AMF) and `PUT .../{ctx}/5g-aka-confirmation` (constant-time RES* ≟ XRES*, derive and return KSEAF on match). Auth-ctx store is in-memory and mutex-protected — fine for single-instance dev/CI; v1.0 wants a shared store (Redis/etcd) so AMF can hit any replica on the confirmation leg. AUSF tests simulate the UE by re-running Milenage + `DeriveRESStar` against the same Ki/OPc the fake UDM used — no mocked crypto in the loop.
