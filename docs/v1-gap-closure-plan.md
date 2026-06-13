# QCore v1 Gap-Closure Plan

**Created:** 2026-05-30
**Status:** Historical. The v1 core gap-closure work is substantially complete; the
active executable plan is now `docs/next-phases-plan.md`.
**Purpose:** A self-contained, executable plan to close the gap between today's
build and the v1 the charter defines. Written to be handed to an executing agent
(Claude Sonnet) with no prior context.
**Authoritative inputs:** `docs/experience-charter.md` (vision, North Star §3–§4,
Golden Path §7, AI §9, Decision Log §13) · `docs/audit-v1.0.md` (true build state +
long-term decisions D-1…D-4) · `CLAUDE.md` (build order + cadence).

---

## 0. Read-me-first for the executing agent

**Where things stand (verified 2026-06-13).** 4G EPC is end-to-end real. The 5G SA
control + user plane build and pass an *in-process* E2E test over native SCTP and the
bundled UERANSIM Docker/cloud-Linux T10 replay. **Track A
(Interop Hardening, A1–A4 / D-1…D-4) is complete and merged** — standards-correct PLMN
codec, real SUCI + genuine unprovisioned-IMSI reject, NRF register/discover, and N11
AMF→SMF. **B1 (diagnostic catalog depth) is complete** (13 typed rules, 4G+5G). The
dashboard experience layer (gNB hero screen / Gate 1, live signaling-trace) has shipped.
Everything compiles, `go vet` is clean, `go test ./...` passes (CI green on `main`,
including `-race`). T10 is shipped for the bounded UERANSIM profile by GitHub Actions
run `27115478758`, which proves registration, PDU session establishment, and UE ping
over `uesimtun0` through UPF. C2/C3 credibility-gate UX is also merged and
runtime-proven: the dashboard chooses 4G/5G, launches backend simulator happy/failure
paths, streams real SSE, and renders real diagnostic output.

**C1 (5G telemetry, T7), C2/C3 credibility gate, and T10 are complete for their stated
scopes.** This document is retained to explain how v1 was closed. For new execution,
start from `docs/next-phases-plan.md`: P1.1 measure TTFC/TTRC and P1.2 validate B2 live
offline-SLM serving.

**Non-negotiable working rules:**
1. **No Go toolchain on the host.** Build/test in Docker:
   `docker run --rm -v "$PWD":/src -w /src -v qcore-gomod:/go/pkg/mod golang:1.23 sh -c "go build ./... && go vet ./... && go test ./..."`
2. **Trust rule (charter §4 honesty counter-metric):** never mark a task or status
   row "done/✅" unless build + vet + tests pass. "Code exists" ≠ "shipped."
