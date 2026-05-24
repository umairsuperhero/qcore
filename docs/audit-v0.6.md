# QCore Codebase Audit — v0.6

> **Date:** 2026-05-23  
> **Scope:** Repository at v0.6, audited against the experience charter §11 "Now" scope  
> **Purpose:** Pre-Phase-A baseline required by CLAUDE.md before implementation begins  
> **Method:** Read-only survey of all source files, tests, docs, and configuration

---

## 1. Network Functions — Implementation Status

### 4G EPC

| NF | cmd/ binary | Status | What exists |
|----|-------------|--------|-------------|
| **HSS** | `cmd/hss/` | **Complete** | Milenage auth-vector generation (TS 35.208), subscriber CRUD, SQN management, S6a HTTP facade, PostgreSQL via GORM, Prometheus metrics, CLI (`subscriber add/list/auth/export`), auto-seeds TS 35.208 Test Set 1 subscriber on empty DB |
| **MME** | `cmd/mme/` | **Complete** | S1AP listener (port 36412), full 4G attach/detach/auth/security-mode flow, E-RAB setup, S11 HTTP client to SPGW, S6a HTTP client to HSS, Prometheus metrics, integration tests verified against mock eNB |
| **SGW/PGW** | `cmd/spgw/` | **Complete** | Collapsed SGW+PGW binary. GTP-U v1 codec, TEID pool, IPv4 UE address pool, S11 HTTP server, uplink decapsulation, TUN egress (Linux) / log egress (other platforms), Prometheus metrics |

4G EPC is the most complete part of the codebase. The integration test `TestEndToEndUserPlane` in `pkg/mme/` stands up real MME and SPGW processes, runs a Go-based mock eNB through full attach and uplink GTP-U, and passes. UERANSIM (4G mode) has been verified end-to-end per `docs/UERANSIM.md`.

**Known limitations in shipped 4G code:**

- `pkg/mme/handler_s1ap.go:939` — UE aggregate bit-rate is hardcoded 50 Mbps; real value from HSS subscription profile is not wired.
- `pkg/mme/mme.go:43` — fallback IP allocator used when SPGW is offline; marked "no real PGW yet — placeholder."
- `pkg/s1ap/decoder.go:219` — long-form ASN.1 length extension handling is present but noted as incomplete for rare cases.
- `pkg/spgw/pool.go` — IPv6 UE pools are not supported.
- S1AP/NGAP transport uses TCP length-prefix, not native SCTP. The `pkg/sctp/` abstraction layer exists (176 LOC) but only implements the TCP fallback; native SCTP via pion/sctp is planned but not started.
- Diameter is a REST facade over S6a, not a native Diameter stack.

---

### 5G SA

| NF | cmd/ binary | Status | What exists |
|----|-------------|--------|-------------|
| **AMF** | None | **Partial** | NGAP listener, 5G-NAS RegistrationRequest/Accept/SecurityMode/Auth procedures, AUSF and UDM client integration, gNB registration, basic UE context management. GUTI-based re-registration is a stub (`pkg/amf/nas.go:257`). No binary entrypoint yet. |
| **AUSF** | None | **Partial** | 5G-AKA auth-vector generation per TS 29.509, Nausf_UEAuthentication SBI endpoints, UDM client integration. No binary entrypoint; not network-exposed. |
| **UDM** | None | **Partial** | Nudm_SDM `am-data` endpoint, UEAU (`auth-data`) endpoint skeleton. **Two UEAU operations return HTTP 501 Not Implemented** (`pkg/udm/ueau.go:127`, `:159`). `pkg/udm/doc.go` explicitly marks UEAU as "stubbed for v0.6 when AMF + AUSF land." UDR chaining implemented. No binary entrypoint. |
| **UDR** | None | **Partial** | Nudr_DataRepository HTTP/2 endpoints, subscription-data storage in memory. Supports UDM-only access. No binary entrypoint; not network-exposed. |
| **NRF** | None | **Minimal** | In-memory NF registry, NFManagement register/deregister/query endpoints. No persistence, no heartbeat logic, no binary entrypoint. |
| **SMF** | None | **Not started** | Required for 5G session management; no code. |
| **UPF** | None | **Not started** | Required for 5G user plane; no code. PFCP (N4) codec also missing. |
| **PCF** | None | **Not started** | Deferred. |

**End-to-end 5G registration has not been verified against a real gNB or UERANSIM in 5G mode.** The AMF, AUSF, UDM, and NRF packages exist and the unit tests pass, but the full control-plane path—NGAP ↔ AMF ↔ AUSF ↔ UDM ↔ NRF—has no integration test equivalent to the 4G `TestEndToEndUserPlane`. The `docs/UERANSIM.md` guide covers 4G only and notes "5G NGAP partial" as a current limitation.

