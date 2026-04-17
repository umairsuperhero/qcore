# QCore Product Requirements Document
### The World's Best 4G + 5G Core Deployment & Operations Experience

**Status:** Draft v0.2
**Owner:** QCore Project
**Last updated:** 2026-04-16
**Audience:** Contributors, early adopters, and the future team building QCore

> **⚠ Revised 2026-04-16 — 5G-first pivot.**
> The original v0.1 of this PRD ordered 5G (v0.5) after the 4G stack and
> dashboard. [RFC 0001](rfc/0001-5g-sba-pivot.md) reverses that: 5G SA
> becomes the primary v1.0 product and 4G EPC becomes a supported legacy
> track. The dashboard is built **in parallel** from Phase 0 forward, not
> at the end. See [ARCHITECTURE.md](ARCHITECTURE.md) for the target shape
> and §9 (Milestones) below for the updated sequencing.

---

## 0. TL;DR

Open-source mobile cores today (open5GS, free5GC, Magma) are powerful but
punishing. A competent engineer spends days wrestling YAML, 3GPP specs, and
tribal Docker tricks before their first successful UE attach. Commercial cores
(Ericsson, Nokia, Affirmed) are locked behind six-figure contracts and hide
behind GUIs nobody enjoys using.

**QCore's product bet:** the winner of the next decade of mobile core software
will be decided by developer experience, not by feature checklists. We will
ship the first mobile core that:

- Runs in **under 60 seconds** from `git clone` to successful UE attach
- Explains every failure in **plain English** with a fix
- Provides a **live topology view** you'd actually show a customer
- Is **debuggable without a PCAP** for 90% of issues
- Has an **SDK + CLI + Web UI + API** that all feel like they were designed
  by the same person on the same day

This PRD defines what "world's best" means, what we must ship, and how we
measure it.

---

## 1. Vision & Strategy

### 1.1 Vision
> Every developer, researcher, enterprise, and telco should be able to stand
> up a production-grade 4G/5G core in the time it takes to make a cup of
> coffee — and operate it without a PhD.

### 1.2 Strategic Pillars

| Pillar | Thesis | Counter-positioning |
|--------|--------|---------------------|
| **Delightful DX** | Every CLI flag, error message, log line, and dashboard is designed on purpose. | open5GS: "RTFM or suffer." |
| **Honest defaults** | Sensible out-of-box config; zero mandatory YAML for local dev. | free5GC: "Here's 47 YAML files, good luck." |
| **Observable by default** | Structured logs, Prometheus metrics, OpenTelemetry traces, and call flow diagrams are first-class. | Commercial cores: "We'll send a support engineer." |
| **Standards-correct** | Verified against 3GPP test vectors; spec sections cited in code comments. | Hobby projects: "It works on my laptop." |
| **Modern Go stack** | Single static binary per NF, no kernel modules, no C toolchain. | srsRAN/Magma: "Build from source, pray." |
| **Enterprise-grade** | Kubernetes-native, HA, RBAC, audit logs, TLS everywhere. | open5GS: "You're on your own." |

### 1.3 Non-Goals (v1)

- RAN (eNodeB/gNodeB) — QCore consumes radios, does not emulate them beyond
  testing helpers
- IMS/VoLTE — separate future initiative
- Carrier-scale performance (>1M UEs per NF instance) — explicit v2 target
- Legacy 2G/3G (GERAN/UTRAN) support

---

## 2. Users & Use Cases

### 2.1 Personas

**P1 — The Researcher / PhD Student (primary design target)**
Wants to prototype a 5G slicing algorithm by tomorrow's meeting. Has a
laptop, no lab, no budget, and no patience for bash one-liners from a 2019
forum post.

**P2 — The Enterprise Private-5G Engineer**
Deploying a private 5G network at a factory, port, or campus. Needs HA,
monitoring, audit, and integration with their identity system (LDAP/OIDC).
Time-to-production is their KPI.

**P3 — The Telco DevEx Architect**
Evaluating cloud-native cores to replace or complement a legacy vendor.
Cares about O-RAN compliance, scale testing, CI/CD, and GitOps.

**P4 — The CTF / Security Researcher**
Wants to poke at SS7/Diameter/NAS to find bugs. Needs easy packet tap,
replay, and fuzzing hooks.