3. **One task per branch/PR.** Branch `plan/<task-id>-<slug>` off `main`; small,
   reviewable commits; trailer `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.
4. **Correctness counter-metric:** wire-format work (PLMN, SUCI, NAS, NGAP, PFCP)
   must follow the cited 3GPP spec and be covered by golden test vectors — internal
   round-trip tests are not sufficient.
5. **Keep docs honest:** when a task lands, update `docs/audit-v1.0.md` §3 + revision
   log and `docs/wiki.md` per the cadence in `CLAUDE.md`. Update the status box in
   this file (§7).
6. **Each task lists explicit Acceptance Criteria and a Verify command. Do not close
   a task until its Verify passes.**

**Effort key:** S ≤ 1 day · M = 2–4 days · L = 1–2 weeks (rough, agent-execution).

---

## 0.5 Product framing: the two Gates (added 2026-05-30)

A first-principles pass on *who tries this in the first 10 minutes* sharpened the
sequencing. The product lead's first-10-minutes spec: **download/run → a real gNB
connects → a clear "core is connected" status → and if not connected, exactly why and
what to do next.** That reframes v1 around two gates the tracks below feed into:

- **Gate 1 — "It connects."** A real gNB completes **NG Setup** with the AMF and the
  dashboard says so, in plain language — with deterministic diagnosis of the 5 NG Setup
  failure modes (PLMN/TAC mismatch, no connection, slice mismatch, malformed) when it
  fails. *Key realization:* NG Setup is gNB↔AMF only — it needs **A1** (done), not the
  full stack. So Gate 1 is mostly **experience**, not protocol: the hero screen (C3) +
  NG-Setup events/diagnostics (C1). **Landed in PR #20.**
- **Gate 2 — "It's QCore, not just another core."** Failures explain themselves and the
  journey is watchable: the live signaling trace (C3, PR #21) + the offline diagnostic
  AI (B1/B2). This is the moat.

**Design contract for all dashboard work:** `docs/ui-ux-design.md` (Apple/Tesla
language, the single-question "Is your gNB connected?" hero screen + its three states).
The dashboard's hero is the gNB connection status — **not** an NF-health grid.

Mapping to the tracks: Gate 1 = A1 + C1(NG-Setup slice) + C3(hero). Gate 2 = C3(trace)
+ B1 + B2. Track A (interop) is the prerequisite for both and is now **complete**.

---

## 1. Track A — Interop Hardening (makes the 5G headline real)

> These are the four decisions D-1…D-4 from the audit, turned into tasks. They gate
> any real-RAN / UERANSIM claim (T10) and the D11 "5G-leading v1".

### A1 — One standards-correct PLMN codec (D-1) · effort **S**
**Goal.** A single TS 24.008 §10.5.1.13 / TS 23.003-correct PLMN encode/decode used
everywhere; remove the divergent `ngap.PLMNFromMCCMNC` ordering.
**Files.** `pkg/ngap/ngap.go` (`PLMNFromMCCMNC`), `pkg/ngap/ie.go` (PLMN encode/decode
in TAI/NR-CGI/GUAMI), `pkg/subscriber/service.go` (`ParsePLMN`, already correct),
`pkg/amf/nas.go:~338` (comment says it "mirrors PLMNFromMCCMNC"). Audit `pkg/s1ap`
for its own PLMN handling.
**Approach.** Make `subscriber.ParsePLMN`'s byte layout the canonical one (it is the
standards-correct one). Either (a) have `ngap`/`s1ap` call a shared codec, or (b) fix
`ngap.PLMNFromMCCMNC` + its decode to match the standard layout. Prefer a shared
helper (e.g. `pkg/ident` or reuse `subscriber`) to prevent re-divergence.
**Acceptance.**
- `PLMNFromMCCMNC("001","01")` and the matching decode produce the TS 24.008 2-digit
  layout `[0x00,0xF1,0x10]`; 3-digit MNC `"001","001"` → `[0x00,0x11,0x00]`.
- A new `plmn_vectors_test.go` asserts ≥6 golden vectors (2- and 3-digit MNCs)
  captured from the 3GPP spec **and** a real UERANSIM PLMN (00101) — values
  cross-checked against a UERANSIM config, not just self-round-trip.
- All existing ngap/amf/mme tests still pass after migration.
**Verify.** docker sweep (above) green; `go test ./pkg/ngap/... ./pkg/subscriber/... ./pkg/amf/...`.
**Depends on.** none. **Do first** (smallest, also a correctness fix).

### A2 — Real SUCI in the 5G simulator (D-3) · effort **S–M**
**Goal.** The built-in 5G simulator sends a real null-scheme SUCI built from the
configured IMSI; the `unprovisioned_imsi` scenario genuinely triggers
unknown-subscriber rejection.
**Files.** `pkg/simulator/simulator_5g.go` (the `_ = imsi` placeholder + hardcoded
`suciBytes` in `initialUE5g`), `pkg/nas5g/messages.go` (`EncodeSUCI`/`DecodeSUCI` —
already exist), `pkg/nas5g/nas5g.go` (`SUCI` type, `MobileIdentitySUCI`), AMF
`suciToString` in `pkg/amf/nas.go` (verify it recovers `imsi-<MCC><MNC><MSIN>`).
**Approach.** Replace the placeholder with `nas5g.EncodeSUCI(...)` from `c.opts.IMSI`
+ PLMN (use A1's codec). Confirm the AMF→AUSF→UDM path resolves the SUPI and that a
non-provisioned IMSI yields Authentication Reject / Registration Reject.
**Acceptance.**
- Simulator happy path still registers (uses real SUCI end-to-end).
- New test: `unprovisioned_imsi` scenario produces a reject the simulator surfaces as
  an error (not a silent no-op).
- Round-trip test: `DecodeSUCI(EncodeSUCI(s)) == s` for null scheme.
**Verify.** `go test ./pkg/nas5g/... ./pkg/simulator/... ./pkg/amf/...`; sweep green.
**Depends on.** A1 (PLMN codec used in SUCI).

### A3 — NRF registration + discovery (D-2) · effort **M**
**Goal.** Every NF registers with the NRF on boot and discovers peers via it; static
config remains the deterministic fallback so zero-config fast-start (§6.1) is never
gated on a discovery race. Each register/discover emits a Phase-A event.
**Files.** new `pkg/sbi` NRF client (Nnrf_NFManagement: register/heartbeat/deregister;
Nnrf_NFDiscovery: discover by NF type) — note `cmd/smf/main.go` already has a removed
stub call site; `pkg/nrf/nrf.go` (server, in-memory registry — extend if needed);
`cmd/{amf,ausf,udm,udr,smf,upf}/main.go` (register on boot, deregister on shutdown);
`pkg/events/event.go` (add `NFRegistration` / `NFDiscovery` payloads).
**Approach.** Client posts an NFProfile on startup, heartbeats on a ticker, deregisters
on SIGTERM. Discovery returns candidate endpoints; if NRF is unreachable, fall back to
the configured static URL and emit a warning event. Keep it lean — this is SBA plumbing
+ observability, not full Nnrf feature parity (§11 "Not Now").
**Acceptance.**
- With the 5G profile up, each NF appears in `GET /nf-instances` on the NRF.
- AMF discovers AUSF via NRF (not only static config); killing NRF falls back to static
  with a logged+emitted warning, registration still works.
- Phase-A events for registration/discovery show in a journey/health view.
**Verify.** extend the AMF integration test to assert NRF registration; sweep green;
`make up-5g` then `curl localhost:8083/nf-instances` (or equivalent) lists all NFs.
**Depends on.** none hard; pairs with A4.

### A4 — N11 AMF→SMF, real user-plane control flow (D-4) · effort **M–L**
**Goal.** A UE PDU Session Establishment Request flows UE→gNB→AMF→SMF→UPF for real;
the E2E test's direct-SMF REST shortcut is removed.
**Files.** `pkg/amf` (handle UL NAS Transport `MsgTypeULNASTransport` carrying a 5GSM
PDU Session Establishment Request; select SMF via A3 NRF or static; call
`Nsmf_PDUSession` Create SM Context on `pkg/smf`), `pkg/smf/pdu_session.go`
(`SMContextCreate*` types already exist), `pkg/nas5g` (**add minimal 5GSM messages if
absent**: PDU Session Establishment Request/Accept — only ULNASTransport exists today),
`pkg/amf/integration_test.go` (replace the `http.Post(smfURL...)` shortcut with the
AMF-driven path).
**Approach.** AMF parses the 5GSM container from UL NAS Transport, selects an SMF,
issues the SBI call, and returns the PDU Session Accept to the UE; SMF drives PFCP to
UPF (already implemented). Keep SMF selection static if A3 not yet landed.
**Acceptance.**
- Integration test: full Registration **and** PDU session via AMF→SMF→UPF, GTP-U tunnel
  installed, **without** any direct SMF REST call from the test.
- 5GSM round-trip tests for the new messages with golden vectors.
**Verify.** `go test ./pkg/amf/... ./pkg/smf/... ./pkg/nas5g/...`; sweep green.
**Depends on.** A2 (registered UE), ideally A3 (SMF discovery); can use static selection first.

---

## 2. Track B — AI: load-bearing & offline (the moat, §9)

### B1 — Deepen the diagnostic catalog (§9.1) · effort **M**
**Goal.** Cover the cause categories the charter enumerates (PLMN/TAC mismatch, wrong
AMF/MME address, bad slice/DNN, unprovisioned SUPI, wrong Ki/OPc, SCTP/NGAP issues, NAS
algorithm mismatch, data-plane routing) for both 4G and 5G — moving toward the ">90% of
failures surface an actionable cause" metric (§4).
**Files.** `pkg/ai/catalog.go` (today: 4 hardcoded heuristics), `pkg/events/event.go`
(ensure each cause has a distinguishing event signature to match on).
**Approach.** Express each cause as a matcher over the structured journey trace
(category/severity/protocol/payload), returning Explanation/RootCause/Fix. Add a
table-driven test with a synthetic trace per cause. Keep it data/rules-first (§9.4).
**Acceptance.**
- ≥ the 9 §9.1 categories represented, 4G and 5G where applicable.
- Table-driven test: each catalogued cause matches its synthetic trace and yields a
  non-empty fix. A "coverage" test asserts the catalog count ≥ N.
**Verify.** `go test ./pkg/ai/...`; sweep green.
**Depends on.** T7 (for 5G traces to reason over); 4G causes can land first.

### B2 — Embedded SLM, offline-first (§9.3 / D5 / charter Open Q1) · effort **L**
**Goal.** A local small language model ships in the stack and handles the bounded 80%
(narration, cause-code decoding, explaining catalog matches) **offline** — no cloud, no
key — preserving zero-config and air-gapped operation. Frontier escalation stays opt-in
(BYO key), already present via Gemini.
**Decision to make (resolve charter Open Q1, get product sign-off):** recommended —
ship the SLM as its **own container** in the compose (e.g. a `llama.cpp`/`ollama`
server image with a baked-in ~1–3B Q4 GGUF, ~1–2 GB), and add a **`local` provider** to
`pkg/ai/engine.go` that calls it over the in-compose network. This keeps the Go binaries
pure (honors the "pure Go, no C deps" design decision — the C/inference lives in a
sidecar), keeps everything offline (model baked into the image), and makes cloud purely
opt-in. Document the container-size budget.
**Files.** `pkg/ai/engine.go` (add `provider: "local"` branch → SLM endpoint), `pkg/config/config.go` (AIConfig already has Provider/Model/APIKey; add local endpoint), `deployments/docker/` (SLM service + compose entry, default-on for AI), `docs/` (model choice + size).
**Approach.** Engine flow becomes: catalog (deterministic) → local SLM (offline, default)
→ optional frontier (BYO key). The SLM **reasons over the structured layer**, never
free-floats (§9.4): its prompt is the catalog result + the ground-truth trace.
**Acceptance.**
- With no API key and no internet, a failed journey yields a plain-language
  Explanation/RootCause/Fix from the local model grounded in the trace.
- Model choice, container size, and update path documented; size within an agreed budget.
- Cloud path still works when a key is set; local is the default.
**Verify.** `make up` (or `up-5g`), disconnect network, run a failure scenario, confirm a
grounded diagnosis renders. Unit test the provider routing with a mock SLM endpoint.
**Depends on.** B1 (catalog is the grounding); independent of Track A.

**Status — engine + wiring landed (branch `plan/b2-embedded-slm`), model-serve validation pending.**
What is done and **green under `-race`** (build + vet + `go test ./...` in `golang:1.23`):
- `pkg/ai/engine.go` — `local` provider branch; shared `buildPrompt`/`parseModelResult`
  for both backends (one grounded prompt, never free-floats, §9.4); offline path degrades
  gracefully (names cause + `make up-ai` fix) when the sidecar is down, so the dashboard
  never 500s on an absent model.
- `pkg/config/config.go` — `AIConfig.LocalURL` + defaults; provider default flips to `local`.
- `deployments/docker/Dockerfile.slm` — llama.cpp server with a baked **Qwen2.5-1.5B-Instruct
  Q4_K_M GGUF (~1.0 GB, Apache-2.0)**, OpenAI-compatible API on :8088.
- `docker-compose.yml` — `qcore-slm` service behind the **`ai` profile**; dashboard defaults
  to `local` (env-overridable to `gemini`). `Makefile` — `make up-ai`; `down` tears down all profiles.
- `config.example.yaml` — documents `local` vs `gemini` and `local_url`.
- Tests: `pkg/ai/engine_test.go` — provider routing via a mock SLM, grounding assertion,
  unformatted-output degradation, **catalog-wins-first**, and unreachable-sidecar graceful fallback.

**Still gated (like T10 — needs a real environment, not the Go test sandbox):** an actual
`make up-ai` build that pulls the GGUF and serves a real diagnosis end-to-end. The Go side
is validated against a mock endpoint; the live model-serve + air-gapped failure-scenario
render (the §9.3 acceptance) must be run on a host with Docker + network before B2 is
marked ✅ "shipped." Trust rule: code + unit tests green ≠ validated against the real model.

---

## 3. Track C — 5G observability & UX (Golden Path §7 for 5G)

### C1 — Complete 5G Phase-A instrumentation (T7) · effort **M**
**Goal.** AUSF, UDM, UDR, SMF, UPF emit journey-correlated structured events to match
4G coverage, so the diagnostic AI and Live Trace work for 5G. (Today: amf ~9 emits;
others only 2–3, mostly lifecycle.)
**Files.** `pkg/{ausf,udm,udr,smf,upf}/*.go` (add `emitter.Emit` at each signaling
step + state transition, threading the journey/SUCI correlation ID), `pkg/events/event.go`
(5G payload types exist: `NGSetupPayload`, `RegistrationRequestPayload`,
`AuthRequest5GPayload`, `PDUSessionEstablishmentPayload`, `PFCP*Payload`).
**Acceptance.** A 5G registration + PDU session produces ONE correlated, ordered journey
trace spanning all participating NFs (parity with the 4G trace).
**Verify.** integration test asserts the journey event sequence; sweep green.
**Depends on.** A4 (full 5G flow to instrument).

### C2 — 5G control-plane simulator UX (T8) · effort **M**
**Goal.** Built-in 5G sim (NGAP + NAS-5G + 5G-AKA) with the same error-injection menu
as 4G, surfaced as one-click buttons. Builds on A2's real SUCI.
**Files.** `pkg/simulator/simulator_5g.go`, `pkg/dashboard/simulator.go`.
**Acceptance.** Dashboard can run a 5G happy-path attach and inject `wrong_ki`,
`wrong_plmn`, `unprovisioned_imsi`, etc., producing real failures the diagnostic layer
explains.
**Verify.** `go test ./pkg/simulator/...`; manual via dashboard.
**Depends on.** A2.

### C3 — Dashboard 5G mode (T9) · effort **M**
**Goal.** Protocol selector (4G/5G), 5G simulator controls, UDR/5G subscriber view, 5G
RAN-connect snippets.
**Files.** `pkg/dashboard/web/src/*` (React), `pkg/dashboard/server.go`. **Rebuild the
bundle via the Docker web stage** (Dockerfile.dashboard already builds it; do NOT rely
on the committed `dist/`).
**Acceptance.** Toggle to 5G, run the 5G sim, watch the 5G journey, diagnose a 5G
failure — all from the UI.
**Verify.** rebuild dashboard image, exercise in browser; sweep green.
**Depends on.** C1, C2.

---

## 4. Track D — Real-RAN validation (T10) · effort **M** (Linux host)

**Goal.** Prove the headline: a real UERANSIM gNB + UE registers and passes data against
QCore in **native SCTP** mode — the charter's "real RAN, < 15 min TTFC" path.
**Files.** `deployments/docker/docker-compose.yml` (the `5g` profile already includes a
`ueransim` sidecar with `NET_ADMIN` + `/dev/net/tun`), `docs/ueransim-compat.md`.
**Approach.** Run on a **Linux host** (Docker Desktop on macOS does not reliably provide
`/dev/net/tun` + kernel SCTP). Set `amf.sctp_mode: sctp`. Validate registration + PDU
session + uplink.
**Current evidence.** As of 2026-06-08, GitHub Actions run `27115478758` proves real
UERANSIM initial registration, PDU session establishment, NGAP PDU Session Resource
Setup, PFCP remote tunnel update, UPF real TUN/NAT, and UE ping over `uesimtun0`.
**Acceptance.** Documented run where UERANSIM registers a UE and pings out through the
UPF; any interop mismatch fixed (this is where A1/A2/A4 correctness is proven for real).
**Verify.** `make up-5g` on Linux; UERANSIM logs show Registration Accept + PDU session;
data flows. GitHub Actions `ueransim-interop` run `27115478758` satisfies this for the
bundled profile.
**Depends on.** A1, A2, A3, A4, C1.

---

## 5. Track E — Adopt-into-workflow (Golden Path §7 steps 4, 7–8; Phase D)

### E1 — RAN config reconciliation (Step 4) · effort **M**
**Goal.** The RAN-Connect panel doesn't just show snippets — it **reconciles** the
RAN-side values against the core's config and flags mismatches *before* they cause a
silent failure (charter §7 Step 4, Pillar 3).
**Files.** `pkg/dashboard/server.go` (compare endpoint), `pkg/dashboard/web/src/*`.
**Acceptance.** Entering a mismatched PLMN/TAC/AMF address in the panel surfaces an
inline, plain-language warning naming the mismatch and fix (§6.3, §6.4).
**Verify.** manual via dashboard; unit test the reconcile logic.
**Depends on.** A1 (PLMN comparison must use the canonical codec).

### E2 — Scenario authoring (Step 7) · effort **S–M**
**Goal.** Promote the existing declarative scenario support to a first-class authoring
flow. **Files.** `pkg/simulator/scenario.go` (exists), `pkg/dashboard/web/src/*`.
**Acceptance.** A user defines + runs a custom scenario from the UI; it executes against
the simulator and produces a trace. **Depends on.** C2/C3.

### E3 — CI hooks (Step 8) · effort **M**
**Goal.** Headless/CLI scenario execution emitting machine-readable results (e.g. JUnit
XML) for regression suites. **Files.** `cmd/qcore-cli/` (exists), `pkg/simulator`.
**Acceptance.** A documented CLI invocation runs a scenario suite headless and writes a
JUnit report a CI job can consume. **Depends on.** E2.

---

## 6. Sequencing & critical path

```
Wave 1 (correctness + foundation, mostly parallel):
  A1 PLMN ──┐                         B1 catalog (4G) ──┐
            ├─> A2 SUCI                B2 SLM (start early, independent) ──┐
  A3 NRF ───┘                                                              │
Wave 2 (real 5G flow):                                                     │
  A4 N11 ──> C1 5G telemetry ──> B1 (5G causes) ──────────────────────────┤
Wave 3 (5G experience):                                                    │
  C2 5G sim UX ──> C3 dashboard 5G mode                                    │
Wave 4 (prove + adopt):                                                    │
  D/T10 UERANSIM ✅ (bundled profile)      E1 reconcile   E2 scenarios ──> E3 CI
```

**Critical path to the D11 "5G-leading v1":** A1 → A2 → A4 → C1 → T10 is complete for
the bundled UERANSIM profile, and the C2/C3 credibility-gate UX is merged/runtime-proven.
**Critical path to the §9 "AI is load-bearing & offline":** B1 is complete; B2 live
model-serve validation remains. Active execution moved to `docs/next-phases-plan.md`.

## 7. Effort summary & live status

| Task | Charter ref | Effort | Status |
|------|-------------|--------|--------|
| A1 PLMN codec | D-1, §4 correctness | S | ✅ PR #15 — merged |
| A2 SUCI | D-3, §7 | S–M | ✅ PR #16 — merged |
| A3 NRF discovery | D-2, §9 observability | M | ✅ PR #17 — merged |
| A4 N11 AMF→SMF | D-4, §7 | M–L | ✅ PR #19 — merged |
| B1 catalog depth | §9.1, §4 | M | ✅ PR #24 — merged (13 typed rules, 4G+5G, ≥9 §9.1 categories) |
| B2 embedded SLM | §9.3, D5, OpenQ1 | L | ✅ code merged / 🔭 live-serve pending |
| C1 5G telemetry (T7) | §8 Pillar 4 | M | ✅ PR #25 — AUSF/UDM/SMF/UPF emit journey-correlated events; one trace AMF→AUSF→UDM |
| C2 5G simulator (T8) | D10, §7 | M | ✅ credibility-gate slice runtime-proven and merged |
| C3 dashboard 5G (T9) | §7, §8 | M | ✅ credibility-gate slice runtime-proven and merged; broader UDR/operator detail deferred |
| D  UERANSIM (T10) | D11, §4 TTFC | M | ✅ bundled UERANSIM profile validated (`27115478758`) |
| E1 RAN reconcile | §7 Step 4, Pillar 3 | M | ☐ |
| E2 scenario authoring | §7 Step 7 | S–M | ☐ |
| E3 CI hooks | §7 Step 8 | M | ☐ |

> Executing agent: flip a box to ✅ only after that task's Verify passes, then update
> `docs/audit-v1.0.md` and `docs/wiki.md` (cadence in `CLAUDE.md`).

## 8. Definition of done for v1 (maps to charter)

- **D11:** a real UERANSIM gNB+UE registers and passes data against QCore (T10 green for
  the bundled Docker/cloud-Linux profile).
- **§4 TTFC:** < 5 min simulator (5G), < 15 min real-RAN lab — measured, not asserted.
- **§4 TTRC + §9:** offline embedded SLM + catalog explain ≥90% of induced failures with
  an actionable cause; novel failures escalate to BYO frontier.
- **§4 correctness counter-metric:** PLMN/SUCI/NAS/NGAP/PFCP pass golden vectors AND
  real-RAN interop.
- **§7 Steps 0–7** complete for 5G (the headline), with 4G retained (D11).
- All gated by the trust rule: build + vet + tests green, docs reconciled.
