# Build Brief — AUTS/SQN Resync: real-UERANSIM interop validation (tier-2 → tier-3)

> Self-contained brief for Codex. Companion to `docs/briefs/auts-sqn-resync.md`
> (the original feature brief). The **charter wins on any conflict**
> (`docs/experience-charter.md`). When in doubt, do less and flag it.

## Mission (one paragraph)
The 5G SQN-resynchronization flow (AUTS) is **built and merged to `main`** (PR #41,
commit `998ea29`) and its **crypto is vector-validated** against 3GPP TS 35.208.
But it is only **tier-2** ("passes our own crypto tests") — it has **never been
exercised by a real UE forcing a real AUTS end-to-end**. Your job is to make it
**tier-3 (interop-proven)**: add a UERANSIM SQN-desync scenario to the
`ueransim-interop` gate that drives a real UERANSIM UE into a `Synch failure`,
captures the AUTS, and proves QCore recovers the UE's SQN, re-issues a fresh
challenge, and completes registration — with a green GitHub Actions run as the
evidence. Optionally (secondary), add a DB-backed unit test for the resync
orchestration. Then bring all docs/status/memory up to date **with evidence**.

## Read first, in this order
1. `AGENTS.md` and `CLAUDE.md` (root) — evidence rules, verification protocol,
   "no Go on host" Docker discipline, trust rule, §11 "Not Now".
2. `docs/briefs/auts-sqn-resync.md` — the original feature brief (the flow).
3. `docs/ueransim-compat.md` — esp. the SQN section (~line 221): the demo SQN is
   seeded at `000000000020` because UERANSIM starts `SQN-MS=000000000000` and
   rejects a network SQN whose sequence part is not ahead.
4. The merged resync code (already on `main`):
   - `pkg/subscriber/milenage.go` — `F1Star`, `F5Star` (vector-tested).
   - `pkg/subscriber/milenage_test.go` — `TestResyncAUTSRoundTrip`,
     `TestResyncAUTSRejectsTamperedMACS` (the crypto de-risk; **do not break these**).
   - `pkg/subscriber/service.go` — `Resync5GAuthVector`, `AdvanceSQNHex` (+32 jump).
   - `pkg/udm/ueau.go` — `ResyncAndGenerateAv`, `MAC_S_FAILURE`→403.
   - `pkg/ausf/ausf.go` — passthrough of `ResynchronizationInfo`.
   - `pkg/amf/nas.go` — the resync orchestrator in `handleAuthenticationFailure`
     (the `Cause5GMMSynchFailure && len(fail.AUTS)==14` branch; `ResyncAttempted`
     guard). Real log lines you can assert on:
       - `amf: attempting SQN resynchronization`
       - event message `Authentication Request sent (Resync)`
       - `amf: SQN resync already attempted, rejecting` (loop guard)
   - `pkg/nas5g/messages.go` — `AuthenticationFailure.AUTS` decode (IEI `0x30`).

## The interop harness you are extending (verified paths)
- Gate workflow: `.github/workflows/ueransim-interop.yml` (job `interop`,
  `runs-on: ubuntu-latest`, `COMPOSE_PROFILES: "5g"`). It brings up the stack +
  real UERANSIM gNB/UE over **native kernel SCTP**, then polls container logs for
  registration → PDU session → `ping -I uesimtun0`, and prints
  `::notice::T10 DATA PLANE PASS`. **Do not regress this existing happy path.**
- Compose: `deployments/docker/docker-compose.yml` (services include `udr`, `udm`,
  `ausf`, `amf`, `smf`, `upf`, `ueransim-gnb`, `ueransim-ue`, `postgres`).
- UE credentials: `deployments/docker/ueransim-config/ue.yaml` —
  `supi: imsi-001010000000001`, `key: 465b5ce8b199b49faa5f0a2ee238a6bc`,
  `op: cd63cb71954a9f4e48a5994e37a02baf` (`opType: OPC`), `amf: 8000`. These are
  **TS 35.208 Test Set 1** — the same vectors the crypto tests use.
