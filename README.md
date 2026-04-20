# QCore

[![CI](https://github.com/umairsuperhero/qcore/actions/workflows/ci.yml/badge.svg)](https://github.com/umairsuperhero/qcore/actions/workflows/ci.yml)

**The open-source 4G/5G core network that's actually easy to use.**

QCore is a mobile core network designed around developer experience. Start a complete LTE core with a single command, no tribal knowledge required.

> **Status (4G EPC):** Phases 1 (HSS), 2 (MME), and 3 Sessions 1–2 (SPGW + Linux TUN egress + SPGW Prometheus metrics) — shipped. Milenage authentication verified against official 3GPP TS 35.208 test vectors (sets 1, 3, 4, 5, 6). End-to-end attach + uplink GTP-U packet verified in-repo (`TestEndToEndUserPlane`).
>
> **Status (5G SA, v0.5 — in flight):** subscriber plane unified into `pkg/subscriber` (shared by 4G and 5G); `pkg/sbi` + `pkg/sbi/nrf` Phase 0 sketches (HTTP/2 + RFC 7807 Problem Details + in-memory NRF) shipped; first 5G network function cut — `pkg/udm` exposing `Nudm_SDM` `GET /nudm-sdm/v2/{supi}/am-data` — shipped and round-trip tested over h2c. A subscriber added via the REST admin API is now queryable via 5G SBI.
>
> **⚠ Direction change (2026-04-16):** QCore is pivoting to **5G SA as the
> primary track** with 4G EPC as a supported legacy track. The working 4G
> stack above stays; new feature investment shifts to 5G. See
> [RFC 0001](docs/rfc/0001-5g-sba-pivot.md) for the decision and
> [ARCHITECTURE.md](docs/ARCHITECTURE.md) for the target shape.

---

## Quick Start

```bash
git clone https://github.com/qcore-project/qcore
cd qcore
docker compose -f deployments/docker/docker-compose.yml up -d

# Health check
curl http://localhost:8080/api/v1/health

# Add a subscriber (3GPP TS 35.208 Test Set 1)
curl -X POST http://localhost:8080/api/v1/subscribers \
  -H "Content-Type: application/json" \
  -d '{
    "imsi": "001010000000001",
    "ki":   "465b5ce8b199b49faa5f0a2ee238a6bc",
    "opc":  "cd63cb71954a9f4e48a5994e37a02baf",
    "amf":  "8000"
  }'

# Generate an authentication vector
curl -X POST http://localhost:8080/api/v1/subscribers/001010000000001/auth-vector
```

You get back RAND, XRES, AUTN, and KASME as hex — ready to feed into an eNodeB or UE simulator.

> **First-run delight:** On an empty database, QCore auto-seeds the TS 35.208
> Test Set 1 subscriber so the curl commands above work immediately. Disable
> with `QCORE_SKIP_SEED=true`.

### Or skip curl entirely

```bash
qcore-hss subscriber list
qcore-hss subscriber auth 001010000000001
qcore-hss subscriber add --imsi 001010000000002 \
  --ki 465b5ce8b199b49faa5f0a2ee238a6bc \
  --opc cd63cb71954a9f4e48a5994e37a02baf
```

---

## Why QCore?

The existing open-source cores (open5GS, free5GC, Magma) are powerful but brutal to learn. You fight YAML, TS specs, and undocumented assumptions before you get a single attach.

QCore's thesis: **developer experience is the differentiator**.

- **One command** to a running core, not fifteen.
- **Sensible defaults** that work out of the box — no PLMN archaeology.
- **Honest error messages** that tell you what's wrong and how to fix it.
- **Test-vector-verified crypto** you can actually trust (3GPP TS 35.208).
- **REST + Prometheus** from day one, not as an afterthought.
- **Pure Go**, static binary, no C dependencies, no kernel modules.

---

## Architecture

```
+-----------+         +----------+         +----------+
|  eNodeB   |---S1--->|   MME    |---S6a-->|   HSS    |  <-- Phase 1 ✓
+-----------+         +----------+         +----------+
      |                    |
      |                   S11 (HTTP/JSON)
      |                    |
      |               +----v-----+
      +---- S1-U ---->|  SPGW    |--SGi--> Internet   <-- Phase 3 S1 ✓
           (GTP-U)    +----------+
```

### Phase 1: HSS (shipped)

- Subscriber management (CRUD + bulk CSV import/export)
- Milenage authentication vector generation (3GPP TS 35.206)
- KASME derivation (3GPP TS 33.401 Annex A.2)
- SQN management with replay protection
- REST API with structured logging & Prometheus metrics
- PostgreSQL persistence with auto-migration
- Multi-stage Docker build, graceful shutdown

### Phase 3 Session 1: SPGW + GTP-U uplink (shipped)

- Collapsed SGW+PGW ("SPGW") single binary
- GTP-U v1 codec (T-PDU, Echo Request/Response, sequence + extension headers)
- UE IP pool (CIDR) + TEID pool with recycling
- S11 control-plane over HTTP/JSON (create / modify bearer / delete session)
- MME attaches via S11: real UE IPs, real SGW TEIDs flow end-to-end through S1AP
- In-repo E2E test: mock eNB attach → NAS Attach Complete → GTP-U uplink packet arrives at SPGW egress

See [docs/PHASE3.md](docs/PHASE3.md) for architecture, limitations, and what's coming in Session 2.

### Roadmap

| Phase | Component | Status |
|-------|-----------|--------|
| 1 | HSS + subscriber provisioning | ✅ Shipped |
| 2 | MME + S1AP/NAS attach (auth, security, attach accept) | ✅ Shipped — see [docs/UERANSIM.md](docs/UERANSIM.md) |
| 3.1 | SPGW + GTP-U uplink dataplane + S11 | ✅ Shipped — see [docs/PHASE3.md](docs/PHASE3.md) |
| 3.2 | Linux TUN egress + SPGW Prometheus metrics | ✅ Shipped — see [docs/PHASE3.md](docs/PHASE3.md) |
| 3.3 | TestEndToEndPing under `//go:build linux` + native SCTP | 🔜 Next |
| 4 | Web dashboard (Next.js) | Planned |
| 5 | 5G SA (AMF/SMF/UPF) | Planned |
| 6 | Polish, docs, v1.0 release | Planned |

---

## Build from Source

```bash
# Prerequisites: Go 1.23+
make build        # Build all three binaries to bin/qcore-{hss,mme,spgw}
make test         # Run all tests with race detector
make lint         # Run golangci-lint

# Start the HSS with local PostgreSQL
./bin/qcore-hss start --config config.example.yaml

# Start the SPGW (in another terminal)
./bin/qcore-spgw start --config config.example.yaml

# Start the MME (in another terminal)
./bin/qcore-mme start --config config.example.yaml
```

End-to-end attach over the wire (real MME, mock HSS, Go-based mock eNB):

```bash
go test -v -run TestEndToEndAttachOverWire ./pkg/mme/
```

End-to-end user-plane (real MME + real SPGW + uplink GTP-U):

```bash
go test -v -run TestEndToEndUserPlane ./pkg/mme/
```

For driving QCore with [UERANSIM](https://github.com/aligungr/UERANSIM),
see [docs/UERANSIM.md](docs/UERANSIM.md).

---

## API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET`    | `/api/v1/subscribers` | List subscribers (pagination, search) |
| `GET`    | `/api/v1/subscribers/{imsi}` | Get subscriber by IMSI |
| `POST`   | `/api/v1/subscribers` | Create subscriber |
| `PUT`    | `/api/v1/subscribers/{imsi}` | Update subscriber |
| `DELETE` | `/api/v1/subscribers/{imsi}` | Delete subscriber |
| `POST`   | `/api/v1/subscribers/{imsi}/auth-vector` | Generate auth vector |
| `POST`   | `/api/v1/subscribers/import` | Import subscribers from CSV |
| `GET`    | `/api/v1/subscribers/export` | Export subscribers to CSV |
| `GET`    | `/api/v1/health` | Health check |
| `GET`    | `:9090/metrics` | Prometheus metrics |

---

## Configuration

See [config.example.yaml](config.example.yaml) for all options. Environment variables override config values with the `QCORE_` prefix:

```bash
export QCORE_DATABASE_HOST=db.example.com
export QCORE_DATABASE_PASSWORD=secret
export QCORE_LOGGING_LEVEL=debug
```

---

## Contributing

QCore is early. Issues, ideas, and PRs welcome — especially around developer experience papercuts. If something confused you, that's a bug.

## License

Apache 2.0 — see [LICENSE](LICENSE).
