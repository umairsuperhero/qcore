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
