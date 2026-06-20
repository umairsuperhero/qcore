# QCore Growth Automation Plan

> How the adoption loop runs itself — and where a human stays in control. Companion
> to [digital-growth-loop.md](digital-growth-loop.md) (the why) and
> [adoption-plan.md](adoption-plan.md) (the funnel/goals).

_Last updated: 2026-06-19._

## Principle: automate the machine, gate the megaphone

Everything **internal** (reports, evidence capture, tracking, drafting) is automated.
Everything that goes **public** (X posts, announcements) is auto-*drafted* but
published only on an explicit human click. The whole product is credibility; an
unreviewed bot post is pure downside. So the boundary between "drafted" and
"published" is always a person.

## The four tiers

| Tier | What | Trigger | Risk | Status |
|---|---|---|---|---|
| 0 | `make adoption-report` → funnel report artifact | `adoption-loop.yml`, Mon 15:00 UTC | none | ✅ live |
| 0 | **Intake:** tester issue → upsert row in `adoption-tracker.csv` | `adoption-intake.yml`, on labeled issue | none (own-repo) | ✅ live |
| 1 | Weekly claim-safe draft → **review-queue issue** | `social-draft.yml`, Fri 15:00 UTC | none (drafts only) | ✅ live |
| 2 | LLM judgment: polish the post + triage tester friction into fix/rule/caveat | optional Claude routine (see below) | low | ⚙️ opt-in |
| 3 | Publish one approved post to X | `post-to-x.yml`, **manual dispatch only** | public | ✅ scaffolded, needs secrets |

## Tier 0 — auto-intake (no setup)

`adoption-intake.yml` fires when a tester files a `real-ran` / `attach-failure` /
`diagnosis-gap` issue. `scripts/adoption/issue_to_tracker.py` parses the issue-form
body, extracts TTFC/TTRC/setup/friction, and **upserts** one row in
`docs/adoption-tracker.csv` keyed by the issue URL (edits update in place). It records
only product evidence + the public issue link — never private contact data — and
sanitizes all free-text against CSV/formula injection. The tester gets an automatic
thank-you comment. This is what makes the funnel self-populating.

## Tier 1 — auto-draft to a queue (no setup)

`social-draft.yml` runs `scripts/adoption/draft-weekly-update.py`, which reads
`docs/adoption-tracker.csv` and the week's git history (new catalog rules, new compat
findings) and emits a Markdown draft: a funnel snapshot, what shipped, and a
**claim-safe X post stub** with the say/don't-say checklist from
[outreach-kit.md](outreach-kit.md). The draft is posted as a comment on a single
issue labelled `social-queue` ("📣 Social + update queue"). Nothing leaves the repo.

Preview locally any time: `make adoption-draft`.

## Tier 2 — the judgment layer (opt-in)

Templating can't write the *good* post or decide which friction matters most. That's
an LLM job. Run it as a **weekly Claude Code routine** (so it only spends plan tokens
when you choose), with a prompt like:

> Read `docs/adoption-tracker.csv` and the last 7 days of git history. Triage each
> tester's friction into exactly one of fix / catalog-rule / caveat and name the
> single highest-leverage fix for Codex. Then write one claim-safe X post per
> `docs/outreach-kit.md` (obey the say/don't-say list). Post both as a comment on the
> `social-queue` issue. **Do not post anything to X.**

Left opt-in deliberately: a recurring cloud agent costs plan tokens every run, so the
operator decides when to turn it on. Tier 1 is useful on its own without it.

## Tier 3 — publish to X (manual, needs your credentials)

`post-to-x.yml` is **`workflow_dispatch` only** — never scheduled. You paste the
already-approved text and click Run; `scripts/adoption/post_to_x.py` posts it via the
X API v2 (OAuth 1.0a). Without credentials it exits with a clear "not configured"
message instead of posting.

To enable (one time):
1. Create an X developer app with **read+write** permission; generate the API
   key/secret and an access token/secret.
2. Add four repo secrets (Settings → Secrets and variables → Actions):
   `X_API_KEY`, `X_API_SECRET`, `X_ACCESS_TOKEN`, `X_ACCESS_SECRET`.
3. (Optional 2nd gate) In Settings → Environments, add a required reviewer to the
   `social-publish` environment so a dispatch waits for an approval.

> This path is **unverified by us** — it needs live X credentials we don't hold. The
> first real dispatch is the test; check the post before trusting the workflow.

## The weekly loop, wired

| Day | Automated | Human |
|---|---|---|
| Mon | `adoption-loop` builds the report | skim the funnel |
| Tue | (Tier 2) triage proposes the one fix | hand it to Codex |
| Wed–Thu | Codex ships fix + catalog rule | review PR |
| Fri | `social-draft` drops a claim-safe draft in the queue | edit + approve |
| any | — | run `post-to-x` on the approved text |

The flywheel turns itself; the only human touch is the publish button — exactly where
it belongs.
