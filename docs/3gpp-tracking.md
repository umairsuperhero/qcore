# 3GPP Conformance & Interop Tracking

> Living reference. Maintained on the quarterly interop check-in cadence (see bottom)
> and updated whenever a real-RAN interop finding lands.
> Last updated: 2026-06-07

## Principle: relevance, not parity

QCore does **not** chase 3GPP feature-count parity with open5GS/free5GC — the charter
(§11) explicitly defers that. This document tracks the **subset of 3GPP that the wedge
needs**: the protocol surface required for control-plane interop with the RAN/UE
simulators and real devices QCore's users actually test against (UERANSIM, srsRAN, real
basebands). New 3GPP features are added **only when an interop gap demands them**, not
because the spec grew. Each gap is discovered demand-first (e.g. T10 surfaced the K_AMF
bug) and recorded in the Interop Findings log below.

**Trust-rule legend:**
- ✅ **In-process** — implemented and passing QCore's own automated E2E test.
- 🔭 **Real-RAN pending** — implemented but not yet validated against an external gNB/UE.
- ➖ **Simplified** — intentionally not spec-faithful (a dev-environment shortcut); noted.
- ❌ **Not implemented.**

Target baseline: the **Rel-15/16 SA control-plane subset** needed for simulator interop.
This is not a claim of Release completeness.

## 5G SA

| Interface / Spec | Implemented surface | Status |
|---|---|---|
| **NGAP** (TS 38.413) | NG Setup, InitialUEMessage, DL/UL NAS Transport, Initial Context Setup, PDU Session Resource Setup | ✅ in-process · 🔭 real-RAN registration path validated |
| **NGAP transport** | Native Linux kernel SCTP (`pkg/sctp/sctp_linux.go`, no CGO/pion); TCP fallback on non-Linux for dev | ✅ in-process · 🔭 real-RAN registration path validated |
| **NAS-5G MM** (TS 24.501) | Registration Request/Accept/Complete, Authentication Request/Response/Failure, Security Mode Command/Complete, Registration Reject | ✅ in-process · 🔭 real-RAN initial registration validated |
| **NAS-5G SM** (TS 24.501) | UL NAS Transport carrying 5GSM PDU Session Establishment. DL NAS Transport carrying PDU Session Establishment Accept is not yet implemented. | ✅ UL path real-RAN validated · 🔭 DL accept pending |
| **5G-AKA** (TS 33.501) | Full AKA: AUSF/UDM vector generation, RES* verify, K_AUSF/K_SEAF/K_AMF chain | ✅ in-process |
| **Key derivation** (TS 33.501 Annex A) | K_AMF (A.7), K_NASint/K_NASenc (A.8), K_gNB (A.9). **K_AMF P0 = bare IMSI** (fixed; see Findings) | ✅ in-process |
| **NAS security algorithms** | 128-NIA2 (AES-CMAC) integrity; NEA0 (null) ciphering | ✅ in-process |
| **SUCI** (TS 33.501 / 23.003) | Null-scheme (protection scheme 0) decode + genuine reject of unsupported schemes | ✅ in-process · ❌ Profile A/B (ECIES) |
| **SBI** (TS 29.5xx) | N8/N12/N13 (AUSF↔UDM↔UDR), N11 (AMF↔SMF), NRF register/discover (Nnrf) — HTTP/JSON | ✅ in-process · ➖ simplified service set |
| **PFCP / N4** (TS 29.244) | Association Setup, Session Establishment | ✅ in-process · 🔭 real-RAN |
| **GTP-U / N3** (TS 29.281) | Tunnel establishment + egress (Linux TUN) | ✅ in-process |
| **5G-GUTI re-registration** | — | ❌ Not implemented (GUTI path stubbed) |

## 4G EPC

