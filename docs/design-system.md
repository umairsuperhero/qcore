# QCore — Design System (v0.2)

> **What this is.** The concrete, buildable design system that operationalizes the design
> *language* in `docs/ui-ux-design.md` (v0.1 — the *why*: Apple/Tesla, one-hero, dark-first,
> data-alive, named errors). This doc is the *what* and *how*: tokens, components, screens,
> motion, and the tooling to build it. **Charter wins on any conflict**
> (`docs/experience-charter.md`). UX is the product (charter §3).
>
> **Status:** v0.2 draft · **Owner:** Product · Companion mockup: `docs/mockups/hero.html`.

---

## 1. Design tokens (the single source of truth)

Implement as CSS custom properties on `:root` (dark is the default theme), consumed by
Tailwind via `theme.extend`. **Tokens are the contract** — components reference tokens,
never raw hex.

### 1.1 Color — dark-first, signal-led

The palette is a near-black canvas so *the data is the only thing that emits light*. One
"alive" accent (signal cyan), plus the semantic triad (success / warn / fail) that maps 1:1
to the hero's three states and to "every error names its cause."

| Token | Hex | Use |
|---|---|---|
| `--bg` | `#0A0B0D` | App canvas (near-black, not pure `#000`) |
| `--surface` | `#121419` | Cards, panels |
| `--surface-2` | `#1A1D24` | Raised / hover surfaces |
| `--border` | `#252A33` | Hairline dividers (1px) |
| `--text` | `#E8EAED` | Primary text |
| `--text-muted` | `#9099A6` | Secondary / labels |
| `--text-faint` | `#5B6472` | Tertiary / metadata |
| `--signal` | `#22D3EE` | The "alive" accent — live data, links, focus |
| `--signal-dim` | `#0E7490` | Signal at rest / trails |
| `--success` | `#34D399` | Connected / pass (the satisfying green) |
| `--warn` | `#FBBF24` | Reconcilable mismatch / degraded |
| `--fail` | `#F87171` | Rejected / fail (never harsh red) |
| `--data` | `#A78BFA` | Numbers/counters that count up (violet = "compute") |

Light theme is a **secondary** map of the same tokens (charter: dark-first); ship dark first,
derive light later. Never hardcode a color outside this table.

### 1.2 Typography

Two families, doing different jobs:
- **Sans (UI):** Inter (or system `-apple-system`). Headings + body.
- **Mono (data):** JetBrains Mono / `ui-monospace`. *Everything that is a protocol value* —
  PLMN, IMSI, TEID, NGAP hex, SQN, exit codes. Mono signals "this is wire truth."

Type scale (1.250 major-third, rem):

| Token | Size | Weight | Use |
|---|---|---|---|
| `--text-hero` | 3.052rem | 600 | The one hero answer ("gNB connected") |
| `--text-h1` | 2.441rem | 600 | Screen title |
| `--text-h2` | 1.953rem | 600 | Section |
| `--text-h3` | 1.563rem | 500 | Card title |
| `--text-body` | 1rem | 400 | Body |
| `--text-sm` | 0.8rem | 400 | Labels |
| `--text-mono` | 0.875rem | 450 | Protocol values |

Hierarchy is carried by **size + weight + color**, never by decorative chrome.

### 1.3 Spacing, radius, elevation

- **Spacing:** 4px base — `4 / 8 / 12 / 16 / 24 / 32 / 48 / 64`. Generous; space *is* emphasis.
- **Radius:** `--r-sm 6px`, `--r-md 10px`, `--r-lg 16px`, `--r-full 9999px`. Soft, not pill-y.
- **Elevation:** shadows are subtle on dark; lift via `--surface-2` + a 1px `--signal`-tinted
  border on the *active/hero* element, not drop shadows.

### 1.4 Motion (this is what makes data feel alive — non-negotiable)

| Token | Value | Use |
|---|---|---|
| `--ease-out` | `cubic-bezier(0.16, 1, 0.3, 1)` | Enter / settle (the "latch closing") |
| `--ease-in-out` | `cubic-bezier(0.65, 0, 0.35, 1)` | State-to-state |
| `--dur-fast` | 150ms | Hover / press feedback |
| `--dur-base` | 280ms | Card/panel transitions |
| `--dur-hero` | 600ms | The waiting→connected flip (feels physical) |
| `--dur-count` | 900ms | Counter count-up (PLMN confirmed, throughput) |

