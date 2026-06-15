# QCore — Strategy & Competitive Positioning

> The **positioning layer**: where QCore sits in the landscape and the strategic bets that
> follow. The *what/why* lives in `docs/experience-charter.md`; the *execution plan* in
> `docs/next-phases-plan.md`; the *living, commit-by-commit audit* in `docs/audit-v1.0.md`.
> This doc is reconciled to what is **actually shipped** — it is not a to-do list.
> Last updated: 2026-06-14.

## 1. The thesis
QCore is a **development & test environment for cellular networks** — DX is the product,
not protocol feature-count. Primary user: the RAN/device developer who needs a core to
test against. We win on **fast start, correlated observability, AI that explains failures,
and spec-fidelity at the boundaries** (so real RAN/devices attach) while simplifying
internal conveniences.

The category is **unserved**. Open-source cores compete on protocol completeness;
test-equipment vendors compete on conformance at $100K+. No one occupies *"git clone →
observable, debuggable cellular connection in minutes, with AI that explains the failure."*

## 2. The competitive landscape (and the gap we occupy)

### 2.1 Open-source cores — standards-correct engines, brutal DX
- **Open5GS** (C, ~2.6k★): fast + stable, but per-NF YAML sprawl (`amf.yaml`/`smf.yaml`/
  `upf.yaml`…), logs that grow unbounded in `/var/log/open5gs` (needs external logrotate),
  and **no correlated tracing** — you `tcpdump`/Wireshark on Linux bridges to diagnose.
- **free5GC** (Go, ~2.3k★): modular, but install is heavy — a custom kernel module
  (`gtp5g`) that breaks on host-kernel updates, MongoDB requiring AVX, drift-prone setup
  scripts, and NFs that **silently crash** on DB/PFCP bind failures without naming the cause.
- **OAI-CN** (C, K8s/Helm): standards-compliant + cloud-native, but disaggregated deploy
  needs multi-host routing, custom `iptables`/`nftables`, rigid startup ordering
  (DB→NRF→AMF→SMF→UPF); diagnostics = crank console logging to DEBUG and parse stdout.
- **Magma / SD-Core**: fuller NMS GUIs, but enterprise-operator-complex, not developer-loop tools.

**The pattern:** all optimize spec coverage; none treat the *developer's debugging loop* as
the product. None have correlated tracing, AI failure-diagnosis, built-in error injection,
or a measured fast-start.

### 2.2 Commercial private 5G — simple to deploy, closed black boxes
- **Druid (Raemis):** excellent, RAN-agnostic, **REST-API-first** (the GUI rides the API) —
  closest in spirit to us, but proprietary + high-license. The API-first lesson is one to steal.
- **Attocore (AttoNGC):** lightweight converged 4G/5G, embeddable on a radio CPU;
  telecom-interface-centric, no developer REST / local sim.
- **Cumucore:** cloud-native K8s, TSN-over-5G for Industry 4.0 robotics; operator-oriented
  config, not developer-oriented.

### 2.3 The hyperscaler retreat (2025) — the cautionary tale
- **AWS Private 5G** (retired May 2025) and **Azure Private 5G Core** (retired Sep 2025)
  both pulled out: physical radio-site friction, CBRS spectrum constraints, heavy
  proprietary edge hardware (Outposts / Azure Stack Edge), cloud-consumption models that
  didn't fit the slow physical-network lifecycle, and enterprises lacking edge-to-radio
  troubleshooting expertise.
- **Lesson:** the value is the *developer loop on a laptop*, decoupled from
  spectrum/hardware/cloud-egress. Our offline-first, simulator-first, static-binary posture
  is the anti-hyperscaler bet — validated by their failure.

### 2.4 Commercial test tools — the price umbrella
Amarisoft ($10–50K), Keysight/VIAVI/Spirent/R&S/Anritsu ($100K–$1M+). They own
RF/conformance. The chasm between "$100K conformance rig" and "configure 15 YAMLs for
Open5GS" is exactly our wedge.

### 2.5 6G direction (Rel-19/20) — where the puck is going
AI-native SBA (AI in the NF control loop, not bolted-on NWDAF), native telemetry/data-
fabrics as first-class protocols, and edge autonomy. **Our Phase-A structured event
substrate is a down-payment on this** — a machine-readable telemetry layer the AI reasons
over, built *into* the NFs, not bolted on after.

## 3. The moats we've actually built (not just planned)
1. **Pure-Go, static-binary, zero-CGO** — hand-rolled Aligned-PER S1AP/NGAP, native SCTP via
   raw syscalls. Cross-compiles anywhere, tiny Alpine images, no C toolchain. Competitors
   got this wrong (C build chains, `gtp5g` kernel module, MongoDB/AVX).
2. **Event substrate + journey correlation** — structured events with `journey_id` threaded
   through every NF; the AI reasons over a deterministic timeline, not flat logs. The
   6G-direction bet, already wired in.
3. **Catalog-first + offline SLM hybrid (validated)** — deterministic rules first (fast,
   grounded), a local llama.cpp SLM for misses, no cloud/key. Works in the air-gapped /
   RF-shielded lab the hyperscalers can't reach.
4. **Real-RAN data plane (T10)** — UERANSIM over native SCTP: registration → PDU session →
   NGAP resource setup → PFCP tunnel update → UPF TUN/NAT → **UE ping over `uesimtun0`**, in
   CI. Not "passes our own test" — *interoperates*.
5. **Input-time config reconciliation** — diff the RAN/UE config against the core *before*
   attach; turn a silent runtime reject into a named mismatch + fix.
6. **Measured DX** — TTFC ≈ 77s, worst-case TTRC ≈ 3.6s, both beating charter targets. We
   say the number, not assert it.

## 4. The strategic bets ahead (the forks)
Execution detail in `docs/next-phases-plan.md`. Ordered by *"where 'demand-driven' quietly
became 'we weren't ready when the developer showed up.'"*
1. **AUTS/SQN resync** — the #1 first-run papercut for *physical* UEs across a core restart;
   cheap (reverse-Milenage); defends the zero-config promise.
2. **Headless CLI / CI harness (P3.2)** — `qcore test --scenario x.yaml` → exit code + JUnit;
   turns a laptop tool into a *blocking test gate* in CI. The adoption flywheel.
3. **SUCI Profile A/B (ECIES)** — the gateway to real silicon (commercial SIMs encrypt the
   SUPI). Build it as *readiness* so the first real-device attach succeeds; validation gated
   on a real device.
4. **Catalog data-plane depth** — GTP-U drops, path-MTU, PFCP transaction desync; compounds
   the most defensible asset.
5. **(resist) standing journey DB** — the diff-yesterday-vs-today use case is real, but serve
   it as *file-based journey snapshots* (reuse the scenario store), not a time-series DB —
   keeps charter §11.

## 5. The discipline that wins (charter §11)
The reason to win is **not** feature parity. No carrier-scale HA, no billing (Gx/Gy), no
3GPP completeness. 100% of the budget goes to observability, config reconciliation, and
plain-language diagnosis. The hyperscaler retreat and the open-source DX gap both confirm:
the durable category is the developer loop, kept tight.
