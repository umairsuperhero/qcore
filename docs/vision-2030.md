# QCore — 2030 Vision Audit

> **What this is.** A forward-looking strategic audit: where QCore is *today* (reconciled
> to what shipped), where it must be by **2030**, the market/technology currents it is
> sailing into, the moats that compound, the risks that kill it, and the decisions that
> decide the outcome. Companion to `docs/audit-2030.md` (the *retrospective*),
> `docs/strategy.md` (positioning), and `docs/audit-v1.0.md` (the live, commit-by-commit
> audit). **Where this doc and the code disagree, the code and the live audit win.**
>
> **Premise:** an expert evaluator in 2030 — caring equally about intellectual
> contribution and commercial survival — looks back at the path from mid-2026.

---

## 0. What changed since the retrospective (read first)

`docs/audit-2030.md` named two "delayed forks" as the remaining real-silicon gate:
**AUTS/SQN resync** and **SUCI Profile A/B (ECIES)** — plus the **CLI** as the unbuilt
adoption vector. Reconciled to today:

- **AUTS/SQN resync — SHIPPED & interop-validated.** Reverse-Milenage (f1\*/f5\*,
  TS 35.208-vector-tested) at the UDM SIDF; a real UERANSIM UE forced a Synch failure and
  re-registered (`ueransim-interop` run `27529970131`, `T10 SQN RESYNC PASS`). One of the
  two real-silicon forks is closed.
- **CLI / CI hooks — SHIPPED (P3.2).** `qcore-cli test run --scenario x.yaml` returns a
  CI exit-code contract (0 pass / 1 fail) + `--json`. The adoption vector exists.
- **Scenario authoring — SHIPPED (P3.1).** Save/list/run named scenarios with
  deterministic PASS/FAIL + trace.
- **SUCI Profile A/B (ECIES) — the LAST real-silicon gate.** Still null-scheme only;
  the executable brief is queued (`docs/briefs/p4.1-suci-profile-ab-ecies.md`). When this
  lands, *first-contact with a commercial SIM/baseband succeeds* instead of failing silently.

**So the 2030 question is no longer "can it interoperate?" It is "will anyone outside the
repo run it?"** The remaining gates are **adoption** (external users) and **one** protocol
readiness item (ECIES). Everything else the thesis needed is built and verified.

---

## I. The 2030 destination (what winning concretely looks like)

Not "a better Open5GS." Three falsifiable end-states, in priority order:

1. **The default core a RAN/device engineer reaches for to test against.** When someone
   gets a new gNB, baseband, or O-RU on the bench, `git clone qcore && make up` is the
   reflex — the way `localhost` + Postman is the reflex for a web API. Measured by: external
   golden-path completions with their *own* TTFC/TTRC, and inbound interop findings.
2. **A blocking CI gate in RAN/device release pipelines.** `qcore test --scenario …` runs
   on every firmware/RAN build; a registration or PDU-session regression fails the build
   *before* it reaches a lab. This is how a laptop tool becomes infrastructure — and the
   stickiest position in the stack.
3. **The reference "explainable core."** When a 5G signaling failure needs explaining —
   in a classroom, a vendor bake-off, or a 3 a.m. lab — QCore's journey trace + diagnostic
   narration is the artifact people screenshot. The catalog is cited, not just used.

If only (1) happens, QCore is a beloved tool. (1)+(2) make it infrastructure. (3) makes it
the category's center of gravity. **None of these requires violating §11.**

---

## II. Where we actually are (mid-2026, reconciled)

**Built and externally validated:** pure-Go zero-CGO core; native-SCTP NGAP; 4G EPC +
5G SA control & user plane; **T10 real-RAN data plane** (UE ping over `uesimtun0`);
**AUTS/SQN resync** interop-proven; journey-correlated event substrate; un-mocked dashboard
(live SSE + diagnostics); **28-rule catalog** (9 mined from real interop failures);
**offline SLM** (air-gap validated); **config reconciliation**; **scenario authoring + CI
hooks**; measured **TTFC ≈ 77s / TTRC ≈ 3.6s**.

**The two honestly-open gates:**
- **External adoption = 0.** No outside user has completed the golden path with their own
  measured numbers. *"The difference between a prototype and a product is someone else using
  it."* This is the single most important unbuilt thing — and it is not a code task.