| Interface / Spec | Implemented surface | Status |
|---|---|---|
| **S1AP** (TS 36.413) | S1 Setup, Initial UE / Attach, DL/UL NAS Transport, Initial Context Setup | ✅ in-process (E2E attach + uplink verified) |
| **NAS-EPS EMM/ESM** (TS 24.301) | Attach, Authentication, Security Mode, ESM default bearer | ✅ in-process |
| **Security** (TS 33.401) | Milenage (TS 35.205/206), KASME, 128-EIA2 / EEA0 | ✅ in-process · test-vector-verified (TS 35.208 Test Set 1) |
| **GTP-U / S1-U** (TS 29.281) | Tunnel + Linux TUN egress | ✅ in-process |
| **S11 (MME↔SGW)** | HTTP/JSON control | ➖ **simplified** — not GTPv2-C on the wire |
| **S6a (MME↔HSS)** | HTTP control | ➖ **simplified** — not Diameter on the wire |

> The S11/S6a simplifications are deliberate dev-environment choices, not gaps to "fix"
> toward parity. They only matter if a user needs to interop QCore's EPC with an external
> MME/HSS over the real interfaces — which is outside the current wedge.

## Interop Findings log

Real-RAN interop surfaces conformance gaps the in-process test can't. Each is logged here
with the spec reference and the fix.

| Date | Finding | Spec | Resolution |
|---|---|---|---|
| 2026-06-04 | UERANSIM rejected `DownlinkNASTransport` with an APER `transfer-syntax-error` | TS 38.413 | Fixed (UE-NGAP-ID + NAS Auth Request encoding) — merged |
| 2026-06-05 | UERANSIM rejects the Security Mode Command (`integrity check failed`). Root cause: K_AMF derived from the SBI `imsi-<digits>` string instead of the bare IMSI | TS 33.501 A.7 | **Fixed on PR #28 and validated by `ueransim-interop` on GitHub Actions cloud Linux.** UE accepts SMC and AMF receives Security Mode Complete. |
| 2026-06-06 | After SMC Complete, AMF sends Registration Accept but UE never receives it; T3510 expires. The UERANSIM gNB reports APER `transfer-syntax-error` on QCore's `InitialContextSetupRequest`. | TS 38.413 / TS 24.501 | **Fixed.** `UEAggregateMaximumBitRate` and `UESecurityCapabilities` APER encoding corrected and validated by UERANSIM replay run `27057637533`. |
| 2026-06-07 | UERANSIM aborted after InitialContextSetup because Registration Accept encoded Assigned 5G-GUTI IEI `0x77` with a one-byte length instead of IE6/TLV-E two-byte length. | TS 24.501 | **Fixed.** `TestRegistrationAcceptUERANSIMMobileIdentityLengthGolden` pins the bad and fixed fixtures; replay run `27080274240` reaches Registration Complete. |
| 2026-06-07 | After registration, AMF initially failed to parse UERANSIM's protected UL NAS Transport and then used `localhost:8002` as SMF fallback inside Docker. | TS 24.501 / TS 29.502 | **Fixed.** AMF routes decrypted NAS to UL transport handling, accepts UERANSIM IE1/IE3 shapes, and compose sets `QCORE_AMF_SMF_URL=http://smf:8002`. Replay run `27080274240` shows AMF forwarding the PDU request and SMF returning `201`. |
| 2026-06-07 | QCore creates the SMF context but does not send PDU Session Establishment Accept back to UERANSIM; no external PDU-session completion or ping is proven. | TS 24.501 / TS 38.413 / TS 29.502 | **Open.** Next work: encode protected DL NAS Transport with 5GSM PDU Session Establishment Accept, replay UERANSIM, then validate UPF data-plane on a Linux/TUN-capable runtime. |

## How this doc is maintained

1. **Per interop finding:** every real-RAN gap fixed (or found) gets a row in the Findings
   log with its spec reference, same commit/PR.
2. **Quarterly interop check-in** (scheduled routine `qcore-3gpp-interop-checkin`):
   flag new UERANSIM/srsRAN releases and any 3GPP change reports touching the *implemented*
   surface above — **flag and propose only; never auto-implement** (that would drift into
   the deferred feature-count race). A human decides whether each is wedge-relevant.
3. **Authoritative source is the code**, not this table — re-verify against the packages
   cited before asserting status (trust rule).