- Demo subscriber seed: `cmd/udr/main.go` `seedDemoSubscriberIfEmpty` and
  `cmd/udm/main.go` reset (direct-mode auth). SQN seeded at `000000000020`.
  Env: `QCORE_RESET_DEMO_SUBSCRIBER=true` (re-applies the seed, resetting SQN back
  to `000000000020`); `QCORE_SKIP_SEED=true` (skip seeding).

## Why a desync needs two rounds (don't fight this)
A **fresh** UERANSIM UE has `SQN-MS=0` and accepts any network SQN > 0, so it can
**never** desync on first contact. A desync only happens when the UE's stored
SQN-MS is **ahead** of the network — the real "physical UE survived a core
restart/DB reset" scenario. Therefore the scenario is inherently:
1. **Round 1 — normal registration.** UE registers; its internal SQN-MS advances
   past the network's `000000000020`. (Do **not** restart the UE container after
   this — restarting resets SQN-MS to 0 and destroys the desync.)
2. **Force the core behind the UE.** While the UE keeps running, move the core's
   subscriber SQN to a value **below** the UE's current SQN-MS. Two viable
   mechanisms — pick the most reliable in CI and document which:
   - Re-apply the demo seed (`QCORE_RESET_DEMO_SUBSCRIBER=true`) and restart
     **only** `udr`+`udm` (NOT the UE/gNB), resetting SQN to `000000000020`; or
   - Write the SQN directly via the subscriber API (whatever endpoint
     `pkg/subscriber`/dashboard exposes — verify it; do not invent one) to a low
     value like `000000000001`.
3. **Round 2 — trigger re-authentication** without restarting the UE container, so
   it re-auths with its advanced SQN-MS. Use the UERANSIM UE CLI `nr-cli` inside
   the `ueransim-ue` container (e.g. `nr-cli imsi-001010000000001 --exec
   "deregister normal"` then let it re-register, or the appropriate `nr-cli`
   command — confirm the exact verb against the UERANSIM build in the image).
4. **Observe resync.** Core issues AUTN with the now-behind SQN → UE returns
   `Authentication Failure` (Synch failure) **with AUTS** → AMF runs the resync →
   UE accepts the re-issued challenge → registration completes.

## Primary deliverable — the interop assertion
Add a **bounded, deterministic** SQN-resync check to the `ueransim-interop`
workflow (either a new step in the existing job, or a sibling job — your call;
keep the existing T10 happy-path intact). It must assert, from real container
logs / `nr-cli` status (pin exact strings from the first CI artifacts — **capture,
then assert**; do not hardcode guessed strings):
- **UE sent a Synch-failure / AUTS** (UERANSIM UE log — confirm the exact text
  from artifacts; likely candidates mention `Synch`, `AUTS`, or `SQN`).
- **AMF orchestrated the resync**: log `amf: attempting SQN resynchronization`
  **and** event `Authentication Request sent (Resync)`.
- **Registration completed after resync**: the UE reaches
  `Initial Registration is successful` / `MM-REGISTERED` **following** the resync
  (not the round-1 registration — order matters; gate on the AMF resync line
  appearing before the final success).
- On success print a new annotation, e.g.
  `::notice::T10 SQN RESYNC PASS — UERANSIM forced a Synch failure; QCore recovered
  SQN_MS, re-issued the challenge, and the UE completed registration.`
- Upload the same kind of evidence artifact the existing step does (UE/AMF/gNB
  logs) so the run is auditable.

Keep the run **time-bounded** (the existing loop is ~180s; stay within the job's
30-min budget). If `nr-cli` re-auth proves flaky in CI, document the flake
honestly rather than masking it.

