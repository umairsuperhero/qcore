#!/usr/bin/env bash
set -euo pipefail

COMPOSE=${COMPOSE:-"docker compose -f deployments/docker/docker-compose.yml"}
UE_NODE=${UE_NODE:-"imsi-001010000000001"}
RESET_SQN=${RESET_SQN:-"000000000020"}
ARTIFACT_DIR=${ARTIFACT_DIR:-"artifacts/ueransim"}
UDR_URL=${UDR_URL:-"http://localhost:8084"}
COLLECTOR_URL=${COLLECTOR_URL:-"http://localhost:9099"}

mkdir -p "$ARTIFACT_DIR"

log_count() {
  $COMPOSE logs --no-color "$1" 2>/dev/null | wc -l | tr -d ' '
}

logs_since() {
  local svc=$1
  local start_line=$2
  $COMPOSE logs --no-color "$svc" 2>/dev/null | tail -n +"$((start_line + 1))"
}

collector_has_message() {
  local needle=$1
  python3 - "$COLLECTOR_URL" "$needle" <<'PY'
import json
import sys
import urllib.request

base = sys.argv[1].rstrip("/")
needle = sys.argv[2]

try:
    with urllib.request.urlopen(f"{base}/journeys", timeout=2) as resp:
        journey_ids = json.load(resp)
    for journey_id in journey_ids:
        with urllib.request.urlopen(f"{base}/journeys/{journey_id}/events", timeout=2) as resp:
            events = json.load(resp)
        for event in events:
            if needle in str(event.get("message", "")):
                print(journey_id)
                raise SystemExit(0)
except Exception:
    pass

raise SystemExit(1)
PY
}

dump_collector_events() {
  python3 - "$COLLECTOR_URL" > "$ARTIFACT_DIR/collector-events.json" <<'PY' || true
import json
import sys
import urllib.request

base = sys.argv[1].rstrip("/")
out = {}
try:
    with urllib.request.urlopen(f"{base}/journeys", timeout=2) as resp:
        journey_ids = json.load(resp)
    for journey_id in journey_ids:
        with urllib.request.urlopen(f"{base}/journeys/{journey_id}/events", timeout=2) as resp:
            out[journey_id] = json.load(resp)
finally:
    print(json.dumps(out, indent=2, sort_keys=True))
PY
}

echo "Preparing SQN-resync interop check for ${UE_NODE}."
echo "nr-cli visible nodes:"
$COMPOSE exec -T ueransim-ue /ueransim/nr-cli --dump | tee "$ARTIFACT_DIR/nr-cli-dump-before-resync.txt"

echo "Forcing core SQN behind the live UE via verified UDR PATCH endpoint: ${RESET_SQN}"
patch_body="[{\"op\":\"replace\",\"path\":\"/sequenceNumber/sqn\",\"value\":\"${RESET_SQN}\"}]"
http_code="$(curl -sS -o "$ARTIFACT_DIR/sqn-reset-response.txt" -w "%{http_code}" \
  -X PATCH \
  -H "Content-Type: application/json" \
  --data "$patch_body" \
  "${UDR_URL}/nudr-dr/v2/subscription-data/${UE_NODE}/authentication-data/authentication-subscription")"
if [ "$http_code" != "204" ]; then
  echo "::error::Could not reset subscriber SQN through UDR PATCH; HTTP ${http_code}"
  cat "$ARTIFACT_DIR/sqn-reset-response.txt" || true
  exit 1
fi

base_ue_lines="$(log_count ueransim-ue)"
base_amf_lines="$(log_count amf)"
base_collector_lines="$(log_count collector)"
echo "Baseline log lines before re-auth trigger: ue=${base_ue_lines} amf=${base_amf_lines} collector=${base_collector_lines}"

echo "Triggering UE-originated normal de-registration to force a fresh authentication without restarting the UE."
set +e
cli_output="$($COMPOSE exec -T ueransim-ue /ueransim/nr-cli "$UE_NODE" --exec "deregister normal" 2>&1)"
cli_status=$?
set -e
printf "%s\n" "$cli_output" | tee "$ARTIFACT_DIR/nr-cli-deregister-normal.txt"
if [ "$cli_status" -ne 0 ]; then
  echo "::error::nr-cli deregister normal failed with exit code ${cli_status}"
  exit 1
