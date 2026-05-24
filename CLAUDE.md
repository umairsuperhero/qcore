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

## FIRST TASK — audit before building
Before implementing anything new, audit the current repository against the
charter's "Now" scope (§11) and produce a short report covering:
- Which network functions / protocols are implemented and working — 4G EPC, 5G
  SA, 5G NSA, and to what degree.
- Current state of: the dashboard, config validation, the telemetry/event model,
  any simulator integration, any AI features.
- A gap list: charter "Now" scope minus what exists today.
Do not start Phase A until the product lead has reviewed this audit.

## Build order — the re-sequenced roadmap
The pre-charter roadmap optimized for protocol coverage and parked zero-config,
5G, and AI as late "advanced features." The charter makes those the core of the
product. Build in this order instead:

### Phase A — Substrate (foundation; everything depends on it)
- **Structured telemetry & event model.** Every signaling message, network-
  function state change, and config change emitted as structured, machine-
  readable events. This is the substrate the observability UI and the AI both
  consume — design it model-friendly from day one (charter §9.5).
- **Input-time configuration validation framework** (charter principle 3).
- **5G SA core** brought to the same maturity as the 4G EPC, if the audit shows
  a gap. 5G SA is the lead — the Golden Path, demo, and positioning are built
  around it; 4G/EPC remains fully supported but is not the headline (charter D11).

### Phase B — The Golden Path (happy path)
- One-command, zero-config launch (Golden Path 1–2).
- Dashboard: system-health landing view (Pillar 1).
- Guided, validated subscriber + network configuration in the UI (Pillar 2).
- **Simulator integration:** bundle and orchestrate an existing open-source
  RAN/UE simulator — UERANSIM for 5G, srsRAN or similar for 4G. **Do not build a
  simulator from scratch.** Add one-click start and controllable error/
  misconfiguration injection. Target fidelity: a scriptable control-plane tool,
  not an RF emulator.
- Live "see it work" observability view (Pillar 4).
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

## How this project is run
The product lead is a product manager, not a programmer, and directs the build
in plain English. Explain technical tradeoffs accordingly. When a request
conflicts with the charter or this brief, flag it and explain why before
proceeding.