Rules: state changes **animate**, never snap. Live signaling **streams in** line-by-line.
Counters **count up**. Respect `prefers-reduced-motion` (reduce, don't remove meaning).

---

## 2. Tooling (modern, embedded-friendly — already chosen)

- **React 18 + TypeScript**, bundled by **Vite** (the esbuild→Vite migration already shipped),
  embedded into the Go binary (`//go:embed pkg/dashboard/web/dist`). The browser only ever
  talks to the local dashboard. **No Next.js** (SSR framework; wrong category for an embedded
  local tool — see `ui-ux-design.md` §4).
- **shadcn/ui** — copy-in components on Radix primitives + Tailwind. We own every component
  file; theme entirely through the tokens above. (Adoption is the v0.2 build's first step.)
- **Framer Motion** — the animation layer. The latch flip, streaming trace, count-ups.
- **Lucide React** — geometric icons at the visual weight of the type.
- **Tailwind 3.4** — utility layer driven by the token theme; no ad-hoc hex.
- **Storybook (target)** — render every component in its states (incl. the hero's three) so
  design and the agents building it share one source of truth. Optional but high-leverage.

> **"Claude Design" workflow:** mock a screen as a self-contained HTML/SVG artifact first
> (see `docs/mockups/`), agree the composition, *then* implement it as tokened React +
> Framer Motion. The mockup is the design contract; the code matches it.

---

## 3. Component inventory (build order)

Tier 1 — primitives (shadcn-based, tokened): `Button` (primary/ghost/danger), `Card`,
`Badge` (status: idle/live/pass/warn/fail), `Input` (with input-time validation slot),
`Tooltip`, `Tabs`, `Separator`, `Toast`.

Tier 2 — QCore signatures (the product's identity):
- **`HeroStatus`** — the one-question hero with its three states (Waiting / Connected /
  Failed) and the 600ms latch flip.
- **`SignalTrace`** — the live signaling stream; rows stream in (Framer), mono protocol
  values, expandable to raw NGAP/NAS hex (progressive disclosure).
- **`DiagnosisCard`** — *what happened · why · the fix (both sides)*; the named-error format,
  never a raw code. Pulls from the catalog/SLM.
- **`ReconcilePanel`** — RAN/UE YAML vs core config diff; each mismatch = field + fix,
  pre-attach.
- **`ScenarioRunner`** — author/run a scenario, deterministic PASS/FAIL + trace; surfaces the
  `qcore-cli` exit-code contract for the CI story.
- **`CountUp`** — the `--data`-colored numeric that animates (throughput, confirmed PLMN, TEID).

Tier 3 — depth-on-demand: NF topology view, raw-bytes inspector, journey timeline. Available,
never imposed (charter principle 6).

---

## 4. The screens (each is "one hero, one question")

1. **Hero / landing — "Is your gNB connected?"** The product demo; three states; the latch
   flip. (Spec in `ui-ux-design.md` §2; mockup in `docs/mockups/hero.html`.)
2. **Live trace — "What is happening right now?"** Streaming `SignalTrace`; a failure routes to
   a `DiagnosisCard` inline.
3. **Diagnosis — "Why did it fail, and what do I change?"** The flagship moment; named cause +
   both-sides fix; offline-SLM on catalog misses.
4. **Reconcile — "Will this attach before I try?"** `ReconcilePanel`; input-time, pre-SCTP.
5. **Scenarios / CI — "Make this a gate."** `ScenarioRunner` + the copyable
   `qcore-cli test run … → exit code` contract; the adoption/infrastructure screen.
6. **Subscribers / config — guided, validated.** Input-time validation; the dashboard is the
   source of truth, YAML is an export (charter).

Each screen: dark canvas, one unmistakable hero, mono for wire values, animated state, and —
on any failure — a named cause and fix.

---

## 5. Accessibility & quality bar

- Contrast ≥ WCAG AA on `--text`/`--text-muted` over `--bg`/`--surface` (the palette is tuned
  for it); never encode state by color alone — pair with icon + label (Lucide + text).
- Full keyboard path to the golden path; visible `--signal` focus ring.
- `prefers-reduced-motion`: keep meaning, drop the flourish.
- The test for any screen (from v0.1): *could this go on a Tesla dashboard or in an Apple
  keynote without embarrassment?* If not, it is not done.

---

## 6. Build plan (how v0.2 actually ships)

1. **Tokens first.** Land the CSS-variable token layer + Tailwind theme; migrate existing
   components off ad-hoc hex. (Cross-cutting; do once, early — `ui-ux-design.md` §5.)
2. **Adopt shadcn + Framer Motion + Lucide** behind the tokens.
3. **Rebuild the signatures** (`HeroStatus`, `SignalTrace`, `DiagnosisCard`) to the mockups.
4. **Storybook** the three hero states + a passing/failing diagnosis (shared contract).
5. **Then** the remaining screens, in golden-path order.

Visual polish is the last 10% (charter §12) — but tokens, one-hero, dark-first, live data, and
named errors are **the experience itself**, built in from the first screen, not retrofitted.