**P5 — The Course Instructor**
Teaching LTE/5G to 30 students who each need their own core. Classroom
setup must be zero-touch.

### 2.2 Top Use Cases (Ranked)

1. **Local dev loop** — stand up a core, attach a UE simulator, test an idea.
2. **Private network deployment** — deploy to K8s or bare-metal, connect to
   real eNodeB/gNodeB, provision subscribers, monitor health.
3. **CI integration testing** — spin a core in 5 seconds inside a GitHub Action
   to test a downstream app.
4. **Protocol research** — inspect NAS/S1AP/NGAP flows, modify message
   handlers, replay captures.
5. **Education** — follow a tutorial from UE attach to successful data plane
   bearer establishment with call flow visualization.

---

## 3. Competitive Landscape

| Project | Strengths | Weaknesses QCore Exploits |
|---------|-----------|---------------------------|
| **open5GS** | Feature-complete 4G+5G, mature, widely used | C codebase, config nightmare, weak observability, painful debugging |
| **free5GC** | Go, 5G-first, SA+NSA | Incomplete NFs, sparse docs, fragmented, unclear ownership |
| **Magma** | Production-grade, carrier-grade features | Dead (archived), complex, hard to run locally |
| **srsRAN 5G** | Integrated RAN+core, easy attach demo | Bundled; not a stand-alone core |
| **Open5GCore (Fraunhofer)** | Research-grade, cited | Academic UX, licensing |
| **Ericsson / Nokia / Mavenir** | Production scale, vendor support | $$$, proprietary, no DX |

**QCore's wedge:** the DX gap between open-source and commercial is where we
win. Researchers and private-5G will adopt us first because we remove pain
instantly. Telcos will adopt us later because we bring hyperscaler-grade
operational quality without vendor lock-in.

---

## 4. Experience Principles

Every feature decision is tested against these:

1. **Time-to-first-success < 60 seconds.** If a new user can't send NAS
   messages within 60s of `git clone`, we've failed.