- **ECIES (SUCI A/B).** The last first-contact-with-real-silicon blocker. Brief queued.

Everything else is a compounding asset, not a gap.

---

## III. The currents 2026 → 2030 (what the market does *to* us)

1. **6G shifts toward an AI-native, service-based core (3GPP Rel-19/20→21, ~2028–2030).**
   The architecture trend is *exactly* QCore's bet: structured, observable, service-based
   signaling with AI in the loop. QCore's journey-correlated event substrate is a
   down-payment on the 6G management/observability model. **Optionality, not a near-term
   build** — track it (the quarterly 3GPP check-in exists), don't chase it (§11).
2. **O-RAN disaggregation widens the test surface.** More vendors, more interfaces (O1/O2,
   RIC), more integration pain → more demand for a laptop-class core that *explains* why an
   O-RU/O-DU won't attach. QCore's reconciliation + catalog model extends here; building the
   RAN side is §11-deferred until a concrete target pulls it.
3. **AI coding agents collapse the cost of protocol breadth — and commoditize it.** By
   2030, "implement NF X" is cheap for everyone, including competitors. **Feature count stops
   being a moat for anyone.** What does *not* commoditize: the curated catalog of real-world
   failure→cause→fix knowledge, the measured-DX reputation, and the trust of "every ✅ is
   honest." QCore must lean *harder* into the things AI can't copy by reading a spec.
4. **The shielded-lab / air-gap reality persists.** RF-attenuated chambers, classified
   labs, no cloud egress. Cloud-dependent diagnosis can't follow the engineer in there;
   QCore's offline-first SLM can. This advantage *grows* as frontier models get more
   cloud-bound.
5. **Hyperscaler-managed-core retreat (2025) holds.** The value was never a managed cloud
   core on proprietary HW; it's the developer's debugging loop on the machine in front of
   them. The anti-bet is confirmed; don't re-litigate it.
6. **Test-equipment price umbrella stays high ($100K–$1M+).** A free, fast, explainable
   software testbed keeps undercutting the bottom of that market — the structural wedge.

---

## IV. The moats — and how each compounds toward 2030

| Moat | Why it holds | How it compounds |
|---|---|---|
| **Real-failure catalog** | Encodes knowledge you only get by *hitting* the failures | Every interop run / external user adds rules competitors can't derive from the spec. The flywheel. |
| **Journey-correlated event substrate** | The AI is only as good as its telemetry; flat logs can't be reasoned over | Becomes the 6G AI-native-SBA observability model; every new NF/scenario enriches it |
| **Spec-fidelity at the boundary** | Real RAN/devices attach instead of silently failing | Each validated target (UERANSIM → srsRAN → real gNB → baseband) is a credibility asset and a catalog source |
| **Offline-first tiered AI** | Works in air-gapped labs where cloud AI can't go | Advantage widens as frontier models get more cloud-bound |
| **Measured DX (TTFC/TTRC)** | A *number*, not a claim; publishable | External users' own numbers turn it into social proof |
| **Honest-✅ discipline** | Trust is the scarce asset in a category full of overclaiming | Compounds into the reputation that makes (3) — "reference explainable core" — possible |

