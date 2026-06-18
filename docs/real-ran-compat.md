# QCore Real-RAN Compatibility Findings

> Scope rule: this file records one target at a time. A passing run here is
> evidence for that exact target/version/host only. It is not a 3GPP conformance
> claim and it is not a promise that other gNBs, UEs, eNBs, or basebands behave
> the same way.

_Last updated: 2026-06-18._

## How Findings Are Added

Use the evidence harness:

```bash
TARGET_NAME=<target-name> RAN_KIND=<ueransim|srsran|real-gnb|real-enb> \
  scripts/ci/real-ran-capture.sh
```

For a running stack, add `--no-up`. If you have RAN/UE config files, pass
`--gnb-yaml path/to/gnb.yaml --ue-yaml path/to/ue.yaml` so the bundle includes
the pre-attach reconciliation result.

Each bundle lives under `artifacts/real-ran/<target>-<UTC-date>/` and should
contain `summary.md`, `timings.json`, `trace.json`, `ran-config.json`, and
optionally `diagnosis.json` / `reconcile.json`.

## Findings Log

### Bundled UERANSIM Docker Profile - Harness Self-Test

| Field | Value |
|---|---|
| Target | Bundled UERANSIM profile from `deployments/docker/docker-compose.yml` |
| Kind | `ueransim` |
| Mode | 5G SA |
| Host | GitHub Actions Ubuntu/Linux or local Linux with SCTP + TUN |
| Result | Harness captures a structured evidence bundle after the existing T10 gates |
| Evidence | `real-ran-capture` artifact from the `ueransim-interop` workflow |

This entry is intentionally a harness smoke target. The authoritative bundled
UERANSIM compatibility claim remains in [docs/ueransim-compat.md](ueransim-compat.md),
which records the T10 data-plane, SQN resync, and SUCI Profile-A GitHub run IDs.

Scope caveat: this validates the capture loop on the known bundled target. It
does not broaden QCore's compatibility claim beyond that profile.

## Entry Template

Copy this section when adding an external target.

### <Target Name And Version>

| Field | Value |
|---|---|
| Target |  |
| Kind | `real-gnb` / `real-enb` / `srsran` / `ueransim` |
| Mode | 5G SA / 4G EPC |
| Host |  |
| Result | pass / fail / partial |
| TTFC |  |
| TTRC |  |
| Evidence | artifact path or CI run ID |

#### Verified In Replay

- 

#### Findings

- 

#### Scope And Caveats

This is one target/version on one host. Do not generalize it into a conformance
matrix claim without separate replay evidence.
