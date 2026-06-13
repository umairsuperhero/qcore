# QCore — Next-Phases Plan (post-v1-core)

> **What this is.** The executable plan for the work that comes **after** the v1 core
> thesis landed. It is the successor to `docs/v1-gap-closure-plan.md` (tracks A–E, now
> mostly ✅). Hand this doc to the executing agent (Codex). It is self-contained:
> every task has a Why, Scope, Acceptance criteria, Verify commands, Evidence to
> capture, and a Stop condition.
>
> **Authoritative companions:** `docs/experience-charter.md` (vision/scope — wins on
> conflict), `CLAUDE.md` / `AGENTS.md` (build order + guardrails), `docs/audit-v1.0.md`
> (living audit), `docs/3gpp-tracking.md` (interop status), `docs/ueransim-compat.md`
> (T10 evidence).
>
> Last updated: 2026-06-13.

## Current verified baseline (read before starting)

As of 2026-06-13, `main` is green: `go build ./...`, `go vet ./...`, `go test -race ./...`
pass in `golang:1.23`; the dashboard `tsc --noEmit` + `vite build` pass; GitHub Actions
`CI` **and** `ueransim-interop` are green on `main`. Shipped and validated:

- **4G EPC**, **Phase A** event model, **Phase B** golden path (one-command launch,
  dashboard, built-in simulator) — ✅.
- **5G SA control + user plane** with **T10 real-RAN data plane** for the bundled
  UERANSIM v3.2.8 Docker/cloud-Linux profile: real UE `ping -c 3 -I uesimtun0` succeeds
  (`ueransim-interop` run `27115478758`). ✅.
- **Diagnostic catalog** (13 typed rules, 4G+5G) + AI Level 1/2 + optional Gemini
  escalation — ✅. **Offline SLM (B2): code merged + unit-tested**, live model-serve
  **pending** (P1.2 below).
- **Dashboard C2/C3 credibility gate runtime-proven** (audit v1.16): hero 4G/5G selector,
  backend-driven simulator happy/failure runs, real `/api/events/stream` SSE in Live
  Trace, real Diagnostic AI report on `wrong_ki`. ✅.

**The v1 5G-SA-leading thesis is achieved.** What remains is: prove the promised numbers,
deepen the AI moat, drive workflow adoption, and broaden real-RAN demand-first.

## Guardrails (non-negotiable — same as CLAUDE.md/AGENTS.md)

- **No Go toolchain on the host.** Build/vet/test in Docker:
  `docker run --rm -v "$PWD":/src -w /src -v qcore-gomod:/go/pkg/mod golang:1.23 sh -c "go build ./... && go vet ./... && go test -race ./..."`
- **Trust rule.** A task is ✅ only when build + vet + tests pass **and** the task's own
  Acceptance evidence exists. "Code exists" ≠ "shipped." "Passes our test" ≠ "validated
  against a real external gNB/UE." Never mark a doc row ✅ without the evidence cited.
- **One branch per task** (suggested names below; Codex should use the `codex/` prefix);
  open a PR; **CI + `ueransim-interop`
  are the merge gate**. New commits only; never force-push; never commit secrets.
- **Don't broaden.** Each task is scoped. Do not rewrite NGAP/PER/SCTP, the catalog
  engine, or the dashboard architecture unless the task says so. If a task drifts toward
  charter §11 "Not Now" (carrier-scale/HA, feature-count parity, AI Levels 3–4, team/
  hosted), **stop and flag it**.
- **Keep docs honest.** If a task stops at a new blocker, document the exact blocker with
  evidence (run ID / log excerpt); do not mark the milestone shipped.

---

## Phase 1 — Prove the Promise (do first; highest ROI)

Goal: convert "we built it" into "we measured/validated it," so v1 can be announced
honestly. Both tasks finish the last 10% of already-built work.

### P1.1 — Measure TTFC & TTRC · branch `codex/measure-ttfc-ttrc` · effort **S–M**
**Why.** The charter's two PRIMARY success metrics (§4) — Time-to-First-Connection and
Time-to-Root-Cause — have never been measured. The product is sold on them.

