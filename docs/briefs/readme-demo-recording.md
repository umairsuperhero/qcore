# Codex brief — README demo recording (the 30-second proof)

> A stranger decides in ~30 seconds whether QCore is worth 15 minutes. The README
> currently asks for that trust with text alone. This brief produces the missing
> artifact: a short, **real** (never staged/fabricated) recording of the golden
> path — boot → gNB connects → inject `wrong_ki` → Diagnosis names the cause and
> the fix — embedded at the top of the README.

## Read first (every session)
- `CLAUDE.md` — evidence rules, verification protocol, dist-embed order (frontend
  build **before** Go build), no Go on host (use `golang:1.23` Docker).
- `docs/outreach-kit.md` — claim-safety say/don't-say list; the recording's captions
  must obey it.
- `docs/design-system.md` — the dashboard aesthetic the capture should show off.

## Goal
One looping demo, ≤90 seconds, at the top of `README.md`, showing the **real**
product doing the one thing that sells it: explaining a failure.

Storyboard (four beats, tight):
1. `make up-5g` in a terminal (can be time-compressed/cut — cold build takes
   minutes; label any cut honestly, e.g. "~60s later").
2. Dashboard hero flips **Waiting → Connected** (the latch closes) as the bundled
   UERANSIM gNB attaches.
3. Inject the failure: `curl -X POST localhost:3000/api/simulator/inject/wrong_ki
   -H "Content-Type: application/json" -d '{"mode":"5g"}'` → hero flips to
   **gNB rejected · Authentication failure**.
4. The Diagnosis screen: cause + fix on both sides. Hold ~4s. End card: repo URL +
   "make up-5g".

## Implementation notes
- **Capture must be a real run** — Linux host or the GitHub Actions ueransim-interop
  environment (which already reaches all four beats). Options, in order of
  preference:
  1. **Playwright-driven browser capture** against the live dashboard (screenshots →
     ffmpeg, or video API), driven by the same states the interop workflow proves.
     Terminal beats via `vhs` (charmbracelet) or `asciinema` + `agg`.
  2. If a single continuous recording is impractical, compose the four beats from
     separately captured **real** segments — never mock data into the UI for the
     camera.
- **Output**: an `.mp4` (README `<video>` via GitHub asset link) *and/or* an
  optimized `.gif` (<10 MB hard cap for README embedding; prefer mp4 + gif
  fallback). Store under `docs/outreach/assets/`; if size is a problem, attach the
  mp4 to a GitHub Release and hot-link it instead of committing it.
- **Repeatability**: land the capture script as `make demo-recording` (or
  `scripts/demo/record.sh`) so the recording regenerates after UI changes instead
  of going stale. A stale demo that no longer matches the product is worse than none.
- README placement: directly under the title/one-liner, above the status table.

## Out of scope
No marketing site, no voiceover, no music, no fabricated terminal output or spliced
fake logs. No re-recording of 4G (5G SA is the headline). Don't block on pixel
perfection — a real 80-second capture this week beats a cinematic one next month.

## Acceptance (trust rule)
- The recording is generated from a **real run** (say where: local Linux, or cite the
  CI run ID if captured in Actions).
- All four beats visible; captions pass the outreach-kit claim-safety list.
- README renders it correctly on github.com (check on a cold browser profile).
- `make demo-recording` (or the script) reproduces it end-to-end.
- Repo hygiene: gif/mp4 within size caps; `make verify-fast` still green; if any
  dashboard/Go files were touched, the standard verification protocol applies.

## Guardrails (non-negotiable)
Isolated git worktree; new commits only (never amend/force-push); no secrets in
captures (demo subscriber TS 35.208 Test Set 1 is fine — it's public spec data);
open a PR and let CI prove it.
