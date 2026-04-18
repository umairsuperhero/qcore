# RFC 0001 — 5G SBA-First Pivot with Refactor-Not-Rebuild

**Status:** Proposed
**Author:** QCore core team
**Created:** 2026-04-16
**Last updated:** 2026-04-16

## Summary

Pivot QCore's primary development track from 4G EPC to 5G Standalone (SA).
Refactor existing HSS / S1AP / NAS / GTP-U / SPGW code along clean service
boundaries so it can be reused as UDM/UDR/AUSF, ASN.1 PER codec, 5G-NAS core,
N3 U-plane, and UPF respectively. Keep the working 4G stack as a supported
legacy track, not the lead product. Build a Next.js dashboard in parallel
from this point forward, not after.

## Context

Phases 1–3 shipped a working 4G LTE core: HSS with test-vector-verified
Milenage (Phase 1), MME with full S1AP/NAS attach (Phase 2), and SPGW with
GTP-U uplink + Linux TUN egress + SPGW Prometheus metrics (Phase 3 Sessions
1–2). All on `master`, all tested, all functional.

Two design calls proved instinctively right but standards-incorrect for 4G:

1. **S6a over HTTP/JSON** instead of Diameter.
2. **S11 over HTTP/JSON** instead of GTPv2-C.

Both were chosen for developer ergonomics. Both wall QCore off from
interop with commercial 4G elements (any deployment must use QCore on both
sides of the interface).

**Critical realisation:** 3GPP 5G SBA (Service-Based Architecture) is
literally HTTP/2 + OpenAPI. Every service-based interface in the 5G core
(`Namf`, `Nsmf`, `Nudm`, `Nausf`, `Npcf`, `Nnrf`) is REST. The ergonomic
deviations we made in 4G are the actual standards in 5G. QCore's DNA is
natively 5G. Fighting the 4G standards cost us interop; flowing with the 5G
standards gains it.

Additionally:
- 4G is in deployment-decline; 5G is in growth. A 2026 investment in a
  great 4G core serves a shrinking market.
- Private 5G (enterprise, campus, industrial) is where monetisation lives,
  and the open-source gap there is real (open5GS has weak DX; Magma is
  dormant; free5GC is academic-grade; commercial options are expensive and
  opaque).
- Our existing codebase is ~50% reusable as the 5G base once refactored
  along SBA lines. Rewriting from scratch would be wasteful.

The user has explicitly stated a preference for "slow but great" over
"fast but incremental," which favours pausing to reset the architecture
rather than continuing to bolt features onto the current monolithic
`pkg/mme` / `pkg/spgw` structure.

## Decision

**Pivot to 5G SA as the primary product track, effective next commit.**

Specifically:

1. **5G SA becomes v1.0.** Target: a UE registers, authenticates, and
   establishes a PDU session through QCore-native AMF/SMF/UPF/AUSF/UDM with
   a supporting dashboard.

2. **4G EPC becomes a supported legacy track.** MME/SGW/PGW stay working,
   but new features and ergonomic investments go into the 5G path first.
   The legacy track consumes the shared subscriber plane via an S6a facade.

3. **Adopt Service-Based Architecture strictly.** Each 5G network function
   is its own binary, its own HTTP/2 service, registered with an internal
   NRF for discovery. Inter-function contracts are OpenAPI-described.

4. **Refactor existing code into shared foundations before net-new work.**
   - Extract `pkg/asn1per` from `pkg/s1ap` so NGAP can reuse it.
   - Extract `pkg/nas/core` from `pkg/nas` so 5G-NAS can reuse framing +
     security primitives.
   - Refactor `pkg/hss` into `pkg/subscriber` (storage + crypto) with thin
     face packages for `hss` (S6a), `udm`, `udr`, `ausf`.
   - Evolve `pkg/spgw` into `pkg/upf` with PFCP control-plane in front.

5. **Dashboard from day one.** A Next.js admin app starts this phase,
   pointed at the existing subscriber service, and grows alongside every
   new 5G service. No more "dashboard at the end."

6. **Linux CI with `NET_ADMIN` + UERANSIM** as a precondition. We have
   untested TUN code today and no real radio-side integration check. Fix
   this before adding more protocol surface.

**Out of scope for v1.0 (explicit non-goals):**
- 5G NSA (non-standalone, dual-connectivity). SA is cleaner; NSA is not a
  v1.0 goal and may never be one.
