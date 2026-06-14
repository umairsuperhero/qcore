# QCore Strategic Roadmap: From Prototype to Product

> **Status:** Strategic Roadmap · **Audited:** 2030 Retrospective Perspective · **Target Audience:** RAN/Device Developers

---

## 1. The Strategic Thesis

QCore's moat is **not** protocol completeness — Open5GS and free5GC will always have more Rel-17/18 protocol features. QCore's moat is **cellular developer experience (DX)**. Every roadmap decision is driven by a single question: *"Does this make a developer's first hour with cellular networking dramatically better?"*

We succeed by transforming cellular protocols from an opaque "telecom black box" into a web-like developer utility: **observable, self-explaining, and running locally in seconds.**

---

## 2. Strategic Context: Lessons from the Industry

To secure our position, QCore's roadmap is designed around the structural failures of open-source stacks, the commercial strategies of private cores, and the upcoming AI-native paradigms of 6G.

### 2.1 The Open-Source DX Gap
*   **YAML and Dependency Hell:** Open-source cores (Open5GS, free5GC, OAI-CN) require managing disjointed configurations across dozens of files. Setting them up requires complex Linux compilation, custom kernel modules (like free5GC's `gtp5g` which breaks on OS updates), and strict CPU instruction sets (AVX for MongoDB).
*   **Uncorrelated, Flat Logs:** Diagnosing a connection failure requires manually tracing signaling across independent Network Function (NF) log files or running `tcpdump` captures on virtual bridges.
*   *QCore's Strategy:* We provide a single-command dockerized launch, zero host-dependency compilation (pure Go, native SCTP), and a structured event model (Phase A) that maps all NF state changes to a single, correlated `journey_id` streamed live to a visual timeline.

### 2.2 Commercial Private 5G & The Hyperscaler Retreat
*   **API-First Moat:** Druid Software (Raemis) proved the value of building a core on top of a comprehensive RESTful API, allowing the management GUI and enterprise IT systems to control and monitor subscriber and slice states programmatically.
*   **The AWS/Azure Retraction (2025):** Both AWS Private 5G and Azure Private 5G Core retired their enterprise offerings. They failed because they locked customers into heavy, expensive proprietary edge hardware (AWS Outposts, Azure Stack Edge) and public cloud subscriptions, failing to solve the physical constraints of on-site RF deployment (spectrum licensing, radio placement) or the edge-to-cloud diagnostic blind spot.
*   *QCore's Strategy:* We remain hardware-agnostic and container-local (running on generic VMs or developer laptops, including macOS). We prioritize local simulation (UERANSIM/srsRAN) to bypass physical RF constraints during the first stages of development, and we implement a **tiered AI model (B2)** utilizing an offline local SLM (Qwen2.5-1.5B via llama.cpp) to run diagnostics in secure, air-gapped labs without cloud dependencies.

### 2.3 6G Ambitions (3GPP Rel-19/20)
*   **AI-Native SBA & Telemetry:** Upcoming 6G specifications (TR 23.801) embed AI directly into the NF control loops rather than utilizing add-on analytics layers (NWDAF). It defines native real-time telemetry fabrics as primary core services.
*   *QCore's Strategy:* Our Phase A event substrate aligns perfectly with this 6G vision. We treat structured events as first-class citizens, feeding our deterministic heuristics catalog and local SLM to enable automated, explainable troubleshooting.

---

## 3. Implementation Phases

The roadmap is divided into 4 sequential phases. Each phase acts as a credibility gate; skipping ahead destroys the trust required for developers to adopt QCore.

```
Phase 1: REAL ──→ Phase 2: SAFE ──→ Phase 3: PROVEN ──→ Phase 4: EXPAND
(4-6 weeks)        (2-3 weeks)       (ongoing)            (quarterly)

"It works with      "I trust it       "Others use it       "It grows with
 real data"          with real keys"    and validate it"     the specs"
```

### Phase 1: REAL — Close the Demo-to-Product Gap
*Goal: Every feature displayed on the dashboard must be wired to real backend network functions. No mocks or simulation overrides.*

*   **1.1 Un-Mock the Live Trace View:**
    *   Set `USE_MOCK_STREAM = false` in `traceStream.ts`.
    *   Wire the dashboard's `JourneyTimeline` to stream real events from `/api/events/stream`.
    *   Connect the UI's diagnostic panels to the real `/api/diagnostics/journey/{id}` endpoint.
*   **1.2 Resolve State Management & Dual Hooks:**
    *   Consolidate the dual `EventSource` connections by moving `useGNBConnection()` to a shared state provider (e.g., Zustand).
    *   Remove unused mock data files and dead components (`RANConnectView.tsx`).
*   **1.3 Fix CI to Build and Test All 5G Binaries:**
    *   Currently, CI only builds the 4G EPC. Expand CI to build all 5G network functions (`amf`, `smf`, `upf`, `ausf`, `udm`, `udr`, `nrf`) and run unit tests under `-race`.
*   **1.4 Validate UERANSIM Interop:**
    *   Run the external `ueransim-interop` workflow to prove registration, PDU session establishment, and GTP-U data plane ping (`uesimtun0`) on a clean Linux environment.
*   **1.5 Empirical TTFC/TTRC Measurements:**
    *   Implement baseline performance measurement scripts (`make measure`). Capture and publish cold-start Time to First Connection (TTFC < 5m) and Time to Root Cause (TTRC < 30s) in the README.

### Phase 2: SAFE — Security & Hardening
*Goal: Secure user key handling, protect cryptographic vectors, and harden protocol decoders against external attacks.*

*   **2.1 Cryptographic & Memory Hardening:**
    *   **Key Zeroing:** Zero out intermediate Milenage buffers (Ki, OPc) in Go memory immediately after authentication computations.
    *   **Constant-Time Comparisons:** Enforce `crypto/subtle.ConstantTimeCompare` for all RES/XRES/HXRES* authentication tokens.
    *   **SBI TLS:** Expose options for TLS encryption on inter-NF REST interfaces (e.g., AMF↔AUSF↔UDM).
*   **2.2 Decoder Robustness & Fuzzing:**
    *   Implement Go native fuzzing (`go test -fuzz`) for the hand-rolled APER NGAP and S1AP decoders to ensure malformed packets from rogue basebands return errors and never trigger nil pointer panics.
*   **2.3 Context Lifecycle Cleanup:**
    *   Verify complete garbage collection of UE contexts upon SCTP association teardown or NAS Deregistration to prevent memory leaks during long-running tests.

### Phase 3: PROVEN — Community & Validation
*Goal: Open-source community readiness and third-party developer adoption.*

*   **3.1 Community Infrastructure:**
    *   Add standard open-source repositories: `CONTRIBUTING.md`, issue/PR templates, `SECURITY.md`, and a clear license file.
*   **3.2 Documentation Reconciliation:**
    *   Mark the old PRD as superseded by `docs/experience-charter.md`.
    *   Create `docs/3gpp-tracking.md` to map our hand-rolled codecs and state machines to specific 3GPP TS versions, explicitly documenting supported procedures.
*   **3.3 First External User Test:**
    *   Onboard a third-party developer (e.g., IoT device developer or RAN engineer) to run the golden path from scratch. Document onboarding friction and refine configuration CLI parameters.

### Phase 4: EXPAND — Spec Evolution & Advanced Diagnostics
*Goal: Keep pace with the 3GPP specs and productize advanced network debugging.*

*   **4.1 Protocol Expansion (User-Driven):**
    *   Implement Xn/N2 Handover, UE Paging, and Service Requests based on UERANSIM multi-UE scenarios.
*   **4.2 Dashboard Debugging Tools:**
    *   **Step-Through Debugging:** Pause incoming signaling at key checkpoints (e.g., Security Mode Command), inspect raw parameters, and manually step/inject failures.
    *   **Comparative Tracing:** Highlight differences between a known-good connection trace and a failed trace.
*   **4.3 Extend Config Reconciliation:**
    *   Expand `POST /api/ran-config/reconcile` to accept and parse configuration profiles from commercial private RAN nodes (e.g., Baicells, Airspan) and open-source stacks (srsRAN).

---

## 4. Key Strategic Choices

1.  **Vite Migration:** Migrate the dashboard build pipeline from pure `esbuild` to `Vite`. HMR (Hot Module Replacement) and modern proxy configurations will significantly reduce iteration times on the visual trace components.
2.  **State Management:** Adopt `Zustand` to manage the central SSE event feed and shared NF connection states across the dashboard.
3.  **Minimum Credibility Gate:** Establish **Phase 1 (REAL) + UERANSIM Interop** as the baseline for open-sourcing loudly. A live video demonstrating UERANSIM registering and pinging through QCore with real-time visual diagnostics is our highest value marketing asset.