fi

sqn_failure=0
amf_resync=0
resync_event=0
registered_after_resync=0
resync_journey=""
ue_sqn_line=0
ue_registered_line=0

echo "Watching post-trigger logs for SQN failure → AUTS resync → successful registration (up to ~180s)..."
for _ in $(seq 1 60); do
  ue_new="$(logs_since ueransim-ue "$base_ue_lines")"
  amf_new="$(logs_since amf "$base_amf_lines")"

  if printf "%s\n" "$ue_new" | grep -qiE "Sending Authentication Failure due to SQN out of range|Authentication Failure.*SQN|Synch.*failure|synchroni[sz]ation failure"; then
    sqn_failure=1
    ue_sqn_line="$(printf "%s\n" "$ue_new" | grep -niE "Sending Authentication Failure due to SQN out of range|Authentication Failure.*SQN|Synch.*failure|synchroni[sz]ation failure" | head -1 | cut -d: -f1)"
  fi

  if printf "%s\n" "$amf_new" | grep -q "amf: attempting SQN resynchronization"; then
    amf_resync=1
  fi

  if resync_journey="$(collector_has_message "Authentication Request sent (Resync)")"; then
    resync_event=1
  fi

  if [ "$sqn_failure" = 1 ] && printf "%s\n" "$ue_new" | grep -qiE "Initial Registration is successful|MM-REGISTERED"; then
    ue_registered_line="$(printf "%s\n" "$ue_new" | grep -niE "Initial Registration is successful|MM-REGISTERED" | tail -1 | cut -d: -f1)"
    if [ "${ue_registered_line:-0}" -gt "${ue_sqn_line:-0}" ]; then
      registered_after_resync=1
    fi
  fi

  if [ "$sqn_failure" = 1 ] && [ "$amf_resync" = 1 ] && [ "$resync_event" = 1 ] && [ "$registered_after_resync" = 1 ]; then
    break
  fi

  sleep 3
done

logs_since ueransim-ue "$base_ue_lines" > "$ARTIFACT_DIR/sqn-resync-ueransim-ue-post-trigger.log" || true
logs_since amf "$base_amf_lines" > "$ARTIFACT_DIR/sqn-resync-amf-post-trigger.log" || true
logs_since collector "$base_collector_lines" > "$ARTIFACT_DIR/sqn-resync-collector-post-trigger.log" || true
dump_collector_events

echo "sqn_failure=${sqn_failure} amf_resync=${amf_resync} resync_event=${resync_event} registered_after_resync=${registered_after_resync} resync_journey=${resync_journey:-unknown}"

if [ "$sqn_failure" != 1 ]; then
  echo "::error::UERANSIM did not emit an SQN/synch-failure line after a genuine re-auth attempt. See sqn-resync-ueransim-ue-post-trigger.log."
  tail -120 "$ARTIFACT_DIR/sqn-resync-ueransim-ue-post-trigger.log" || true
  exit 1
fi
if [ "$amf_resync" != 1 ]; then
  echo "::error::AMF did not log SQN resynchronization attempt after UERANSIM SQN failure."
  tail -120 "$ARTIFACT_DIR/sqn-resync-amf-post-trigger.log" || true
  exit 1
fi
if [ "$resync_event" != 1 ]; then
  echo "::error::Collector did not receive the AMF event 'Authentication Request sent (Resync)'."
  tail -120 "$ARTIFACT_DIR/sqn-resync-collector-post-trigger.log" || true
  exit 1
fi
if [ "$registered_after_resync" != 1 ]; then
  echo "::error::UERANSIM did not complete registration after the SQN resync attempt."
  tail -160 "$ARTIFACT_DIR/sqn-resync-ueransim-ue-post-trigger.log" || true
  exit 1
fi

echo "::notice::T10 SQN RESYNC PASS — UERANSIM forced a Synch failure; QCore recovered SQN_MS, re-issued the challenge, and the UE completed registration."