- Network slicing beyond single default S-NSSAI.
- Roaming (N32 / SEPP). Private-5G is the target market; roaming is not.
- CHF / charging.
- N3IWF / non-3GPP access.
- Carrier-grade scale targets (>100k UEs/instance). Enterprise-scale is
  enough for v1.0.

## Rationale

- **Standards alignment.** REST-first was a deviation in 4G and is the
  standard in 5G. The same code philosophy becomes interop-compliant by
  switching which generation we target first.
- **Market alignment.** 5G growth + private-5G open-source gap = product
  opportunity. 4G alone is not.
- **Reuse.** ~50% of existing code maps onto 5G components with refactor.
  Starting over would be wasteful; continuing to stack 4G features would
  entrench the monolithic service shape that will hurt us in 5G.
- **Ergonomics compound.** If every 5G service is a small, typed,
  OpenAPI-described HTTP service from the start, the dashboard UX story
  becomes trivial (generated clients, live topology, tail events).
- **Risk reduction.** Writing the dashboard alongside every service forces
  UX feedback into API design instead of freezing bad APIs first.

## Alternatives considered

### A. Continue on 4G as planned (Phase 4 = dashboard, Phase 5 = 5G)
- **Why rejected:** locks in another 6+ months of investment in a
  declining-market product with interop debt; 5G gets done late, by which
  point private-5G competitors have more runway.

### B. Scrap everything and start clean with 5G
- **Why rejected:** wasteful. Our HSS, GTP-U, NAS codec, S1AP codec, TUN
  egress, pools, and core infrastructure are all valuable and tested.
  "Rebuild" as a synonym for "throw away" is an expensive mistake.

### C. Build 4G and 5G in parallel as peer products
- **Why rejected:** dilutes focus. With finite effort, two half-built
  products ship slower than one well-built one. Keep 4G alive but
  secondary; lead with 5G.

### D. Only do the refactor (no 5G yet), treat this as a "Phase 3.5"
- **Why rejected:** refactor without a target is yak-shaving. Tying the
  refactor to concrete 5G service boundaries keeps the work grounded.

## Consequences

### Good
- Philosophical alignment between QCore's REST-native instincts and the 5G
  SBA spec.
- Clean service boundaries become enforced rather than aspirational.
- Dashboard-driven UX becomes possible because every service has a
  machine-readable API.
- Subscriber data model unifies across 4G + 5G (one source of truth).
- Rich observability story: every inter-service call is an HTTP/2 span.

### Bad
- Net surface area grows (NGAP + 5G-NAS + PFCP + SBI framework).
- We lose the "docker compose up and UE attaches" demo temporarily —
  until the 5G equivalent is working, the pitch narrative weakens.
- Microservices bring microservices problems (service discovery, auth
  between services, cert rotation, distributed tracing).