**The strategic instruction:** as AI commoditizes protocol breadth (current #3 above),
**invest in the moats AI can't copy** — catalog depth, real-target validation, and trust —
not in feature parity.

---

## V. The 2026 → 2030 path (forks, in order)

1. **Now → close the real-silicon gate.** Ship **SUCI A/B (ECIES)** so first-contact with
   commercial silicon succeeds. (Brief queued.) This is the last "it won't even attach" blocker.
2. **Then → get ONE external developer through the golden path** with their own measured
   TTFC/TTRC, and publish it. Add `CONTRIBUTING.md`, issue templates, a 5-minute quickstart.
   *This is the highest-leverage non-code work in the whole plan.* Then ten.
3. **Then → make the CI gate real in someone else's pipeline.** P3.2 built the contract;
   adoption means a documented GitHub Action + JUnit output a real RAN/device team runs on
   every build. This is the (2)→infrastructure transition.
4. **Continuously → mine every interop failure into the catalog.** Each external user and
   each new validated target (srsRAN, other UERANSIM versions, a real gNB) feeds the flywheel.
   The catalog is the compounding, defensible IP.
5. **Optionally / demand-driven → broaden real-RAN targets and track 6G.** Per-target replay
   (§P4.2) only when a concrete target pulls it; 6G AI-native-SBA readiness as optionality,
   tracked by the quarterly check-in. **Never** generalize bundled-profile results into a
   conformance-matrix claim.

---

## VI. Commercial model — sustainable without breaking §11

The product is the open developer loop; revenue must not corrupt it.

- **Stays free / OSS:** the core, the dashboard, the catalog, the CLI, offline AI. This is
  the wedge and the trust; never paywall it.
- **Plausible, §11-safe revenue (only once adoption is real):** hosted/managed **CI runners**
  for the gate (heavy SCTP/TUN Linux infra teams don't want to maintain); **enterprise
  catalog/knowledge** curation + private-target validation; **training/certification** for the
  learner persona; **support/SLA**. Each is a *service around* the loop, not a tax on it.
- **Explicit non-goals (§11):** carrier-scale HA, billing (Gx/Gy), feature-parity racing,
  AI Levels 3–4, team/hosted *core* productization. The moment QCore chases these it becomes a
  worse Open5GS. **Discipline is the strategy.**

Sequencing rule: **adoption before monetization.** A revenue motion before (V.2) is premature
and risks corrupting the free loop that creates the value.

---

## VII. Risks & failure modes (what actually kills QCore) + leading indicators

| Failure mode | Why it's lethal | Leading indicator to watch |
|---|---|---|
| **No external users by mid-2027** | A tool nobody outside the repo runs is a personal project, not a product | Golden-path completions by non-maintainers; inbound issues/findings |
| **Scope creep into §11** | Becomes a worse Open5GS; loses the unoccupied quadrant | PRs drifting toward HA/billing/parity; "just one carrier feature" |
| **Trust erosion (an over-claimed ✅)** | The one scarce asset; one inflated "validated" claim poisons the well | Any status row green without build+vet+test + acceptance evidence |
| **Catalog stops compounding** | The AI-commoditization era leaves nothing defensible | Rule count flat; interop failures not mined into rules |
| **Single-maintainer bus factor** | No contributors = no continuity | Contributor count; reviewed-by-others ratio |
| **Real silicon still fails first-contact** | The wedge's endgame; silent failure = the dev just leaves | ECIES shipped? AUTS/SQN shipped (✅)? per-target replay evidence |
| **Frontier-model dependence creeps in** | Breaks the air-gap advantage and TTRC | Diagnosis paths that require a cloud key to work |

---

## VIII. Scorecard — updated to today

| Dimension | Retrospective (mid-2026) | Today (reconciled) | Note |
|---|---|---|---|
| Vision | 9 | 9 | Charter still exceptional |
| Architecture | 8–9 | 9 | Event substrate + native SCTP + SIDF resync, externally validated |
| Protocol implementation | 8 | 8–9 | AUTS/SQN interop-proven; ECIES is the last attach-blocker |
| Dashboard / DX | 8 | 8 | Un-mocked; the *polish* tier (this UX pass) is the next lift |
| Documentation | 9 | 9 | Living-audit + brief discipline intact |
| Test quality | 8 | 8–9 | `-race`, gofmt-gated CI, interop data-plane + SQN-resync gates |
| Adoption / community | 3 | **3** | **The unmoved number. The whole game now.** |
| Research contribution | 7 | 7–8 | Measured-DX comparison + reverse-Milenage interop story publishable |

**One number hasn't moved: adoption.** Every engineering fork that mattered is closing.
The 2030 outcome is now decided almost entirely by whether QCore gets *someone else* to run it.

---

## IX. What must stay true (2026 → 2030)

1. **Hold §11.** Discipline is the strategy; the unoccupied quadrant is the whole bet.
2. **Keep every ✅ honest.** Trust is the scarce asset and the path to "reference explainable core."
3. **Get external users — then mine their failures into the catalog.** Adoption *is* the moat
   accelerant; the catalog is what AI can't commoditize.
4. **Invest where AI can't copy:** real-world failure knowledge, validated real targets, and
   trust — not feature breadth.
5. **Offline-first forever.** The air-gap advantage only grows; never make core diagnosis
   require a cloud key.

> *The 2026 work made QCore a real product on the inside. The 2030 win is making it one on the
> outside.* The engine is done; the forks that remain are **ECIES** (last attach-blocker),
> **adoption** (the whole game), and **discipline** (the moat's guardrail).
