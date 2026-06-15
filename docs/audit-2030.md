# QCore — 2030 Retrospective Audit

> **Premise:** an expert evaluator in 2030 looks back at QCore's trajectory from summer
> 2026 — caring about both intellectual contribution and commercial viability.
>
> **Why this supersedes the earlier draft:** an earlier version of this exercise described a
> *very early* snapshot (mocked dashboard, no UERANSIM, ~5 catalog rules, unmeasured TTFC)
> and went stale the moment the team shipped the un-mock, T10, B2, the catalog, TTFC/TTRC,
> config reconciliation, and scenario authoring. This audit is **reconciled to what was
> actually shipped and verified**. Companion to `docs/strategy.md` (positioning) and
> `docs/audit-v1.0.md` (the live, commit-by-commit audit). Where this doc and the live audit
> disagree, **the live audit and the code win.**

---

## I. Ground truth — reconciled (mid-2026 baseline)

By mid-2026 QCore was no longer the "impressive prototype with a mocked dashboard" of the
first gut-check. The intervening work closed exactly the last-mile gaps that draft named:

| Early-audit "weakness" | What actually shipped (verified) |
|---|---|
| Dashboard 60% demo-ware (`USE_MOCK_STREAM=true`) | **Un-mocked.** Live trace + diagnostics run on the real `/api/events/stream` SSE; runtime-proven for 4G + 5G, happy path + injected failure, with the real diagnostic engine. |
| No offline AI | **B2 offline SLM live-validated** — llama.cpp + baked Qwen2.5-1.5B, catalog-first, internal-network air-gap smoke passed. No cloud, no key. |
| No UERANSIM interop / 5G unvalidated | **T10 shipped (bundled UERANSIM profile):** native SCTP → registration → PDU session → NGAP resource setup → PFCP tunnel update → UPF TUN/NAT → **UE ping over `uesimtun0`**, in CI (run `27115478758`). |
| Catalog ~5 patterns | **28 typed rules**, incl. 9 mined from real T10 interop failures. |
| TTFC/TTRC unmeasured | **Measured:** ≈ 77s cold-start TTFC, ≈ 3.6s worst-case TTRC — both beat the charter targets. |
| CI builds 3 of 11 binaries | Builds **all 13** + `go vet` + `-race`. |
| Config mismatch = silent runtime reject | **Input-time reconciliation:** `/api/ran-config/reconcile` names the mismatched field + the fix, pre-attach. |
| Scenario authoring absent | **Save/list/run named scenarios** with deterministic PASS/FAIL + trace. |
| Dual-hook bug, dead `RANConnectView`, light-mode remnants | Fixed / deleted. |

The ~20K-line pure-Go protocol core (real PER codecs, Milenage/AKA against known vectors,
real GTP-U/PFCP data plane) was genuine the whole time. What changed is that the
**experience and the external validation caught up to the engine.**

## II. Why QCore won the developer-loop category

The 2026 market had four quadrants, and QCore sat in the one nobody occupied:
- **Open-source cores** — spec-complete, DX-brutal (YAML sprawl, no correlated tracing,
  kernel-module install hell, silent crashes).
