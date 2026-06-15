# UERANSIM Compatibility (5G SA)

**Status: UERANSIM registration + PDU session + data-plane ping pass. T10 is shipped
for the bundled UERANSIM Docker/cloud-Linux profile, including Profile-A concealed SUCI
and AUTS/SQN resync gates.**

As of 2026-06-08, QCore has been replayed against UERANSIM v3.2.8 in Docker
Compose over native SCTP on GitHub Actions cloud Linux. The replay completes the 5G
flow end-to-end for this profile: full initial registration, the UE's PDU Session
Establishment Request, SMF `201 Created`, protected PDU Session Establishment Accept,
NGAP PDU Session Resource Setup, PFCP remote tunnel update, UPF real TUN/NAT, and
`ping -c 3 -I uesimtun0 8.8.8.8` from the UERANSIM UE container. As of 2026-06-15,
the same workflow also proves the bundled UE registers with SUCI Profile A concealment:
UDM de-conceals the `suci-<hex>` identity to `imsi-001010000000001`.

Confirmed by GitHub Actions `ueransim-interop` run `27115478758`:

```text
T10 DATA PLANE PASS — UERANSIM completed PDU session establishment and ping over uesimtun0 succeeded.
```

Confirmed by GitHub Actions `ueransim-interop` run `27545087715`:

```text
SUCI PROFILE A PASS — UERANSIM sent concealed SUCI (106 hex chars), UDM de-concealed it to imsi-001010000000001, journey=j-bb82e83f-ce87-4e63-9de9-31095b0d53d7.
T10 DATA PLANE PASS — UERANSIM completed PDU session establishment and ping over uesimtun0 succeeded.
T10 SQN RESYNC PASS — UERANSIM forced a Synch failure; QCore recovered SQN_MS, re-issued the challenge, and the UE completed registration.
```

## Verified In Replay

- Native SCTP association from UERANSIM gNB to QCore AMF.
- NGSetup Request/Response succeeds.
- InitialUEMessage is received and decoded.
- AMF sends Authentication Request; UERANSIM parses RAND/AUTN.
- UERANSIM sends Authentication Response.
- AUSF confirmation succeeds.
- AMF sends Security Mode Command.
- UERANSIM accepts the SMC (`Selected integrity[2] ciphering[0]`) with no integrity
  failure.
- AMF receives Security Mode Complete.
- AMF sends InitialContextSetupRequest carrying Registration Accept.
- UERANSIM gNB decodes the InitialContextSetupRequest and sends InitialContextSetupResponse.
- AMF logs `amf: InitialContextSetup confirmed by gNB`.
- UERANSIM UE logs `Registration accept received`,
  `MM-REGISTERED/NORMAL-SERVICE`, and `Initial Registration is successful`.
- UERANSIM sends `Registration Complete`; AMF logs
  `amf: Registration Complete — UE fully registered`.
- UERANSIM sends a PDU Session Establishment Request.
- AMF decodes the protected UL NAS Transport and forwards Create SM Context to SMF.
- SMF allocates the UE IPv4 and returns HTTP `201` on
  `/nsmf-pdusession/v1/sm-contexts`.
- AMF sends a protected DL NAS Transport carrying a PDU Session Establishment Accept.
- UERANSIM UE logs `PDU Session Establishment Accept received` and
  `PDU Session establishment is successful PSI[1]` (UE stays running, no crash).
- AMF sends NGAP `PDUSessionResourceSetupRequest` with the protected PDU Session
  Establishment Accept.
- UERANSIM gNB sends NGAP `PDUSessionResourceSetupResponse` with the gNB N3 tunnel.
- AMF updates the SMF context with the gNB N3 IP/TEID.
- SMF sends PFCP Session Modification to UPF.
- UPF updates the remote gNB tunnel and uses a real Linux TUN/NAT egress.
- UERANSIM UE pings through `uesimtun0`.
- Bundled UERANSIM UE sends a concealed SUCI (Profile A, key ID 1), and UDM SIDF
  de-conceals it before subscriber lookup.

