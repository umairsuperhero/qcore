# QCore Baseline Audit

**Document status:** Living baseline audit. Re-audited at every milestone and on a
recurring cadence (see *Audit cadence* below).
**Current revision:** v1.1 — 2026-05-29
**Auditor of record this revision:** build-time evaluation (full `go build` + `go vet`
+ `go test ./...` in a `golang:1.23` container; React `tsc --noEmit`).

---

## Revision log

| Rev | Date | Summary |
|-----|------|---------|
| v1.3 | 2026-05-30 | **Track A complete (A1–A4).** PLMN codec (D-1), real SUCI + Registration Reject (D-3), NRF lifecycle + discovery (D-2), N11 AMF→SMF + 5GSM UL NAS Transport (D-4). All PRs merged to main. All 24 packages green. |
| v1.2 | 2026-05-30 | **A1 landed.** `pkg/ident` canonical PLMN codec; `ngap.PLMNFromMCCMNC` and `subscriber.ParsePLMN` now both delegate to it. 7 golden test vectors (not just round-trips). All 24 packages green. D-1 closed. |
| v1.1 | 2026-05-29 | **Correction.** v1.0 claimed the 5G user plane / Phase C AI / 5G E2E test were "shipped and verified." They were uncommitted and had never compiled (~20 build errors + 2 real logic bugs). All fixed; full suite now green. Four interop gaps identified and turned into long-term decisions (D-1…D-4) + an Interop-Hardening track (I1–I4). |
| v1.0 | 2026-05-28 | Initial post-Phase-C audit. Overstated completion — superseded by v1.1. |

---

## 1. Executive summary

QCore's **4G EPC is real and end-to-end verified.** The **5G SA control plane
works end-to-end in an automated test** (AMF + AUSF + UDM + UDR + NRF + SMF + UPF,
mock gNB, Registration → PDU Session → GTP-U tunnel) **over native SCTP**, and the
**Phase C Diagnostic AI** (catalog heuristics + optional Gemini escalation) is wired
into the dashboard. As of this revision **every package compiles, `go vet` is clean,
and the full `go test ./...` suite passes.**

The important correction over v1.0: that state did **not** exist when v1.0 was
written. The 5G user-plane work (`pkg/pfcp`, `pkg/smf`, `pkg/upf`), the AI
(`pkg/ai`), the native SCTP path (`pkg/sctp/sctp_linux.go`), and the T5 integration
test were all written but **never built** — they carried ~20 compile errors and two
genuine logic bugs. They have been repaired (see §2). The lesson is process, not
code: **"shipped" must mean "builds + tests pass in CI," never "code was written."**
This audit is now the living source of truth for that distinction.

## 2. What was repaired this revision

All of the following were uncommitted when found and are now fixed and green:

- **Won't-compile (mechanical):** wrong Go module path in `pkg/ai`; missing `strings`
  import; `unix.Pointer`→`unsafe.Pointer` + dead var in native SCTP; NGAP struct/field
  mismatches in `pkg/amf` and the 5G simulator; `logger.Fields`→`map[string]interface{}`
  in SMF/UPF; wrong GTP-U codec API in UPF + the integration test; a method defined
  *inside* `upf.NewService`; a call to a nonexistent `sbi.NewNRFClient`; UPF `nodeID`
  typed `[]byte` instead of `net.IP`; a missing argument to
  `pfcp.NewSessionEstablishmentResponse`.
- **Real logic bug — SMF IPAM:** the allocator wrapped on `nextIP.Mask(...)` instead of
  the pool's network address, so once the pool filled it handed out addresses *outside*
  the configured subnet. Fixed to wrap to the pool network address.
- **Test correctness:** `subscriber.ParsePLMN`'s test asserted a wrong expected value
  (the implementation is standards-correct); the SPGW TUN test only skipped on
  `EPERM`, not on a missing `/dev/net/tun` device node (`ENOENT`) — so it failed in
  any unprivileged container instead of skipping.

## 3. Component status (verified)

| Area | Status | Evidence |
|------|--------|----------|
| 4G EPC (HSS/MME/SPGW) | ✅ Shipped | `pkg/mme` E2E attach + user-plane tests pass |
| Phase A event model | ✅ Shipped (4G) · ⚠️ partial (5G) | 4G NFs fully instrumented; 5G NFs partially (T7 incomplete) |
| Phase B golden path / dashboard / simulator | ✅ Shipped | builds; frontend type-checks; simulator tests pass |
| 5G SA control plane | ✅ Works in E2E test | `pkg/amf` integration test green over native SCTP |
| 5G SA user plane (SMF/UPF/PFCP) | ✅ Builds + unit-tested + in E2E | compiles; SMF/PFCP unit tests pass; exercised by the E2E test |
| Native SCTP | ✅ Linux path compiles + used by E2E | `pkg/sctp/sctp_linux.go`; macOS keeps TCP fallback |
| Phase C Diagnostic AI | ✅ Wired | `pkg/ai` builds; dashboard diagnose endpoint present |

## 4. Open interop gaps → long-term decisions

These are the four items that stand between "passes our own tests" and "a real RAN/UE
plugs in and it works." Each is recorded as a **long-term decision** framed by where
QCore must be by **2030**: the default local cellular core a RAN/device developer
reaches for, that interoperates flawlessly with real gNBs/UEs and whose **diagnostic
AI is the moat.** The wedge is real-RAN interop — so where a choice trades spec-fidelity
against internal convenience, **spec-fidelity wins**, because by 2030 every connection
is assumed to be a real, standards-compliant device.

