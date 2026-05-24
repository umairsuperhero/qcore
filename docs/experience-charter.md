# QCore — Product Experience North Star & Charter

> **Status:** v0.3 of the charter · supersedes v0.2 · **Stage:** product at v0.6
> **v0.3 change:** two open questions resolved — simulator scope and the 5G SA lead (see D10, D11).
> **Owner:** Product
>
> **What this document is.** The single source of truth for *what QCore is, who it is
> for, and what experience it must deliver.* Every decision downstream of it —
> UI/UX, API shape, default values, error copy, roadmap order, what we say no to —
> should be traceable to a statement in here. If a proposed feature does not move
> us toward the North Star, it is out of scope until this document says otherwise.
>
> **How to use it.** Read §1–§3 before any product or design work. Check the
> Decision Log (§13) before re-opening a settled question. Revisit the whole
> document at each minor version and whenever a major experience decision is made.

---

## 1. What QCore Is

**QCore is a development and test environment for cellular networks.**

This is a deliberate category choice, and it is the most important sentence in
this document. QCore is *not* "another 5G core" competing with open5GS and
free5GC on protocol completeness. In that category, ease of use is a feature —
and features get matched. As a *development environment*, ease of use is not a
feature; it is the entire reason the category exists, and no one is currently
serving it.

The analogy we hold ourselves to: QCore is to cellular development what modern
developer tools (containers, API clients, deployment platforms) are to software
development. They did not win by building a better version of the hard thing
underneath — they won by building the *experience layer* around it, and that
experience became the product.

**One-sentence pitch:** *QCore lets an engineer stand up a real cellular core,
point a device or RAN at it, watch exactly what happens, and understand why —
in minutes, on one screen.*

---

## 2. The Problem

Bringing up and testing against a cellular core is painful even for competent
engineers. The journey is always the same:

1. Stand up a core (a dozen interdependent network functions).
2. Configure it — PLMN, TAC, S-NSSAI, DNN, subscriber SUPI/Ki/OPc — where every
   value must agree across the core, the RAN, and the SIM.
3. Attempt registration.
4. **It fails.** It always fails the first time.
5. Diagnose: which of *forty* possible causes is it? The evidence is scattered
   across signaling messages and the logs of a dozen network functions.
6. Fix, retry, iterate.

Steps 4–5 are where engineers lose hours and days. Existing open-source cores
serve research and testing well but treat this diagnostic pain as the user's
problem. **That pain is QCore's reason to exist.**

---

## 3. North Star Vision

> **Developing for cellular should feel like developing for the web:
> fast to start, observable by default, and the system tells you what is wrong.**

We compete on experience, not on protocol scale or feature count. The incumbents
already win on completeness and carrier-grade hardening. The ground we take —
and intend to own — is the engineer's day-to-day experience.

For QCore, UX is not a layer on top of the product. **UX is the product.**

---

## 4. North Star Metrics

QCore has two critical-path moments: getting a connection working, and
understanding why one failed. We measure both.

**Primary — Time to First Connection (TTFC).** From `docker run` to a device or
UE successfully registered *and passing data*.

| Mode | Target TTFC |
|------|-------------|
| Simulator mode (built-in RAN/UE sim) | **< 5 minutes** |
| Real RAN/device, lab setup | **< 15 minutes** |

**Primary — Time to Root Cause (TTRC).** From a failed registration to the
engineer *knowing the specific cause and the fix*, stated in plain language.

| Failure type | Target TTRC |
|--------------|-------------|
| Known misconfiguration pattern | **< 30 seconds** |
| Novel / multi-symptom failure | **< 5 minutes** |

**Supporting metrics:** config files a user must hand-edit to get started: **0** ·
time to provision a subscriber: **< 60 seconds** · CLI commands required for
routine operation: **0** · % of failures that surface an actionable, plain-language
cause: **> 90%**.

**Counter-metrics (we will not cheat the North Star at their expense):**
correctness — fast and easy must never mean non-compliant or insecure;
honesty — the product must never claim a success or a diagnosis it cannot verify
against ground truth.

---

## 5. Who We Build For

**Primary persona — the RAN / device developer.** An engineer building or
integrating a gNodeB, a chipset, a module, or a device, who needs a core to
test against. Competent with Linux, Docker, and networking; not necessarily a
3GPP specialist. They want a core that just works, that they can configure
trivially, observe completely, and trust — and that tells them what is wrong
when their device misbehaves. **Every default, every screen, and the entire
first-run experience is optimized for this person.**