**Scope.** Add a measurement harness (a script + a small Go or shell driver, e.g.
`scripts/measure-ttfc-ttrc.sh` or `make measure`). Do NOT change product behavior to game
the number; if a target is missed, fix the *flow*, not the measurement.
- **TTFC:** from a cold `git clone` (or `make up`) to the first successful attach
  (4G simulator) and first 5G registration (5G simulator), wall-clock.
- **TTRC:** from an injected failure (`wrong_ki`, `wrong_plmn`, `unprovisioned_imsi`,
  `wrong_mme_address`) to the Diagnostic AI report being available (via the dashboard / the
  `/api/diagnostics/journey/{id}` path), wall-clock.

**Acceptance.**
- A repeatable harness prints TTFC (4G + 5G) and TTRC (per scenario) numbers.
- Numbers recorded in `README.md` and `docs/audit-v1.0.md` with the date + how measured.
- Charter targets: TTFC < 5 min (simulator); TTRC < 30 s (known/catalogued failures). If
  missed, file the gap honestly and fix the flow until met (or document why not).

**Verify.** Run the harness on a fresh checkout; `make verify-fast` still green.

**Stop condition.** Numbers measured + recorded, or the exact flow blocker documented.

### P1.2 — B2 offline SLM live model-serve validation · branch `codex/b2-live-serve` · effort **M**
**Why.** B2 is "code merged + unit-tested" but never run against a real model. It's the
offline-AI moat (charter §9.3); it can't be called shipped until it serves.

**Scope.** Validate the existing `pkg/ai` local provider + `make up-ai` llama.cpp sidecar
(baked Qwen2.5-1.5B GGUF). Do NOT redesign the engine.
- Pull/build the GGUF sidecar; bring it up with `make up-ai`.
- Drive a catalog-miss failure and confirm the **offline** SLM produces a grounded
  explanation **with no cloud account and no API key**.
- Prove **air-gapped**: disable network egress and confirm it still explains.

**Acceptance.**
- Evidence (log/screenshot/transcript) of the offline SLM explaining a real failure with
  no key, and the air-gapped run.
- The catalog still runs first (deterministic), SLM only on misses, grounded in the trace.
- Flip B2 to ✅ in README / wiki / audit / `3gpp-tracking` is irrelevant; flip it in the
  AI status rows only after this evidence exists.

**Verify.** `make verify-fast`; the documented `make up-ai` run.

**Stop condition.** Offline + air-gapped explanation proven, or the serve blocker documented.

---

## Phase 2 — Deepen the Flagship (the moat)

Goal: the charter calls AI Level-2 Diagnose **the flagship**. Make it diagnose *real*
failures, not toy ones, and add the highest-value diagnostic feature.

### P2.1 — Mine interop findings into the catalog · branch `codex/catalog-real-failures` · effort **M**
**Why.** Every real T10 blocker we fixed (K_AMF/SUPI-prefix, GUTI TLV-E length, NGAP APER
extensibility, SMF URL in Docker) is a failure a real user could hit. Turn them into
catalog rules so QCore *diagnoses* them.

**Scope.** Extend `pkg/ai/catalog.go` (and the event tagging the catalog matches on) with
typed rules for the real-world failure classes surfaced during T10 and C2/C3. Each rule:
symptom → cause → fix, grounded in observable events.

**Acceptance.**
- New typed rules covering the documented interop failures (see `docs/3gpp-tracking.md`
  Interop Findings log) with table-driven tests, like B1.
