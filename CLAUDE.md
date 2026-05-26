# QCore — Claude Code Build Brief

> Operational companion to `docs/experience-charter.md`. The **charter is
> authoritative** on *what* QCore is and *why*; this brief covers *how* and *in
> what order*. When the two conflict, the charter wins. Recommended location:
> `CLAUDE.md` at the repo root, so Claude Code loads it automatically each session.

## Read first, every session
1. `docs/experience-charter.md` — especially §1 (what QCore is), §3 (North Star),
   §5 (persona), §11 (scope), §13 (Decision Log).
2. This brief.

## What QCore is (one paragraph)
QCore is a development and test environment for cellular networks — **not** a 5G
core competing on protocol features. Primary user: the RAN/device developer who
needs a core to test against. QCore wins on experience: fast start, deep
observability, and AI that explains failures. UX is the product.

## Current baseline — post-Phase B (2026-05-25)
Phase A (event model) and Phase B (dashboard, simulator, one-command launch) are
**shipped**. The 4G EPC is complete and end-to-end verified. The 5G SA Track is
in progress (T1 next). Phase C (diagnostic AI) comes after the 5G SA Track
lands. See `docs/5g-sa-track.md` for the 5G plan and `docs/audit-v0.6.md` for
the original audit baseline. Re-audit only if the codebase has changed
substantially from what is described here.

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
2. **CLAUDE.md** (this file) — update the "Current baseline" section so the
   next session starts with the right picture. Do not leave it pointing at a
   prior state.
3. **Memory** — update `project_state.md` in the session memory to match.

Do not wait to be asked. A stale status block misleads the next session and
causes wasted re-audit work.

## How this project is run
The product lead is a product manager, not a programmer, and directs the build
in plain English. Explain technical tradeoffs accordingly. When a request
conflicts with the charter or this brief, flag it and explain why before
proceeding.