**Secondary — the learner / educator.** Students, researchers, and instructors
who want to *understand* the core. Served by the same product with **Learning
Mode** — the curtain pulled back. Important as a long-term distribution channel:
engineers who learn on QCore reach for it later at work.

**Tertiary — the small operator / private-network builder.** Someone running
QCore for a real (typically shared- or unlicensed-spectrum) deployment. Real,
but explicitly an *expansion* audience (see §11), not who we optimize first-run
for today.

---

## 6. Experience Principles

The rules. When a design choice is unclear, pick whichever option best honors these.

1. **Zero-config gets you running; config gets you to production.** A first
   launch works with no editing. Every setting has a sane default. Configuration
   is something you grow into, never a prerequisite.
2. **The dashboard is the source of truth — not YAML.** Anything a user can do,
   they can do in the UI. Files are an export/advanced path, never the required one.
3. **Validate at input time, not at runtime.** Catch a PLMN mismatch or malformed
   SUPI the moment it is entered, with the fix stated inline.
4. **Every error names its cause and its fix.** No raw stack traces, no bare codes.
5. **Make the invisible visible.** Registration, session establishment, and data
   flow must be *watchable* live. Knowing whether it works is the hardest part of
   a core — so we show it.
6. **Progressive disclosure.** Simple mode hides the twelve-network-function
   reality behind one conceptual "core." Complexity is available on demand,
   never imposed.
7. **The AI must be load-bearing and trustworthy.** AI features must do a job the
   product genuinely could not do well without them, and must reason over
   verifiable ground truth — never present a guess as a fact.
8. **The first five minutes are sacred.** They decide whether QCore gets a second
   chance. They get disproportionate design attention and relentless measurement.

---

## 7. The Golden Path

The canonical journey for the primary persona. This narrative *is* a spec — UI
work that does not serve a step here should be questioned.

**Step 0 — Discover.** One screen explains what QCore is and shows the one-line
quick start. *Target: belief that this will be easy.*

**Step 1 — Launch.** One command brings up the whole core — all network
functions, database, dashboard — as a single thing.

**Step 2 — Land.** The dashboard opens to a clear health view: the core is up and
green, with a guided "get your first connection" call to action.

**Step 3 — Configure.** A guided, validated flow: provision a subscriber
(SUPI/Ki/OPc, auto-generated or entered), set PLMN/TAC, pick a slice and DNN from
sane defaults. Each field explained in one plain sentence. Done in under a minute.

**Step 4 — Connect.** Two first-class paths: start the built-in RAN/UE simulator
with one click (no hardware), or connect a real gNodeB/device — in which case
QCore shows the exact RAN-side values to enter and reconciles the RAN config
against its own, catching mismatches before they cause a silent failure.

**Step 5 — See it work.** A live view shows the UE registering, the session
establishing, and data flowing — the actual signaling, narrated in human terms.

**Step 6 — Diagnose.** When something fails, QCore states *what* failed, *why*,
and *the fix* — in one place, plain language, reasoning over the full trace.
This is the step that defines the product (see §9).

**Step 7 — Iterate & test.** The engineer fixes, retries, and then exercises the
device against scenarios — handovers, load, edge cases, malformed input.

**Step 8 — Adopt into workflow.** QCore moves from a one-off into the engineer's
real workflow: regression suites, CI integration, shared team use.

---

## 8. Experience Pillars

The five surfaces where QCore wins or loses. Each gets dedicated design attention.

1. **First-Run & Install** — nothing to a green dashboard. (Steps 1–2)
2. **Configuration** — subscribers, slices, DNNs as obvious, guided, validated
   objects. (Step 3)
3. **RAN/Device Integration** — making core↔RAN agreement foolproof. (Step 4)
4. **Live Observability** — registration and data flow made watchable. (Step 5)
5. **Diagnostic Intelligence** — failures that explain themselves. (Step 6) —
   *the pillar where the AI is load-bearing.*

---

## 9. AI-Native Architecture

QCore is AI-native, which we define strictly: **AI is load-bearing, not garnish.**
If the AI were removed and the product were still 90% as good, it would not be
AI-native. The AI must sit on the critical path of the magic.

### 9.1 The job: failure diagnosis

The one job the product fundamentally cannot do well without AI is **diagnosing
why a registration failed.** The cause space is large (PLMN/TAC mismatch, wrong
AMF address, bad slice, unprovisioned SUPI, wrong Ki/OPc, SCTP/NGAP issues, NAS
algorithm mismatch, data-plane routing), and the evidence is scattered.

