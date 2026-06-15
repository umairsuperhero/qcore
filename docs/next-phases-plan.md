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
> Last updated: 2026-06-15.

## Current verified baseline (read before starting)

As of 2026-06-14, `main` is green: `go build ./...`, `go vet ./...`, `go test -race ./...`
pass in `golang:1.23`; the dashboard `tsc --noEmit` + `vite build` pass; GitHub Actions
`CI` **and** `ueransim-interop` are green on `main`. Shipped and validated:

- **4G EPC**, **Phase A** event model, **Phase B** golden path (one-command launch,
  dashboard, built-in simulator) — ✅.
- **5G SA control + user plane** with **T10 real-RAN data plane** for the bundled
  UERANSIM v3.2.8 Docker/cloud-Linux profile: real UE `ping -c 3 -I uesimtun0` succeeds
  (`ueransim-interop` run `27115478758`). ✅.
- **Diagnostic catalog** (28 rules, including 9 UERANSIM/T10 interop findings, 4G+5G) + AI Level 1/2 + optional Gemini
  escalation — ✅. **Offline SLM (B2): live-validated** with the baked llama.cpp/Qwen
  sidecar, QCore engine live test, dashboard diagnostics API replay, and internal-network
  air-gap smoke test. ✅.
- **Dashboard C2/C3 credibility gate runtime-proven** (audit v1.16): hero 4G/5G selector,
  backend-driven simulator happy/failure runs, real `/api/events/stream` SSE in Live
  Trace, real Diagnostic AI report on `wrong_ki`. ✅.
- **RAN/device config reconciliation (P2.2)**: `POST /api/ran-config/reconcile` and the
  dashboard "Check my RAN config" panel compare UERANSIM gNB/UE YAML against QCore
  AMF/subscriber config and report PLMN, TAC, S-NSSAI, serving-network-name, IMSI, Ki,
  OPc/OP, DNN, and SUCI-scheme mismatches before attach. ✅.
- **5G AUTS/SQN resynchronization (TS 33.102 §6.3.5): interop-validated** (audit v1.22).
  Real reverse-Milenage (f1\*/f5\*, TS 35.208-vector-validated) recovers SQN_MS from AUTS
  across UDM/AUSF/AMF; a real UERANSIM UE forced a Synch failure and completed registration
  after resync (`ueransim-interop` run `27529970131`, `T10 SQN RESYNC PASS`). Crypto slice
  merged as PR #41; interop on `codex/auts-sqn-interop`. ✅.

**The v1 5G-SA-leading thesis is achieved.** What remains is: prove the promised numbers,
drive workflow adoption, and broaden real-RAN demand-first.

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
**Status.** Measured on 2026-06-13 from the current checkout with Docker layer cache
available; evidence is `measurements/latest.json`. TTFC and TTRC are inside charter
targets for this cold compose run. A stricter fresh-clone/no-cache benchmark remains a
future measurement variant, not a blocker for P1.1.

**Why.** The charter's two PRIMARY success metrics (§4) — Time-to-First-Connection and
Time-to-Root-Cause — had not been measured before this task. The product is sold on them.

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
**Status.** Validated on 2026-06-13. `make up-ai` pulled/built the real GGUF sidecar,
the SLM served `qwen2.5-1.5b-instruct` at `/v1`, `TestB2_LiveLocalSLM` passed against
`http://qcore-slm:8088/v1`, the Docker dashboard diagnostics API explained a
collector-injected catalog-miss journey without falling back to `make up-ai`, and the
baked image answered on an internal Docker network with no external egress.

**Why.** B2 was previously "code merged + unit-tested" but not run against a real model.
The 2026-06-13 validation closes that gap for the local sidecar path.

**Scope.** Historical validation scope for the existing `pkg/ai` local provider +
`make up-ai` llama.cpp sidecar (baked Qwen2.5-1.5B GGUF). Do NOT redesign the engine.
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

