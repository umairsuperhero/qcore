# Phase 3 — User Plane

Phase 3 is where QCore stops being a "control-plane toy" and starts moving
actual packets. This document describes what Session 1 of Phase 3 ships today,
the architectural trade-offs we made, and what's intentionally still missing.

---

## Goal

> "A UE can ping 8.8.8.8 through QCore."

Session 1 gets us most of the way there: the full attach now allocates a real
UE IP from an address pool, plumbs a real SGW TEID end-to-end through S1AP to
the eNB, and the SPGW decapsulates an uplink GTP-U T-PDU into an egress
adapter. The actual "send to internet" half — a TUN device, NAT, and downlink
delivery — lands in Session 2.

---

## Architecture

```
              S6a (HTTP)           S1AP/NAS
  +-----+  ------------->  +-----+ <---------- +-------+
  | HSS |                  | MME |             | eNB   |
  +-----+  <-------------  +-----+ ----------> +-------+
                              |                    |
                              | S11 (HTTP/JSON)    | S1-U (GTP-U/UDP)
                              v                    v
                           +------------------------+
                           |         SPGW           |
                           |  (SGW + PGW collapsed) |
                           +------------------------+
                                       |
                                       | SGi (egress adapter)
                                       v
                                   [Internet]
```

### Components shipped in Session 1

| Package | Responsibility |
|---------|----------------|
| `pkg/gtp/` | GTP-U v1 codec: T-PDU, Echo, sequence/extension-header chain |
| `pkg/spgw/pool.go` | UE IP pool (CIDR, gateway excluded) + TEID pool (monotonic, recycled) |
| `pkg/spgw/session.go` | Per-UE bearer state indexed by IMSI, SGW TEID, and UE IP |
| `pkg/spgw/dataplane.go` | UDP listener on port 2152, decapsulates uplink to egress |
| `pkg/spgw/egress.go` | `Egress` interface; `LogEgress` counts + logs packets (TUN is Session 2) |
| `pkg/spgw/api.go` | S11 HTTP: `POST /sessions`, `POST /sessions/{imsi}/modify`, `DELETE /sessions/{imsi}` |
| `pkg/spgw/service.go` | Orchestrates pool + sessions + dataplane + egress |
| `cmd/spgw/main.go` | `qcore-spgw start` cobra CLI |
| `pkg/mme/s11_client.go` | MME → SPGW HTTP client used during attach |

### What changed in MME

- `handleSecurityModeComplete` now calls S11 `CreateSession` (fallback to
  local fake IP allocator if `spgw_url` is empty — so Phase 2 behaviour still
  works out-of-the-box when the SPGW isn't running).
- `sendInitialContextSetup` reads the real SGW address + TEID from the UE
  context and emits them in the S1AP IE.
- `handleInitialContextSetupResponse` now parses the
  `E-RABSetupListCtxtSURes` IE from the eNB, extracts the eNB's GTP-U
  endpoint, and sends `ModifyBearer` over S11 so the SPGW can forward
  downlink to the right eNB later.
- `cleanupUE` fires `DeleteSession` asynchronously.

### What changed in S1AP

Added decode + encode support for `E-RABSetupListCtxtSURes` (IE ID 50) and
`E-RABSetupItemCtxtSURes` (IE ID 51), including the fiddly ALIGNED PER
`BIT STRING(SIZE(1..160,...))` framing with explicit length determinant.
Round-trip verified by `TestERABSetupResultRoundTrip`.

---

## Trade-offs

### S11 is HTTP/JSON, not GTPv2-C

GTPv2-C is the standards-blessed protocol between the MME and SGW. We picked
HTTP/JSON for the same reason we picked HTTP for S6a: developer ergonomics
massively outweighs interop with commercial MMEs during early development.
You can `curl` an SPGW session. You can't `curl` a GTPv2-C endpoint.

When a real commercial MME needs to attach to our SPGW (or vice-versa),
we'll add GTPv2-C as a second front-end. Until then, the internal control
plane stays legible.

### Collapsed SGW+PGW ("SPGW")

The 3GPP spec splits these into two boxes. In every real deployment we've
seen, they're deployed side-by-side anyway. Collapsing them removes an
interface boundary (S5/S8) we don't need yet. If someone later wants to
run them split, the bearer state is already factored cleanly enough to
support it.

### Egress adapter pattern

Session 1 ships a `LogEgress` that counts uplink packets and logs src/dst.
Session 2 will add a `TUNEgress` behind a build tag (Linux-only), so CI on
Windows/macOS keeps running without needing root or kernel modules.

---

## Known gaps (intentional for Session 1)

1. **No real SGi egress.** Packets hit the egress adapter and are logged,
   not forwarded to a real network. Session 2: Linux TUN device + NAT.
2. **No downlink.** The SPGW knows the eNB TEID (via `ModifyBearer`) but
   nothing hands it packets to forward yet. Session 2: once TUN is in,
   downlink from the internet side follows naturally.
3. **No native SCTP.** The MME still ships the TCP-framed SCTP fallback
   from Phase 2. Real SCTP binding is a separate slice.
4. **No 5G NGAP.** Phase 5.
5. **No charging, no QoS enforcement, no usage reporting.** Bearers are
   "allow-all" once established.
6. **Single PDN per UE.** Multi-APN selection is a later concern.

---

## Verify it works

```bash
# Unit + integration tests (includes TestEndToEndUserPlane)
go test -count=1 ./...

# E2E user-plane only
go test -v -run TestEndToEndUserPlane ./pkg/mme/

# Or bring up the whole thing in Docker
make docker-up
curl http://localhost:8082/api/v1/health
curl http://localhost:8082/api/v1/stats
```

`TestEndToEndUserPlane` spins up a real SPGW (with ephemeral S1-U UDP port),
an httptest HSS, a real MME, and a mock eNB; drives a full attach, sends an
ICMP Echo inside GTP-U to the SPGW, and asserts the egress adapter saw it.

---

## What's next (Session 2 preview)

- Linux TUN egress (build-tagged, so non-Linux CI stays green)
- Simple SNAT so uplink packets actually reach the public internet
- Downlink: TUN read → lookup UE-IP in `SessionStore` → encap → send to eNB
- Native SCTP for S1AP (drop the TCP fallback in `linux`/`freebsd` builds)
- Metrics for every packet class (uplink, downlink, drops, echo)