This is a strong AI job *only because QCore owns the data.* QCore sees every
signaling message, every network function's state, and the full config. The AI
reasons over a clean, structured, ground-truth trace the platform hands it — it
is not a chatbot guessing. That distinction is the entire difference between a
diagnostic engineers trust and one they switch off.

### 9.2 The capability ladder

The AI capability is built in levels. **We do not skip levels.**

1. **Explain** — narrate signaling, decode NAS/NGAP cause codes. Smart help,
   not agentic. Ship first.
2. **Diagnose** — root-cause a failure from the live trace and propose the fix;
   the engineer applies it. **This is the AI-native flagship and the v1 target.**
3. **Act** — apply the fix, re-run the test, confirm resolution. A closed agentic
   loop. Earned only after Level 2 is trusted; requires guardrails because it
   mutates config.
4. **Autonomous test agent** — "here is my device, go exercise it and report what
   is broken." The north-star direction, not a v1 commitment.

### 9.3 The model strategy: tiered, local-first

Our best customers — RAN and device vendors — test in security-restricted, often
air-gapped labs and cannot ship unreleased-product traces to a cloud API.
Therefore:

- **Embedded Small Language Model (SLM), shipped in the container.** Handles the
  bounded, high-frequency 80%: field-validation explanations, cause-code decoding,
  config walkthrough from templates, matching against *known* misconfiguration
  patterns, plain-language narration. Preserves zero-config and works offline,
  everywhere, including air-gapped labs.
- **Optional frontier-model escalation (bring-your-own-key).** For the hard 20% —
  novel multi-symptom diagnosis, vendor-specific interop bugs, complex scenario
  generation — the user may opt in to a frontier model. The default works without it.

### 9.4 The reliability principle

Reliability must **not** come from a model's parametric knowledge. QCore maintains
a **structured diagnostic layer** — a curated knowledge base of symptom→cause
patterns, plus the platform's own ground-truth telemetry. The model is the
*interface and reasoner over that layer*, not the brain. Build it model-first and
you get a confident liar; build it data-and-rules-first with the model on top and
you get something an engineer trusts.

### 9.5 Build-order discipline

AI-native does **not** mean AI-first in build order. The AI is only as good as the
telemetry it reasons over. So: build the deterministic substrate first —
excellent structured observability and input-time validation — but design that
telemetry and event model from day one to be machine-readable and model-friendly.
That is what AI-native architecture means in practice: the AI is a designed-in,
first-class consumer of the platform's data, even before the AI features exist.

---

## 10. Commercial Logic

QCore is not "just another open-source project." It has a path to being a real,
fundable thing, and that path is built into the positioning.

The market reality: nobody buys "a 5G core" — they buy a working network, of which
the core is ~10–15%. The people who most value ease of use (researchers, students,
individual engineers) have little budget; the enterprises with budget want a
supported vendor, not self-serve software. This is the willingness-to-pay
inversion that has kept open-source cores from becoming businesses.

The dev-tools category resolves this: **bottom-up adoption, top-down monetization.**
Individual engineers adopt QCore free because it makes their day better. The
*companies that employ them* pay for what teams need — collaboration, shared test
environments, hosted/managed deployments, support, and the frontier-model-grade
diagnostic tier. We do not need to charge the individual to build a business; we
need the individual to love the product.

Monetization is deliberately *not* a v1 priority. v1 success is defined as
**adoption and engineer love.** But the architecture (multi-user readiness,
hosted-friendly deployment, an escalation tier) is built so monetization is a
later switch, not a later rewrite.

---

## 11. Scope — Now / Next / Not Now

Discipline about what QCore is *not* doing yet is as important as what it is.

**Now (the wedge).** The RAN/device developer's local test-and-dev experience:
Golden Path Steps 0–7, the five Experience Pillars, AI Levels 1–2, the embedded
SLM, the built-in simulator.

**Next (expansion, earned by winning the wedge).**
- Team / collaboration features and hosted/managed QCore (the monetization path).
- AI Levels 3–4 (closed-loop fix-and-verify; autonomous test agent).
- The small-operator / shared-spectrum (CBRS, TVWS) production path — a real
  market, but it reintroduces the full-network-puzzle problem, so it is expansion,
  not wedge.

**Not Now (explicitly deferred).**
- Carrier-scale performance tuning and HA as a v1 priority.
- Embedded-core-for-appliance-builders as a product line — a viable later
  monetization path, but a B2B integration play with no magical first-run, so
  it is not the identity we build now.