- Each rule names the cause and the fix (charter principle: "every error names its cause
  and its fix").
- `go test -race ./pkg/ai/...` green.

**Verify.** Docker `go test -race ./pkg/ai/...`; `make verify-fast`.

### P2.2 — RAN/device config reconciliation (E1) · branch `codex/e1-ran-reconciliation` · effort **M**
**Why.** Most real failures are config mismatches — PLMN, TAC, S-NSSAI, Ki/OPc, SUCI
scheme — exactly what bit us repeatedly. Comparing the gNB/UE config against the core and
explaining the mismatch is the single highest-value diagnostic feature.

**Scope.** Per `docs/v1-gap-closure-plan.md` §5 E1. Accept a RAN/UE config (e.g. a
UERANSIM gnb.yaml/ue.yaml or pasted values), diff it against the core's config, and
surface human-readable mismatches in the dashboard + via the diagnostic layer.

**Acceptance.**
- Given a deliberately-mismatched gNB/UE config, the dashboard/API reports the specific
  mismatched field(s) and the fix, before/without needing a live attach.
- Focused tests for the reconciliation logic; `make verify-fast` green.

**Verify.** Docker Go tests for the reconciliation package; dashboard `tsc`/build.

---

## Phase 3 — Adopt-into-Workflow (Phase D / Track E)

Goal: turn QCore from "a core you run" into "a tool in your dev loop." Lower urgency than
Phases 1–2; sequence by user pull.

### P3.1 — Scenario authoring (E2) · branch `codex/e2-scenario-authoring` · effort **S–M**
Author + replay failure/feature scenarios (generalize the `wrong_ki`/`wrong_plmn`
injections). Acceptance: a user can define a scenario (YAML or UI), run it against the
simulator, and get a deterministic pass/fail + trace. Tests + `make verify-fast` green.

### P3.2 — CI hooks (E3) · branch `codex/e3-ci-hooks` · effort **M**
Productize the `ueransim-interop` pattern so users run QCore scenarios in *their* CI
(a documented GitHub Action / CLI exit-code contract). Acceptance: a scenario run returns
a CI-usable exit code + machine-readable result; documented.

### P3.3 — Learning Mode (secondary persona) · branch `codex/learning-mode` · effort **M**
Lowest priority. Guided explanations of the signaling flow for the learner persona
(charter secondary persona). Acceptance: deferred until Phases 1–3 land.

---

## Phase 4 — Broaden Real-RAN (demand-driven; resist feature-count, §11)

Only build these when a concrete target needs them. The quarterly 3GPP/interop check-in
(`qcore-3gpp-interop-checkin`) watches for triggers.

### P4.1 — SUCI Profile A/B (ECIES) · branch `codex/suci-profile-ab` · effort **M–L**
**Why.** Real devices encrypt the SUCI (Profile A/B ECIES); QCore supports only
null-scheme today, which is the easy simulator case. This is the #1 gap before testing
against **real** UEs/basebands. Acceptance: a Profile-A SUCI from a real UE (or a
conformance vector) is de-concealed and registration completes; tests pin the crypto.

### P4.2 — Per-target real-RAN replay · effort **M** each (Linux host)
srsRAN, other UERANSIM versions, real gNBs/UEs — one replay + evidence per target. Update
`docs/ueransim-compat.md` / `docs/3gpp-tracking.md` Findings log per target. **Never**
generalize the bundled-profile result into a conformance-matrix claim.

---

## Sequencing & critical path

```
NOW (parallel):
  ├─ P1.1 Measure TTFC/TTRC      codex/measure-ttfc-ttrc     ⭐ credibility headline
  └─ P1.2 B2 live-serve          codex/b2-live-serve         ⭐ finishes the offline moat

THEN (the moat):
  ├─ P2.1 Catalog ← real failures codex/catalog-real-failures
  └─ P2.2 E1 reconciliation       codex/e1-ran-reconciliation ⭐ highest-value diag feature

THEN (adoption, by user pull):
  └─ P3.1 E2 → P3.2 E3 → P3.3 Learning Mode

DEMAND-DRIVEN (only when a target needs it):
  └─ P4.1 SUCI Profile A/B → P4.2 per-target replay
```

## Definition of done (per phase)

1. **Phase 1:** TTFC/TTRC measured + in README (targets met or gap documented); B2 offline
   + air-gapped explanation proven. Every AI/metric ✅ is evidence-backed.
2. **Phase 2:** real interop failures are diagnosable catalog rules; config reconciliation
   reports a deliberately-mismatched gNB/UE config with the fix.
3. **Phase 3:** a user can author + replay a scenario and wire it into CI.
4. **Phase 4:** at least one real-device (Profile A/B) or non-UERANSIM target validated,
   with per-target evidence — not a blanket claim.

Flip a box to ✅ only after that task's Verify passes and its Acceptance evidence exists,
then update README + `docs/audit-v1.0.md` + this file in the same pass (trust rule).
