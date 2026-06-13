# QCore — Codex Build Brief

> Operational companion to `docs/experience-charter.md`. The **charter is
> authoritative** on *what* QCore is and *why*; this brief covers *how* and *in
> what order*. When the two conflict, the charter wins. Recommended location:
> `AGENTS.md` at the repo root, so Codex loads it automatically each session.

## Read first, every session
1. `docs/experience-charter.md` — especially §1 (what QCore is), §3 (North Star),
   §5 (persona), §11 (scope), §13 (Decision Log).
2. This brief.

## What QCore is (one paragraph)
QCore is a development and test environment for cellular networks — **not** a 5G
core competing on protocol features. Primary user: the RAN/device developer who
needs a core to test against. QCore wins on experience: fast start, deep
observability, and AI that explains failures. UX is the product.

## Evidence rules — read every turn
- Before claiming a function, type, constant, import, endpoint, workflow, or Make target
  exists, verify it by reading the file or running `rg`. Never fabricate symbols.
- Before adding a dependency, verify it in the relevant manifest (`go.mod`,
  `pkg/dashboard/web/package.json`, Dockerfile, workflow, etc.) or ask first.
- Do not claim tests, builds, CI, dashboard checks, or T10 replay passed unless you ran
  the command in this session or cite a concrete GitHub run ID.
- Do not claim "5G shipped", "UERANSIM compatible", or T10 status without evidence from
  `docs/ueransim-compat.md` and a passing `ueransim-interop` run.
- Never invent logs, stack traces, API responses, packet hex, or error messages. If you
  did not see them, say so.
- When you cannot verify something, say "I haven't verified this" or "I need to check
  first." Both are better than a confident guess.

## Verification protocol
- For edits touching symbols, first read the defining file or find it with `rg`.
- For edits touching Go, run at least `make verify-fast`; before PR/merge, run or cite
  `make verify-full` / GitHub CI.
- For edits touching dashboard code, run the dashboard portion of `make verify-fast`
  (`tsc --noEmit` + Vite build).
- For edits touching T10, NGAP, NAS-5G, PFCP, SCTP, SMF, or UPF behavior, run/cite the
  focused package tests and the `ueransim-interop` workflow when making external
  compatibility claims.
- For docs/status edits, run `make fact-check` or `make verify-fast` so stale T10/status
  language is caught before summary or commit.

## Current baseline (2026-06-13)
Phase A (event model), Phase B (dashboard, simulator, one-command launch), and the
diagnostic-AI **catalog** are **shipped**. The 4G EPC is complete and end-to-end verified.
For 5G SA: AMF, AUSF, UDM, UDR are integrated, and NRF discovery works using Docker bridge networking (FQDNs). The 5G SA control plane **and** user plane (SMF/UPF/PFCP) build and pass an in-process
end-to-end test (Registration → PDU session → GTP-U tunnel) over **native SCTP** on
Linux. As of this date **every package compiles, `go vet` is clean, and `go test ./...`
passes** (verified in `golang:1.23` — there is no Go toolchain on the host; CI is green
on `main`, including under `-race`).

