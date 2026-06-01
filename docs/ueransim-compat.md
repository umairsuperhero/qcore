# UERANSIM Compatibility (5G SA)

> **Status: NOT yet verified.** Real UERANSIM interop is the open credibility gate,
> tracked as **T10** in `docs/v1-gap-closure-plan.md`. Until an independent UERANSIM
> gNB/UE registers and establishes a PDU session through QCore, this page describes the
> *target* and the scaffolding that is ready — not a result. (Trust rule: "passes our
> own end-to-end test" is **not** "validated against an external gNB/UE.")

## What is actually verified today
QCore's 5G SA stack (AMF, AUSF, UDM, UDR, NRF, SMF, UPF) passes an **in-process,
automated end-to-end test driven by a built-in mock gNB** — Registration → PDU session
→ GTP-U tunnel — over **native Linux kernel SCTP** (`pkg/sctp/sctp_linux.go`; raw
syscalls, no CGO). This proves the protocol logic is internally self-consistent. It
does **not** prove interoperability with an independent implementation such as UERANSIM.

## What is ready for the real test
- **The `ueransim` service is wired into `deployments/docker/docker-compose.yml`**
  (behind the `5g` Compose profile) and points at the AMF — the interop attempt is one
  command away.
- **Native SCTP transport exists.** Set `QCORE_AMF_SCTP_MODE=sctp` (the AMF speaks NGAP
  on `38412`) so a real gNB's SCTP association is accepted instead of the TCP dev fallback.
- **A Linux host is required** — kernel SCTP plus `/dev/net/tun` for the UPF data plane.

## What is NOT yet validated (this is T10)
- A real UERANSIM gNB completing **NG Setup** against QCore's NGAP codec (expect optional
  IEs our internal tests don't exercise — each becomes a fix and, ideally, a catalog rule).
- A real UERANSIM UE completing **Registration**.
- **PDU Session Establishment** and end-to-end **GTP-U** with UERANSIM.

## Running the interop attempt
```bash
# Linux host. Brings up the 5G core + the UERANSIM gNB/UE (the `5g` profile),
# with the AMF accepting native SCTP.
QCORE_AMF_SCTP_MODE=sctp make up-5g
```
Findings and every fix required will be logged in `docs/ueransim-interop-log.md` as T10
is worked. **Until that log shows a passing run, no real-RAN / UERANSIM compatibility
may be claimed.**

## Lightweight alternative
`pkg/simulator` provides a built-in, scriptable gNB/UE for quick control-plane checks
without a full UERANSIM container or kernel SCTP — useful on macOS and in CI.
