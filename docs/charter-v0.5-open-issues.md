# Charter v0.5 — Open Issues Memo

> **Status:** working memo · raised 2026-05-24 during Phase B kickoff review on Opus 4.7
> **Owner:** Product
> **Disposition:** track these through Phase B; revisit at the next charter revision (target: end of Phase B / start of Phase C)
>
> This memo records strategic concerns surfaced during a deeper review of the
> charter (v0.4) but **does not** change any current decision. The charter is
> sound enough to proceed; these are sharpenings to address when there's
> accumulated build experience to draw on, rather than guessing now.

---

## Issue 1 — §9.3 has an unreckoned tension: air-gapped + novel-diagnosis

**The contradiction:**
- §9.3 says the wedge user is in security-restricted, often air-gapped labs.
- §9.3 also says the embedded SLM handles "the bounded high-frequency 80%" (known patterns, pattern-matching, cause decoding).
- §9.3 routes "novel multi-symptom diagnosis" to the BYOK frontier model.
- Air-gapped labs cannot reach a frontier model.
- Therefore: **for the charter's stated wedge user, the AI does not deliver Level 2 (Diagnose) — only Level 1 (Explain) plus pattern matching against the curated catalog.** That is in tension with the v1 flagship being Diagnose (§9.2 / D4).

**Three possible resolutions, in order of how plausible they seem today:**

1. **Bet on the pattern catalog being deeper than it sounds.** Cellular failures concentrate in a few dozen recurring patterns. A well-curated catalog plus a competent SLM may cover ~95% of real-world failures, leaving "novel multi-symptom" as a true edge case rather than the headline use case. This is empirically defensible but requires the structured diagnostic layer to be far more substantial than the charter currently implies. Implication: more work on §9.4, less reliance on §9.3's frontier-tier story.

2. **Accept the constraint explicitly.** Re-write §9.3 to say: in air-gapped labs, Diagnose is pattern-matching; novel diagnosis requires the connected tier. This is honest but reduces the v1 ambition.

3. **Commit to a heavier embedded model.** Ship a 30B+ model that can do real multi-step reasoning offline. Container balloons, hardware requirements rise, zero-config gets harder.

**Decision needed at v0.5:** which of these we're betting on. Today the charter implicitly bets on (3) without saying so, which is the most expensive and least proven path. My instinct is (1) is closest to the real product, but it should be a conscious call after Phase C has surfaced what the catalog actually needs to contain.

**Action this phase:** none. Track. Resolve when Phase C work begins to give us empirical evidence.

---

## Issue 2 — §9.3 + §10 are missing a middle commercial tier

The AI tiering in §9.3 is binary: local SLM (free, offline) or BYOK frontier (user supplies the key). The commercial model in §10 mentions "team features, shared test environments, hosted/managed deployments" but does not connect to the AI strategy.

The natural middle tier — **QCore-hosted frontier-model diagnosis, billed by QCore** — is missing from both sections. It is the right monetization fit:

- Solves the BYOK friction (most engineers do not want to manage an API key for a tool they are evaluating).
- Connects free→paid via a clear value prop (better diagnosis on hard cases).
- Does not require team features as the pricing fulcrum, which §10 currently relies on and which is shaky for the wedge persona — most RAN/device developers work either solo or inside vendor tooling shops where team features are unusable internally.

**Proposed shape for v0.5:** three AI tiers — free-local-SLM, paid-hosted-frontier (QCore-operated), BYOK-frontier (user-operated).

**Action this phase:** none. Track. The hosted tier is a Next-stage concern per §11.

---

## Issue 3 — Sharpen the primary persona in §5

"RAN/device developer" is ambiguous between two very different people:

- An engineer at Qualcomm/MediaTek/Ericsson/Nokia, who already has sophisticated internal tooling and may dismiss QCore as redundant.
- An engineer at an independent IoT module vendor, a small device maker, or a private-network builder, who lacks internal tooling and has acute pain.

The product we'd build for each is different. The charter's emphasis on zero-config, magical first-run, and self-serve adoption fits the second persona far better than the first. My read is that the charter implicitly targets the second, but it does not say so explicitly — and that ambiguity will eventually pull design and roadmap decisions in conflicting directions.

**Action this phase:** none. Track. Make the call at v0.5.

---

## Issue 4 — Promote "not an RF/PHY emulator" to a §12 anti-goal

D10 in the Decision Log commits to the simulator being a control-plane tool, not an RF/PHY emulator. This is correct and load-bearing. But anti-goals (§12) are louder than Decision Log entries — they're the things we point to when someone proposes drift.

The pull to add RF fidelity for "realism" will be persistent (especially from contributors familiar with srsRAN-style simulation). Pre-empt it.

**Proposed addition to §12:** "**Not** an RF or physical-layer emulator. The built-in simulator is a control-plane tool; RF fidelity is what real SDR hardware is for."

**Action this phase:** none. Apply at v0.5.

---

## Issue 5 — Add an Open Question (§14) about parallel-track tempo

Charter D12 commits to running the 5G SA Track in parallel with the experience track. With a one-person engineering team (Claude Code), two tracks running in genuine parallel is not realistic — one will starve.

Honest answer: this is probably a 4G-first, 5G-when-experience-stabilizes build, with re-prioritization based on early adoption signals. The charter should say so rather than imply two tracks proceed at equal pace.

**Proposed addition to §14:** "Parallel-track tempo. D12 commits to running the 5G SA Track in parallel with the experience track. Engineering capacity in practice may force serialization. Accept this and re-prioritize based on adoption signals from the 4G experience-layer wedge."

**Action this phase:** none. Apply at v0.5.

---

## Issue 6 — Phase A (event model) deserves charter-level commitment in §9

The structured event substrate (now built — Phase A) is the thing that makes everything downstream possible. It currently lives in the build brief (CLAUDE.md) and the Phase A plan (`docs/phase-a-event-model.md`) but not in the charter.

§9.4 says "the platform's own ground-truth telemetry" is one of the two reliability sources. §9.5 says "design that telemetry and event model from day one to be machine-readable and model-friendly." Both reference the event model implicitly; neither names it as a load-bearing platform commitment.

**Proposed addition to §9:** a new sub-section (§9.6) titled "The event substrate" describing the structured event model as a first-class architectural commitment alongside the AI strategy. This recognizes that the substrate is not a build-order concession — it's an enduring part of what QCore is.

**Action this phase:** none. Apply at v0.5.

---

## Summary

| # | Issue | Severity | Action |
|---|-------|----------|--------|
| 1 | §9.3 air-gapped + novel diagnosis tension | High — affects what "Level 2 Diagnose" actually delivers in v1 | Resolve at v0.5, after Phase C surfaces empirical catalog needs |
| 2 | Missing hosted middle tier | Medium — affects commercial model | Resolve at v0.5 |
| 3 | Persona ambiguity in §5 | Medium — affects design decisions | Resolve at v0.5 |
| 4 | Anti-goal omission (RF/PHY) | Low — clarification, not change | Apply at v0.5 |
| 5 | Parallel-track tempo realism | Low — honesty about capacity | Apply at v0.5 |
| 6 | Promote event substrate to §9 | Low — recognizes what already exists | Apply at v0.5 |

**None of these block Phase B.** The charter is sound. These are the things to fix when there's experience to draw on, not guesses to make.
