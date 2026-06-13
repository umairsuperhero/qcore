#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DASHBOARD_URL="${QCORE_DASHBOARD_URL:-http://localhost:3000}"
TIMEOUT_SECONDS="${QCORE_MEASURE_TIMEOUT_SECONDS:-300}"
POLL_SECONDS="${QCORE_MEASURE_POLL_SECONDS:-1}"
OUTPUT_FILE=""
COLD_START=0
KEEP_STACK=0

usage() {
  cat <<'EOF'
Usage: scripts/measure-ttfc-ttrc.sh [options]

Measures QCore's charter metrics using the real dashboard simulator APIs:
  - TTFC: simulator launch to successful 4G attach / 5G registration
  - TTRC: injected failure launch to available Diagnostic AI result

Options:
  --dashboard-url URL   Dashboard base URL (default: http://localhost:3000)
  --timeout SECONDS     Max wait per step (default: 300)
  --cold                Tear down and start the full 5G compose profile first
  --keep-stack          With --cold, leave containers running after measurement
  --output FILE         Write JSON results to FILE
  -h, --help            Show this help

Examples:
  make measure
  scripts/measure-ttfc-ttrc.sh --cold --output measurements/latest.json
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --dashboard-url)
      DASHBOARD_URL="${2:-}"
      shift 2
      ;;
    --timeout)
      TIMEOUT_SECONDS="${2:-}"
      shift 2
      ;;
    --cold)
      COLD_START=1
      shift
      ;;
    --keep-stack)
      KEEP_STACK=1
      shift
      ;;
    --output)
      OUTPUT_FILE="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required for JSON parsing" >&2
  exit 1
fi

now_ms() {
  python3 - <<'PY'
import time
print(int(time.time() * 1000))
PY
}

json_field() {
  local path="$1"
  python3 -c '
import json
import sys

path = sys.argv[1]
try:
    data = json.load(sys.stdin)
except Exception:
    print("")
    raise SystemExit(0)

value = data
for part in path.split("."):
    if isinstance(value, dict):
        value = value.get(part, "")
    else:
        value = ""
    if value is None:
        value = ""
        break

if isinstance(value, bool):
    print("true" if value else "false")
else:
    print(value)
' "$path"
}

curl_get() {
  local path="$1"
  curl -fsS --max-time 10 "${DASHBOARD_URL}${path}"
}

curl_post() {
  local path="$1"
  local payload="$2"
  curl -fsS --max-time 10 -X POST "${DASHBOARD_URL}${path}" \
    -H "Content-Type: application/json" \
    -d "$payload"
}

