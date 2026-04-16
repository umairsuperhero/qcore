# Connecting UERANSIM to QCore

[UERANSIM](https://github.com/aligungr/UERANSIM) is the most popular open-source
4G/5G UE + RAN simulator. This guide walks through pointing a UERANSIM eNB and
UE at a running QCore HSS + MME so you can see a real attach end-to-end.

QCore status: 4G S1AP/NAS attach is implemented. 5G NGAP is not yet implemented,
so use UERANSIM in **EPC (4G) mode**, not 5GC mode.

## 1. Prerequisites

- Linux host (UERANSIM requires kernel SCTP and TUN devices)
- Go 1.23+ and the QCore repo cloned
- UERANSIM built from source (see UERANSIM README)
- A subscriber configured in QCore HSS matching the UERANSIM UE

## 2. Provision a subscriber in QCore HSS

QCore auto-seeds a demo subscriber on first boot
(IMSI=`001010000000001`, Ki=`465b5ce8b199b49faa5f0a2ee238a6bc`,
OPc=`cd63cb71954a9f4e48a5994e37a02baf`) — these are the
3GPP TS 35.208 Test Set 1 values.

Configure the same values in UERANSIM's UE config so its computed RES matches the HSS XRES.

## 3. Start QCore in SCTP mode

QCore defaults to a length-prefixed TCP transport for development on
non-Linux hosts. To talk to UERANSIM you must switch to native SCTP.

```yaml
# config.yaml
mme:
  s1ap_port: 36412
  sctp_mode: sctp     # native SCTP — Linux only
  bind_address: 0.0.0.0
  plmn: "00101"
  hss_url: "http://localhost:8080"
  tac: 1
```

> **Note:** Native SCTP support requires the `pion/sctp` adapter (planned).
> Until then, run QCore via the TCP fallback and use a thin SCTP↔TCP bridge,
> or run UERANSIM with a custom transport. See **Limitations** below.

```bash
make build-all
./bin/qcore-hss start --config config.yaml &
./bin/qcore-mme start --config config.yaml &
```

Verify both are up:

```bash
curl localhost:8080/api/v1/health   # HSS
curl localhost:8081/api/v1/health   # MME
curl localhost:8081/api/v1/status   # MME state
```

## 4. Configure UERANSIM eNB (gNB in EPC mode)

```yaml
# ueransim/config/free5gc-gnb.yaml — adapt for QCore
mcc: '001'
mnc: '01'
nci: '0x000000010'
idLength: 32
tac: 1

linkIp: 127.0.0.1
ngapIp: 127.0.0.1     # for 5G; ignore in EPC mode
gtpIp: 127.0.0.1

amfConfigs:           # for 5G; ignore in EPC mode
  - address: 127.0.0.1
    port: 38412

# For EPC (4G) mode use UERANSIM's S1AP-capable build
# and point at the QCore MME:
mmeConfigs:
  - address: 127.0.0.1
    port: 36412

slices:
  - sst: 1
```

Start the eNB:

```bash
build/nr-gnb -c config/qcore-enb.yaml
```

You should see a S1 SETUP REQUEST in QCore MME logs followed by a
SETUP RESPONSE.

## 5. Configure and start UERANSIM UE

```yaml
# ueransim/config/qcore-ue.yaml
supi: 'imsi-001010000000001'
mcc: '001'
mnc: '01'
key: '465B5CE8B199B49FAA5F0A2EE238A6BC'
op:  'CDC202D5123E20F62B6D676AC72CB318'
opType: 'OP'
amf: '8000'

gnbSearchList:
  - 127.0.0.1

sessions:
  - type: 'IPv4'
    apn: 'internet'
    slice:
      sst: 1
```

Start the UE:

```bash
build/nr-ue -c config/qcore-ue.yaml
```

Expected QCore MME log sequence:
```
INFO  S1AP InitiatingMessage: InitialUEMessage
INFO  ATTACH REQUEST: IMSI=001010000000001
INFO  Sent AUTH REQUEST to UE
INFO  Auth verified for UE (IMSI=001010000000001)
INFO  Sent SECURITY MODE COMMAND to UE
INFO  Security Mode Complete from UE
INFO  Sent INITIAL CONTEXT SETUP REQUEST (IP=10.45.0.2)
INFO  ATTACH COMPLETE — UE is now registered
```

Confirm via the MME debug API:
```bash
curl localhost:8081/api/v1/ues
# [{"mme_ue_s1ap_id":1,"imsi":"001010000000001","emm_state":"Registered","pdn_addr":"10.45.0.2", ...}]
```

## 6. Limitations

- **No GTP-U yet**: QCore MME does not currently set up the user-plane bearer
  to a real S-GW/P-GW, so the UE will get an IP but cannot send data.
  Phase 3 will add SGW-U/PGW-U.
- **Native SCTP transport not yet wired**: the `pkg/sctp` package abstracts
  the transport, but only the TCP fallback is implemented. To use UERANSIM
  today you can run QCore behind an `sctptunnel` or run a small Go bridge.
  Native SCTP via `pion/sctp` is planned.
- **5G NGAP not implemented**: UERANSIM UE/gNB must run in EPC mode.
- **No KEY_RES* (5G AKA)**: Only EPS-AKA (4G) is implemented for now.

## 7. Troubleshooting

| Symptom | Likely cause |
| --- | --- |
| `S1 Setup rejected: no matching PLMN` | UERANSIM `mcc`/`mnc` doesn't match QCore `plmn` |
| `HSS auth vector request failed` | Subscriber not provisioned in HSS or HSS unreachable |
| `RES mismatch` | UE's `key`/`op` doesn't match HSS `Ki`/`OP` |
| Connection refused on 36412 | QCore MME not running, or bound to wrong interface |

## 8. Quick verification without UERANSIM

If you just want to verify QCore's S1AP/NAS encoding works end-to-end without
installing UERANSIM, run the integration test:

```bash
go test -v -run TestEndToEndAttachOverWire ./pkg/mme/
```

This stands up a real MME, mocks the HSS, and runs a Go-based eNB through the
full attach flow over the same encoders/decoders UERANSIM would use.
