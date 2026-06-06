# UERANSIM Compatibility (5G SA)

**Status: T10 is partially reproduced, not shipped.**

As of 2026-06-04, QCore has been replayed against UERANSIM v3.2.8 in Docker Compose
over native SCTP. The original `DownlinkNASTransport` APER `transfer-syntax-error`
blocker is fixed, but external registration is not complete.

## Verified In Replay

- Native SCTP association from UERANSIM gNB to QCore AMF.
- NGSetup Request/Response succeeds.
- InitialUEMessage is received and decoded.
- AMF sends Authentication Request; UERANSIM parses RAND/AUTN.
- UERANSIM sends Authentication Response.
- AUSF confirmation succeeds.

## Current T10 Blocker

UERANSIM rejects QCore's Security Mode Command:

```text
Security Mode Command received
Security Mode Command integrity check failed
Rejecting Security Mode Command with cause [SEC_MODE_REJECTED_UNSPECIFIED]
```

Do not claim "UERANSIM compatible", "5G shipped", Registration Accept, PDU session, or
data-plane success until this blocker is resolved and replay evidence exists.

## Candidate Fix (identified 2026-06-05, NOT yet confirmed against UERANSIM)

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

**This is a candidate, not a confirmation.** Per the trust rule, T10 stays BLOCKED until
a real UERANSIM replay on a Linux host gets past the SMC and the run is logged here. If
the SMC still fails after this fix, the next instrumentation step is to log QCore's exact
P0/K_AMF/K_NASint and compare against UERANSIM's.

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
