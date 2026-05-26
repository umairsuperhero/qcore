# 5G SA Track

> **Status:** plan · started 2026-05-25 after Phase B shipped
> **Sequencing:** runs before Phase C (the diagnostic AI). See the project
> memory `[[project-build-sequencing]]` and `docs/charter-v0.5-open-issues.md`
> Issue 5 for why.
> **Exit criterion:** a 5G SA end-to-end test passes against QCore — gNB→AMF
> registration + PDU session establishment that creates a working GTP-U
> tunnel. Phase C work then runs over a substrate that covers both 4G and 5G.

## Audit at start (2026-05-25)

Re-checked vs. the v0.6 audit (`docs/audit-v0.6.md`, 2026-05-23). Phase A
and B did not touch the 5G NFs, so the v0.6 picture still holds:

| Component | State |
|-----------|-------|
| `pkg/ngap` | ~2600 LOC. Codec exists. Used by AMF. |
| `pkg/nas5g` | ~1200 LOC. Codec exists. Used by AMF. |
| `pkg/amf` | ~900 LOC (excl. tests). NGAP listener, registration flow, key derivation. `TestAMF_RegistrationFlow` passes. **No binary entrypoint.** No Phase A events. |
| `pkg/ausf` | ~430 LOC. 5G-AKA create + confirm. Tests green. **No binary entrypoint.** No Phase A events. |
| `pkg/udm` | ~700 LOC (excl. tests). Nudm_SDM + Nudm_UEAU + UECM. Tests green. **No binary entrypoint.** No Phase A events. |
| `pkg/udr` | ~390 LOC. AM-data + authentication-subscription. Tests green. **No binary entrypoint.** No Phase A events. |
| `pkg/nrf` + `pkg/sbi/nrf` | ~580 LOC. In-memory registration / discovery. **No binary entrypoint.** No Phase A events. |
| `pkg/smf` | **Does not exist.** |
| `pkg/upf` | **Does not exist.** |
| `pkg/pfcp` | **Does not exist.** |
| `pkg/sctp` | TCP fallback only — real gNBs (UERANSIM included) speak NGAP over native SCTP. |

## Build order

### T1 — Make the existing 5G NFs runnable
- Add `cmd/amf`, `cmd/ausf`, `cmd/udm`, `cmd/udr`, `cmd/nrf` (same shape as
  cmd/hss / cmd/mme / cmd/spgw — cobra root + `start` + `version`).
- Add Dockerfiles and docker-compose entries.
- Add config sections to `pkg/config/config.go` and `config.example.yaml`.
- Wire NF-to-NF discovery via NRF: each NF registers itself on startup;
  each NF looks up its dependencies via the NRF.
- After T1 we can `make up` and the 5G control plane is reachable, even
  though no UE can do anything useful yet (no SMF/UPF).

### T2 — Build the PFCP/N4 codec
- New `pkg/pfcp` package.
- Encode/decode for the PFCP messages needed for session establishment:
  Heartbeat Req/Resp, Association Setup Req/Resp, Session Establishment
  Req/Resp, Session Modification Req/Resp, Session Deletion Req/Resp.
- IE codec for the load-bearing IEs: F-TEID, PDI, FAR, PDR, QER, URR,
  Apply Action, Forwarding Parameters.
- Tests against known-good PFCP message bytes.

### T3 — Build SMF (5G session management)
- New `pkg/smf` package + `cmd/smf` binary.
- Endpoints: Nsmf_PDUSession (`POST /sm-contexts`, `POST /sm-contexts/{id}/modify`).
- Behavior: on a Create SM Context from AMF, fetch SM data from UDM,
  establish a PFCP session with UPF (allocate TEIDs, install PDR/FAR),
  return PDU session info to AMF.
- Tests: end-to-end Create SM Context against a mock UPF.

### T4 — Build UPF (5G user plane)
- New `pkg/upf` package + `cmd/upf` binary.
- PFCP endpoint (N4) accepting Association + Session messages from SMF.
- GTP-U (N3) listener — same model as SPGW's GTP-U: log egress for
  dev, TUN egress for real packet forwarding.
- Forwards based on installed PDR/FAR rules.
- Tests: PFCP session establishment + GTP-U packet round-trip.

### T5 — 5G end-to-end test
- `pkg/amf/integration_test.go` (or `pkg/smf/integration_test.go`):
  spin up AMF + AUSF + UDM + UDR + NRF + SMF + UPF in-process; run a mock
  gNB through full Registration + PDU Session Establishment; verify a
  GTP-U tunnel is installed.
- Equivalent in coverage to `pkg/mme/integration_test.go::TestEndToEndAttachOverWire`.
- **This is the Track's exit criterion.**

### T6 — Native SCTP transport
- Replace the TCP fallback in `pkg/sctp` with kernel-level SCTP on Linux
  (`syscall.AF_INET`, `syscall.SOCK_SEQPACKET`, `syscall.IPPROTO_SCTP`).
- Mac falls back to TCP since macOS has no native SCTP — but with a clear
  warning in startup logs that the deployment is dev-mode-only.
- Verify a real UERANSIM gNB attaches against the QCore AMF.

### T7 — Phase A event instrumentation for 5G
- Add 5G payload types in `pkg/events`: `NGSetupPayload`,
  `RegistrationRequestPayload`, `AuthRequestPayload5G` (or reuse 4G one),
  `SecurityModeCommandPayload5G`, `RegistrationAcceptPayload`,
  `PDUSessionEstablishmentPayload`, `PFCPAssociationPayload`,
  `PFCPSessionEstablishmentPayload`.
- Wire `events.Emitter` into every 5G NF using the same setter pattern as
  the 4G work in Phase A.
- Journey ID propagation via the `X-QCore-Journey-ID` header on SBI calls;
  the AMF mints a journey ID at first UE contact (same as the MME).

### T8 — 5G control-plane simulator
- New `pkg/simulator5g` package (or 5G mode on `pkg/simulator`).
- Speaks NGAP + NAS-5G to the AMF; computes RES* via 5G-AKA.
- Same shape of error-injection scenarios as the 4G simulator: wrong K,
  wrong PLMN, unprovisioned SUPI, wrong AMF address, plus 5G-only ones
  (wrong slice / S-NSSAI, unsupported DNN).

### T9 — Dashboard 5G mode
- Protocol mode selector (4G / 5G) on the dashboard.
- 5G simulator buttons in the live-trace view.
- Subscribers view talks to UDR for 5G (currently only HSS for 4G).
- RAN-connect view shows gNB config snippets for UERANSIM and srsRAN-5G.

### T10 — UERANSIM compatibility verification
- Run UERANSIM gNB+UE in a sidecar container against the full QCore stack.
- Document any spec divergences. This is the truth-test that "QCore speaks
  5G to a real-world simulator," not "QCore speaks 5G to QCore."

## Sequencing inside the track

T1 → T2 → T3 → T4 → T5 is the critical path that hits the exit criterion.

T6 (native SCTP), T7 (events), T8 (5G sim), T9 (dashboard), T10 (UERANSIM)
all attach after T5 and can land in whichever order is most useful. T7
(event instrumentation) should land before Phase C starts.

## Out of scope (deferred)

- Network slicing beyond the single default S-NSSAI used in the happy
  path (Phase D candidate).
- Roaming, handover (mobility), and inter-AMF reallocation.
- PCF, NEF, NSSF, SMSF, and other downstream 5G NFs.
- IPv6 / dual-stack PDU sessions.
- N9 (UPF-to-UPF) — single-UPF only.
