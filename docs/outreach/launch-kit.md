# QCore Launch Kit

> Reusable, public-facing copy for the first adoption push. Edit freely before
> posting. Pairs with [docs/adoption-plan.md](../adoption-plan.md) (who to reach) and
> [docs/digital-growth-loop.md](../digital-growth-loop.md) (the compounding loop).
> **Don't post the X thread until PR #52 is merged** — tweet 3 needs the redesigned
> hero + Diagnosis screenshots.

_Last updated: 2026-06-18._

## The one-liner (use everywhere — README, bio, post hooks)

> **A 5G/4G core you run with one command that tells you in plain English why an
> attach failed — and how to fix it on both sides.**

## X / Twitter thread

**1/**
> Every RAN/device dev knows the pain: your UE won't attach, and you're staring at
> 200 lines of open5GS logs trying to guess why.
>
> I built QCore to kill that — a 5G/4G core you run with one command that explains
> the failure in plain English. 🧵

**2/**  ·  _attach: `assets/waiting-hero.png`_
> One command → a running 5G core + live signaling trace in your browser. No YAML
> archaeology, no 12-container compose puzzle. It boots and shows you what's happening.

**3/**  ·  _attach: `assets/failed-hero.png` + `assets/diagnosis.png`_
> The point isn't protocol checkboxes. It's this: when an attach fails, QCore names
> the *cause* and the *fix* — on both the gNB and the core side. Wrong Ki, PLMN
> mismatch, unprovisioned IMSI — it tells you, instead of making you decode it.

**4/**
> Open source (Apache-2.0). I want it in the hands of people who actually fight this:
>
> `git clone https://github.com/umairsuperhero/qcore && cd qcore && make up-5g`
>
> Got a gNB/UE? Point it at QCore and tell me where it breaks 👇
> https://github.com/umairsuperhero/qcore

## Show HN / Reddit (r/telecom, r/5G) post

**Title:** Show: QCore — a 5G/4G core that explains why your attach failed

**Body:**
> One command to a running core + a live signaling trace, and a diagnosis screen that
> names the *cause and the fix* when a UE fails to attach — instead of leaving you to
> decode logs.
>
> Built for RAN/device devs tired of fighting open5GS/free5GC just to get a core to
> test against. Runs on your laptop, Apache-2.0.
>
> ```
> git clone https://github.com/umairsuperhero/qcore && cd qcore
> make up-5g     # 5G core + bundled UERANSIM simulator
> ```
> Open http://localhost:3000, watch it connect, then break it on purpose and see if
> the diagnosis actually helps (15-min walkthrough: `docs/try-qcore.md`).
>
> Honest scope: end-to-end validated against the bundled UERANSIM profile on Linux.
> Real gNB/UE/eNB is new ground — if you've got hardware, that's exactly the feedback
> I need (`docs/try-qcore-real-ran.md`). Bring it and tell me where it breaks.

## Issue-reply / community-comment template (lead with value, not the project)

> If it helps — there's an open-source core called QCore that's built to *explain*
> attach failures (it'll tell you if it's a Ki/PLMN/IMSI/SUCI mismatch and what to
> change on each side). Might be a fast second opinion on this trace:
> https://github.com/umairsuperhero/qcore — curious if it nails your case or misses it.

## Screenshots → which post slot

| Asset | State shown | Capture (live preview) | Used in |
|---|---|---|---|
| `assets/waiting-hero.png` | "Waiting for gNB…" — open amber latch ring | `make up-5g`, open dashboard (or `npm --prefix pkg/dashboard/web run dev`) | X tweet 2 |
| `assets/failed-hero.png` | "gNB rejected · Authentication failure" — red latch + X, both fix CTAs | inject `wrong_ki`, view gNB Connection tab | X tweet 3, Show HN |
| `assets/diagnosis.png` | "QCore diagnosis" — CONNECTION BLOCKED → cause → RAN-sent vs QCore-expects | inject `wrong_ki`, open Diagnosis tab | X tweet 3 |

> The three PNGs aren't committed yet. Drop them into `docs/outreach/assets/` before
> posting — export from the running dashboard, or ask Codex to add a small headless
> capture target (`make screenshots`) so they regenerate on every redesign and never
> go stale.

## Posting checklist

- [ ] PR #52 merged (hero + Diagnosis live on `main`).
- [ ] Three screenshots exported into `assets/`.
- [ ] README hook = the one-liner above; Quick Start = `make up-5g`.
- [ ] Send the friendly email first (private), then post publicly 1–2 days later once
      2–3 friendlies confirm the path isn't broken.
- [ ] Log every responder in `docs/adoption-tracker.csv`.