2. **Every error includes a fix.** "Connection refused" is not acceptable;
   "The HSS at localhost:8080 is not responding. Start it with `qcore up
   hss`, or point elsewhere via QCORE_HSS_URL." is.
3. **No config is the best config.** Sensible defaults > documented defaults
   > required flags. Config files are for production, not first-run.
4. **Show, don't tell.** Terminal output, UI, and logs visualize state
   changes (spinners, progress bars, topology diagrams).
5. **Observability is a feature, not an add-on.** Every NF emits structured
   logs, metrics, and traces from day one.
6. **Composable everywhere.** CLI → SDK → API → UI, all driven by the same
   underlying primitives. No feature is UI-only or CLI-only.
7. **Fail loudly, degrade gracefully.** A dead SGW must not silently drop
   bearers — it must surface in the topology view, metrics, and logs.

---

## 5. Product Surfaces

### 5.1 The `qcore` CLI (the primary interface)

A single binary, `qcore`, runs the whole show.

```
qcore up                    # Start the full core (HSS+MME+SGW+PGW or AMF+SMF+UPF)
qcore up hss                # Start one NF
qcore down                  # Stop everything, preserving state
qcore status                # One-page summary of what's running + health
qcore logs [nf] [--follow]  # Tail structured logs, with smart filtering
qcore flows                 # Live call-flow diagram in the terminal
qcore subscriber add        # Interactive subscriber provisioning
qcore subscriber import foo.csv
qcore scenario run attach   # Run an end-to-end scenario
qcore doctor                # Self-diagnose: ports, Docker, DB, certificates
qcore upgrade               # Rolling upgrade with version pinning
qcore capture --nas         # Live NAS packet tap with decode
```

**Delight features:**
- **Spinners + color + unicode diagrams** in the TTY; JSON when piped.
- **`qcore doctor`** checks 20+ common pitfalls (port in use, Docker not
  running, cert expired, PLMN mismatch, clock skew) and prints a fix.
- **Tab-completion** shipped for bash/zsh/fish/pwsh on install.
- **Config linter** (`qcore config check`) that explains *why* a value is
  suspicious, not just that it failed validation.

### 5.2 The Web Dashboard

Modern, opinionated, fast. Built with Next.js 14 + Tailwind + shadcn/ui.

**Top pages:**
- **Topology** — live graph of all NFs with health, throughput, and click-
  through to logs/metrics/traces per edge.
- **Subscribers** — searchable table, provisioning wizard, CSV import,
  IMSI/IMEI/MSISDN lookup, live session view.
- **Call Flows** — timeline of NAS/NGAP/GTP messages per UE with a
  sequence-diagram renderer. Click a message to see decoded ASN.1.
- **Sessions** — active PDN/PDU sessions, QoS flows, bearer state, data
  volume, throughput sparklines.
- **Policies** — edit QoS profiles, APN/DNN config, slicing (S-NSSAI).
- **Observability** — Grafana-quality dashboards embedded natively; metrics
  catalog with descriptions.
- **Scenarios** — pre-built scripts (attach, handover, emergency call) with
  one-click replay.
- **Settings** — PLMN, TAI, crypto material, TLS certs, backups, audit log.

**Delight features:**
- **Command palette (⌘K)** jumps to any subscriber, NF, session, metric.
- **"Why isn't this working?" panel** on failed attaches — traces root cause
  to the misconfigured field with a one-click fix.
- **Live CLI mirror** — every UI action prints the equivalent `qcore …`
  command, so users learn the CLI naturally.
- **Dark mode + a11y AAA** out of the box.

### 5.3 The SDK (Go + Python + TypeScript)

Programmatic control for custom integrations, testing, and research.

```go
client, _ := qcore.New("http://localhost:8080")
sub, _ := client.Subscribers.Create(ctx, &qcore.Subscriber{IMSI: "001010000000001"})
av, _ := client.Subscribers.AuthVector(ctx, sub.IMSI)
fmt.Println(av.KASME)
```

SDK features:
- **Zero-config discovery** — SDK finds a local core automatically.
- **Streaming APIs** — tail events, NAS messages, metrics over gRPC/WebSocket.
- **Typed everything** — no `map[string]any` escape hatches unless explicit.
- **Generated from OpenAPI** so all three SDKs stay in sync.

### 5.4 The HTTP + gRPC API

Single OpenAPI spec, published and versioned. All UI/CLI/SDK traffic rides
the same API — no private backdoors.

### 5.5 Kubernetes / Helm

- **One Helm chart per deployment topology** (dev, HA, edge, multi-site).
- **Operator CRDs**: `Core`, `Subscriber`, `Slice`, `QoSProfile`, `Bearer`.
- **GitOps-ready** — declarative subscriber provisioning via CRs.

---

## 6. Functional Requirements (v1.0)

### 6.1 4G EPC
- HSS, MME, SGW, PGW (control + user plane)
- Diameter S6a, S1AP, GTP-C v2, GTP-U v1
- EPS-AKA authentication, NAS/AS security, bearer management
- X2/S1 handover, tracking area update, detach
- DHCP + static IP allocation, IPv4 + IPv6 + IPv4v6 PDN types
- Charging offline (Gy/Rf stub)

### 6.2 5G SA
- AMF, SMF, UPF, AUSF, UDM, UDR, PCF, NRF, NSSF, NEF, BSF
- NGAP (N2), NAS-5G (N1), PFCP (N4), SBI (N5/N7/N8/N10/N11/N12/N13/N22)
- 5G-AKA + EAP-AKA' authentication
- Network slicing (S-NSSAI), QoS flows, URSP
- N3IWF for untrusted non-3GPP access (v1.1)
- UPF with eBPF-accelerated datapath (v1.0 cap: 10 Gbps)

### 6.3 Common
- Subscriber provisioning (SIM bulk import, Milenage/Tuak, K/OP/OPc rotation)
- Multi-PLMN, multi-tenancy
- TLS everywhere with auto-rotating internal CA
- RBAC (admin, operator, auditor, read-only)
- Audit log (tamper-evident, exportable)
- Backup / restore / disaster recovery

### 6.4 Observability
- **Metrics:** RED + 3GPP KPIs (attach success rate, auth failure rate, S1
  setup time, bearer modification time, PFCP latency, UPF throughput)
- **Logs:** JSON structured, per-UE correlation ID, redacted by default
- **Traces:** OTel with NAS/NGAP/PFCP spans, exemplars tying logs → metrics
- **Packet taps:** live NAS/S1AP/NGAP/PFCP decode in the UI and CLI
- **Call flow recorder:** every attach/detach recorded as replayable timeline

### 6.5 Testing & Scenarios
- **Scenario engine** — YAML-described end-to-end tests (attach, handover,
  paging, PDU session mod) that run in CI.
- **UE simulator** (built-in, 100s of UEs) to avoid needing srsUE for
  basic tests.
- **Chaos hooks** — deterministic fault injection (drop 50% of auth
  responses, delay PFCP 200ms, reorder NGAP) for resilience testing.

---

## 7. Non-Functional Requirements

| Category | Target |
|----------|--------|
| **Time-to-first-attach** (fresh clone → UE attached) | ≤ 60s on a modern laptop |
| **`qcore up` cold start** | ≤ 8s |
| **API p99 latency** (subscriber read) | ≤ 20ms |
| **Auth vector generation** | ≥ 5k req/s per HSS instance |
| **UPF throughput** (commodity x86) | ≥ 10 Gbps bidirectional |
| **Memory per NF** (idle) | ≤ 64 MiB |
| **Binary size** (per NF) | ≤ 40 MiB |
| **Startup failure recovery** | Automatic with exponential backoff, surfaced in UI |
| **Test coverage** | ≥ 85% line, ≥ 90% on crypto paths |
| **Standards conformance** | 100% on 3GPP TS 35.208 test vectors; documented gaps elsewhere |
| **Supported platforms** | Linux amd64/arm64, macOS, Windows WSL2 |
| **Uptime (HA deployment)** | 99.99% measured over rolling 30 days |

---

## 8. The "60 Seconds to Wow" Flow (north-star journey)

```
t=0    git clone github.com/qcore-project/qcore && cd qcore
t=3    qcore up
       ▶ Checking Docker ... ok
       ▶ Starting postgres ......... ok (1.2s)
       ▶ Starting HSS ............. ok (0.4s)
       ▶ Starting MME ............. ok (0.5s)
       ▶ Starting SGW/PGW ......... ok (0.6s)
       ▶ Seeding demo subscriber (IMSI 001010000000001) ... ok
       ✓ QCore is ready.

         Dashboard:  http://localhost:3000
         API:        http://localhost:8080
         Metrics:    http://localhost:9090/metrics

         Next: qcore scenario run attach
