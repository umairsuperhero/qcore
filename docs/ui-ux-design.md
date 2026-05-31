# QCore — UI/UX Design Language

> **Status:** v0.1 · **Owner:** Product
> **Companion to** `experience-charter.md` (§3 North Star, §6 Experience Principles,
> §7 Golden Path, §8 Experience Pillars). The charter says *what experience QCore
> must deliver*; this document says *what that experience looks and feels like* and
> *what we build it with*. When this conflicts with the charter, the charter wins.

For QCore, **UX is the product** (charter §3). This document exists so that design
intent is not lost between sessions or between the humans and agents building it.

---

## 1. The design north star: Apple meets Tesla

Two reference sensibilities, deliberately chosen:

**Apple** — restraint and clarity.
- Every element on screen has a reason to exist. If you cannot state the reason, remove it.
- Typography does the heavy lifting: generous size, clear hierarchy, no decorative chrome.
- Space (dark or light) is not emptiness — it is emphasis.
- Feedback is immediate and physical. Buttons press. States transition. The UI responds.

**Tesla** — the screen in service of the data.
- **Dark by default.** The interface recedes; the data leads.
- The **status is the hero** — not navigation, not settings. *What is happening right now?*
- **Data feels alive.** Numbers count up smoothly, connections feel physical, state changes animate rather than snap.
- One clear action available at any moment. No decision paralysis.

The test for any screen: *could this go on a Tesla dashboard or in an Apple keynote
without embarrassment?* If not, it is not done.

---

## 2. The hero principle: one screen, one question

The dashboard landing view is **not** a grid of network-function health indicators.
It is a single, clear question with a single, clear answer:

> **"Is your gNB connected?"**

This is the first-10-minutes moment (see `v1-gap-closure-plan.md`, Gate 1). It *is*
the product demo. Everything else — UE registration, sessions, the data plane, the
full diagnostic catalog — is **progressive disclosure** revealed *after* the gNB
connects. The twelve-network-function reality stays behind one conceptual "core"
(charter principle 6).

### The three states of the hero screen

**Waiting** — zero-config, tells the operator exactly what to do:
```
  Point your gNB here:
    AMF   192.168.1.50 : 38412  (SCTP)
    PLMN  001 / 01
    TAC   1
  ○  Waiting for gNB...
  [Using UERANSIM instead? →]
```

**Connected** — the satisfying flip; physical, green, alive:
```
  ✅  gNB connected — Nokia AirScale
      PLMN 001/01 confirmed · TAC 1 · eMBB slice negotiated
  [Register a UE →]
```

**Failed** — the QCore differentiator; names the cause AND the fix, both sides:
```
  ⚠  NG Setup rejected
     Your gNB sent PLMN 310/260. QCore is configured for 001/01.
     Fix on gNB side:   set MCC=001, MNC=01
     Fix on QCore side: [Change QCore to 310/260 →]
```

The transition between these states is animated (Framer Motion), not a hard swap.
The "waiting → connected" flip should feel like a latch closing.

---

## 3. Design principles (the rules)

1. **One hero per screen.** The most important thing is unmistakably the most
   important thing — by size, contrast, and position.
2. **Dark-first.** Design the dark theme as the primary; light is secondary.
3. **Data is alive.** State changes animate. Counters count. Live signaling streams
   in rather than appearing fully formed. (Charter pillar 4: "make the invisible visible.")
4. **Every error names its cause and its fix** (charter principle 4). No raw codes,
   no stack traces, no "registration failed." Always: what happened, why, what to change.
5. **Progressive disclosure** (charter principle 6). Simple by default; depth on demand.
   The NF topology, raw NGAP/NAS bytes, and logs are available — never imposed.
6. **Validate at input time** (charter principle 3). A malformed SUPI or PLMN mismatch
   is caught the moment it is entered, with the fix inline — not at runtime.
7. **Zero-config to first paint.** The operator never edits a file to read the AMF
   address or see the dashboard. It is on screen at launch.

---

## 4. The tech stack

### Current (as built)
- **React 18 + TypeScript**, bundled by **esbuild**, styled with **Tailwind CSS 3.4**.
- Embedded into the Go binary; the browser only ever talks to the dashboard (`pkg/dashboard/web/`).
- No component library, no animation library yet.

> **Doc correction:** `PRD.md` and `ARCHITECTURE.md` describe the dashboard as
> "Next.js 14 + shadcn/ui." That is aspirational and does not match the codebase.
> QCore is a locally-run tool embedded in a Go binary — **Next.js (an SSR web
> framework) is the wrong fit.** Those two lines should be corrected to match this
> document. We are not adopting Next.js.

### Target additions (to reach the Apple/Tesla bar)
- **shadcn/ui** — copy-in components built on Radix primitives + Tailwind. You own
  every component file; no library opinions to fight. Clean and minimal out of the box.
- **Framer Motion** — the animation layer. This is what makes data feel alive and
  state changes feel physical. Non-negotiable for the aesthetic.
- **Lucide React** — clean, geometric icons matching the visual weight.
- **Bundler:** stay on esbuild if shadcn integrates cleanly into the embedded build;
  otherwise migrate to **Vite**. Do **not** introduce Next.js.

### Why not the alternatives
- **Material UI / Ant Design** — Google/enterprise aesthetics; wrong identity, hard to escape.
- **Chakra UI** — reads as generic SaaS; not premium.
- **Next.js** — an SSR framework for web apps; QCore is an embedded local tool. Wrong category.

---

## 5. Build-order note

The UI/UX foundation (shadcn + Framer Motion + Lucide) is a **cross-cutting
prerequisite**, done once and early — before the hero screen is built on top of it,
not retrofitted after. See `v1-gap-closure-plan.md` for where this sits in the
milestone sequence.

Visual polish is the last 10% (charter §12) — but the *structure* above (one hero,
dark-first, live data, named errors) is not polish. It is the experience itself, and
it is built in from the first screen.