- Protocol feature-count parity with open5GS/free5GC.

---

## 12. Anti-Goals

What "world-class" does **not** mean for QCore, so we do not drift:

- **Not** a feature-count race. Fewer features, flawlessly delivered.
- **Not** a configuration framework. If users live in our YAML, we have failed.
- **Not** an AI demo. If the AI is decoration rather than load-bearing diagnosis,
  cut it.
- **Not** a beautiful skin over a confusing model. Visual polish is the last 10%,
  never a substitute for a sound experience.
- **Not** all things to everyone. We optimize hard for the primary persona (§5).

---

## 13. Decision Log

The reasoning behind the charter, so future decisions inherit the *why*, not just
the *what*. Add an entry whenever a major decision is made or reversed.

| # | Decision | Why |
|---|----------|-----|
| D1 | QCore's category is a **development & test environment for cellular**, not a 5G core competing with open5GS/free5GC. | In the "5G core" category, ease of use is a matchable feature. As a dev-tools category, it is the whole point and is currently unserved. |
| D2 | Primary wedge: **RAN/device test-and-development.** | The only positioning where ease of use is the binding constraint, QCore is useful with zero other network pieces (via the simulator), the magic fits one screen, and adoption compounds engineer-to-engineer. |
| D3 | North Star: **"developing for cellular feels like developing for the web."** Primary persona: the RAN/device developer. | Follows directly from D1/D2; sharpens the earlier, vaguer "feels like running a database." |
| D4 | AI is **load-bearing via failure diagnosis**, not garnish. The v1 flagship is Level 2 (Diagnose). | Diagnosis is the one job the product cannot do well without AI, and it sits on the critical path of the magic. Autonomous agents (L3–4) are deferred to avoid over-reaching before the diagnosis layer is trusted. |
| D5 | **Tiered model strategy:** embedded SLM for the bounded 80%, optional bring-your-own-key frontier-model escalation for the hard 20%. | Best customers test in air-gapped/security-restricted labs and cannot send traces to a cloud API. A local model keeps AI working everywhere and preserves zero-config. |
| D6 | Reliability comes from a **structured diagnostic layer + ground-truth telemetry**; the model is interface/reasoner, not brain. | Model-first design produces a confident liar; data-and-rules-first with the model on top produces something engineers trust. |
| D7 | **Build the deterministic substrate first** (observability + validation), designed model-friendly from day one. AI-native ≠ AI-first in build order. | The AI is only as good as the telemetry it reasons over. |
| D8 | Commercial model: **bottom-up adoption, top-down monetization.** v1 success = adoption and engineer love, not revenue. | Resolves the willingness-to-pay inversion that has kept open-source cores from becoming businesses. Architecture is built so monetization is a later switch, not a rewrite. |
| D9 | Small-operator/shared-spectrum is **expansion**; embedded-core and education are a later monetization path and a product mode respectively — none is the wedge. | "It can't be all things to everyone." Each fails at least one wedge test in §1/D2 (puzzle-piece dependency, no magical first-run, or weak economic gravity). |
| D10 | The built-in RAN/UE simulator is bounded to a **scriptable control-plane tool with error injection** — not an RF/physical-layer emulator — and QCore **bundles existing open-source simulators** (UERANSIM for 5G; srsRAN or similar for 4G) rather than building its own. | The wedge user tests their own hardware against QCore; the simulator's job is zero-hardware demo, CI, and producing failures for the diagnostic layer to reason about. High RF fidelity would mean building a second hard product off-wedge. |
| D11 | The Golden Path, demo, and positioning **lead with 5G SA.** 4G/EPC remains fully supported but is not the headline. | New RAN/device development is overwhelmingly 5G and the pain narrative is sharpest there. 4G/EPC stays because it is already built, many devices are still 4G, and 5G NSA depends on it. Implication: 5G SA core work is promoted to v1-critical (see build brief, Phase A). |

---

## 14. Open Questions

The remaining agenda. Persona, positioning, simulator scope, and the 5G SA lead
are now settled (§5, §13); these are not.

1. **SLM selection & packaging.** Which small model, how it ships, container size
   budget, and the update path.
2. **Monetization timing.** When does the first paid (team/hosted) tier appear,
   and what is the precise free/paid line.

---

*This is a living charter. It is revisited at each minor version and whenever a
major experience decision is made. Settled questions are not re-opened without a
new Decision Log entry recording why.*