t=15   qcore scenario run attach
       ▶ UE 001010000000001 authenticating ... ok
       ▶ NAS security context established ... ok
       ▶ Default bearer activated (10.45.0.2) ... ok
       ✓ Attach successful in 842ms.
t=22   (user opens dashboard, sees topology, live flow, metrics)
t=60   user is modifying QoS profile in the UI
```

Every second above is a product-review target.

---

## 9. Milestones

Milestones are ordered by logical dependency. No dates committed — see
RFC 0001 for the "slow but great" philosophy.

**Shipped (pre-pivot, 4G track):**

| Release | Scope | Status |
|---------|-------|--------|
| **v0.1** | HSS + Milenage, REST API, Docker, auto-seed | ✅ Shipped 2026-04-05 |
| **v0.2** | MME + S1AP + NAS + full attach (TCP-SCTP fallback) | ✅ Shipped 2026-04-16 |
| **v0.3a** | SPGW + GTP-U uplink + S11 HTTP + E2E user-plane test | ✅ Shipped 2026-04-16 |
| **v0.3b** | Linux TUN egress + SPGW Prometheus metrics | ✅ Shipped 2026-04-16 |

**Planned (post-pivot, 5G-first track):**

| Release | Scope | Phase (per RFC 0001) |
|---------|-------|----------------------|
| **v0.4** | Phase 0 foundations: `pkg/sbi`, Linux CI w/ NET_ADMIN + UERANSIM smoke, `config.Validate()` | Phase 0 |
| **v0.5** | Subscriber plane refactor: `pkg/subscriber` extracted; `pkg/udm` + `pkg/udr` SBI faces | Phase 1 |
| **v0.6** | 5G control plane: `pkg/ngap`, `pkg/asn1per`, `pkg/nas5g`, `pkg/amf`, `pkg/ausf`, `pkg/nrf` — UERANSIM 5G REGISTRATION works | Phase 2 |
| **v0.7** | 5G user plane: `pkg/pfcp`, `pkg/smf`, evolve `pkg/spgw` → `pkg/upf` — UERANSIM PDU session + ping | Phase 3 |
| **v0.8** | Dashboard parity: subscribers, live attach visualiser, topology, `qcore doctor` | Phase 4 (parallel) |
| **v0.9** | 4G legacy re-skinning onto unified subscriber plane; Diameter S6a facade | Phase 5 |
| **v1.0** | GA: mTLS, tracing, Helm, UERANSIM conformance suite, stable SBI APIs | Phase 6 |

**Explicitly deferred past v1.0:** 5G NSA, network slicing beyond default
S-NSSAI, roaming (N32/SEPP), CHF/charging, N3IWF, carrier-scale (>100k
UEs/instance).

---

## 10. Success Metrics

**Adoption**
- 5,000 GitHub stars within 12 months of v0.2
- 500 Docker pulls/week within 6 months of v0.3
- 50 production private-5G deployments by v1.0

**Experience**
- Median time-to-first-attach (telemetry, opt-in) < 90s
- `qcore doctor` resolves ≥ 80% of reported first-run issues autonomously
- NPS from onboarding survey ≥ 60

**Quality**
- Zero open sev-1 bugs for > 7 days
- All 3GPP TS 35.208 vectors pass on every commit
- Release cadence: predictable monthly minor, weekly patch

**Community**
- ≥ 30 external contributors by v0.5
- Response-to-first-comment on issues: median < 24h
- Docs CSAT ≥ 4.5/5

---

## 11. Open Questions

1. **License:** Apache 2.0 confirmed for code. Dashboard: same Apache 2.0
   for the OSS dashboard. Enterprise dashboard tier (post-v1.0): likely
   BUSL or source-available. RFC 0001 places the OSS/paid seam at the
   dashboard repo level — protocol code stays fully OSS.
2. **Datapath strategy:** eBPF-first (fastest path to 10 Gbps on Linux) or
   DPDK-first (higher ceiling, harder to operate)? Recommendation: eBPF v1.0,
   DPDK as v1.x plugin.
3. **5G SA feature order:** ~~AMF/SMF/UPF first (basic data), or AMF/AUSF/UDM
   first (auth parity with 4G)?~~ **Resolved by RFC 0001** — subscriber
   plane (UDM/UDR/AUSF) first (Phase 1), then AMF + NGAP + 5G-NAS
   (Phase 2), then SMF + UPF + PFCP (Phase 3).
4. **Business model:** donations, Open Core (enterprise add-ons), hosted
   cloud (QCore Cloud), or services? Recommendation: OSS + hosted cloud for
   sustainability, no paywalled protocol features. RFC 0001 confirms: all
   protocol code stays fully OSS; paid tier (if any) is dashboard-only.
5. **Should we ship our own minimal RAN emulator?** Would slash time-to-wow
   further but forks scope. Recommendation: Phase 2+ minimal built-in 5G UE
   for CI; full UERANSIM integration for acceptance tests.
6. **OpenAPI source:** 3GPP publishes OpenAPI specs for the 5G core. Use
   them as-is, patch where needed, or hand-write ours? Leaning: import
   theirs, patch, regenerate clients.
7. **Session-state persistence:** in-memory only, in-memory with periodic
   disk snapshots, or external (etcd)? Leaning: snapshots for v1.0.

---

## 12. Risks

| Risk | Mitigation |
|------|------------|
| 3GPP spec complexity overwhelms scope | Strict milestone gates; ship vertical slices; cite spec sections in code |
| Carrier-grade features balloon timeline | Separate "enterprise" from "carrier" tiers; prioritize private-5G first |
| Crypto correctness bugs are existential | Test vectors on every commit; security audit pre-v1.0 |
| Ecosystem lock to specific RAN vendors | Conformance test matrix against srsRAN, OAI, Amarisoft, UERANSIM |
| Maintainer burnout | Open governance early; paid maintainers via hosted offering |

---

## 13. What "Done" Looks Like

A senior engineer, on a stock laptop, with no prior mobile-core experience,
runs three commands, sees a working attach with a live topology view in
their browser, modifies a QoS profile from the UI, and exports the change
as Git-committable YAML — all within **five minutes**, and without reading
a single spec.

That is the product. Everything in this document is in service of it.

---

*End of PRD v0.1 — feedback welcome via GitHub issues tagged `prd`.*