- The existing `pkg/mme` service will be refactored substantially; any
  external consumers of its internals (there shouldn't be any this early)
  break.
- Docs and examples across the repo need sweeping updates.

### Work created
- `pkg/sbi` — shared HTTP/2 server + OpenAPI tooling + NRF client.
- `pkg/asn1per` — extracted PER codec.
- `pkg/subscriber` — unified subscriber store.
- New services: `pkg/amf`, `pkg/smf`, `pkg/upf`, `pkg/ausf`, `pkg/udm`,
  `pkg/udr`, `pkg/nrf`.
- Dashboard app under `web/` (Next.js + TypeScript).
- Linux CI workflow with `NET_ADMIN` + UERANSIM.
- Update PRD + README + roadmap.

## Migration plan

### Phase 0 — Foundations (this RFC + immediate follow-ups)
Goal: set the stage without breaking 4G.

- Land this RFC, `ARCHITECTURE.md`, updated PRD milestones.
- Add Linux CI with `NET_ADMIN` + UERANSIM smoke test against the
  existing 4G stack (proves the harness works before relying on it for
  5G).
- Sketch `pkg/sbi` shape: HTTP/2 server + OpenAPI validation middleware
  + client stubs.
- Add `config.Validate()` method with friendly errors.

### Phase 1 — Subscriber plane refactor
Goal: unify 4G + 5G subscriber data under one service.

- Extract `pkg/subscriber` (storage + Milenage + KASME + SUCI crypto) from
  `pkg/hss`.
- Retire the current `pkg/hss` package. It contains no Diameter today — the
  name was aspirational. `pkg/hss` will be reintroduced in Phase 5 as a
  real S6a facade alongside the 4G legacy track work.
- Move the admin REST + CSV surface to `pkg/subscriber/admin` (the
  dashboard's customer — separate from any 3GPP SBI face).
- Add `pkg/udr` + `pkg/udm` SBI faces over `pkg/subscriber`. UDM consumes
  UDR via an interface — a Local client for combo deployments, an HTTP
  client for disaggregated ones.
- Add `pkg/ausf` — thin authentication service that calls `pkg/udm`.
- **Milestone:** a subscriber added via the admin API is queryable via
  N8/Nudr (SBI), and `pkg/ausf` produces auth vectors via Nausf. The data
  tier is ready for S6a once Diameter lands in Phase 5.

### Phase 2 — NGAP + 5G-NAS + AMF
Goal: control-plane registration.

- Extract `pkg/asn1per` from `pkg/s1ap`.
- Build `pkg/ngap` on top of `pkg/asn1per`.
- Extract NAS core from `pkg/nas`; build `pkg/nas5g` for 5G-NAS.
- Build `pkg/amf` using `pkg/sbi` + `pkg/ngap` + `pkg/nas5g`. AMF uses
  `pkg/ausf` for auth, `pkg/udm` for subscription.
- Build `pkg/nrf` (service discovery).
- **Milestone:** UERANSIM 5G UE completes REGISTRATION procedure through
  QCore AMF. Dashboard shows it live.

### Phase 3 — PFCP + SMF + UPF
Goal: user-plane session.

- Build `pkg/pfcp` codec (the one binary protocol left in 5G core).
- Build `pkg/smf` using `pkg/sbi` + `pkg/pfcp`.
- Evolve `pkg/spgw` into `pkg/upf`: adds PFCP server, keeps GTP-U + TUN
  egress + pools.
- **Milestone:** UERANSIM 5G UE establishes a PDU session and pings
  through QCore UPF.

### Phase 4 — Dashboard parity
Goal: the "60-seconds-to-wow" experience from the PRD.

- Subscriber CRUD UI (against `pkg/subscriber`).
- Service topology view (against NRF).
- Live attach flow visualiser (SSE stream from AMF of NAS events).
- `qcore doctor` command that validates the whole stack.

### Phase 5 — 4G legacy track refactor
Goal: keep 4G working without being the lead.

- `pkg/mme` keeps running but consumes the unified `pkg/subscriber` via
  S6a facade.
- SPGW is re-expressed as a UPF with legacy-mode configuration.

### Phase 6 — Production polish
- Distributed tracing (OTel) across all SBI calls.
- mTLS between services with rotating internal CA.
- Helm chart for the SBA deployment.
- Conformance test suite (UERANSIM + open5GS-RAN + commercial when
  possible).
- v1.0 release.

## Open questions

1. **Service discovery transport.** NRF uses SBI (HTTP REST). Do we also
   need mTLS between services from day one, or can dev mode be plaintext
   with a warning? Leaning toward plaintext-with-big-warning for v1.0 dev
   ergonomics, mTLS mandatory for production Helm.

2. **OpenAPI source.** Hand-written or generated from 3GPP YAML? 3GPP
   publishes OpenAPI for the 5G core — we should use them where they
   exist, but they're verbose and sometimes buggy. Likely answer: import
   theirs, patch where needed, regenerate.

3. **Persistence model.** Stick with PostgreSQL for subscriber data;
   unresolved for session state (in-memory? etcd? SQLite embedded?).
   Bias toward in-memory with periodic persistence snapshots.

4. **Monetisation seam.** Open-core with paid dashboard tier? Paid
   enterprise features (SSO, RBAC, audit, HA)? Hosted cloud? This affects
   where the commercial/OSS boundary lives in the code. Defer decision,
   but note that a dashboard-as-paid-tier implies the dashboard code lives
   in a separate repo behind a license boundary.

5. **UE simulator.** Do we ship one? PRD hints yes (v0.7). Earlier would
   help demos and CI. Likely answer: minimal built-in 5G UE for CI by
   Phase 2; full external UERANSIM integration for acceptance tests.

6. **Language for the dashboard.** TypeScript + Next.js is the plan
   (PRD section 5.2). Confirmed.

## Amendment history

None yet.
