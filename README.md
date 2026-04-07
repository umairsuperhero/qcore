# QCore

**The open-source 4G/5G core network that's actually easy to use.**

QCore is a mobile core network designed around developer experience. Start a complete LTE core with a single command, no tribal knowledge required.

> **Status:** Phase 1 (HSS) — shipped. Milenage authentication verified against official 3GPP TS 35.208 test vectors (sets 1, 3, 4, 5, 6).

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
                           |
                          S11
                           |
                      +----v-----+
                      | SGW/PGW  |--SGi--> Internet
                      +----------+
```

### Phase 1: HSS (shipped)

- Subscriber management (CRUD + bulk CSV import/export)
- Milenage authentication vector generation (3GPP TS 35.206)
- KASME derivation (3GPP TS 33.401 Annex A.2)
- SQN management with replay protection
- REST API with structured logging & Prometheus metrics
- PostgreSQL persistence with auto-migration
- Multi-stage Docker build, graceful shutdown

### Roadmap

| Phase | Component | Status |
|-------|-----------|--------|
| 1 | HSS + subscriber provisioning | ✅ Shipped |
| 2 | MME + S1AP protocol stack | 🔜 Next |
| 3 | SGW/PGW + GTP tunneling | Planned |
| 4 | Web dashboard (Next.js) | Planned |
| 5 | 5G SA (AMF/SMF/UPF) | Planned |
| 6 | Polish, docs, v1.0 release | Planned |

---

## Build from Source

```bash
# Prerequisites: Go 1.23+
make build        # Build binary to bin/qcore-hss
make test         # Run all tests with race detector
make lint         # Run golangci-lint

# Start with local PostgreSQL
./bin/qcore-hss start --config config.example.yaml
```

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
