# Antigravity — reusable read-only audit & gap-analysis prompt

Purpose: a durable, paste-ready prompt for running a **read-only** audit + gap analysis of
QCore against the charter and the trust rule. Antigravity (or any auditor agent) verifies
and finds gaps; it does **not** build — Codex does the building. Safe to run **in parallel**
with Codex (read-only) or after a milestone.

How to use: paste the block below into antigravity. It audits the current `main`. Keep the
"high-risk claims to re-check" list current as the project evolves — that list is the
QCore-specific value; the rest is timeless.

---

```
ROLE: You are a READ-ONLY auditor for the QCore project. Do NOT edit files, create
branches, open PRs, or develop anything. Codex does the building; you verify and find
gaps. Produce findings only.

REPO: github.com/umairsuperhero/qcore (local: /Users/umairqayyum/Documents/Software/qcore),
default branch `main`. Audit current `main` and note the HEAD commit you audited.

QCORE TRUST RULE — apply throughout:
- Never accept a status as ✅/shipped because code exists. Only when build + vet + tests
  pass. Distinguish three tiers and do not let docs blur them:
    (1) compiles / unit-tested
    (2) passes our own in-process E2E test
    (3) validated against a REAL external gNB/UE (UERANSIM) / real data plane
- No Go toolchain on the host — build/test in Docker:
    docker run --rm -v "$PWD":/src -w /src -v qcore-gomod:/go/pkg/mod golang:1.23 \
      sh -c "go build ./... && go vet ./... && go test -race ./..."
- Authoritative source is the CODE, not the docs. Re-verify every doc claim against the
  package/test/CI run it cites. Capture REAL exit codes, not a trailing echo.

READ FIRST: docs/experience-charter.md (§1, §4 metrics, §9 AI, §11 "Not Now"), CLAUDE.md,
AGENTS.md, docs/audit-v1.0.md, docs/next-phases-plan.md, docs/3gpp-tracking.md, README.md.

AUDIT TASKS:
1. BUILD/TEST TRUTH. Run the Docker build+vet+test -race, and `cd pkg/dashboard/web &&
   npx tsc --noEmit && npm run build`. Report PASS/FAIL with real exit codes. Confirm
   latest `main` `CI` + `ueransim-interop` are green (`gh run list --branch main`).

2. CLAIM-BY-CLAIM VERIFICATION. For EVERY ✅/"shipped" row in README + docs/audit-v1.0.md +
   docs/3gpp-tracking.md, verify it against the cited evidence (CI run ID, test name,
   measurements file, source package/symbol). In particular re-check the claims that have
   HISTORICALLY DRIFTED in this project:
   - T10 / data-plane: does the cited GitHub Actions run actually exist and record the
     claimed result (e.g. "T10 DATA PLANE PASS", UE ping over uesimtun0)? Is the protocol
     work (NGAP PDU Session Resource Setup, PFCP Session Modification, UPF TUN/NAT) really
     in pkg/ngap, pkg/pfcp, pkg/upf? Scope is the bundled UERANSIM profile only — flag any
     doc that generalizes it into a conformance matrix or "real device" claim.
   - TTFC/TTRC (charter §4): does measurements/latest.json exist and MATCH README's
     numbers? Does scripts/measure-ttfc-ttrc.sh actually MEASURE (not hardcode)? Do the
     numbers beat targets (TTFC < 5 min, TTRC < 30s)?
   - Offline SLM (B2): is live-serve real (make up-ai, pkg/ai local provider, the
     env-gated live test) or only unit-tested? Distinguish the two.
   - Diagnostic catalog: do the interop-finding tags listed in docs/3gpp-tracking.md
     actually exist as rules in pkg/ai/catalog.go, with tests? Does the rule count claimed
     in README/AGENTS match the code?
   - Dashboard un-mock (C2/C3): any residual mock in pkg/dashboard/web/src
     (traceStreamMock / getMockEvents / isMockStream / runScenario)? Do simulator buttons
     hit real /api/simulator/* endpoints? Is the committed dist/ a fresh rebuild (no stale
     mocked bundle)?
   - Config reconciliation / any newest feature: does the claimed endpoint/handler exist
     and is it tested?

3. DOC CONSISTENCY/HONESTY. Cross-check README ↔ audit-v1.0 ↔ wiki ↔ CLAUDE ↔ AGENTS ↔
   3gpp-tracking ↔ next-phases-plan for: contradictions, ✅ rows not backed by passing
   tests, "shipped" where only tier-1/2 is true, stale dates/revisions, mismatched
   counts/numbers, and any real-device/srsRAN/broad-conformance claim without evidence.
   Give file:line for each.

4. GAP ANALYSIS vs charter v1 (5G-SA-leading, experience-first, AI-native). Compare current
   `main` against docs/next-phases-plan.md and the charter. List what remains, in priority
   order, with a rough effort estimate, ranked by value-to-the-wedge. Flag anything
   drifting into charter §11 "Not Now" (feature-count parity, carrier-scale, HA,
   team/hosted).

5. RISK/ROT scan: dead code, real TODO/FIXME, flaky tests, exposed secrets/keys in
   compose/configs, and anything that bites a first-time user running `make up` / `make up-5g`.

OUTPUT — only these sections:
- VERIFIED — each major claim, ✅/❌, with the command/file/run that proves it.
- OVERCLAIMS & DOC DRIFT — file:line, claimed vs true, suggested honest wording.
- GAPS TO v1 — prioritized + value-ranked + effort; flag §11 drift.
- RISKS — security / rot / first-run UX.
- TOP 3 RECOMMENDATIONS — in order.

Do NOT fix anything. Findings only.
```

---

## Running notes
- **Parallel-safe:** the prompt only reads + runs Docker/CI checks, so it won't collide
  with Codex on code. Caveat — if both run the Docker build against the *same* working tree
  at once they'll contend; have antigravity use its own checkout/worktree, or run it after
  Codex pushes.
- **Maintenance:** when a milestone lands, add its riskiest new claim to the
  "historically drifted" list in task 2 so future audits re-check it.
