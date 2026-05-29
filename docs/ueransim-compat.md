# UERANSIM Compatibility (5G SA)

QCore has been verified against UERANSIM in 5G SA mode.

## Components Verified
- **AMF (NGAP + NAS-5G):** Successfully handles NG Setup, Initial UE Message, Authentication, Security Mode Control, and Registration flows.
- **AUSF & UDM:** Successfully delegates and generates 5G AKA authentication vectors based on UDR credentials.
- **Native SCTP:** UERANSIM connects to QCore AMF using native Linux kernel SCTP sockets.

## Running UERANSIM via Docker Compose
A `ueransim` service has been added to `deployments/docker/docker-compose.yml`. This allows for a one-click end-to-end verification of the 5G SA control plane.

```bash
# Start the full 5G core + UERANSIM gNB & UE
docker-compose up -d amf ausf udm udr nrf postgres hss spgw collector dashboard ueransim
```

## Known Limitations
- The current integration primarily tests the control plane (Registration). User plane (PDU sessions and GTP-U) validation will be strengthened in Phase 3.
- `pkg/simulator` provides a lightweight, built-in alternative for quick checks without requiring a full UERANSIM container or native SCTP kernel modules.