**Stop condition.** Met. Offline + air-gapped explanation proven.

---

## Phase 2 — Deepen the Flagship (the moat)

Goal: the charter calls AI Level-2 Diagnose **the flagship**. Make it diagnose *real*
failures, not toy ones, and add the highest-value diagnostic feature.

### P2.1 — Mine interop findings into the catalog · branch `codex/catalog-real-failures` · effort **M**
**Status.** ✅ Complete on 2026-06-13. Added catalog rules:
`t10_downlink_nas_transport_aper`, `t10_smc_kamf_supi_prefix`,
`t10_initial_context_setup_aper`, `t10_registration_accept_guti_tlve`,
`t10_ul_nas_transport_shape`, `t10_smf_url_localhost`,
`t10_pdu_session_accept_missing`, `t10_data_plane_n2_n3_gap`, and
`t10_upf_tun_unavailable`.

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
**Status.** ✅ Complete on 2026-06-14 for the dashboard/API credibility slice. Added
`pkg/diag/reconcile.go`, `POST /api/ran-config/reconcile`, and the dashboard "Check my
RAN config" panel. The runtime check deliberately changed the gNB PLMN to `001/02`; the
dashboard reported `gnb.plmn` / `plmn_mismatch`, showed QCore's expected `001/01`, and
gave a before-attach fix.

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

### P3.1 — Scenario authoring (E2) · branch `codex/e2-scenario-authoring` · ✅ **DONE**
Author + replay failure/feature scenarios. Users save/list/run scenarios via
`GET/POST /api/scenarios`, `GET/DELETE /api/scenarios/{name}`, and `POST
/api/scenarios/{name}/run` (deterministic PASS/FAIL via `ScenarioDefinition.Expect`
+ `CompareScenarioOutcome`), plus a dashboard authoring panel; YAML store under
`dashboard.scenario_dir`. Runtime-proven on the local 4G stack: a happy scenario →
`pass:true`, a `wrong_ki` failure scenario → `pass:true` (`cause: wrong_ki`). Build +
vet + `test -race` + frontend `tsc`/`vite build` green. See audit v1.22.

### P3.2 — CI hooks (E3) · branch `codex/e3-ci-hooks` · effort **M**
**Status.** ✅ Complete on 2026-06-15. Added `POST /api/scenarios/run` for direct, stateless scenario execution that fully evaluates `Expect` contracts. Updated `qcore-cli test run` to use this endpoint synchronously, properly exiting with `0` for passing scenarios and `1` for failing ones. Added `--json` flag to emit machine-readable results.

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
  └─ P1.2 B2 live-serve          codex/b2-live-serve         ✅ finishes the offline moat

THEN (the moat):
  ├─ P2.1 Catalog ← real failures codex/catalog-real-failures ✅
  └─ P2.2 E1 reconciliation       codex/e1-ran-reconciliation ✅

THEN (adoption, by user pull):
  └─ P3.1 E2 scenario authoring ✅ → P3.2 E3 CI hooks ✅ → P3.3 Learning Mode

DEMAND-DRIVEN (only when a target needs it):
  └─ P4.1 SUCI Profile A/B → P4.2 per-target replay
```

## Definition of done (per phase)

1. **Phase 1:** TTFC/TTRC measured + in README (targets met or gap documented); B2 offline
   + air-gapped explanation proven. Every AI/metric ✅ is evidence-backed.
2. **Phase 2:** real interop failures are diagnosable catalog rules, and config
   reconciliation reports a deliberately-mismatched gNB/UE config with the fix.
3. **Phase 3:** a user can author + replay a scenario and wire it into CI.
4. **Phase 4:** at least one real-device (Profile A/B) or non-UERANSIM target validated,
   with per-target evidence — not a blanket claim.

Flip a box to ✅ only after that task's Verify passes and its Acceptance evidence exists,
then update README + `docs/audit-v1.0.md` + this file in the same pass (trust rule).