RESULTS_TMP="$(mktemp)"
cleanup() {
  rm -f "$RESULTS_TMP"
  if [ "$COLD_START" -eq 1 ] && [ "$KEEP_STACK" -eq 0 ]; then
    docker compose -f deployments/docker/docker-compose.yml --profile 5g down >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

record_result() {
  python3 - "$RESULTS_TMP" "$@" <<'PY'
import json
import sys

path = sys.argv[1]
keys = ["metric", "mode", "scenario", "duration_ms", "state", "journey_id", "status", "note"]
record = dict(zip(keys, sys.argv[2:]))
for key in ("duration_ms",):
    try:
        record[key] = int(record[key])
    except Exception:
        record[key] = None
with open(path, "a", encoding="utf-8") as f:
    f.write(json.dumps(record, sort_keys=True) + "\n")
PY
}

wait_for_health() {
  local started_ms="$1"
  local deadline=$((SECONDS + TIMEOUT_SECONDS))
  local body ready duration

  echo "[measure] waiting for dashboard health at ${DASHBOARD_URL}/api/health"
  while [ "$SECONDS" -lt "$deadline" ]; do
    if body="$(curl_get /api/health 2>/dev/null)"; then
      ready="$(printf '%s' "$body" | json_field ready_for_use)"
      if [ "$ready" = "true" ]; then
        duration=$(($(now_ms) - started_ms))
        record_result "stack_ready" "-" "-" "$duration" "ready" "-" "pass" "dashboard /api/health ready_for_use=true"
        return 0
      fi
    fi
    sleep "$POLL_SECONDS"
  done

  duration=$(($(now_ms) - started_ms))
  record_result "stack_ready" "-" "-" "$duration" "timeout" "-" "fail" "dashboard health did not become ready"
  return 1
}

wait_for_simulator_done() {
  local expected="$1"
  local deadline=$((SECONDS + TIMEOUT_SECONDS))
  local body state journey error step

  while [ "$SECONDS" -lt "$deadline" ]; do
    if body="$(curl_get /api/simulator/status 2>/dev/null)"; then
      state="$(printf '%s' "$body" | json_field state)"
      journey="$(printf '%s' "$body" | json_field last_journey)"
      error="$(printf '%s' "$body" | json_field last_error)"
      step="$(printf '%s' "$body" | json_field failed_step)"
      if [ "$state" = "$expected" ]; then
        printf '%s\t%s\t%s\t%s\n' "$state" "$journey" "$step" "$error"
        return 0
      fi
      if [ "$state" = "failed" ] && [ "$expected" = "success" ]; then
        printf '%s\t%s\t%s\t%s\n' "$state" "$journey" "$step" "$error"
        return 1
      fi
    fi
    sleep "$POLL_SECONDS"
  done

  printf 'timeout\t\t\t\n'
  return 1
}

wait_for_diagnosis() {
  local journey="$1"
  local deadline=$((SECONDS + TIMEOUT_SECONDS))
  local body matched root_cause fix

  while [ "$SECONDS" -lt "$deadline" ]; do
    if body="$(curl_get "/api/diagnostics/journey/${journey}" 2>/dev/null)"; then
      matched="$(printf '%s' "$body" | json_field Matched)"
      root_cause="$(printf '%s' "$body" | json_field RootCause)"
      fix="$(printf '%s' "$body" | json_field Fix)"
      if [ "$matched" = "true" ] && [ -n "$root_cause" ] && [ -n "$fix" ]; then
        printf '%s\n' "$root_cause"
        return 0
      fi
    fi
    sleep "$POLL_SECONDS"
  done

  return 1
}

measure_happy_path() {
  local mode="$1"
  local start_ms duration result state journey step error status note

  echo "[measure] TTFC ${mode}: launching happy path"
  curl_post /api/simulator/stop '{}' >/dev/null || true
  start_ms="$(now_ms)"
  curl_post /api/simulator/start "{\"mode\":\"${mode}\"}" >/dev/null

  if result="$(wait_for_simulator_done success)"; then
    status="pass"
  else
    status="fail"
  fi

  duration=$(($(now_ms) - start_ms))
  state="$(printf '%s' "$result" | cut -f1)"
  journey="$(printf '%s' "$result" | cut -f2)"
  step="$(printf '%s' "$result" | cut -f3)"
  error="$(printf '%s' "$result" | cut -f4)"
  note="$step $error"
  record_result "ttfc" "$mode" "happy_path" "$duration" "$state" "$journey" "$status" "$note"

  if [ "$status" != "pass" ]; then
    return 1
  fi
}

measure_ttrc() {
  local mode="$1"
  local scenario="$2"
  local start_ms duration result state journey step error root_cause status note

  echo "[measure] TTRC ${mode}/${scenario}: launching failure and waiting for diagnosis"
  curl_post /api/simulator/stop '{}' >/dev/null || true
  start_ms="$(now_ms)"
  curl_post "/api/simulator/inject/${scenario}" "{\"mode\":\"${mode}\"}" >/dev/null

  if ! result="$(wait_for_simulator_done failed)"; then
    duration=$(($(now_ms) - start_ms))
    state="$(printf '%s' "$result" | cut -f1)"
    journey="$(printf '%s' "$result" | cut -f2)"
    record_result "ttrc" "$mode" "$scenario" "$duration" "$state" "$journey" "fail" "simulator did not reach failed state"
    return 1
  fi

  state="$(printf '%s' "$result" | cut -f1)"
  journey="$(printf '%s' "$result" | cut -f2)"
  step="$(printf '%s' "$result" | cut -f3)"
  error="$(printf '%s' "$result" | cut -f4)"

  if [ -z "$journey" ]; then
    duration=$(($(now_ms) - start_ms))
    record_result "ttrc" "$mode" "$scenario" "$duration" "$state" "-" "fail" "failed run had no journey id: $step $error"
    return 1
  fi

  if root_cause="$(wait_for_diagnosis "$journey")"; then
    status="pass"
    note="$root_cause"
  else
    status="fail"
    note="diagnosis unavailable for journey ${journey}"
  fi

  duration=$(($(now_ms) - start_ms))
  record_result "ttrc" "$mode" "$scenario" "$duration" "$state" "$journey" "$status" "$note"

  if [ "$status" != "pass" ]; then
    return 1
  fi
}

print_summary() {
  python3 - "$RESULTS_TMP" "$OUTPUT_FILE" "$DASHBOARD_URL" <<'PY'
import json
import pathlib
import sys

records = []
with open(sys.argv[1], encoding="utf-8") as f:
    for line in f:
        if line.strip():
            records.append(json.loads(line))

print("")
print("QCore TTFC/TTRC measurement summary")
print("")
print("| Metric | Mode | Scenario | Duration | Status | Journey |")
print("|--------|------|----------|----------|--------|---------|")
for r in records:
    duration = r.get("duration_ms")
    duration_s = "" if duration is None else f"{duration / 1000:.3f}s"
    print(
        f"| {r.get('metric', '')} | {r.get('mode', '')} | {r.get('scenario', '')} | "
        f"{duration_s} | {r.get('status', '')} | {r.get('journey_id', '')} |"
    )

payload = {
    "schema": "qcore.measurement.v1",
    "dashboard_url": sys.argv[3],
    "results": records,
}

output = sys.argv[2]
if output:
    path = pathlib.Path(output)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print("")
    print(f"Wrote JSON results to {path}")
PY
}

overall=0
stack_start_ms="$(now_ms)"
if [ "$COLD_START" -eq 1 ]; then
  echo "[measure] cold start requested: rebuilding full 5G compose profile"
  docker compose -f deployments/docker/docker-compose.yml --profile 5g down
  COMPOSE_PROFILES=5g docker compose -f deployments/docker/docker-compose.yml up -d --build
fi

if ! wait_for_health "$stack_start_ms"; then
  print_summary
  exit 1
fi

measure_happy_path "4g" || overall=1
measure_happy_path "5g" || overall=1
measure_ttrc "5g" "wrong_ki" || overall=1
measure_ttrc "5g" "wrong_plmn" || overall=1
measure_ttrc "5g" "unprovisioned_imsi" || overall=1
measure_ttrc "5g" "wrong_mme_address" || overall=1

print_summary
exit "$overall"
