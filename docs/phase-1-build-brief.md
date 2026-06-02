# QCore — Phase 1 (REAL): Master Build Brief

## 0. Mission
QCore is a **dev/test environment for cellular networks** — the product is the *experience*, not protocol feature-count. The one goal of this phase: **everything the dashboard shows must run on real data from real network functions.** Today the flagship live-trace view runs on hardcoded mocks; closing that is the credibility gate. North star: shrink the time from *"my gNB registration failed"* → *"I know why and how to fix it."*

The Go backend (4G EPC end-to-end; 5G SA control + user plane) and the event pipeline (`events.Emitter` → collector → SSE → dashboard) are already built and tested. **You are connecting wires, not building new systems.**

## 1. Hard guardrails (read first)
- **No Go toolchain on the host.** Every Go build/vet/test runs in Docker:
  ```bash
  docker run --rm -v "$PWD":/src -w /src -v qcore-gomod:/go/pkg/mod golang:1.23 \
    sh -c "go build ./... && go vet ./... && go test -race ./..."
  ```
- **Frontend** (`pkg/dashboard/web`): `npx tsc --noEmit` must pass and the dashboard build must produce working output.
- **Trust rule:** a task is done only when build + vet + tests pass. "Code exists" ≠ "shipped." Never mark a checkbox green on code alone. Never claim external interop ("works with UERANSIM/a real gNB") until a real external tool has actually connected — that's a separate, higher bar than "our own E2E test passes."
- **Do not touch `pkg/ai`** — the diagnostic engine (catalog of 13 typed rules + optional escalation) is built; the offline-SLM provider is already done on `plan/b2-embedded-slm` (PR #26).
- **Git:** one branch per lane (mapped below — *use the existing branch if it exists*). New commits only (never amend), never force-push, never commit secrets, never skip hooks. **Don't open PRs or push unless asked.**
- If a request conflicts with `docs/experience-charter.md` or `CLAUDE.md`, **stop and flag it.**

## 2. Current state — CONTINUE, don't restart
- `feat/vite-migration` — **Vite migration is ~90% done but uncommitted** (vite.config.ts added, index.html moved to web root, esbuild.config.mjs removed). First action on this lane: verify it builds + type-checks, then commit. Don't redo it.
- `fix/ci-5g-builds`, `fix/dark-theme-consistency`, `plan/d-ueransim-validation`, `plan/c2-5g-simulator`, `plan/c3-dashboard-5g-mode`, `plan/e1-ran-reconciliation`, `plan/e2-scenario-authoring`, `plan/e3-ci-hooks` — **already exist.** Check each out and continue from its current state; do not create duplicates.
- `fix/honest-status-docs` — a docs honesty fix is already committed here (incl. the corrected `docs/ueransim-compat.md`). Build the rest of docs reconciliation on top of it.
- **No branch exists yet for the un-mock work** (the #1 priority) — create `feat/real-dashboard`.

## 3. Priority + parallelization map
```
NOW (parallel — disjoint files, run concurrently):
  ├─ LANE 1  Un-mock live trace + dual-hook fix   feat/real-dashboard   ⭐ highest ROI
  ├─ LANE 2  Harden CI                             fix/ci-5g-builds
  ├─ LANE 3  Finish + commit Vite                  feat/vite-migration
  └─ LANE 4  Dark-theme cleanup                    fix/dark-theme-consistency  (cosmetic, lowest)

        ▼ LANE 1 must merge before LANE 5 (UERANSIM is debugged THROUGH the real dashboard)

NEXT:
  └─ LANE 5  UERANSIM interop / T10                plan/d-ueransim-validation  ⭐ 5G credibility gate
                                                    (needs a Linux host / privileged Docker)
THEN:
  ├─ LANE 6  Measure TTFC/TTRC + record in README
  └─ LANE 7  Docs reconciliation                   fix/honest-status-docs (extend)
```
**Conflict rule:** Lanes 1–4 all touch `pkg/dashboard/web` to some degree. Lane 3 (Vite) changes build config — **land and commit it first**, then rebase Lanes 1 & 4 on top, so the web/ build config isn't edited by two agents at once. Lane 2 (`ci.yml` only) is fully isolated and can run anytime.

## 4. Lane briefs

### LANE 1 — Un-mock the live trace + dual-hook fix ⭐ (`feat/real-dashboard`)
- **Files:** `src/App.tsx`, `components/GNBHeroScreen.tsx`, `api/gnbConnection.ts`, `api/traceStream.ts`, `components/JourneyTimeline.tsx`, a new store; delete `components/RANConnectView.tsx`.
- **Steps:**
  1. Add `zustand`; create one connection/stream store.
  2. Lift `useGNBConnection` into it. It's currently called in **both** `App.tsx` (~L20) and `GNBHeroScreen.tsx` (~L29) → two EventSource connections. Collapse to one.
  3. Delete `RANConnectView.tsx` (dead — confirm with a repo-wide search first).
  4. Set `USE_MOCK_STREAM = false` (`traceStream.ts`, L6). The real SSE path already exists in that file but is dormant — activate it.
  5. **Reconcile the real collector event format with what `JourneyTimeline` expects** — budget real time here; the formats may not line up. Map real event types onto the journey steps.
  6. Replace `getMockDiagnostic()` with a call to the real diagnose endpoint (the catalog-first engine already returns real results — do not modify it).
  7. Test against the **4G simulator**: trigger an attach, watch real events animate, force a wrong-Ki failure, confirm a real diagnostic renders.
- **Acceptance:** mocks off; a real 4G attach animates from live SSE; exactly **one** EventSource; a forced failure yields a real (not hardcoded) diagnostic; `tsc --noEmit` clean.

### LANE 2 — Harden CI (`fix/ci-5g-builds`)
- **File:** `.github/workflows/ci.yml` only.
- **Steps:** in the `build` job (today it compiles only hss/mme/spgw), add `go build` for **`cmd/amf cmd/ausf cmd/udm cmd/udr cmd/nrf cmd/smf cmd/upf`**; add a **`go vet ./...`** step.
- **Do NOT add** `go test -race` or "5G package tests" — `go test -race ./...` (L36) **already** runs every package. Those are already covered.
- **Acceptance:** a PR that breaks any NF's `cmd/` build or trips `go vet` fails CI.

### LANE 3 — Finish + commit Vite (`feat/vite-migration`)
- **Steps:** verify the in-progress migration builds (`vite build`) and the Go dashboard server serves the output; `tsc --noEmit` clean; HMR works in dev; then commit. Keep React/TS source behavior unchanged.
- **Acceptance:** dashboard builds via Vite and renders identically to the esbuild output.

### LANE 4 — Dark-theme cleanup (`fix/dark-theme-consistency`)
- **Files:** `components/HealthView.tsx`, `components/SubscribersView.tsx` (grep the rest of `web/src` for stragglers).
- **Steps:** replace light-mode Tailwind classes (`bg-red-50`, `bg-emerald-50`, `bg-amber-50`, any `*-50`/`*-100`) with dark-theme equivalents. *(Cosmetic — lowest priority.)*

### LANE 5 — UERANSIM interop / T10 ⭐ (`plan/d-ueransim-validation`) — blocked on LANE 1
- **Environment:** Linux containers with kernel SCTP + `/dev/net/tun` (Docker Desktop's Linux VM works with a privileged container). Native SCTP is built (`pkg/sctp/sctp_linux.go` — raw syscalls, no CGO, no pion). The `ueransim` service is already in compose under the `5g` profile.
- **Steps:**
  1. Run the AMF in SCTP mode: `QCORE_AMF_SCTP_MODE=sctp` (AMF NGAP is on **38412**; it defaults to TCP).
  2. `QCORE_AMF_SCTP_MODE=sctp make up-5g` (brings up the 5G core **and** UERANSIM).
  3. Configure UERANSIM for QCore's PLMN/TAC/S-NSSAI.
  4. **NG Setup** → verify NGSetupResponse. **Registration** → debug through the now-real dashboard. **PDU Session** → verify the GTP-U tunnel.
  5. Expect codec gaps (UERANSIM sends optional IEs internal tests don't exercise). Fix in `pkg/ngap`/`pkg/amf`. **Log every fix in a new `docs/ueransim-interop-log.md`.**
- **Acceptance:** a real UERANSIM gNB/UE registers and establishes a PDU session through QCore over native SCTP, shown live in the dashboard. Full Go suite still green under `-race`. *Only then* may `docs/ueransim-compat.md` claim verification.

### LANE 6 — Measure TTFC/TTRC (after T10)
- **Steps:** write a measurement script; on a fresh clone, time `git clone` → first successful attach (TTFC) and "it failed" → "I know why" via the dashboard (TTRC); record both in the README. If TTFC > 5 min or TTRC > 30s, fix the flow until they meet the charter targets.

### LANE 7 — Docs reconciliation (extend `fix/honest-status-docs`)
- **Steps:** mark `PRD.md` superseded by the charter; fix `ARCHITECTURE.md` (React + Vite, not Next.js); update `wiki.md` NF binary states; add `CONTRIBUTING.md`; create `docs/3gpp-tracking.md`. Finalize `docs/ueransim-compat.md` with the real LANE 5 results.

## 5. Phase-1 Definition of Done
1. Dashboard live trace + diagnostics run on **real data** (no mocks); one EventSource; dead code removed.
2. CI builds **all 11 binaries** + runs `go vet` + `go test -race ./...`; every package green.
3. **UERANSIM** registers a UE and establishes a PDU session through QCore, shown live.
4. Measured **TTFC/TTRC** in the README, meeting charter targets.
5. Docs reconciled to reality; PRD marked superseded.