- **Commercial private cores** — simple to deploy, closed black boxes, high license.
- **Hyperscaler-managed cores** — *retreated in 2025* (cloud egress + heavy edge HW +
  spectrum friction didn't fit the physical-network lifecycle).
- **Test rigs** — $100K–$1M+ conformance equipment.

Each competitor weakness mapped to a QCore decision:

| Competitor pain | QCore answer |
|---|---|
| Open5GS: per-NF YAML + no tracing | Dashboard-as-source-of-truth + journey-correlated events |
| free5GC: `gtp5g` kernel module, MongoDB/AVX, silent crashes | Pure-Go static binary, zero kernel modules, zero CGO, "every error names its cause" |
| OAI: startup-ordering + `iptables` | One-command launch + NRF discovery + config reconciliation |
| Druid: REST-first GUI (but closed) | Same API-first lesson, open-source |
| Hyperscaler retreat: cloud egress + edge HW | Offline-first SLM + simulator-first + laptop-class footprint |
| Test rigs: $100K+ | Free, `git clone`, < 5-min TTFC |

The thesis — *"DX is the product"* — was the unoccupied quadrant. The hyperscaler retreat
*proved* the anti-bet: the value isn't a managed cloud core on proprietary hardware; it's
the developer's debugging loop on the machine in front of them.

## III. The forks in the road (decisions, and the ones we delayed)

1. **Hybrid catalog + offline SLM vs. LLM hype — WON.** We grounded diagnosis in the
   structured event model: deterministic catalog first, a tiny local SLM only on misses.
   Cores that piped raw packet hex into frontier LLMs were too slow (TTRC > 30s), expensive,
   and hallucinated spec violations. The **catalog became the IP** — and mining real interop
   failures into rules compounded it.
2. **Spec-fidelity at the boundary — WON.** Canonical PLMN bytes, real null-scheme SUCI,
   standards-correct NGAP/NAS, NRF discovery. Internal logic simplified; boundary interfaces
   absolute. That's why real RAN attaches instead of silently failing.
3. **Simulator-first, hardware-decoupled — WON.** UERANSIM/srsRAN out of the box let
   developers build and inject failures on a laptop before buying an SDR — sidestepping the
   spectrum/RF/site friction that killed the hyperscaler cores.
4. **SUCI Profile A/B (ECIES) — the readiness fork (delayed too long).** Through mid-2026
   QCore was null-scheme-only. That's fine for UERANSIM, but commercial SIMs/basebands
   encrypt the SUPI (Profile A/B ECIES) — so the first real device failed *silently*. The
   lesson: when demand is **predictable** (real silicon is the wedge's natural endgame),
   build readiness *before* the device shows up. Validation stays gated on a real device, but
   the de-concealment must already be there so first-contact succeeds.
5. **AUTS/SQN resync — the papercut fork (delayed too long).** The UDM returned `501` for
   resync. The single most frequent first-run papercut for a *physical* UE: restart the core
   while the UE stays powered, the SQN desyncs, auth silently rejects. For a tool sold on
   "zero-config and seamless," that broke the promise. Reverse-Milenage resync was cheap and
   should have been a core *experience* requirement, not a Phase-4 afterthought.

## IV. Scorecard — reconciled to reality

| Dimension | Early gut-check | Mid-2026 (real) | Note |
|---|---|---|---|
| Vision | 9 | 9 | Charter still exceptional |
| Architecture | 8 | 8–9 | Pure-Go + event substrate + native SCTP, now externally validated |
| Protocol implementation | 7 | 8 | 5G SA validated against the UERANSIM **data plane** (was internal-only) |
| Dashboard / DX | 5 | 8 | Un-mocked real SSE + diagnostics + 5G mode + scenario authoring |
| Documentation | 9 | 9 | Living-audit discipline intact |
| Test quality | 7 | 8 | CI builds all 13, `-race`, `ueransim-interop` data-plane gate |
| Commercial readiness | 3 | 5–6 | The named blockers (mocked dashboard, no UERANSIM) are gone; still pre-users/community |
| Research contribution | 6 | 7 | Pure-Go PER + event correlation + **measured** DX comparison now publishable |

The two honestly-stuck dimensions:
- **Commercial readiness** — no external users / community yet. `CONTRIBUTING.md`, issue
  templates, and *one external developer through the golden path with their own measured
  TTFC/TTRC* remain. "The difference between a prototype and a product is someone else using it."
- **The real-silicon gate** — ECIES + AUTS/SQN, the two delayed forks above.

## V. The 2030 strategic lessons (expanded)

1. **The observability substrate is the moat.** An AI diagnostic engine is only as good as
   its telemetry. Structured, journey-correlated events built *into* the NFs gave the AI a
   deterministic timeline to reason over — not flat logs to guess at. This is also the
   6G AI-native-SBA direction, pre-paid.
2. **Input-time config reconciliation is the highest-value DX.** Shifting failure detection
   to the boundary (diff RAN/UE YAML vs. core *before* SCTP) converts opaque runtime rejects
   into a named field + fix, and collapses TTFC.
3. **Tiered, local-first AI wins the shielded lab.** RAN/device engineers work in
   RF-attenuated, often air-gapped chambers. A local SLM (Qwen2.5-1.5B via a llama.cpp
   sidecar) kept deep diagnosis working with no internet and no API key — exactly where the
   cloud-dependent cores couldn't go.
4. **Decouple the core from hardware via simulation-first.** Spectrum licensing, RF noise,
   and radio installation are the real barriers. Shipping high-fidelity sim integration
   out of the box let the whole build/test/inject loop happen on a standard laptop.
5. **Spec-fidelity at the boundaries; simplification inside.** A credible testbed must be
   standards-correct where real RAN/devices touch it. Internal conveniences can be simplified
   (e.g., HTTP S11/S6a); the wire-facing interfaces cannot.
6. **(new) Readiness beats "demand-driven" for predictable demand.** ECIES and AUTS/SQN were
   parked as "Phase-4, when a user asks." But real silicon *is* the wedge's endgame — the
   demand was predictable. Silent failure at first-contact is the most expensive bug to
   discover, because the developer just leaves. Buy readiness early; it's cheap in advance.
7. **(new) The CLI is the adoption vector; the dashboard is the conversion.** The animated
   live-trace converts a curious developer in the first five minutes. But the headless
   `qcore test --scenario x.yaml → exit code + JUnit` is what makes QCore a *blocking gate*
   every RAN release must pass — which is how a laptop tool becomes infrastructure.

## VI. What must stay true to win (2026 → 2030)

- **Hold §11.** The moment QCore chases carrier-scale HA, billing (Gx/Gy), or feature
  parity, it becomes a worse Open5GS. The discipline *is* the strategy.
- **Keep every ✅ honest.** Build + vet + tests; "interoperates with UERANSIM" ≠ "passes our
  own test"; "bundled-profile T10" ≠ "conformance matrix." The living audit is the conscience.
- **Get one external developer through the golden path** with their own measured TTFC/TTRC,
  and publish it. Then ten.
- **Mine every real interop failure into a catalog rule.** The catalog is the compounding,
  defensible IP — the one asset competitors can't copy by reading the spec.

> *"The difference between a prototype and a product is someone else using it."* The 2026
> work made QCore a real product on the inside. The 2030 win was making it one on the
> outside — and the forks that decided it were readiness (ECIES, AUTS/SQN), adoption (the
> CLI), and discipline (§11).
