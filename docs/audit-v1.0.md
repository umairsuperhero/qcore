# QCore Baseline Audit

**Document status:** Living baseline audit. Re-audited at every milestone and on a
recurring cadence (see *Audit cadence* below).
**Current revision:** v1.23 — 2026-06-15
**Auditor of record this revision:** P3.1 scenario authoring + P3.2 CI hooks combined
onto current main (`codex/p3-scenario-ci`, verified by Claude); frontend `tsc --noEmit`
+ `vite build`, then Docker `go build`/`vet`/`test -race`, plus a `qcore-cli`
exit-code proof (PASS→exit 0, FAIL→exit 1, `--json`).

---

## Revision log

| Rev | Date | Summary |
|-----|------|---------|
| v1.23 | 2026-06-15 | **P3.1 scenario authoring + P3.2 CI hooks complete.** *P3.1:* author, save, list, and re-run simulator scenarios with a deterministic PASS/FAIL + trace — `pkg/simulator` `ScenarioDefinition.Expect{result,cause,failed_step}` (optional pointer, back-compat `pass=null`); `pkg/dashboard/scenario_store.go` (config-driven `dashboard.scenario_dir` store, name-validated against path traversal, atomic writes); `pkg/dashboard/scenarios.go` (`GET/POST /api/scenarios`, `GET/DELETE /api/scenarios/{name}`, `POST /api/scenarios/{name}/run` with `CompareScenarioOutcome`); dashboard `ScenarioAuthoringPanel`. Runtime-proven earlier on a local 4G stack (`result:success`→`pass:true`; `wrong_ki` `result:failure`→`pass:true`, `failed_step: security_mode_command`). *P3.2:* stateless `POST /api/scenarios/run` runs a posted scenario without touching the store and returns the `CompareScenarioOutcome` contract; `qcore-cli test run --scenario <f> [--json]` calls it synchronously with a strict CI exit-code contract — **exit 0 on PASS, exit 1 on FAIL** — proven by running the built `qcore-cli` binary against the `/api/scenarios/run` contract: a passing `ScenarioRunResult` returned exit 0, a failing one exit 1, and `--json` emitted the payload. The server-side scenario execution + contract evaluation are covered by `TestScenarioHTTPRunDirect` and P3.1's live 4G runtime evidence. Verification for this revision: frontend `tsc --noEmit` + `vite build`, then — separately (dist embed race) — Docker `go build ./... && go vet ./... && go test -race ./...` green; live `qcore-cli` exit-code proof. Scope: dashboard/API + CLI adoption slice; P3.3 Learning Mode remains. |
| v1.22 | 2026-06-15 | **5G AUTS/SQN resynchronization is interop-validated (tier-3 for the bundled UERANSIM profile).** The crypto slice (PR #41, merged) replaced the UDM resync 501 with real reverse-Milenage: `F1Star`/`F5Star` recover SQN_MS from AUTS and verify MAC-S (AMF=0 per TS 33.102 §6.3.5), wired UDM→AUSF→AMF with a one-attempt `ResyncAttempted` guard. `pkg/subscriber/milenage_test.go::TestResyncAUTSRoundTrip` (added during review) pins the reverse-Milenage round-trip against all 5 official **TS 35.208** vectors, plus a tampered-MAC-S negative case. The interop branch `codex/auts-sqn-interop` then drove a **real UERANSIM** desync: a CI step forces the core SQN behind the live UE via the UDR PATCH endpoint (HTTP 204 required), triggers re-auth with `nr-cli deregister normal`, and gates on an **ordered** sequence — UE `Authentication Failure (SQN out of range)` → AMF `attempting SQN resynchronization` → collector `Authentication Request sent (Resync)` → UE registration success *after* the failure line (line-number ordering guard). Three real blockers were found and fixed along the way: AMF now accepts UE-originated deregistration (`4c36007`), keeps SUPI separate from the AUSF auth-context URL so resync sends `imsi-…` not a URI (`7bc55a6`), and resets NAS security counters after deregistration so the post-resync SMC verifies (`266c380`). Verification for this revision: first-hand Docker `go build ./... && go vet ./... && go test -race ./...` green (all 25 packages); `ueransim-interop` run `27529970131` prints `T10 SQN RESYNC PASS` with `T10 DATA PLANE PASS` intact. Scope note: 5G only; integer SQN with a +32 advance (not the TS 33.102 array scheme); 4G AUTS resync and per-target real-RAN replay remain follow-ups — this does **not** broaden T10 beyond the bundled UERANSIM profile. |
| v1.21 | 2026-06-14 | **P2.2 RAN/device config reconciliation complete.** Added `pkg/diag/reconcile.go`, `POST /api/ran-config/reconcile`, and the dashboard "Check my RAN config" panel. The reconciler compares UERANSIM gNB/UE YAML against QCore AMF/subscriber config and reports PLMN, TAC, S-NSSAI SST/SD, serving-network-name, IMSI, Ki, OPc/OP, DNN, and SUCI-scheme mismatches before attach without echoing raw secrets. Dashboard runtime check: changing the gNB PLMN to `001/02` reported `gnb.plmn` / `plmn_mismatch`, showed QCore `001/01`, and gave the exact fix. Verification for this revision: Docker `go test ./pkg/diag ./pkg/dashboard`, dashboard `npx tsc --noEmit --pretty false`, dashboard `npm run build`, `make verify-fast`, and Docker `go build ./... && go vet ./... && go test -race ./...` passed. The branch also hardens `pkg/smf`'s health test to avoid a fixed PFCP-port collision during package-parallel race tests. Scope note: this is a dashboard/API diagnostic slice; it does not broaden T10 beyond the bundled UERANSIM profile or add new protocol features. |
| v1.20 | 2026-06-13 | **P2.1 real-failure catalog rules complete.** `pkg/ai/catalog.go` now includes 9 deterministic UERANSIM/T10 interop-finding rules for the exact failures recorded in `docs/3gpp-tracking.md`: DownlinkNASTransport APER rejection, SMC K_AMF/SUPI-prefix integrity failure, InitialContextSetupRequest APER rejection, Registration Accept 5G-GUTI TLV-E length, UL NAS Transport shape, container-local SMF URL, missing PDU Session Establishment Accept, N2/N3 data-plane gap, and UPF TUN unavailability. Each rule returns Explanation/RootCause/Fix and is pinned by table-driven tests using stable `events.ErrorPayload` tags plus captured-message fallbacks. Verification for this revision: focused catalog tests passed in Docker; `docker run --rm -v "$PWD":/src -w /src -v qcore-gomod:/go/pkg/mod golang:1.23 go test -race ./pkg/ai/...` and `make verify-fast` passed. Scope note: this adds diagnoses for known real failures; it does not broaden real-RAN compatibility beyond the bundled T10 profile. |
| v1.19 | 2026-06-13 | **P1.2 B2 offline SLM live-serve validated.** Fixed the llama.cpp sidecar entrypoint to `/app/llama-server`, then ran `make up-ai` with the baked Qwen2.5-1.5B GGUF sidecar. Evidence: `curl http://localhost:8088/health` returned `{"status":"ok"}`; `/v1/models` reported `qwen2.5-1.5b-instruct`; a direct `/v1/chat/completions` call returned structured JSON; `docker run --network docker_default ... QCORE_AI_LIVE_LOCAL_URL=http://qcore-slm:8088/v1 go test ./pkg/ai -run TestB2_LiveLocalSLM -count=1 -v` passed; a collector-injected catalog-miss journey fetched through `http://localhost:3000/api/diagnostics/journey/{id}` returned populated `Explanation`/`RootCause`/`Fix` without the unavailable-model fallback; and the baked `docker-qcore-slm:latest` image answered on an internal Docker network (`--internal`) with no external egress. Scope note: this validates the local llama.cpp sidecar path and dashboard diagnostic API integration, not broad model-quality coverage. |
| v1.18 | 2026-06-13 | **P1.1 TTFC/TTRC measured.** Added `scripts/measure-ttfc-ttrc.sh` / `make measure` and ran `scripts/measure-ttfc-ttrc.sh --cold --output measurements/latest.json`. Evidence file: `measurements/latest.json`. Results from this checkout with Docker layer cache available: cold compose start to dashboard ready 76.253s; 4G simulator TTFC after dashboard ready 0.121s; 5G simulator TTFC after dashboard ready 1.245s; computed cold start + 4G TTFC 76.374s; computed cold start + 5G TTFC 77.498s. TTRC: `wrong_ki` 3.556s, `wrong_plmn` 0.177s, `unprovisioned_imsi` 0.183s, `wrong_mme_address` 3.545s. All measured P1.1 values are within the charter targets of TTFC < 5 min simulator and TTRC < 30s for known catalogued failures. Scope note: this is a cold compose/current-checkout measurement, not a fresh-clone/no-cache benchmark. |
| v1.17 | 2026-06-13 | **Post-v1 planning surface reconciled.** Added `docs/next-phases-plan.md` as the active executable plan now that T10 data-plane and the C2/C3 credibility gate are merged on `main`. Updated README, AGENTS/CLAUDE instructions, wiki, and the historical v1 gap-closure plan so future agents do not keep treating C2/C3 as the next critical path. New critical path: P1.1 TTFC/TTRC measurement, P1.2 B2 live offline-SLM validation, then real-failure catalog rules and RAN/device config reconciliation. Docs-only revision; no new product behavior is claimed. |
| v1.16 | 2026-06-13 | **C2/C3 credibility gate runtime-proven.** The Docker dashboard now launches the selected built-in simulator mode end-to-end from the hero/Live Trace controls: 4G happy path reached attach complete (`j-ec2d20ec-61fd-48fd-8c18-123d2eef409e`), 5G happy path reached registration complete over AMF NGAP/SCTP (`j-f0f1265e-0c48-4e08-aa73-bf6a1783da78`), and browser replay from the hero showed Live Trace raw logs populated by real `/api/events/stream` SSE events (`j-6ac891b9-0121-4853-a356-72fd82616233`). The `wrong_ki` failure scenario is also runtime-proven through the UI: it emits a real failed journey (`j-d909999e-fbdc-4865-9b74-c7fc37d40372`) and renders the Diagnostic AI report with a Ki/OPc root cause. Fixes made during replay: dashboard diagnostics now fetch the collector's real `/journeys/{id}/events` route, Docker passes native SCTP mode into the dashboard simulator, Linux SCTP resolves Docker DNS names, the 5G simulator emits a valid SupportedTA/S-NSSAI in NGSetup, decodes real Authentication Request layout, derives RES*, unwraps protected SMC for simulator validation, and tags injected failure events by scenario so the catalog can diagnose them deterministically. Verification performed this revision: focused Docker Go tests for `pkg/ai`, `pkg/dashboard`, `pkg/simulator`, `pkg/sctp`, `pkg/ngap`, `pkg/nas5g`, and `pkg/amf` passed; `npx tsc --noEmit --pretty false` and `make verify-fast` passed locally. GitHub CI remains the merge gate after push. Scope note: this ships the C2/C3 credibility-gate slice (real simulator UX + mode selection + SSE + diagnostics), not the broader later UDR/operator view. |
| v1.15 | 2026-06-11 | **C2/C3 credibility gate advanced; still not claimed as fully shipped.** The dashboard production source no longer references `traceStreamMock`, `getMockEvents`, `runScenario`, or `isMockStream`; the dead mock stream file was removed. The hero screen now has a global 4G EPC / 5G SA selector, shows the matching RAN endpoint, and launches happy-path or failure scenarios through the real backend simulator API. Hero, Health, and Live Trace all route simulator starts through the same store path backed by `/api/simulator/start`, `/api/simulator/inject`, `/api/simulator/status`, real `/api/events/stream` SSE events, and the real diagnostics path. App tab gating now allows a hero-launched simulator run to navigate directly to Live Trace and keep it visible while the trace is active, even before the connection state flips to connected. Verification performed this revision: `npx tsc --noEmit --pretty false` and `make verify-fast` passed, including dashboard typecheck and Vite build. Remaining proof before marking C2/C3 shipped: manual/browser UX replay against the running dashboard and acceptance evidence that the trace visible to the user is the backend simulator/SSE trace end-to-end. |
| v1.14 | 2026-06-08 | **C3/T9 dashboard 5G mode started; not yet shipped as a full milestone.** `/api/ran-config` now exposes first-class AMF/NGAP connection values (`amf_address`, `amf_ngap_port`, `amf_plmn`, `amf_tac`, `serving_network_name`) alongside the legacy 4G MME/S1AP values. The dashboard hero now derives its gNB endpoint from AMF/NGAP fields instead of MME/S1AP fields, and the API provides a UERANSIM gNB snippet keyed from the AMF configuration. Verification performed this revision: `make verify-fast` passed, including Docker Go targeted tests for `pkg/dashboard`, dashboard `tsc --noEmit`, and `vite build`. |
| v1.13 | 2026-06-08 | **C2/T8 simulator UX started; not yet shipped as a full milestone.** The dashboard now has separate 4G and 5G simulator templates, so 5G starts target the configured AMF NGAP endpoint instead of accidentally reusing the 4G MME S1AP listener. The built-in simulator accepts configured transport mode (`tcp` dev fallback or native `sctp`), Docker/dashboard config exposes `dashboard.amf_ngap_addr`, and the Health view renders real simulator mode/status/failure-step/journey data plus scenario buttons for wrong Ki, wrong PLMN, unknown IMSI, and timeout. The prior modal no longer claims to launch UERANSIM or force a simulated success after backend failure. Verification performed this revision: `make verify-fast` passed, including Docker Go targeted tests for `pkg/config`, `pkg/dashboard`, `pkg/simulator`, plus dashboard `tsc --noEmit` and `vite build`. |
| v1.12 | 2026-06-08 | **T10 shipped for the bundled UERANSIM Docker/cloud-Linux profile.** The final data-plane gate is green: QCore now sends NGAP `PDUSessionResourceSetupRequest`, decodes the gNB's `PDUSessionResourceSetupResponse`, forwards the gNB N3 tunnel to SMF, performs PFCP Session Modification into UPF, configures real TUN/NAT in the UPF container, and proves `ping -c 3 -I uesimtun0 8.8.8.8` from the UERANSIM UE. GitHub Actions `ueransim-interop` run `27115478758` records `T10 DATA PLANE PASS`; CI run `27115479708` is green. Scope is explicit: this validates the bundled UERANSIM v3.2.8 Docker profile on Linux/TUN, not a broad conformance matrix. |
| v1.11 | 2026-06-08 | **T10 now reaches external UERANSIM PDU session establishment; T10 still not shipped.** The AMF now relays a protected DL NAS Transport carrying a 5GSM PDU Session Establishment Accept after SMF returns `201`. The Accept includes the mandatory UERANSIM-visible IEs (Selected PDU session type + SSC mode, Authorized QoS rules, Session-AMBR, and PDU address) and is pinned by NAS golden/unit tests plus the AMF DL-count/security-header test. GitHub Actions run `27108387027` proves UERANSIM logs `PDU Session Establishment Accept received` and `PDU Session establishment is successful PSI[1]`; latest PR checks are green (`ueransim-interop` run `27108723209`, CI run `27108724052`). Current gap: no external UE→UPF→peer packet or ping is proven; the final T10 gate is data-plane validation with NGAP PDU Session Resource Setup, PFCP remote tunnel update, and UPF real TUN/NAT. |
| v1.10 | 2026-06-07 | **T10 now reaches external UERANSIM initial registration and AMF→SMF handoff; T10 still not shipped.** The post-InitialContextSetup UE abort was fixed by encoding Registration Accept Assigned 5G-GUTI IEI `0x77` as IE6/TLV-E with a two-byte length. The later UL NAS Transport blocker was fixed by routing decrypted protected NAS into `handleULNASTransport` and decoding UERANSIM's low-nibble payload container type plus IE1/IE3 optional fields. Compose now sets `QCORE_AMF_SMF_URL=http://smf:8002`, so AMF no longer falls back to container-local `localhost:8002`. GitHub Actions run `27080274240` proves UERANSIM logs `Initial Registration is successful`, AMF logs `Registration Complete — UE fully registered`, UERANSIM sends PDU Session Establishment Request, AMF forwards it, and SMF returns `201` on Create SM Context. Current gap: QCore has not yet sent PDU Session Establishment Accept back to UERANSIM, and no external PDU-session completion or data-plane ping is proven. |
| v1.9 | 2026-06-06 | **T10 InitialContextSetup APER blocker resolved on `codex/t10-initial-context-setup-aper`, T10 still blocked.** A traced UERANSIM replay captured the rejected InitialContextSetupRequest hex, then the branch fixed two narrow APER bugs: NGAP `BitRate` extensible constrained integer encoding for `UEAggregateMaximumBitRate`, and extension markers for the UE security-capability algorithm BIT STRINGs. The fixes are pinned by external-corpus/golden tests (`TestUEAggregateMaximumBitRateAPERGolden`, `TestUESecurityCapabilitiesAPERGolden`, `TestInitialContextSetupUERANSIMRejectedFixture`). GitHub Actions run `27057637533` confirms UERANSIM logs `Initial Context Setup Request received` and QCore logs `amf: InitialContextSetup confirmed by gNB`. Full registration is still not complete; current external blocker is post-InitialContextSetup UE failure (`ueransim-ue` exit 139 / gNB UE signal lost), with no PDU-session or data-plane claim. |
| v1.8 | 2026-06-06 | **T10 SMC-integrity blocker resolved on PR #28, T10 still blocked.** The K_AMF derivation path now strips the SBI `imsi-` prefix and feeds bare IMSI digits into the TS 33.501 A.7 K_AMF KDF input. The GitHub Actions `ueransim-interop` job validated this against real UERANSIM on cloud Linux: UE reaches Security Mode Command, selects integrity[2]/ciphering[0], does not log `Security Mode Command integrity check failed`, and AMF receives Security Mode Complete. Full registration is still not complete; current external blocker is Registration Accept delivery after SMC Complete. The gNB log now confirms APER `transfer-syntax-error` on QCore's `InitialContextSetupRequest`; adding mandatory `UEAggregateMaximumBitRate` is standards-correct but not sufficient by itself. |
| v1.7 | 2026-06-05 | **Post-antigravity integration sweep.** Audited the multi-agent (antigravity + codex) output and integrated it to main: Lane 1 (live dashboard un-mocked onto the real `/api/events/stream` SSE feed + real-engine diagnostics; esbuild→Vite; collapsed dual EventSource into one zustand store), Lane 2 (CI now builds all 13 binaries + `go vet`), Lane 4 (dark-theme consistency), and the T10 progress branch (honest UERANSIM replay docs + NGAP/NAS/SCTP fixes). Fixed a `tsc --noEmit` blocker on Lane 1 (dead `hasError`). Reconciled B2 status across README/wiki/CLAUDE — B2 code is merged + unit-tested green, live model-serve still pending (the docs previously under-claimed it as "planned"). Verified on the integrated tree: full `go build` / `go vet` / `go test -race ./...` green in `golang:1.23`; dashboard `tsc --noEmit` + `vite build` green. T10 remains blocked at Security Mode Command integrity (unchanged from v1.6). |
| v1.6 | 2026-06-04 | **Grounded T10 audit/replay.** Reproduced UERANSIM over native SCTP from clean compose. The original APER `transfer-syntax-error` at `DownlinkNASTransport` is fixed by UE-NGAP-ID and NAS Authentication Request encoding corrections; UERANSIM now reaches Authentication Response and AUSF confirmation. T10 remains **blocked**, not shipped: current external blocker is UERANSIM rejecting the Security Mode Command with `Security Mode Command integrity check failed`. Full Go build/vet/test, race, and dashboard build/tsc are green. |
| v1.5 | 2026-06-01 | **C1 5G telemetry landed (T7).** AUSF, UDM, SMF, UPF now emit journey-correlated Phase-A events at every key signaling step (auth request/vector/result, auth-data generation, PDU session, PFCP association/session). A 5G registration produces one correlated trace spanning AMF→AUSF→UDM with a shared `journey_id`, verified by `TestC1_RegistrationEventTrace`. Event schema documented in `docs/phase-a-event-model.md`. Full suite green. **Next: C2 (5G simulator UX) / B2 (offline SLM).** |
| v1.4 | 2026-06-01 | **B1 catalog depth landed + dashboard experience layer.** Diagnostic catalog deepened to 13 typed rules spanning the ≥9 §9.1 cause categories, 4G + 5G (PR #24). Dashboard gNB-connection hero screen (Gate 1 "is your gNB connected?", dark-first, latch-flip animation) and live signaling-trace view shipped. SBI `Server.Serve`/`Shutdown` data race fixed (was failing `go test -race`). Full suite green under `-race`. **Next gates: C1 (5G telemetry) on the 5G-leading path; B2 (offline SLM) on the AI path.** |
| v1.3 | 2026-05-30 | **Track A complete (A1–A4).** PLMN codec (D-1), real SUCI + Registration Reject (D-3), NRF lifecycle + discovery (D-2), N11 AMF→SMF + 5GSM UL NAS Transport (D-4). All PRs merged to main. All 24 packages green. |
| v1.2 | 2026-05-30 | **A1 landed.** `pkg/ident` canonical PLMN codec; `ngap.PLMNFromMCCMNC` and `subscriber.ParsePLMN` now both delegate to it. 7 golden test vectors (not just round-trips). All 24 packages green. D-1 closed. |
| v1.1 | 2026-05-29 | **Correction.** v1.0 claimed the 5G user plane / Phase C AI / 5G E2E test were "shipped and verified." They were uncommitted and had never compiled (~20 build errors + 2 real logic bugs). All fixed; full suite now green. Four interop gaps identified and turned into long-term decisions (D-1…D-4) + an Interop-Hardening track (I1–I4). |
| v1.0 | 2026-05-28 | Initial post-Phase-C audit. Overstated completion — superseded by v1.1. |

---

## 1. Executive summary

QCore's **4G EPC is real and end-to-end verified.** The **5G SA control and user plane
now pass both the in-process automated test** (AMF + AUSF + UDM + UDR + NRF + SMF + UPF,
mock gNB, Registration → PDU Session → GTP-U tunnel) **and the bundled external
UERANSIM Docker/cloud-Linux T10 replay**, and the
**Phase C Diagnostic AI** (catalog heuristics + optional Gemini escalation) is wired
into the dashboard. As of this revision **every package compiles, `go vet` is clean,
and the full `go test ./...` suite passes.**

The important correction over v1.0: that state did **not** exist when v1.0 was
written. The 5G user-plane work (`pkg/pfcp`, `pkg/smf`, `pkg/upf`), the AI
(`pkg/ai`), the native SCTP path (`pkg/sctp/sctp_linux.go`), and the T5 integration
test were all written but **never built** — they carried ~20 compile errors and two
genuine logic bugs. They have been repaired (see §2). The lesson is process, not
code: **"shipped" must mean "builds + tests pass in CI," never "code was written."**
This audit is now the living source of truth for that distinction.

## 2. What was repaired this revision

All of the following were uncommitted when found and are now fixed and green:

- **Won't-compile (mechanical):** wrong Go module path in `pkg/ai`; missing `strings`
  import; `unix.Pointer`→`unsafe.Pointer` + dead var in native SCTP; NGAP struct/field
  mismatches in `pkg/amf` and the 5G simulator; `logger.Fields`→`map[string]interface{}`
  in SMF/UPF; wrong GTP-U codec API in UPF + the integration test; a method defined
  *inside* `upf.NewService`; a call to a nonexistent `sbi.NewNRFClient`; UPF `nodeID`
  typed `[]byte` instead of `net.IP`; a missing argument to
  `pfcp.NewSessionEstablishmentResponse`.
- **Real logic bug — SMF IPAM:** the allocator wrapped on `nextIP.Mask(...)` instead of
  the pool's network address, so once the pool filled it handed out addresses *outside*
  the configured subnet. Fixed to wrap to the pool network address.
- **Test correctness:** `subscriber.ParsePLMN`'s test asserted a wrong expected value
  (the implementation is standards-correct); the SPGW TUN test only skipped on
  `EPERM`, not on a missing `/dev/net/tun` device node (`ENOENT`) — so it failed in
  any unprivileged container instead of skipping.

## 3. Component status (verified)

| Area | Status | Evidence |
|------|--------|----------|
| 4G EPC (HSS/MME/SPGW) | ✅ Shipped | `pkg/mme` E2E attach + user-plane tests pass |
| Phase A event model | ✅ Shipped (4G + 5G) | 4G NFs fully instrumented; 5G NFs instrumented via C1/T7 — one correlated trace per registration (`TestC1_RegistrationEventTrace`) |
| Phase B golden path / dashboard / simulator | ✅ Shipped | builds; frontend type-checks; simulator tests pass |
| 5G SA control plane | ✅ In-process E2E + bundled UERANSIM T10 replay pass | `pkg/amf` integration test green; UERANSIM completes registration and PDU session on cloud Linux (`27115478758`) |
| 5G SA user plane (SMF/UPF/PFCP) | ✅ In-process E2E + external data-plane ping pass | compiles; SMF/PFCP/UPF tests pass; UERANSIM UE ping over `uesimtun0` succeeds through UPF (`27115478758`) |
| Native SCTP | ✅ Linux path compiles + used by E2E | `pkg/sctp/sctp_linux.go`; macOS keeps TCP fallback |
| Phase C Diagnostic AI — catalog (§9.1) | ✅ Wired + deepened | `pkg/ai/catalog.go` = 28 rules, including 9 UERANSIM/T10 interop-finding rules, across ≥9 §9.1 categories, 4G+5G; table-driven tests pass (B1 + P2.1) |
| Phase C Diagnostic AI — offline SLM (§9.3) | ✅ Shipped for local sidecar path | `pkg/ai` local provider + `make up-ai` llama.cpp sidecar are live-validated with real GGUF, dashboard diagnostics API replay, and internal-network air-gap smoke test |
| Dashboard experience layer (hero + live trace) | ✅ Base shipped / ✅ C2/C3 credibility gate runtime-proven | gNB-connection hero screen (Gate 1) + animated live signaling-trace view; current C2/C3 branch removes frontend fake scenario traces and routes simulator controls to backend API + real SSE. Browser replay proves hero-launched 5G happy path raw SSE logs and UI-rendered `wrong_ki` Diagnostic AI output. |

## 4. Open interop gaps → long-term decisions

These are the four items that stand between "passes our own tests" and "a real RAN/UE
plugs in and it works." Each is recorded as a **long-term decision** framed by where
QCore must be by **2030**: the default local cellular core a RAN/device developer
reaches for, that interoperates flawlessly with real gNBs/UEs and whose **diagnostic
AI is the moat.** The wedge is real-RAN interop — so where a choice trades spec-fidelity
against internal convenience, **spec-fidelity wins**, because by 2030 every connection
is assumed to be a real, standards-compliant device.

### D-1 — One standards-correct identifier codec (PLMN first)
**Problem.** Two PLMN encoders disagree on byte order: `ngap.PLMNFromMCCMNC`
(non-standard) vs. `subscriber.ParsePLMN` (TS 24.008-correct). NGAP round-trips with
itself, so tests pass — but a standards-compliant gNB/UE (UERANSIM) would be misparsed.
**Decision.** Establish a **single, standards-correct codec** for PLMN — and over time
TAC, S-NSSAI, GUAMI, GUTI — used by 4G, 5G, and the subscriber store alike. Validate it
with **golden test vectors captured from real UERANSIM/srsRAN/Amarisoft**, not just
internal round-trips. Decommission the NGAP ordering.
**Why long-term.** Two private encoders that "agree with themselves" are the canonical
bug that passes CI and fails the first real device. Consolidating the primitive removes
a whole class of future interop failures and is a prerequisite for an honest T10
(UERANSIM compatibility) claim.

### D-2 — NRF as the discovery backbone; static config as the zero-config fallback
**Problem.** No NF actually registers with or discovers via the NRF; wiring is static
URLs. The "NRF-based discovery" claim (T1) is overstated.
**Decision.** Implement real **Nnrf_NFManagement** (register / heartbeat / deregister)
and **Nnrf_NFDiscovery** in `pkg/sbi`. Every NF registers on boot and discovers peers
via NRF — **emitting a Phase-A event for each registration and discovery** so the AI can
diagnose "SMF never registered" or "AMF couldn't find SMF." Keep static/env config as an
explicit override **and** as the deterministic default for the golden path.
**Why long-term.** SBA discovery is both the standards-correct design and a rich
observability surface that feeds the diagnostic moat. By 2030 users will add, swap, and
mock NFs; discovery must be real. But experience wins over purity — zero-config
fast-start (TTFC < 5 min) must never be gated on a discovery race, so discovery is
**layered, not mandatory.**

### D-3 — Real SUCI in the NAS layer and simulator (null-scheme first)
**Problem.** The 5G simulator sends a hardcoded placeholder SUCI; the
`unprovisioned_imsi` scenario therefore does not actually exercise unknown-subscriber
rejection.
**Decision.** Implement proper **SUCI** (5GS mobile identity) encode/decode in
`pkg/nas5g` — **null scheme (protection scheme 0) first** (the structured, standard form
and the common UERANSIM test default), with **ECIES Profile A/B planned behind the same
interface** as a later increment. Wire the simulator's configured IMSI through it so the
error scenarios test what they claim.
**Why long-term.** A test scenario that silently doesn't test what it advertises
violates "every error names its cause" and erodes trust in the simulator — and the
simulator's credibility *is* the product. Real-device registration needs correct SUCI
regardless of scheme.

### D-4 — Implement N11 (AMF→SMF) so the user plane is real end-to-end
**Problem.** The AMF does not forward PDU sessions to the SMF; the E2E test calls the
SMF REST endpoint directly to fake the AMF's step.
**Decision.** AMF performs **SMF selection** (via NRF once D-2 lands; static until then)
and calls **Nsmf_PDUSession Create SM Context** on PDU-session establishment, so the flow
is genuinely UE → gNB → AMF → SMF → UPF. Replace the test's shortcut with the real call.
**Why long-term.** The data plane is half of "test against a core." By 2030 a 5G core
that can't actually carry a PDU session through the real control flow isn't credible.
This closes the loop with D-2 (discovery) and D-3 (a fully registered UE then opens a
session).

## 5. Interop-Hardening track (I1–I4) — ✅ complete

These were required **before** claiming T10 (UERANSIM compat) or marking the 5G SA track
"shipped." All four are now done (Track A, merged to main):

| Step | Decision | Effort | State |
|------|----------|--------|-------|
| I1 | D-1 PLMN codec consolidation + golden vectors | Small | ✅ Done |
| I2 | D-3 SUCI null-scheme + wire simulator IMSI | Medium | ✅ Done |
| I3 | D-2 NRF register/discover + Phase-A events | Medium | ✅ Done |
| I4 | D-4 N11 AMF→SMF, real E2E (drop the shortcut) | Medium | ✅ Done |

T7 (5G event instrumentation, C1) is also complete, so Phase C can reason over real 5G
traces. **T10 is shipped for the bundled UERANSIM Docker/cloud-Linux profile:** UERANSIM
accepts NGSetup, registration, PDU Session Establishment Accept, NGAP PDU Session
Resource Setup, PFCP remote tunnel update, and data-plane traffic; `ping -c 3 -I
uesimtun0 8.8.8.8` succeeds in GitHub Actions run `27115478758`. C2/C3 credibility-gate
UX is also merged and runtime-proven. The active post-v1 critical path is now captured in
`docs/next-phases-plan.md`: TTFC/TTRC measurement, B2 live offline-SLM serving, and
P2.1 real-failure catalog rules are now validated, and P2.2 RAN/device config
reconciliation is shipped. The next critical path is Phase 3 workflow adoption, starting
with scenario authoring.

## 6. Deferred (unchanged from charter §11)

Carrier-scale performance/HA; embedded-core productization; protocol feature-count
parity with open5GS/free5GC; AI Levels 3–4; team/hosted features. The decisions above are
interop-correctness, **not** feature-count parity — they stay in scope.

## 7. Audit cadence

This document is **living**, not a one-time artifact.

- **On every milestone** (a T-/I-step or phase landing): bump the revision log, re-verify
  the build/vet/test claims, and reconcile §3 against reality before closing the session.
- **Recurring sweep:** on a fixed cadence (default: weekly), re-run
  `go build ./... && go vet ./... && go test ./...` (in the `golang:1.23` container — no
  Go toolchain on the host) and `tsc --noEmit` for the dashboard, and update §3 + the
  revision log if anything drifted. `docs/wiki.md` is refreshed in the same pass.
- **Trust rule:** never mark a row ✅ on the strength of code existing. Mark it ✅ only
  when build + vet + tests pass in CI.

> Build/test command (host has no Go):
> `docker run --rm -v "$PWD":/src -w /src -v qcore-gomod:/go/pkg/mod golang:1.23 sh -c "go build ./... && go vet ./... && go test ./..."`