---

### 5G NSA

Not started. No code, no design documents. 5G NSA requires a working 4G EPC (which exists) plus X2/Xn interface handling and dual-connectivity coordination, none of which has been begun.

---

## 2. Web Dashboard

**Status: Not started.**

No frontend code exists anywhere in the repository — no JavaScript, TypeScript, HTML, CSS, or web framework. There is no `web/`, `ui/`, `frontend/`, or `dashboard/` directory. All user interaction today is via:

- REST API (curl against HSS/MME/SPGW HTTP ports)
- CLI subcommands (`qcore-hss subscriber add`, `qcore-mme status`, etc.)

The charter's Golden Path Steps 2–6 (land on a health dashboard, configure subscribers in the UI, connect a simulator with one click, watch registration live, read a diagnosis) all require a web frontend that does not exist yet.

---

## 3. Configuration Validation

**Status: Implemented and working.**

`pkg/config/` (313 LOC in `validate.go`) provides input-time validation that fires at load, not at runtime, consistent with charter principle 3. It:

- Aggregates all errors before failing (shows the full problem list, not just the first).
- Includes fix text with each error.
- Validates: port collisions, PLMN syntax (MCC 3 digits + MNC 2–3 digits), TAC non-zero, CIDR validity, gateway-in-pool containment, SCTP mode enum, URL scheme, database SSL mode enum, logging level and format enums.
- Supports environment-variable overrides (`QCORE_` prefix) via Viper.

Test coverage: `config_test.go` and `validate_test.go`. This area is in good shape.

**Gap:** Validation exists only for the fields present in `config.example.yaml`. There is no validation for 5G-specific configuration (AMF PLMN/S-NSSAI, DNN, UDM URL, etc.) because the 5G NFs have no binary entrypoints yet and no shared config struct.

---

## 4. Telemetry and Event Model

### What exists

**Structured logging:** `pkg/logger/` wraps Logrus. JSON and console formats are configurable. All NFs emit structured log lines with context fields. This is functional.

**Prometheus metrics:** `pkg/metrics/` (232 LOC) provides a registry pattern. Each of the three shipped NFs exposes a `/metrics` endpoint:

- HSS: auth-vector count, API request latency histograms, subscriber gauge.
- MME: attach request/success/failure counters, active-UE gauge, connected-eNB gauge, S1AP latency.
- SPGW: uplink/downlink packet and byte counters, session create/delete counters, active-session gauge.

### What does not exist

**There is no structured event model.** No event bus, no event types, no pub/sub, no SSE endpoint, no per-message structured record distinct from ad-hoc log lines. The CLAUDE.md Phase A requirement—"every signaling message, network-function state change, and config change emitted as structured, machine-readable events"—is not met.

This is the most consequential gap in the codebase. The event model is the substrate that the observability UI and the AI diagnostic layer both consume. Neither can be built well without it.

**No distributed tracing.** OpenTelemetry is referenced in planning documents but no tracing backend is wired, no spans are instrumented, and the dependency is not in `go.mod`.

---

## 5. Simulator Integration

**Status: Documented and manually verified in 4G mode; not bundled; 5G mode unverified.**

`docs/UERANSIM.md` provides a complete step-by-step guide for connecting UERANSIM (in EPC/4G mode) to QCore's MME and HSS. The 4G attach flow has been verified end-to-end. This is genuinely useful and the document is well-written.

**What is missing vs. charter §11 and CLAUDE.md Phase B:**

- UERANSIM is not bundled. It requires a separate install; there is no `docker-compose` service for it, no vendored binary, no one-click start.
- 5G NGAP mode with UERANSIM has not been verified. `docs/UERANSIM.md` lists "5G NGAP partial" as an explicit current limitation.
- No error injection or misconfiguration injection capability.
- No scriptable control interface (charter D10 calls for "a scriptable control-plane tool").
- No srsRAN integration for 4G (mentioned in CLAUDE.md Phase B; not started).

The charter decision D10 explicitly says QCore "bundles existing open-source simulators (UERANSIM for 5G; srsRAN or similar for 4G) rather than building its own." That bundling has not happened yet.

---

## 6. AI Features

**Status: Not started.**

No LLM integration, no diagnostic prompting system, no embedded SLM, no structured diagnostic knowledge layer (symptom→cause catalog), no cause-code decoder, no signaling narration. The charter's AI vision (§9) is fully articulated; the code is entirely absent.