## Secondary deliverable (optional — only if primary is solid)
A DB-backed unit test for `Service.Resync5GAuthVector` (the orchestration the
crypto tests don't cover: SQN advance/persist/re-issue). **Decision to make and
state explicitly** — there is **no sqlite driver in `go.mod`** today (postgres
only; zero-CGO is a project constraint):
- Option A: add a **pure-Go** sqlite test dependency (e.g. `glebarez/sqlite`,
  which needs **no CGO**) and build an in-memory `subscriber.Service` in a
  `_test.go`. Adds a test-scope dep — verify it in `go.mod` and flag it.
- Option B: a postgres-backed integration test behind a build tag, run only in the
  `linux-integration` CI job (which already has postgres).
- Option C: skip it — once the interop scenario passes (tier-3), the orchestration
  is validated end-to-end through the real DB, so a separate unit test is
  lower-value. **This is an acceptable outcome; say so if you choose it.**
Do **not** add a CGO sqlite driver (`mattn/go-sqlite3`) — it breaks the static,
zero-CGO build.

## Rules (non-negotiable)
- **Build/test only in Docker** — there is no Go toolchain on the host:
  `docker run --rm -v "$PWD":/src -w /src -v qcore-gomod:/go/pkg/mod golang:1.23 \
    sh -c "go build ./... && go vet ./... && go test ./..."`
  Use `-race` for the final pass. Frontend (if ever touched): build it
  **separately** from the Go build (the `//go:embed dist` race).
- **Trust rule**: mark nothing ✅ on "code exists". A status flips to shipped only
  when build+vet+test are green **and** you have acceptance evidence. "Passes our
  own test" ≠ "validated against a real external UE". Cite the concrete GitHub run
  ID for any interop claim.
- **No fabrication**: every log string you assert on must be one you actually saw
  in a CI artifact. Verify every symbol/endpoint/env var by reading the file or
  `rg` before referencing it.
- **Git hygiene**: one feature branch `codex/auts-sqn-interop`; **new commits
  only, never amend, never force-push**; do not touch git config. If another agent
  is sharing the working tree, use an isolated `git worktree` for commits.
- **Scope (charter §11 "Not Now")**: 5G only. No carrier-scale/HA, no billing, no
  AI Levels 3–4, no array-based SQN scheme. If a task drifts there, **flag it,
  don't build it**.

## Bring everything up to date — only AFTER green interop evidence
Per `CLAUDE.md` "Keeping project status current" + the documentation cadence, once
the interop run is green (cite the run ID):
1. `README.md` — reflect SQN-resync as interop-validated (with the run ID), not
   just "implemented".
2. `CLAUDE.md` — update the "Current baseline" so the next session has the right
   picture (AUTS/SQN tier-3 for the bundled UERANSIM profile).
3. `docs/audit-v1.0.md` — bump the revision log; reconcile the status table;
   re-verify the build/vet/test claims in the same pass.
4. `docs/next-phases-plan.md` — mark AUTS/SQN done; surface the next item
   (P3.2 CLI/CI hooks per the roadmap order).
5. `docs/ueransim-compat.md` — add the SQN-resync scenario to the compat evidence.
6. Session memory `project_state.md` — match reality.
7. Push the branch, open one PR, ensure **all** checks (incl. `ueransim-interop`)
   are green, and link the run ID in the PR body. Do **not** mark shipped on a red
   or pending check.

## Definition of done (evidence-gated)
- [ ] `go build ./...` + `go vet ./...` + `go test -race ./...` green in Docker
      (paste the tail + real exit code).
- [ ] `ueransim-interop` is green on the branch **and** prints
      `T10 SQN RESYNC PASS` (cite the GitHub run ID). The pre-existing
      `T10 DATA PLANE PASS` still appears (no regression).
- [ ] (If attempted) the secondary unit test passes, or a one-line note explains
      why Option C was chosen.
- [ ] Docs/status/memory updated **with the run ID**, one PR opened, all checks
      green.

## Stop conditions — stop and report, do not paper over
- The crypto tests (`TestResyncAUTSRoundTrip` etc.) start failing → **stop**; the
  vectors are ground truth, something upstream regressed.
- UERANSIM does not emit a Synch failure / AUTS in the desync scenario after a
  genuine attempt → **stop and report** the actual UE logs; the desync mechanism
  or `nr-cli` verb may need adjustment (do not fake a pass).
- The interop job exceeds its time budget or flakes → report the flake honestly
  with artifacts; do not loosen the assertion to force green.
