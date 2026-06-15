#!/usr/bin/env bash
set -euo pipefail

ARTIFACT_DIR=${ARTIFACT_DIR:-"artifacts/ueransim"}
COLLECTOR_URL=${COLLECTOR_URL:-"http://localhost:9099"}
UE_NODE=${UE_NODE:-"imsi-001010000000001"}

mkdir -p "$ARTIFACT_DIR"

python3 - "$COLLECTOR_URL" "$UE_NODE" "$ARTIFACT_DIR/suci-profile-a-events.json" <<'PY'
import json
import sys
import urllib.request

base = sys.argv[1].rstrip("/")
ue_node = sys.argv[2]
out_path = sys.argv[3]

all_events = {}
concealed = []
resolved = []

with urllib.request.urlopen(f"{base}/journeys", timeout=3) as resp:
    journey_ids = json.load(resp)

for journey_id in journey_ids:
    with urllib.request.urlopen(f"{base}/journeys/{journey_id}/events", timeout=3) as resp:
        events = json.load(resp)
    all_events[journey_id] = events
    for event in events:
        payload = event.get("payload") or {}
        suci = str(payload.get("suci") or "")
        supi_or_suci = str(payload.get("supi_or_suci") or "")
        supi = str(payload.get("supi") or "")
        if event.get("nf") == "amf" and event.get("message") == "Registration Request received":
            if suci.startswith("suci-") and len(suci) > 64:
                concealed.append((journey_id, suci))
        if event.get("nf") == "udm" and event.get("message") == "Authentication vector generated":
            if supi_or_suci.startswith("suci-") and supi == ue_node and payload.get("success") is True:
                resolved.append((journey_id, supi_or_suci, supi))

with open(out_path, "w", encoding="utf-8") as f:
    json.dump(all_events, f, indent=2, sort_keys=True)

if not concealed:
    print("::error::Collector did not show an AMF Registration Request carrying concealed suci-<hex>.")
    raise SystemExit(1)
if not resolved:
    print("::error::Collector did not show UDM resolving concealed SUCI to the expected SUPI.")
    raise SystemExit(1)

concealed_journeys = {j for j, _ in concealed}
resolved_journeys = {j for j, _, _ in resolved}
common = concealed_journeys & resolved_journeys
if not common:
    print("::error::Concealed SUCI and UDM resolution happened on different journeys.")
    print(f"concealed={concealed_journeys} resolved={resolved_journeys}")
    raise SystemExit(1)

journey_id = sorted(common)[0]
sample_suci = next(s for j, s in concealed if j == journey_id)
print(f"::notice::SUCI PROFILE A PASS — UERANSIM sent concealed SUCI ({len(sample_suci) - len('suci-')} hex chars), UDM de-concealed it to {ue_node}, journey={journey_id}.")
PY