This is expected given the CLAUDE.md build order ("Substrate before AI — the AI is only as good as the telemetry it reasons over"), but it means AI Levels 1 and 2, both called "Now" scope in charter §11, are not started.

---

## 7. Gap List — Charter "Now" Scope vs. Today

Charter §11 defines "Now" as: *Golden Path Steps 0–7, the five Experience Pillars, AI Levels 1–2, the embedded SLM, the built-in simulator.*

| Charter "Now" item | Status | Notes |
|----|--------|-------|
| Golden Path Step 1: one-command launch | **Missing** | No single `qcore` binary; requires starting HSS, MME, SPGW separately |
| Golden Path Step 2: health dashboard | **Missing** | No web frontend exists |
| Golden Path Step 3: guided subscriber + network config in UI | **Missing** | No web frontend; CLI only |
| Golden Path Step 4: built-in simulator, one-click start | **Missing** | UERANSIM not bundled; no one-click start |
| Golden Path Step 4: RAN config reconciliation | **Missing** | No implementation |
| Golden Path Step 5: live observability view | **Missing** | No structured event stream; no UI |
| Golden Path Step 6: AI diagnosis (what failed, why, the fix) | **Missing** | No AI layer |
| Golden Path Step 7: scenario authoring + execution | **Missing** | Deferred to Phase D |
| Experience Pillar 1: first-run / install to green dashboard | **Missing** | No dashboard |
| Experience Pillar 2: configuration in the UI | **Missing** | No UI; CLI config only |
| Experience Pillar 3: RAN/device integration, config reconciliation | **Missing** | Not started |
| Experience Pillar 4: live observability | **Missing** | No event model; no UI |
| Experience Pillar 5: diagnostic intelligence | **Missing** | No AI layer |
| AI Level 1 (Explain: cause-code decoding, signaling narration) | **Missing** | Not started |
| AI Level 2 (Diagnose: root-cause + proposed fix) | **Missing** | Not started |
| Embedded SLM (local, works offline) | **Missing** | Not started |
| Built-in simulator (UERANSIM bundled) | **Partial** | Verified 4G, not bundled, 5G unverified |
| 5G SA control plane at same maturity as 4G EPC | **Partial** | AMF/AUSF/UDM/UDR/NRF exist; UEAU stubs; no binary entrypoints; no end-to-end integration test |
| 5G SA user plane (SMF + UPF + PFCP) | **Missing** | Not started |
| Structured event model (substrate for UI and AI) | **Missing** | Phase A requirement; not started |
| Input-time configuration validation | **Done** | Working; 4G fields only |
| Prometheus metrics + structured logging | **Done** | Working across all three 4G NFs |
| 4G EPC: HSS + MME + SPGW | **Done** | End-to-end verified |
| Zero-config first launch | **Missing** | Requires manual config edits today |
| Native SCTP transport | **Missing** | TCP fallback only |

---

## 8. Summary Assessment

**4G EPC is done and well-tested.** HSS, MME, and SPGW implement the full 4G control and user plane, pass integration tests, and connect to UERANSIM. This is the strongest part of the codebase and a solid foundation.

**5G SA control plane is partially built but not production-ready.** The packages exist, the codec work is substantial, and the v0.6 commit wired AMF ↔ AUSF ↔ UDM together. But two UDM UEAU endpoints return 501, GUTI re-registration is stubbed, none of the 5G NFs have binary entrypoints or Docker containers, and no end-to-end 5G registration test has been run against UERANSIM or a real gNB. It is closer to a well-structured sketch than a working feature.

**The Phase A substrate requirements are the most critical gap.** The structured event model—the substrate that both the observability UI and the AI layer consume—does not exist. Until it is designed and implemented, Phase B (dashboard) and Phase C (AI) cannot be built soundly. This is also the item the charter is most explicit about: "design it model-friendly from day one" (§9.5), "every signaling message emitted as structured, machine-readable events" (CLAUDE.md Phase A). The current Prometheus metrics and Logrus logs do not meet this bar.

**Everything from the dashboard outward is unstarted.** The web UI, simulator bundling, AI, and one-command launch are all Phase B and C work. Nothing in those phases has begun.

**The codebase is honest and well-structured.** Stubs are marked as stubs. The code that is shipped is clean, tested, and purposeful. The architecture anticipates the right future shape. There is no misleading work-in-progress being counted as done.

---

*Audit produced 2026-05-23. Do not begin Phase A implementation until the product lead has reviewed this document.*
