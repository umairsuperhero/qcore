# UERANSIM Compatibility (5G SA)

**Status: T10 is partially reproduced, not shipped.**

As of 2026-06-06, QCore has been replayed against UERANSIM v3.2.8 in Docker Compose
over native SCTP on GitHub Actions cloud Linux. The original `DownlinkNASTransport`
APER `transfer-syntax-error` blocker is fixed, PR #28's K_AMF fix gets the UE past
Security Mode Control, and the later `InitialContextSetupRequest` APER
`transfer-syntax-error` is fixed on `codex/t10-initial-context-setup-aper`.
External registration is still not complete.

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

## Current T10 Blocker

Registration does not complete after InitialContextSetupResponse. The latest replay
shows the previous InitialContextSetup APER error is gone:

```text
amf: InitialContextSetup confirmed by gNB
[ngap] [debug] Initial Context Setup Request received
```

However, the UE process exits and the gNB reports the radio link loss before a completed
registration, PDU session, or data-plane proof:

```text
docker-ueransim-ue-1 Exited (139)
[rls] [debug] UE[1] signal lost
```

Current working hypothesis: the next T10 blocker is post-InitialContextSetup UE-side
failure, not an NGAP APER transfer-syntax failure. The next replay should capture the
UERANSIM UE crash context and the final NAS/NGAP messages around
InitialContextSetupResponse before changing QCore again.

Do not claim "UERANSIM compatible", "5G shipped", UE-consumed Registration Accept,
completed registration, PDU session, or data-plane success until this blocker is
resolved and replay evidence exists.

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
Request received`, and QCore logs `amf: InitialContextSetup confirmed by gNB`. The
run still ends with `ueransim-ue` exit 139 / UE signal lost, so this resolves only the
InitialContextSetup APER blocker.

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

This resolves the SMC-integrity blocker. It does **not** complete T10.

## Replay Command

```bash
docker compose -f deployments/docker/docker-compose.yml --profile 5g down
docker compose -f deployments/docker/docker-compose.yml --profile 5g up --build
```

## Notes

- The compose UE config is aligned with QCore's seeded demo subscriber.
- The dev reset seeds the demo SQN at `000000000020`, because UERANSIM starts with
  `SQN-MS=000000000000` and rejects a network SQN whose sequence part is not ahead.
- On macOS Docker, UPF falls back to dummy egress because `/dev/net/tun` is unavailable;
  full PDU/data-plane validation needs a Linux host or privileged TUN-capable runtime.
