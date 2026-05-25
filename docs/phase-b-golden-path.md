# Phase B — The Golden Path

> **Status:** plan · approved 2026-05-24 after Opus 4.7 review
> **Predecessor:** Phase A (event model) shipped at `07a6e8d`
> **Charter alignment:** Golden Path steps 1–5, Experience Pillars 1, 2, 3, 4
> **Exit criterion:** TTFC < 5 min, end-to-end demonstrable on the 4G EPC

## Goal

The Golden Path, steps 1–5: one command brings the core up; the dashboard
lands on a green health view; one click triggers an attach that the engineer
watches complete live. Step 6 (diagnosis) is Phase C; this phase produces the
substrate it needs.

## Deliverables, in build order

### B1 — One-command launch
- Update `deployments/docker/docker-compose.yml` to add `collector` and
  `dashboard` services alongside the existing `postgres`/`hss`/`mme`/`spgw`.
- Add a `Dockerfile.collector` and `Dockerfile.dashboard`.
- Add `make up` / `make down` shortcuts that wrap `docker compose up -d` and
  `docker compose down`.
- After `make up`: `http://localhost:3000` opens to the dashboard.

### B2 — Dashboard BFF (`cmd/dashboard`)
A new Go binary on port 3000 acting as a backend-for-frontend. The only
process the browser talks to.

Endpoints:
- `GET /api/health` — aggregates `/api/v1/health` from HSS, MME, SPGW;
  returns one combined status object with per-NF detail.
- `GET|POST|PUT|DELETE /api/subscribers/*` — proxies to HSS admin API.
- `GET /api/events/stream` — proxies the collector's SSE stream
  **untransformed** (raw structured events on the wire; formatting in React).
  This preserves AI-consumability of the stream — same wire shape feeds the
  human UI and the Phase C diagnostic layer.
- `POST /api/simulator/start`, `POST /api/simulator/stop`,
  `POST /api/simulator/inject/{scenario}` — see B4.
- `GET /api/ran-config` — see B5.
- `GET /*` — serves embedded static frontend files (Go `embed`).

### B3 — Frontend
React + TypeScript + Tailwind CSS, bundled by esbuild (zero-config,
no Webpack/Vite). Three views:

- **Health landing** (Golden Path step 2). Per-NF status cards, aggregate
  "core is up" indicator, primary CTA: "Start simulator → run first
  attach."
- **Subscribers** (Golden Path step 3). List, add (with inline IMSI/Ki/OPc
  validation), delete. Each field labeled with one plain-language sentence.
- **Live trace** (Golden Path step 5). SSE-powered stream of events,
  formatted in human terms ("UE 001010000000001 sent Attach Request").
  Each event row expandable to show the raw structured payload.

  **A "Diagnose this" affordance is present but disabled** when the trace
  ends in an error, with a Phase C placeholder. This makes the load-bearing
  AI commitment visible in the UI from day one rather than as a future
  bolt-on (charter §9.5 and the Opus review).

### B4 — Built-in simulator + error injection
The existing S1AP/NAS test client (currently the harness for
`TestEndToEndAttachOverWire`) is promoted to a first-class, dashboard-
launchable component. We do not introduce srsRAN in Phase B (it is too
heavy for the TTFC budget; it remains a candidate for a later
"high-fidelity mode"). Charter D10 is honored: the simulator is a control-
plane tool, not an RF emulator, and we are surfacing an existing
implementation, not building a new one.

**Required error-injection scenarios** (these become the v1 test fixtures
the Phase C diagnostic layer is built against):

| Scenario | Failure produced | Maps to §9.1 cause space |
|----------|------------------|--------------------------|
| Wrong Ki on the UE side | RES mismatch → AUTH_FAILURE | "wrong Ki/OPc" |
| Wrong PLMN on the eNB side | S1 Setup rejected | "PLMN/TAC mismatch" |
| Unprovisioned IMSI | HSS returns subscriber-not-found | "unprovisioned SUPI" |
| Wrong MME address on the UE | Timeout, no response | "wrong AMF address" |

### B5 — RAN integration stub (Pillar 3, framing)
A read-only "Connect a real RAN" panel that displays the exact values
QCore is configured with (PLMN, TAC, MME address, ports, sample subscriber
credentials) in a copy-paste-ready text block for the most common
gNodeB/eNB config formats. Reconciliation against an actual RAN config
(catching mismatches) is Phase C; this is just the display piece.

The point: frame the simulator and the real RAN as the **two first-class
paths through Connect** (charter Golden Path step 4) from day one.
Otherwise the dashboard implicitly says "this product is a 5G core admin
panel," which is the positioning trap the charter exists to prevent.

## Build order rationale

B2 lands first so the rest has a backend to talk to. B1 follows once there
is something for docker-compose to add. B3 needs B2; we build the views in
order (health → subscribers → live trace) so each is demoable before the
next. B4 is last in the critical path because it exercises the whole
stack end-to-end. B5 can land any time after B3 (small, mostly static).

## Exit criterion (verbatim)

`docker compose up` → browser opens → health view is green → click "Start
simulator" → live trace view shows a full 4G attach completing → elapsed
time from `docker compose up` on a cold machine is under 5 minutes.

Additionally: each of B4's four error-injection scenarios produces an event
trace that ends in a structured `ErrorEvent` queryable by journey ID from
the collector. These traces are the input to Phase C.

## Out of scope (deferred to Phase C or later)

- The diagnostic AI itself (Phase C).
- srsRAN integration as a "high-fidelity mode" (Phase D candidate).
- 5G SA experience-layer wiring — done on the 5G SA Track once that
  track's exit criterion is met (charter D12).
- Authentication on the dashboard. Single-user local-tool assumption.
  Multi-user is Next-stage (charter §11).
- Scenario authoring/library (Golden Path step 7) — Phase D.
