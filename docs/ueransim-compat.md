# UERANSIM Compatibility (5G SA)

**Status: UERANSIM initial registration + AMF-to-SMF handoff pass; T10 is not shipped.**

As of 2026-06-07, QCore has been replayed against UERANSIM v3.2.8 in Docker
Compose over native SCTP on GitHub Actions cloud Linux. The replay now reaches
full initial registration and forwards the UE's PDU Session Establishment Request
to SMF, which returns `201 Created` for Create SM Context.

This is a real milestone, but it is not a full T10 ship claim. QCore has not yet
delivered a PDU Session Establishment Accept back to UERANSIM, and no external
UERANSIM PDU-session completion or data-plane ping is proven.

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
- SMF allocates `10.45.0.1` and returns HTTP `201` on
  `/nsmf-pdusession/v1/sm-contexts`.

## Current T10 Gap

The next missing external step is the network response to the UE's PDU Session
Establishment Request. Current AMF behavior creates the SMF context and stops; it
does not yet send a protected DL NAS Transport carrying a PDU Session
Establishment Accept back to UERANSIM.

```text
Registration accept received
UE switches to state [MM-REGISTERED/NORMAL-SERVICE]
Initial Registration is successful
Sending PDU Session Establishment Request
amf: UL NAS Transport — forwarding PDU session to SMF
amf: SMF created PDU session
POST /nsmf-pdusession/v1/sm-contexts status=201
```

Do not claim "UERANSIM compatible", "5G shipped", completed PDU session, or
data-plane success until UERANSIM logs PDU-session establishment and a Linux
TUN-capable run proves traffic through UPF.

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
Registration Accept and UL NAS Transport fixes now carry the external replay through
initial registration and SMF handoff.

## Replay Command

```bash
docker compose -f deployments/docker/docker-compose.yml --profile 5g down
docker compose -f deployments/docker/docker-compose.yml --profile 5g up --build
```

## Notes

- The compose UE config is aligned with QCore's seeded demo subscriber.
- The dev reset seeds the demo SQN at `000000000020`, because UERANSIM starts with
  `SQN-MS=000000000000` and rejects a network SQN whose sequence part is not ahead.
- On macOS Docker, UPF falls back to dummy egress because `/dev/net/tun` is unavailable.
  The current GitHub replay also shows UPF using `DummyEgress`; full data-plane
  validation needs a Linux host/runtime where UPF itself receives `/dev/net/tun`.