**Track A — Interop Hardening (D-1…D-4 / I1–I4) is COMPLETE** (merged to main): one
standards-correct PLMN codec (`pkg/ident`), real null-scheme SUCI + genuine
unprovisioned-IMSI reject, NRF register/discover with static fallback, and N11 AMF→SMF
(the E2E test no longer fakes the SMF call). **B1 (diagnostic catalog depth)** — 13 typed
rules across ≥9 cause categories, 4G+5G — and **C1 (T7) 5G Phase-A telemetry** —
AUSF/UDM/SMF/UPF emit journey-correlated events, one correlated trace per 5G
registration, verified by `TestC1_RegistrationEventTrace` (PR #25) — are also complete.
The dashboard experience layer (gNB-connection hero screen / Gate 1, and the animated
live signaling-trace view) has shipped.

**Status update (2026-06-05 integration sweep):** The dashboard live trace is now
**un-mocked** — it runs on the real `/api/events/stream` SSE feed (one zustand store; the
dual EventSource collapsed to one) and calls the real diagnostic engine on live failures;
build migrated esbuild→Vite. **B2 (offline embedded SLM) code is merged** (`pkg/ai` local
provider + `make up-ai` llama.cpp sidecar; catalog still runs first). CI now builds all 13
binaries + `go vet`. Lanes 1–4 + the T10 progress branch are integrated to main; full Go
build/vet/test (`-race`) and dashboard `tsc`/`vite build` verified green.

**The remaining critical-path streams are independent — run in parallel:**
  - **T10 (UERANSIM real-RAN validation / LANE 5) is COMPLETE for the bundled
    Docker/cloud-Linux profile.** A real UERANSIM replay over native SCTP reaches
    NGSetup → InitialUEMessage → Authentication Request/Response → AUSF confirmation →
    Security Mode Complete → InitialContextSetupResponse → Registration Complete →
    AMF→SMF Create SM Context (`201`) → protected PDU Session Establishment Accept →
    NGAP PDU Session Resource Setup → PFCP remote tunnel update → UPF real TUN/NAT →
    UE ping over `uesimtun0`. GitHub Actions `ueransim-interop` run `27115478758`
    records `T10 DATA PLANE PASS`; CI run `27115479708` is green. Evidence and scope:
    `docs/ueransim-compat.md`.
  - **C2 (T8) → C3 (T9):** 5G simulator UX (error injection on the real-SUCI 5G
    sim), then dashboard 5G mode (protocol selector, 5G sim controls, UDR view).
    The C2/C3 credibility-gate slice is runtime-proven on
    `codex/c2-c3-unmock-scenarios`: the hero selects 4G EPC or 5G SA, shows the
    matching RAN endpoint, launches backend simulator happy paths and injected
    failures, navigates to Live Trace, renders real SSE raw logs, and shows the real
    Diagnostic AI report for `wrong_ki`. Broader UDR/operator detail remains a later
    C3/product slice.
  - **B2 — embedded offline SLM:** code merged (see status update); only live model-serve
    validation (real GGUF pull / air-gapped render) remains (charter §9.3 / D5).

**T10 is shipped for the bundled UERANSIM Docker/cloud-Linux profile.** Broader
real-RAN/device compatibility still needs per-target replay evidence; do not generalize
this into a conformance-matrix claim. See `docs/audit-v1.0.md` for the living audit;
**re-verify build/vet/test before trusting any ✅ — "code exists" is not "shipped."**

**The executable v1 gap-closure plan is `docs/v1-gap-closure-plan.md`** — tracks A
(interop hardening I1–I4 / D-1…D-4), B (catalog depth + embedded SLM), C (5G
telemetry/sim/dashboard, T7–T9), D (UERANSIM, T10), E (reconciliation, scenarios, CI).
It is self-contained with per-task acceptance criteria + verify commands and is the
doc to hand an executing agent. Critical path now shifts to merging the C2/C3
credibility-gate PR after full verification, then B2 live model-serve validation
(offline AI) and later Phase-D workflow slices.

## Build order — the re-sequenced roadmap
The pre-charter roadmap optimized for protocol coverage and parked zero-config,
5G, and AI as late "advanced features." The charter makes those the core of the
product. The v0.6 audit then refined this further: the 4G EPC works end-to-end
today, while 5G SA still needs major build. So the experience layer is built and
proven against the 4G EPC first, and 5G SA completion runs as a separate
parallel track (charter D12). v1 still ships 5G-SA-leading (charter D11).

### Phase A — The Event Model (the substrate; sole gate for Phases B and C)
- **Build the structured event model.** Every signaling message, NF state
  transition, and config change emitted as a structured, protocol-agnostic
  event, with correlation IDs threading a single UE's journey across all
  network functions. This is the substrate the observability UI and the AI both
  consume. Detailed plan: `docs/phase-a-event-model.md`.
- Input-time configuration validation is already implemented for 4G (charter
  principle 3); 5G config fields are added on the 5G SA Track.
- **Exit criterion:** the existing 4G end-to-end test produces one correlated,
  queryable, streamable event trace.

### 5G SA Track (parallel — does NOT gate Phases B or C)
Per the v0.6 audit, 5G SA is a "well-structured sketch": five NFs (AMF, AUSF,
UDM, UDR, NRF) are partially built with no binary entrypoints, and SMF, UPF, and
the PFCP/N4 codec do not exist.
- Finish the five partial NFs: complete stubbed endpoints (e.g. the UDM 501s)
  and GUTI re-registration; add binary entrypoints and containers.
- Build SMF and UPF (5G session management and user plane) and the PFCP/N4 codec.
- **Native SCTP transport** for S1AP/NGAP — currently TCP-fallback only.
  Wedge-critical: real gNodeBs speak NGAP over SCTP, and the wedge is testing
  real RAN/devices.
- Instrument the 5G NFs against the Phase A event schema as they mature.
- **Exit criterion:** a 5G SA end-to-end registration + session test passes
  against UERANSIM in 5G mode, equivalent to the 4G end-to-end test.

### Phase B — The Golden Path (built protocol-agnostically; proven on 4G first)
- One-command, zero-config launch (Golden Path 1–2).
- Dashboard: system-health landing view (Pillar 1).
- Guided, validated subscriber + network configuration in the UI (Pillar 2).
- **Simulator integration:** bundle and orchestrate an existing open-source
  RAN/UE simulator — UERANSIM for 5G, srsRAN or similar for 4G. **Do not build a
  simulator from scratch.** Add one-click start and controllable error/
  misconfiguration injection. Target fidelity: a scriptable control-plane tool,
  not an RF emulator.
- Live "see it work" observability view (Pillar 4).
- Built against the 4G EPC first; works for 5G SA once that track lands.
- **Exit criteria:** TTFC < 5 min in simulator mode, demonstrable end to end.

### Phase C — The diagnostic flagship (the AI-native core)
- Structured diagnostic knowledge layer — a curated symptom→cause catalog.
- AI Level 1 (Explain): cause-code decoding, signaling narration.
- AI Level 2 (Diagnose): root-cause analysis + proposed fix, reasoning over the
  Phase A event trace. **This is the flagship.**
- Embedded SLM for bounded cases; optional bring-your-own-key frontier-model
  escalation for hard cases (charter §9.3).
- RAN/device config reconciliation against the core's config.
- **Exit criteria:** TTRC targets met (charter §4).

### Phase D — Iterate & adopt-into-workflow
- Scenario authoring + execution against the simulator (Golden Path 7).
- Learning Mode (secondary persona).
- CI integration hooks (Golden Path 8).

Visual polish is the last 10% — after the experience is sound, not before.

## Explicitly deferred — do NOT build now
Per charter §11 "Not Now": carrier-scale performance and HA; embedded-core
productization; protocol feature-count parity with open5GS/free5GC; AI Levels
3–4; team/hosted features. If a task drifts toward any of these, flag it rather
than building it.

## Build principles
- **Substrate before AI** — the AI is only as good as the telemetry it reasons over.
- The dashboard is the source of truth; YAML is an export path, never required.
- Validate configuration at input time, not at runtime.
- Every error names its cause and its fix.
- AI reliability comes from the structured diagnostic layer + ground-truth
  telemetry — not from the model's parametric knowledge.

## Keeping project status current

When a phase or track milestone ships, update these three things before closing
the session:

1. **README.md** — flip the relevant row in the Project Status table from
   "In progress" to "✅ Shipped" and update the Quick Start if the launch
   command or key URL changed.
2. **AGENTS.md** (this file) — update the "Current baseline" section so the
   next session starts with the right picture. Do not leave it pointing at a
   prior state.
3. **Memory** — update `project_state.md` in the session memory to match.

Do not wait to be asked. A stale status block misleads the next session and
causes wasted re-audit work.

### Documentation cadence (audit doc + wiki)

`docs/audit-v1.0.md` (living baseline audit + long-term decision log D-1…) and
`docs/wiki.md` (living reference) are kept current on a cadence, not ad hoc:

1. **Every milestone** (a T-/I-step or phase landing) and **end of every build
   session**: bump the audit revision log, re-verify the build/vet/test claims, reconcile
   the status tables in both docs against reality, and update the wiki "Last updated" date.
2. **Recurring sweep** (default weekly): re-run
   `docker run --rm -v "$PWD":/src -w /src -v qcore-gomod:/go/pkg/mod golang:1.23 sh -c "go build ./... && go vet ./... && go test ./..."`
   plus `tsc --noEmit` for the dashboard; if anything drifted, update the audit §3 status
   table + revision log and refresh the wiki in the same pass.
3. **Trust rule:** never mark a status row ✅ because code exists — only when build + vet +
   tests pass. Distinguish "works in our E2E test" from "validated against a real
   external gNB/UE"; the latter is gated on the Interop-Hardening track (I1–I4).

## How this project is run
The product lead is a product manager, not a programmer, and directs the build
in plain English. Explain technical tradeoffs accordingly. When a request
conflicts with the charter or this brief, flag it and explain why before
proceeding.