## Confirmed: PDU Session Establishment Accept (control plane)

After SMF returns `201`, the AMF now relays a 5GSM PDU Session Establishment
Accept to the UE over a protected DL NAS Transport (`pkg/amf/nas.go`;
`nas5g.EncodePDUSessionEstablishmentAccept` + `nas5g.EncodeDLNASTransport`). The
Accept carries the mandatory IEs UERANSIM requires — Selected PDU session type +
SSC mode, Authorized QoS rules (default match-all rule), Session-AMBR — plus the
PDU address IE with the SMF-assigned IPv4. Bytes pinned by
`TestEncodePDUSessionEstablishmentAcceptGolden`.

Confirmed by GitHub Actions run `27108387027`:

```text
amf: sent PDU Session Establishment Accept
PDU Session Establishment Accept received
PDU Session establishment is successful PSI[1]
```

## Confirmed: N2/N3 Data Plane

The final T10 blocker was that PDU session signaling completed but no external packet
was proven through the UPF. The shipped path is:

- SMF returns UPF N3 IP/TEID and UE IPv4 to AMF on Create SM Context.
- AMF sends `PDUSessionResourceSetupRequest` to the gNB, carrying the protected 5GSM
  Accept and UPF N3 tunnel info.
- AMF decodes `PDUSessionResourceSetupResponse`, extracts the gNB N3 IP/TEID, and calls
  SMF's modify endpoint.
- SMF sends PFCP Session Modification to UPF.
- UPF stores the gNB remote tunnel, creates/configures a real `qcore-upf` TUN interface,
  enables forwarding/NAT in the Linux container, and forwards UE traffic.

Pinned by focused tests in `pkg/ngap`, `pkg/pfcp`, and `pkg/upf`, and by the GitHub
Actions replay run `27115478758`.

## Scope And Remaining Caveats

This is a real T10 ship claim for the bundled UERANSIM v3.2.8 Docker profile on a
Linux/TUN-capable runtime. It is **not** a broad 3GPP conformance matrix or a claim that
every external gNB/UE behaves identically. Additional RAN/device targets should get their
own replay evidence before being marked compatible.

SUCI Profile A/B support is intentionally scoped to de-concealment for IMSI-based SUCI
using configured HN private keys. It does not add carrier-scale key lifecycle management,
and Profile B is vector-tested but not yet separately replayed against an external UE.

Docker Desktop on macOS is still not the validation environment for native SCTP + TUN;
use GitHub Actions/Linux or a Linux host.

## Confirmed Fix: Registration Accept 5G-GUTI IE6

The post-InitialContextSetup UE abort was caused by QCore encoding Assigned
5G-GUTI IEI `0x77` in Registration Accept with a one-byte length. UERANSIM's
`RegistrationAccept::onBuild` uses IE6/TLV-E semantics for `mobileIdentity`, so
the length must be two bytes.

Bad captured plain NAS:

```text
7e00420101770bf200f1100100400000000115020101
```

Fixed plain NAS:

```text
7e0042010177000bf200f1100100400000000115020101
```

Pinned by `TestRegistrationAcceptUERANSIMMobileIdentityLengthGolden`.

## Confirmed Fix: Protected UL NAS Transport Routing And IE Shapes

Once registration completed, UERANSIM sent a protected UL NAS Transport carrying
the 5GSM PDU Session Establishment Request:

```text
7e00670100152e0101c1ffff91a12801007b000780000a00000d00120181220401000001250908696e7465726e6574
```

Two issues were fixed:

- AMF now routes the decrypted/plain NAS payload into `handleULNASTransport`
  instead of re-decoding the original protected NAS wrapper.
- `DecodeULNASTransport` now accepts UERANSIM's IE shapes: payload container
  type in the low nibble, PDU Session ID as IE3 (`0x12 value`), and request type
  as IE1.

Pinned by `TestULNASTransportUERANSIMFixture`.

## Confirmed Fix: Compose SMF URL

