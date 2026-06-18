# QCore Adoption Plan

> Operationalizes the one metric [docs/vision-2030.md](vision-2030.md) calls "the
> whole game": **external adoption**. The engineering is ~90% of the v1 promise;
> adoption is at **zero external golden-path completions**. This doc is how we move
> that number — and how we keep score honestly. Rule from the vision doc:
> **adoption before monetization.**

_Last updated: 2026-06-18._

## North Star

**First 10 external developers complete the golden path on their own machine** —
clone → core up → see a real signaling trace → understand a real failure from the
Diagnosis screen — and tell us whether they'd reach for it again.

We are not optimizing installs or stars. We are optimizing **"a stranger ran it
cold and it explained a failure."** Everything below serves that.

## 1. Tester engagement checklist (per person)

Run every tester through the same lifecycle so feedback is comparable, not random.

**Before**
- [ ] Confirm their setup: simulator-only, real gNB/UE, or eNB? (routes them to the
      right brief — [try-qcore.md](try-qcore.md) for simulator, [try-qcore-real-ran.md](try-qcore-real-ran.md) for real RAN).
- [ ] Confirm Linux host availability for the data-plane / real-RAN path.
- [ ] Send the brief + the **one ask**: capture TTFC, TTRC, and every moment of friction.
- [ ] Set the expectation explicitly: real-RAN is a "prove it / find the gaps" round,
      not "it works." Lowers disappointment, raises signal.

**During**
- [ ] Be reachable for the first 30 min (most drop-off is in bring-up).
- [ ] Do **not** coach them through friction in real time — let them get stuck and
      note where. The stuck point is the deliverable.

**After**
- [ ] Collect the 4 bullets (or the evidence-harness bundle).
- [ ] Log them in the tracker (§3) within 24h while it's fresh.
- [ ] Turn every failure they hit into a fix **or** a diagnostic-catalog rule, and
      tell them which. This is the loop that compounds — and the reason they'll come back.
- [ ] Ask the one question that predicts everything: *"Would you use this next time
      you needed a core to test against?"*

## 2. Where to find users (beyond friendlies)

Friendlies de-risk the brief; they don't validate pull (they're too forgiving).
After 2–3 friendlies confirm the path isn't broken, go to **cold-ish strangers who
actually feel the pain** — RAN/device developers who today fight open5GS/free5GC:

- **Open-source RAN communities** — UERANSIM, srsRAN, OpenAirInterface, free5GC and
  open5GS issue trackers/discussions. People filing "why won't my UE attach" issues
  are your exact persona, mid-pain. Offer QCore's Diagnosis as a second opinion on
  *their* failing trace — lead with the value, not the project.
- **r/telecom, r/5G, telecom-dev Discords/Slacks**, and the 3GPP-adjacent corners of
  Hacker News. A "core that explains why your attach failed" demo post is on-thesis.
- **University / research labs** doing RAN, O-RAN, or device testing — they have real
  gNBs/UEs and chronic tooling pain, and they publish. (See researcher note below.)
- **Conference / meetup hallway**: O-RAN Alliance events, local 5G meetups, academic
  workshops. One real-gNB tester from here is worth 20 GitHub stars.
- **Your network's second degree**: ask each friendly for *one* intro to someone who
  fights this weekly. Warm-but-honest beats cold.

**Pitch in one line** (use everywhere): *"A 5G/4G core you run with one command that
tells you in plain English why an attach failed — and how to fix it on both sides."*

### Should you make a researcher checklist? Yes.

Researchers are a high-leverage early segment: they have real RAN, tolerate rough
edges, reproduce carefully, and **publish** (citations = durable credibility). Engage
them deliberately:

- [ ] Lead with the **diagnostic flywheel + reproducibility** (scenario authoring +
      CI exit-code contract), not feature count — that's what a paper needs.
- [ ] Offer co-authorship-grade support: help capture clean evidence bundles.
- [ ] Ask for a **citable artifact** in return (a repo reference, a workshop note).
- [ ] Feed their hard failures straight into the catalog — a researcher's edge case
      is the best possible catalog rule.
- [ ] Keep the §11 line: a single lab's success is **not** a conformance claim.

## 3. Tracking progress & adoption goals

### The funnel (the only metrics that matter now)

| Stage | Definition | How measured |
|---|---|---|
| Reached | Got the brief | manual count |
| Cloned | Ran `git clone` + a `make up*` | self-report / harness |
| **First connection** | Dashboard showed connected | **TTFC** (the headline) |
| **First diagnosis** | Understood a real failure from the screen | **TTRC** |
| Returned | Came back unprompted for a second run | the retention signal |
| Advocated | Referred someone or cited us | the growth signal |

### Leading indicators to watch (from vision-2030 §VII)

- **Median TTFC / TTRC across external runs** trending down, not just our internal
  cold-start number (`measurements/latest.json`).
- **Catalog rules added from real external failures** per month (the moat compounding).
- **Return rate** — % of testers who run it a second time without being asked.

### Tracker (fill one row per tester)

| Date | Tester | Setup (sim/gNB/eNB) | Cloned? | TTFC | TTRC | Diagnosis nailed it? | Return? | Top friction | Action taken |
|---|---|---|---|---|---|---|---|---|---|
| _ex_ 2026-06-20 | A. (friendly) | UERANSIM sim | ✅ | 78s | 3.5s | yes | — | — | — |
|  |  |  |  |  |  |  |  |  |  |

### Goals (revisit monthly; keep them small and real)

- **30 days** — 3 friendlies + 2 cold strangers complete the **simulator** golden
  path; ≥3 usable friction reports; ≥2 new catalog rules from real failures.
- **60 days** — first **real-gNB** evidence bundle captured (P4.2 harness); first
  `docs/real-ran-compat.md` entry; 1 tester returns unprompted.
- **90 days** — 10 total external golden-path completions; 1 researcher/lab engaged;
  median external TTFC measured and trending down. _Only after this: revisit the
  §11-safe monetization question — not before._

## Cadence

- **Per tester**: log within 24h (§1).
- **Weekly**: update the tracker + leading indicators; pick the single biggest friction
  point and fix it before recruiting the next batch.
- **Monthly**: reconcile against goals here, refresh the vision-2030 scorecard, and
  re-bump this doc's "Last updated" date.

## Automation now in repo

- `scripts/ci/real-ran-capture.sh` creates a structured evidence bundle for a
  simulator, bundled UERANSIM, or external RAN run.
- `docs/adoption-tracker.csv` is the lightweight CRM/funnel source of truth.
- `make adoption-report` summarizes the tracker into `artifacts/adoption/report.md`
  and `report.json`.
- `.github/ISSUE_TEMPLATE/*` routes external users into evidence-first reports.
- `.github/workflows/adoption-loop.yml` runs the adoption report weekly so the next
  operating question stays visible.

See [docs/digital-growth-loop.md](digital-growth-loop.md) for the agent cadence.
