# QCore

[![CI](https://github.com/umairsuperhero/qcore/actions/workflows/ci.yml/badge.svg)](https://github.com/umairsuperhero/qcore/actions/workflows/ci.yml)

**The open-source 4G/5G core network that's actually easy to use.**

> Updated: 2026-06-08

QCore is a development and test environment for cellular networks — **not** a 5G core competing on protocol features. Primary user: the RAN/device developer who needs a core to test against. QCore wins on experience: fast start, deep observability, and AI that explains failures.

See the [Product Experience Charter](docs/experience-charter.md) for the full vision, persona, and North Star. **Read it before any product or design work.**

---

## Project Status

| Track | What shipped | Status |
|-------|-------------|--------|
| 4G EPC | HSS (subscriber management + Milenage), MME (S1AP/NAS attach, auth, security mode), SPGW (GTP-U, S11, Linux TUN egress). End-to-end attach + uplink verified. | ✅ Shipped |
| Phase A — Event model | `pkg/events` structured event schema, journey-ID correlation, HTTP emitter. `cmd/qcore-collector` SSE stream + journey store. All 4G NFs instrumented. | ✅ Shipped |
| Phase B — Golden Path | `make up` one-command launch. Web dashboard (port 3000): health view, subscriber management, live event trace, RAN-connect config panel. Built-in S1AP/NAS simulator with 4 error-injection scenarios. | ✅ Shipped |
| 5G SA Track | AMF/AUSF/UDM/UDR/NRF/SMF/UPF + PFCP/N4 codec all built. Control **and** user plane pass an in-process E2E test (Registration → PDU session → GTP-U tunnel). Interop hardening and 5G Phase-A telemetry are complete. **T10/UERANSIM is partially reproduced, not shipped:** native SCTP, NGSetup, InitialUEMessage, Authentication Request/Response, AUSF confirmation, Security Mode Control, InitialContextSetup, Registration Accept/Complete, AMF→SMF Create SM Context, protected PDU Session Establishment Accept, and UERANSIM PDU session establishment now work against real UERANSIM on cloud Linux. Current external gap: no UE→UPF→peer data-plane packet or ping is proven. Plan: `docs/v1-gap-closure-plan.md`; evidence: `docs/ueransim-compat.md`. | 🔭 In progress |
| Phase C — Diagnostic AI | Symptom→cause catalog deepened to 13 typed rules across ≥9 cause categories (4G + 5G); AI Level 1 (explain) + Level 2 (root-cause + fix); optional cloud (Gemini) escalation. **Offline embedded SLM (B2): code merged + unit-tested green** (`pkg/ai` local provider, `make up-ai` llama.cpp sidecar with a baked Qwen2.5-1.5B GGUF; catalog still runs first, the SLM only handles misses over the same grounded prompt). Live model-serve validation (real GGUF pull / air-gapped render) not yet run. | 🔭 In progress |
| Dashboard experience layer | gNB-connection hero screen ("is your gNB connected?", dark-first, latch-flip animation) + animated live signaling-trace view with progressive disclosure. **Now runs on real `/api/events/stream` data (un-mocked); live failures are decoded by the real diagnostic engine. Build migrated esbuild→Vite.** | ✅ Shipped |
| Phase D — Workflow adoption | Scenario authoring, CI hooks, Learning Mode. | 🔭 In progress |

---

## Roadmap — where we're headed

QCore's bet: **win on developer experience, ship 5G-SA-leading.** Not a protocol
feature-count race against open5GS/free5GC — a core that's fast to run, deeply
observable, and explains its own failures. The full vision is in the
[Experience Charter](docs/experience-charter.md); the executable plan is
[`docs/v1-gap-closure-plan.md`](docs/v1-gap-closure-plan.md).

**Where we are now:** the 4G EPC is end-to-end verified; the 5G SA control + user plane
pass an in-process E2E test over native SCTP; the diagnostic catalog and the offline AI
(B2) are merged; the live dashboard runs on real data.

**The path to v1 (two parallel tracks):**
1. **5G-leading headline — real-RAN validation (T10).** Get a real UERANSIM gNB/UE to
   register and pass data through QCore. *Current gate:* data-plane proof: NGAP PDU
   Session Resource Setup, PFCP remote tunnel update, UPF real TUN/NAT, and a UE ping.
   The prior DownlinkNASTransport APER, SMC-integrity, InitialContextSetup APER,
   Registration Accept, UL NAS Transport, AMF→SMF URL, and PDU Session Establishment
   Accept blockers are validated fixed against real UERANSIM on cloud Linux. Until
   ping passes through UPF, the 5G track is **not** "shipped" and we don't claim full
   UERANSIM compatibility. Then: 5G simulator UX (C2/T8) and dashboard 5G mode (C3/T9).
2. **The AI moat.** Catalog (shipped) + offline SLM (code merged); next is live
   model-serve validation, then deeper root-cause diagnosis.

**Then (Phase D):** scenario authoring, CI hooks, Learning Mode. Conformance & interop
status lives in [`docs/3gpp-tracking.md`](docs/3gpp-tracking.md);
the living audit is [`docs/audit-v1.0.md`](docs/audit-v1.0.md).

> **Honesty note:** rows marked ✅ above mean *build + vet + tests pass* (and, where
> stated, our in-process E2E test). "Validated against a real external gNB/UE" is a higher
> bar, tracked separately as **T10**. We don't blur the two.

---

## Quick Start

```bash
git clone https://github.com/umairsuperhero/qcore
cd qcore
make up          # builds images and starts all NFs + collector + dashboard
```

Open **http://localhost:3000** — the dashboard shows system health and lets you add a subscriber, fire the built-in simulator, and watch the live event trace.

The demo subscriber (3GPP TS 35.208 Test Set 1) is seeded automatically on first run.

### Simulator

From the Health view: click **Start simulator** once all NFs are green. The simulator runs a real S1AP/NAS attach (Milenage, derived KASME, Security Mode) and the trace appears live in the dashboard.

Error injection — from the Live Trace view or directly:

```bash
# Wrong Ki (auth failure)
curl -X POST http://localhost:3000/api/simulator/inject/wrong_ki

# Wrong PLMN (rejected at MME)
curl -X POST http://localhost:3000/api/simulator/inject/wrong_plmn

# Unprovisioned IMSI
curl -X POST http://localhost:3000/api/simulator/inject/unprovisioned_imsi

# Wrong MME address (connect failure)
curl -X POST http://localhost:3000/api/simulator/inject/wrong_mme_address
```

### Manual API

```bash
# Health check
curl http://localhost:8080/api/v1/health

# Add a subscriber
curl -X POST http://localhost:8080/api/v1/subscribers \
  -H "Content-Type: application/json" \
  -d '{
    "imsi": "001010000000001",
    "ki":   "465b5ce8b199b49faa5f0a2ee238a6bc",
    "opc":  "cd63cb71954a9f4e48a5994e37a02baf",
    "amf":  "8000"
  }'

# Generate an authentication vector
curl -X POST http://localhost:8080/api/v1/subscribers/001010000000001/auth-vector
```

---

## Why QCore?

The existing open-source cores (open5GS, free5GC, Magma) are powerful but brutal to learn. You fight YAML, TS specs, and undocumented assumptions before you get a single attach.

QCore's thesis: **developer experience is the differentiator**.

- **One command** to a running core, not fifteen.
- **Sensible defaults** that work out of the box — no PLMN archaeology.
- **Honest error messages** that tell you what's wrong and how to fix it.
- **Test-vector-verified crypto** you can actually trust (3GPP TS 35.208).
- **AI that explains failures** — not log dumps, actual root-cause analysis.
- **Pure Go**, static binary, no C dependencies, no kernel modules.

---

## Architecture

```
+-----------+         +----------+         +----------+
|  eNodeB   |---S1--->|   MME    |---S6a-->|   HSS    |
+-----------+         +----------+         +----------+
      |                    |
      |                   S11 (HTTP/JSON)
      |                    |
      |               +----v-----+
      +---- S1-U ---->|  SPGW    |--SGi--> Internet
           (GTP-U)    +----------+

All NFs emit structured events to:

+------------+      SSE      +------------+
| Collector  |<-----------+->| Dashboard  |  http://localhost:3000
+------------+              +------------+
```

Every signaling message and NF state transition is a structured, correlated event. The dashboard live-trace view streams these in real time. Phase C's diagnostic AI reasons over this same event stream.

---

## Build from Source

```bash
# Prerequisites: Go 1.23+, Docker
make build        # Build all binaries to bin/
make test         # Run all tests with race detector
make lint         # Run golangci-lint
make web          # Rebuild the React frontend bundle (requires Node 20+)

# Start the full stack
make up           # docker compose up -d --build
make down         # docker compose down
```

End-to-end attach over the wire:

```bash
go test -v -run TestEndToEndAttachOverWire ./pkg/mme/
go test -v -run TestEndToEndUserPlane ./pkg/mme/
go test -v -run TestSimulatorHappyPath ./pkg/simulator/
```

---

## API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET`    | `/api/v1/subscribers` | List subscribers |
| `GET`    | `/api/v1/subscribers/{imsi}` | Get subscriber by IMSI |
| `POST`   | `/api/v1/subscribers` | Create subscriber |
| `PUT`    | `/api/v1/subscribers/{imsi}` | Update subscriber |
| `DELETE` | `/api/v1/subscribers/{imsi}` | Delete subscriber |
| `POST`   | `/api/v1/subscribers/{imsi}/auth-vector` | Generate auth vector |
| `POST`   | `/api/v1/subscribers/import` | Import from CSV |
| `GET`    | `/api/v1/subscribers/export` | Export to CSV |
| `GET`    | `/api/v1/health` | Health check |
| `GET`    | `:9090/metrics` | Prometheus metrics |

Dashboard API (port 3000):

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET`  | `/api/health` | Aggregated NF health |
| `GET`  | `/api/ran-config` | RAN config snippets |
| `GET`  | `/api/events/stream` | SSE event stream |
| `GET`  | `/api/journeys` | Journey list |
| `POST` | `/api/simulator/start` | Start simulator |
| `POST` | `/api/simulator/inject/{scenario}` | Error injection |

---

## Configuration

See [config.example.yaml](config.example.yaml) for all options. Environment variables override config values with the `QCORE_` prefix:

```bash
export QCORE_DATABASE_HOST=db.example.com
export QCORE_DATABASE_PASSWORD=secret
export QCORE_LOGGING_LEVEL=debug
```

---

## Contributing

QCore is early. Issues, ideas, and PRs welcome — especially around developer experience papercuts. If something confused you, that's a bug.

## License

Apache 2.0 — see [LICENSE](LICENSE).