The AMF static SMF fallback was `http://localhost:8002`, which is wrong inside
the AMF container. The 5G compose profile now sets:

```text
QCORE_AMF_SMF_URL=http://smf:8002
```

Confirmed by GitHub Actions run `27080274240`: AMF logs static fallback
`http://smf:8002`, forwards the PDU session request, and SMF returns `201`.

## Confirmed Fix: InitialContextSetup APER

Root-caused by packet evidence from the branch `ueransim-interop` job. With
`QCORE_AMF_TRACE_NGAP_HEX=true`, the first rejected InitialContextSetupRequest was
captured as a 130-byte raw PDU:

```text
000e007e000007000a00020001005500020001006e000d0000003b9aca0000003b9aca00001c00070000f110010040007700093c3c3c3c0000000000005e0020daf16094ca7e2f316ab69347a0dca70c887e76f83cde06607ed127e5f0c76e0e0026401e1d7e01ecba4c59017e00420101770bf200f1100100400000000115020101
```

Two APER encoding bugs were then fixed narrowly:

- `UEAggregateMaximumBitRate` now encodes NGAP `BitRate` as an extensible constrained
  integer with the extension marker, byte-count prefix, byte alignment, and minimum-width
  value bytes.
- `UESecurityCapabilities` now includes the extension marker for each 16-bit algorithm
  BIT STRING (`NRencryptionAlgorithms`, `NRintegrityProtectionAlgorithms`,
  `EUTRAencryptionAlgorithms`, `EUTRAintegrityProtectionAlgorithms`).

Pinned tests:

```text
TestUEAggregateMaximumBitRateAPERGolden
TestUESecurityCapabilitiesAPERGolden
TestInitialContextSetupUERANSIMRejectedFixture
```

Confirmed by GitHub Actions run `27057637533`: UERANSIM logs `Initial Context Setup
Request received`, and QCore logs `amf: InitialContextSetup confirmed by gNB`. A later
run (`27080274240`) confirms the downstream Registration Accept and Registration
Complete path is now working too.

## Confirmed Fix: K_AMF Bare-IMSI Input

Root-caused by code inspection to the **K_AMF derivation input**. The AMF was deriving
K_AMF with `P0 = ue.SUPI = "imsi-<15 digits>"` (the SBI/JSON representation), whereas
TS 33.501 Annex A.7 specifies `P0 = SUPI` = the **bare IMSI**. UERANSIM/free5GC derive
K_AMF from the bare IMSI digits, so the `imsi-` prefix produced a different
K_AMF → K_NASint at QCore than at the UE — which is exactly why the SMC MAC fails to
verify while authentication (whose keys come from the auth vector, not K_AMF) succeeds.

