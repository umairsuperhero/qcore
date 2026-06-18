# QCore Digital Growth Loop

> Operating principle: no traditional B2B selling until external pull is visible.
> QCore grows by turning every failed attach into a public, reusable diagnostic
> improvement.

_Last updated: 2026-06-18._

## The Loop

1. A developer sees a demo, issue reply, release note, or compatibility finding.
2. They run QCore locally.
3. They either connect or hit a failure with a Diagnosis result.
4. `scripts/ci/real-ran-capture.sh` captures the evidence bundle.
5. Agents triage the bundle into one of three outcomes:
   - product fix
   - diagnostic catalog rule
   - documentation/compatibility caveat
6. The repo ships the improvement and records it in a public finding.
7. The next developer benefits from that failure.

## Agent Roles

| Agent | Job |
|---|---|
| Codex | Implement fixes, harnesses, tests, CI, PRs |
| Claude Code | Reconcile docs, prompts, release notes, status truth |
| Antigravity | Fast parallel audits, UX checks, compatibility-gap sweeps |
| Local model | Cheap evidence-bundle summarization and issue clustering |

## Weekly Cadence

| Day | Automation / Agent Action | Output |
|---|---|---|
| Monday | Run adoption report and inspect new issues | `artifacts/adoption/report.md` |
| Tuesday | Codex fixes the highest-friction blocker | PR |
| Wednesday | Antigravity audits UX/docs/compatibility gaps | audit artifact |
| Thursday | Claude drafts release note and status reconciliation | docs PR |
| Friday | Publish one compatibility or failure-analysis update | public proof |

## Monetization Timing

Prepare monetization surfaces now, but do not let them bend the product:

- GitHub Sponsors: support/early-access tiers.
- Stripe Payment Link: paid diagnostic review of an evidence bundle.
- Later: GitHub Marketplace Action for scenario checks in user CI.

Do not build accounts, hosted teams, SSO, or enterprise dashboards until at
least ten external golden-path completions and repeated return usage exist.

## Success Metrics

- External golden-path completions.
- Median external TTFC and TTRC.
- Diagnostic catalog rules created from real external failures.
- Return rate.
- Public references from researchers/labs/users.