### D-1 — One standards-correct identifier codec (PLMN first)
**Problem.** Two PLMN encoders disagree on byte order: `ngap.PLMNFromMCCMNC`
(non-standard) vs. `subscriber.ParsePLMN` (TS 24.008-correct). NGAP round-trips with
itself, so tests pass — but a standards-compliant gNB/UE (UERANSIM) would be misparsed.
**Decision.** Establish a **single, standards-correct codec** for PLMN — and over time
TAC, S-NSSAI, GUAMI, GUTI — used by 4G, 5G, and the subscriber store alike. Validate it
with **golden test vectors captured from real UERANSIM/srsRAN/Amarisoft**, not just
internal round-trips. Decommission the NGAP ordering.
**Why long-term.** Two private encoders that "agree with themselves" are the canonical
bug that passes CI and fails the first real device. Consolidating the primitive removes
a whole class of future interop failures and is a prerequisite for an honest T10
(UERANSIM compatibility) claim.

### D-2 — NRF as the discovery backbone; static config as the zero-config fallback
**Problem.** No NF actually registers with or discovers via the NRF; wiring is static
URLs. The "NRF-based discovery" claim (T1) is overstated.
**Decision.** Implement real **Nnrf_NFManagement** (register / heartbeat / deregister)
and **Nnrf_NFDiscovery** in `pkg/sbi`. Every NF registers on boot and discovers peers
via NRF — **emitting a Phase-A event for each registration and discovery** so the AI can
diagnose "SMF never registered" or "AMF couldn't find SMF." Keep static/env config as an
explicit override **and** as the deterministic default for the golden path.
**Why long-term.** SBA discovery is both the standards-correct design and a rich
observability surface that feeds the diagnostic moat. By 2030 users will add, swap, and
mock NFs; discovery must be real. But experience wins over purity — zero-config
fast-start (TTFC < 5 min) must never be gated on a discovery race, so discovery is
**layered, not mandatory.**

### D-3 — Real SUCI in the NAS layer and simulator (null-scheme first)
**Problem.** The 5G simulator sends a hardcoded placeholder SUCI; the
`unprovisioned_imsi` scenario therefore does not actually exercise unknown-subscriber
rejection.
**Decision.** Implement proper **SUCI** (5GS mobile identity) encode/decode in
`pkg/nas5g` — **null scheme (protection scheme 0) first** (the structured, standard form
and the common UERANSIM test default), with **ECIES Profile A/B planned behind the same
interface** as a later increment. Wire the simulator's configured IMSI through it so the
error scenarios test what they claim.
**Why long-term.** A test scenario that silently doesn't test what it advertises
violates "every error names its cause" and erodes trust in the simulator — and the
simulator's credibility *is* the product. Real-device registration needs correct SUCI
regardless of scheme.

### D-4 — Implement N11 (AMF→SMF) so the user plane is real end-to-end
**Problem.** The AMF does not forward PDU sessions to the SMF; the E2E test calls the
SMF REST endpoint directly to fake the AMF's step.
**Decision.** AMF performs **SMF selection** (via NRF once D-2 lands; static until then)
and calls **Nsmf_PDUSession Create SM Context** on PDU-session establishment, so the flow
is genuinely UE → gNB → AMF → SMF → UPF. Replace the test's shortcut with the real call.
**Why long-term.** The data plane is half of "test against a core." By 2030 a 5G core
that can't actually carry a PDU session through the real control flow isn't credible.
This closes the loop with D-2 (discovery) and D-3 (a fully registered UE then opens a
session).

## 5. Interop-Hardening track (I1–I4) — sequencing

Do these **before** claiming T10 (UERANSIM compat) or marking the 5G SA track "shipped."
Ordered smallest-blast-radius / highest-wedge-value first:

| Step | Decision | Effort | Unblocks |
|------|----------|--------|----------|
| I1 | D-1 PLMN codec consolidation + golden vectors | Small | Honest real-gNB interop; T10 |
| I2 | D-3 SUCI null-scheme + wire simulator IMSI | Medium | Real registration; working error scenarios |
| I3 | D-2 NRF register/discover + Phase-A events | Medium | Backbone for I4; discovery observability |
| I4 | D-4 N11 AMF→SMF, real E2E (drop the shortcut) | Medium | True UP end-to-end |

After I1–I4: complete T7 (5G event instrumentation) so Phase C reasons over real 5G
traces, then T8/T9 (5G simulator UX, dashboard 5G mode), then T10 (UERANSIM in a sidecar).

## 6. Deferred (unchanged from charter §11)

Carrier-scale performance/HA; embedded-core productization; protocol feature-count
parity with open5GS/free5GC; AI Levels 3–4; team/hosted features. The decisions above are
interop-correctness, **not** feature-count parity — they stay in scope.

## 7. Audit cadence

This document is **living**, not a one-time artifact.

- **On every milestone** (a T-/I-step or phase landing): bump the revision log, re-verify
  the build/vet/test claims, and reconcile §3 against reality before closing the session.
- **Recurring sweep:** on a fixed cadence (default: weekly), re-run
  `go build ./... && go vet ./... && go test ./...` (in the `golang:1.23` container — no
  Go toolchain on the host) and `tsc --noEmit` for the dashboard, and update §3 + the
  revision log if anything drifted. `docs/wiki.md` is refreshed in the same pass.
- **Trust rule:** never mark a row ✅ on the strength of code existing. Mark it ✅ only
  when build + vet + tests pass in CI.

> Build/test command (host has no Go):
> `docker run --rm -v "$PWD":/src -w /src -v qcore-gomod:/go/pkg/mod golang:1.23 sh -c "go build ./... && go vet ./... && go test ./..."`