Fix: `pkg/amf/nas.go` now strips the `imsi-` prefix for the K_AMF KDF input only
(`ue.SUPI` keeps the `imsi-` form for SBI/N11/telemetry). Pinned by
`TestAMF_KAMF_UsesBareIMSI` (bare vs prefixed K_AMF must differ). Other suspects ruled
out by inspection: the integrity algorithm chain is consistent (NIA2 advertised in the
SMC, K_NASint derived with algID=2, MAC = AES-CMAC), ABBA = `0x0000`, BEARER = 1
(matches free5GC's `Bearer3GPP`).

Confirmed by the `ueransim-interop` GitHub Actions job on PR #28:

```text
reached_smc=1 integrity_failed=0 registered=0
Security Mode Command received
Selected integrity[2] ciphering[0]
amf: SMC Complete — sending Registration Accept
```

This resolves the SMC-integrity blocker. It did not complete T10 by itself; later
Registration Accept, UL NAS Transport, SMF handoff, and PDU Session Establishment Accept
fixes now carry the external replay through PDU session establishment.

## Replay Command

```bash
docker compose -f deployments/docker/docker-compose.yml --profile 5g down
docker compose -f deployments/docker/docker-compose.yml --profile 5g up --build
```

## Notes

- The compose UE config is aligned with QCore's seeded demo subscriber.
- The dev reset seeds the demo SQN at `000000000020`, because UERANSIM starts with
  `SQN-MS=000000000000` and rejects a network SQN whose sequence part is not ahead.
- On macOS Docker, UPF may fall back to dummy egress if `/dev/net/tun` is unavailable.
  The GitHub replay uses a Linux runtime with `/dev/net/tun`, real TUN configuration, and
  NAT; that is the authoritative T10 data-plane evidence.

## AUTS/SQN resynchronization (interop-validated 2026-06-15)

QCore handles the 5G-AKA resync path (TS 33.102 §6.3.5) against a real UERANSIM UE, not
just in unit tests. The `ueransim-interop` workflow runs `scripts/ci/ueransim-sqn-resync.sh`
after the data-plane check:

1. The UE registers normally, advancing its internal `SQN-MS` past the core's seed.
2. The script forces the core's SQN **behind** the live UE via the UDR PATCH endpoint
   `PATCH /nudr-dr/v2/subscription-data/<imsi>/authentication-data/authentication-subscription`
   (JSON-Patch `replace /sequenceNumber/sqn` → `000000000020`; HTTP 204 required).
3. It triggers a fresh authentication **without restarting the UE** via
   `nr-cli <imsi> --exec "deregister normal"` (so the UE keeps its advanced `SQN-MS`).
4. The core issues a challenge with the now-behind SQN; the UE returns
   `Authentication Failure (SQN out of range)` with AUTS; QCore recovers `SQN_MS`
   (reverse-Milenage f1\*/f5\*), advances its SQN, and re-issues the challenge; the UE
   accepts and completes registration.

The gate asserts the **ordered** sequence (UE SQN failure → AMF `attempting SQN
resynchronization` → collector `Authentication Request sent (Resync)` → UE registration
success *after* the failure line) and prints `T10 SQN RESYNC PASS`. Authoritative evidence:
`ueransim-interop` run `27529970131` (with `T10 DATA PLANE PASS` intact). The crypto is
vector-validated against 3GPP TS 35.208 in `pkg/subscriber/milenage_test.go`
(`TestResyncAUTSRoundTrip`).

Reaching this required three real AMF fixes the replay exposed: accepting UE-originated
deregistration, keeping SUPI separate from the AUSF auth-context URL, and resetting NAS
security counters after deregistration so the post-resync Security Mode Command verifies.

Scope: 5G only; integer SQN with a +32 advance (not the TS 33.102 array scheme); 4G AUTS
resync is a follow-up. This does not broaden T10 beyond the bundled UERANSIM profile.

## SUCI Profile A concealment (interop-validated 2026-06-15)

QCore handles IMSI-based SUCI Profile A/B de-concealment in UDM/SIDF, not AMF. AMF passes
the concealed mobile identity through as `suci-<hex>`; UDM uses `udm.sidf_keys` to select
the HN private key, verify the ECIES MAC, decrypt the concealed MSIN, and continue the
existing subscriber lookup as `imsi-...`.

The crypto is pinned before wiring by TS 33.501 Annex C.4 vectors in `pkg/suci`: Profile A
(X25519) and Profile B (P-256) both reproduce the published plaintext MSIN and reject a
tampered MAC. The shipped UERANSIM replay covers Profile A because UERANSIM exposes
Profile-A configuration (`protectionScheme: 1`, `homeNetworkPublicKeyId: 1`,
`homeNetworkPublicKey`).

The workflow gate runs `scripts/ci/ueransim-suci-profile-a.sh` after the data-plane check.
It reads collector events and requires both:

- AMF received a concealed `suci-<hex>` Registration Request from the UE.
- UDM generated the authentication vector for that same concealed SUCI after resolving it
  to `imsi-001010000000001`.

Authoritative evidence: `ueransim-interop` run `27545087715` prints `SUCI PROFILE A PASS`,
with `T10 DATA PLANE PASS` and `T10 SQN RESYNC PASS` intact.
