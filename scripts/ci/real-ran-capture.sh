#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

DASHBOARD_URL="${QCORE_DASHBOARD_URL:-http://localhost:3000}"
MODE="${MODE:-5g}"
RAN_KIND="${RAN_KIND:-real-gnb}"
TARGET_NAME="${TARGET_NAME:-manual-target}"
OUTPUT_ROOT="${OUTPUT_ROOT:-artifacts/real-ran}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-300}"
POLL_SECONDS="${POLL_SECONDS:-2}"
SCENARIO="${SCENARIO:-}"
GNB_YAML="${GNB_YAML:-}"
UE_YAML="${UE_YAML:-}"
NO_UP=0
KEEP_STACK=0

usage() {
  cat <<'EOF'
Usage: scripts/ci/real-ran-capture.sh [options]

Captures a QCore evidence bundle for a simulator, bundled UERANSIM, or external
RAN run. The bundle is written under artifacts/real-ran/<target>-<UTC-date>/.

Options:
  --dashboard-url URL   Dashboard base URL (default: http://localhost:3000)
  --target NAME         Target label, e.g. bundled-ueransim, lab-gnb-a
  --mode 5g|4g          Core mode to start/watch (default: 5g)
  --ran-kind KIND       ueransim|srsran|real-gnb|real-enb|simulator
  --scenario NAME       Trigger a built-in simulator scenario, e.g. wrong_ki
  --gnb-yaml FILE       gNB config to reconcile before attach
  --ue-yaml FILE        UE config to reconcile before attach
  --output-root DIR     Evidence root (default: artifacts/real-ran)
  --timeout SECONDS     Max wait for health/run completion (default: 300)
  --no-up               Do not start compose; watch the already-running stack
  --keep-stack          Leave compose running after this script started it
  -h, --help            Show this help

Examples:
  make capture-real-ran
  TARGET_NAME=bundled-ueransim RAN_KIND=ueransim scripts/ci/real-ran-capture.sh --no-up
  scripts/ci/real-ran-capture.sh --ran-kind simulator --scenario wrong_ki --no-up
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --dashboard-url) DASHBOARD_URL="${2:-}"; shift 2 ;;
    --target) TARGET_NAME="${2:-}"; shift 2 ;;
    --mode) MODE="${2:-}"; shift 2 ;;
    --ran-kind) RAN_KIND="${2:-}"; shift 2 ;;
    --scenario) SCENARIO="${2:-}"; shift 2 ;;
    --gnb-yaml) GNB_YAML="${2:-}"; shift 2 ;;
    --ue-yaml) UE_YAML="${2:-}"; shift 2 ;;
    --output-root) OUTPUT_ROOT="${2:-}"; shift 2 ;;
    --timeout) TIMEOUT_SECONDS="${2:-}"; shift 2 ;;
    --no-up) NO_UP=1; shift ;;
    --keep-stack) KEEP_STACK=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [ "$MODE" != "5g" ] && [ "$MODE" != "4g" ]; then
  echo "mode must be 5g or 4g" >&2
  exit 2
fi
if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

slug="$(python3 - "$TARGET_NAME" <<'PY'
import re
import sys
text = sys.argv[1].strip().lower()
text = re.sub(r"[^a-z0-9._-]+", "-", text).strip("-")
print(text or "target")
PY
)"
utc_stamp="$(date -u +%Y%m%dT%H%M%SZ)"
ARTIFACT_DIR="${OUTPUT_ROOT%/}/${slug}-${utc_stamp}"
mkdir -p "$ARTIFACT_DIR"

now_ms() {
  python3 - <<'PY'
import time
print(int(time.time() * 1000))
PY
}

curl_get_file() {
  local path="$1"
  local output="$2"
  curl -fsS --max-time 10 "${DASHBOARD_URL}${path}" -o "$output"
}

curl_post_json_file() {
  local path="$1"
  local payload="$2"
  local output="$3"
  curl -fsS --max-time 10 -X POST "${DASHBOARD_URL}${path}" \
    -H "Content-Type: application/json" \
    -d "$payload" \
    -o "$output"
}

write_metadata() {
  python3 - "$ARTIFACT_DIR/metadata.json" "$TARGET_NAME" "$RAN_KIND" "$MODE" "$DASHBOARD_URL" "$SCENARIO" <<'PY'
import json
import os
import subprocess
import sys
from datetime import datetime, timezone

out = {
    "schema": "qcore.real_ran_capture.v1",
    "captured_at": datetime.now(timezone.utc).isoformat(),
    "target": sys.argv[2],
    "ran_kind": sys.argv[3],
    "mode": sys.argv[4],
    "dashboard_url": sys.argv[5],
    "scenario": sys.argv[6] or None,
}
try:
    out["git_commit"] = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    out["git_branch"] = subprocess.check_output(["git", "branch", "--show-current"], text=True).strip()
except Exception:
    pass
with open(sys.argv[1], "w", encoding="utf-8") as f:
    json.dump(out, f, indent=2, sort_keys=True)
    f.write("\n")
PY
}

wait_for_health() {
  local deadline=$((SECONDS + TIMEOUT_SECONDS))
  while [ "$SECONDS" -lt "$deadline" ]; do
    if curl_get_file /api/health "$ARTIFACT_DIR/health.json" 2>/dev/null; then
      if python3 - "$ARTIFACT_DIR/health.json" <<'PY'
import json
import sys
try:
    data = json.load(open(sys.argv[1], encoding="utf-8"))
except Exception:
    raise SystemExit(1)
raise SystemExit(0 if data.get("ready_for_use") is True else 1)
PY
      then
        return 0
      fi
    fi
    sleep "$POLL_SECONDS"
  done
  return 1
}

scrub_json_file() {
  local path="$1"
  python3 - "$path" <<'PY'
import json
import sys

path = sys.argv[1]
secret_keys = {"ki", "k", "op", "opc", "key", "private_key", "hn_private_key"}

def scrub(value, key=""):
    if isinstance(value, dict):
        return {k: scrub(v, k) for k, v in value.items()}
    if isinstance(value, list):
        return [scrub(v, key) for v in value]
    if key.lower() in secret_keys and isinstance(value, str) and value:
        return f"<redacted:{len(value)} chars>"
    return value

with open(path, encoding="utf-8") as f:
    data = json.load(f)
with open(path, "w", encoding="utf-8") as f:
    json.dump(scrub(data), f, indent=2, sort_keys=True)
    f.write("\n")
PY
}

run_reconcile() {
  if [ -z "$GNB_YAML" ] && [ -z "$UE_YAML" ]; then
    return 0
  fi
  python3 - "$DASHBOARD_URL" "$GNB_YAML" "$UE_YAML" "$ARTIFACT_DIR/reconcile.json" <<'PY'
import json
import sys
import urllib.error
import urllib.request

base, gnb_path, ue_path, out_path = sys.argv[1:5]
payload = {}
if gnb_path:
    with open(gnb_path, encoding="utf-8") as f:
        payload["gnb_yaml"] = f.read()
if ue_path:
    with open(ue_path, encoding="utf-8") as f:
        payload["ue_yaml"] = f.read()
req = urllib.request.Request(
    base.rstrip("/") + "/api/ran-config/reconcile",
    data=json.dumps(payload).encode(),
    headers={"Content-Type": "application/json"},
    method="POST",
)
try:
    with urllib.request.urlopen(req, timeout=10) as resp:
        body = resp.read().decode()
except urllib.error.HTTPError as exc:
    body = exc.read().decode()
    raise SystemExit(f"reconcile failed: HTTP {exc.code}: {body}")
with open(out_path, "w", encoding="utf-8") as f:
    f.write(body)
PY
  scrub_json_file "$ARTIFACT_DIR/reconcile.json"
}

trigger_simulator() {
  local response="$ARTIFACT_DIR/simulator-start.json"
  if [ -n "$SCENARIO" ]; then
    curl_post_json_file "/api/simulator/inject/${SCENARIO}" "{\"mode\":\"${MODE}\"}" "$response"
  else
    curl_post_json_file /api/simulator/start "{\"mode\":\"${MODE}\"}" "$response"
  fi
}

wait_simulator() {
  python3 - "$DASHBOARD_URL" "$TIMEOUT_SECONDS" "$POLL_SECONDS" "$ARTIFACT_DIR/simulator-status.json" <<'PY'
import json
import sys
import time
import urllib.request

base = sys.argv[1].rstrip("/")
timeout = int(sys.argv[2])
poll = float(sys.argv[3])
out_path = sys.argv[4]
deadline = time.time() + timeout
last = {}
while time.time() < deadline:
    try:
        with urllib.request.urlopen(base + "/api/simulator/status", timeout=5) as resp:
            last = json.load(resp)
    except Exception:
        time.sleep(poll)
        continue
    if last.get("state") in {"success", "failed"}:
        break
    time.sleep(poll)
with open(out_path, "w", encoding="utf-8") as f:
    json.dump(last, f, indent=2, sort_keys=True)
    f.write("\n")
if last.get("state") not in {"success", "failed"}:
    raise SystemExit(1)
print(last.get("last_journey", ""))
PY
}

wait_passive_journey() {
  python3 - "$DASHBOARD_URL" "$TIMEOUT_SECONDS" "$POLL_SECONDS" "$ARTIFACT_DIR/passive-status.json" <<'PY'
import json
import sys
import time
import urllib.request
from datetime import datetime

base = sys.argv[1].rstrip("/")
timeout = int(sys.argv[2])
poll = float(sys.argv[3])
out_path = sys.argv[4]
deadline = time.time() + timeout
success_terms = (
    "ngsetupresponse sent",
    "gnb accepted",
    "registration complete",
    "authentication request sent",
    "authentication vector generated",
    "pdu session",
    "initial context",
)
failure_terms = ("rejected", "transfer-syntax-error", "integrity check failed", "malformed", "unavailable")

def get_json(path):
    with urllib.request.urlopen(base + path, timeout=5) as resp:
        return json.load(resp)

def parse_ts(ev):
    ts = str(ev.get("timestamp") or "")
    try:
        return datetime.fromisoformat(ts.replace("Z", "+00:00")).timestamp()
    except Exception:
        return 0

best = {"state": "timeout", "journey": "", "cause": "", "events": 0}
while time.time() < deadline:
    try:
        journeys = get_json("/api/journeys")
    except Exception:
        time.sleep(poll)
        continue
    candidates = []
    for journey in journeys:
        try:
            events = get_json(f"/api/journeys/{journey}/events")
        except Exception:
            continue
        if not events:
            continue
        messages = [str(ev.get("message", "")).lower() for ev in events]
        text = "\n".join(messages)
        terminal_errors = [
            msg for ev, msg in zip(events, messages)
            if ev.get("severity") == "error" and not ("authentication failure" in msg and "sqn_failure" in msg)
        ]
        failed = bool(terminal_errors) or any(term in text for term in failure_terms)
        passed = any(term in text for term in success_terms)
        latest = max(parse_ts(ev) for ev in events)
        candidates.append((latest, journey, events, failed, passed, text))
    if candidates:
        latest, journey, events, failed, passed, text = sorted(candidates, reverse=True)[0]
        if failed:
            best = {"state": "failed", "journey": journey, "cause": "failure event observed", "events": len(events)}
            break
        if passed:
            best = {"state": "success", "journey": journey, "cause": "", "events": len(events)}
            break
        best = {"state": "observed", "journey": journey, "cause": "", "events": len(events)}
    time.sleep(poll)
with open(out_path, "w", encoding="utf-8") as f:
    json.dump(best, f, indent=2, sort_keys=True)
    f.write("\n")
print(best.get("journey", ""))
raise SystemExit(0 if best.get("state") in {"success", "failed", "observed"} else 1)
PY
}

fetch_journey_artifacts() {
  local journey="$1"
  if [ -z "$journey" ]; then
    echo "[]" > "$ARTIFACT_DIR/trace.json"
    return 0
  fi
  curl_get_file "/api/journeys/${journey}/events" "$ARTIFACT_DIR/trace.json" || echo "[]" > "$ARTIFACT_DIR/trace.json"
}

fetch_diagnosis() {
  local journey="$1"
  if [ -z "$journey" ]; then
    return 0
  fi
  curl_get_file "/api/diagnostics/journey/${journey}" "$ARTIFACT_DIR/diagnosis.json" 2>/dev/null || true
}

capture_state() {
  python3 - "$ARTIFACT_DIR" <<'PY'
import json
import pathlib
import sys

artifact = pathlib.Path(sys.argv[1])
for name in ("simulator-status.json", "passive-status.json"):
    path = artifact / name
    if path.exists():
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except Exception:
            continue
        print(data.get("state") or "unknown")
        raise SystemExit(0)
print("unknown")
PY
}

write_timings() {
  local started_ms="$1"
  local ttrc_started_ms="$2"
  python3 - "$ARTIFACT_DIR" "$started_ms" "$ttrc_started_ms" "$MODE" "$RAN_KIND" "$SCENARIO" <<'PY'
import json
import pathlib
import sys
import time

artifact = pathlib.Path(sys.argv[1])
started_ms = int(sys.argv[2])
ttrc_started_ms = int(sys.argv[3] or "0")
mode, ran_kind, scenario = sys.argv[4:7]
now_ms = int(time.time() * 1000)

status = {}
for name in ("simulator-status.json", "passive-status.json"):
    p = artifact / name
    if p.exists():
        status = json.loads(p.read_text(encoding="utf-8"))
        break

state = status.get("state") or "unknown"
journey = status.get("last_journey") or status.get("journey") or ""
cause = status.get("last_cause") or status.get("failed_step") or status.get("cause") or ""
failed_step = status.get("failed_step") or ""
result = "pass" if state == "success" else "fail" if state == "failed" else state

diag_path = artifact / "diagnosis.json"
diagnosis = None
if diag_path.exists() and diag_path.stat().st_size:
    try:
        diagnosis = json.loads(diag_path.read_text(encoding="utf-8"))
    except Exception:
        diagnosis = None

ttrc_s = None
if diagnosis and (diagnosis.get("RootCause") or diagnosis.get("Explanation")) and ttrc_started_ms:
    ttrc_s = round((now_ms - ttrc_started_ms) / 1000, 3)

payload = {
    "schema": "qcore.real_ran_timings.v1",
    "mode": mode,
    "ran_kind": ran_kind,
    "scenario": scenario or None,
    "result": result,
    "state": state,
    "journey_id": journey,
    "failed_step": failed_step,
    "cause": cause,
    "ttfc_s": round((now_ms - started_ms) / 1000, 3),
    "ttrc_s": ttrc_s,
}
(artifact / "timings.json").write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY
}

write_summary() {
  python3 - "$ARTIFACT_DIR" <<'PY'
import json
import pathlib
import sys

artifact = pathlib.Path(sys.argv[1])

def load(name, default):
    path = artifact / name
    if not path.exists() or path.stat().st_size == 0:
        return default
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except Exception:
        return default

meta = load("metadata.json", {})
timings = load("timings.json", {})
trace = load("trace.json", [])
diag = load("diagnosis.json", {})
reconcile = load("reconcile.json", None)

lines = [
    "# QCore Real-RAN Evidence Bundle",
    "",
    f"- Target: {meta.get('target', '-')}",
    f"- RAN kind: {meta.get('ran_kind', '-')}",
    f"- Mode: {meta.get('mode', '-')}",
    f"- Scenario: {meta.get('scenario') or 'none'}",
    f"- Result: {timings.get('result', '-')}",
    f"- TTFC: {timings.get('ttfc_s', '-')}s",
    f"- TTRC: {timings.get('ttrc_s') if timings.get('ttrc_s') is not None else '-'}",
    f"- Journey: {timings.get('journey_id') or '-'}",
    f"- Events captured: {len(trace) if isinstance(trace, list) else '-'}",
    "",
]
if reconcile is not None:
    lines += [
        "## Reconcile",
        "",
        f"- OK: {reconcile.get('ok')}",
        f"- Summary: {reconcile.get('summary', '-')}",
        "",
    ]
if diag:
    lines += [
        "## Diagnosis",
        "",
        f"- Matched: {diag.get('Matched')}",
        f"- Root cause: {diag.get('RootCause') or '-'}",
        f"- Fix: {diag.get('Fix') or '-'}",
        "",
    ]
lines += [
    "## Files",
    "",
    "- `metadata.json`",
    "- `health.json`",
    "- `ran-config.json`",
    "- `trace.json`",
    "- `timings.json`",
    "- `diagnosis.json` if a diagnosis was available",
    "- `reconcile.json` if RAN/UE config files were supplied",
    "",
    "Scope caveat: this is one run for one target. It is evidence, not a conformance claim.",
    "",
]
(artifact / "summary.md").write_text("\n".join(lines), encoding="utf-8")
PY
}

STARTED_MS="$(now_ms)"
TTRC_STARTED_MS=0
write_metadata

echo "[capture] target=${TARGET_NAME} mode=${MODE} ran_kind=${RAN_KIND} artifact=${ARTIFACT_DIR}"
if [ "$NO_UP" -eq 0 ]; then
  echo "[capture] starting QCore stack"
  if [ "$MODE" = "5g" ]; then
    COMPOSE_PROFILES=5g docker compose -f deployments/docker/docker-compose.yml up -d --build
  else
    docker compose -f deployments/docker/docker-compose.yml up -d --build
  fi
fi

cleanup() {
  if [ "$NO_UP" -eq 0 ] && [ "$KEEP_STACK" -eq 0 ]; then
    docker compose -f deployments/docker/docker-compose.yml --profile 5g down >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

echo "[capture] waiting for dashboard health"
if ! wait_for_health; then
  echo "::error::dashboard did not become ready at ${DASHBOARD_URL}"
  write_timings "$STARTED_MS" "$TTRC_STARTED_MS"
  write_summary
  exit 1
fi

if curl_get_file /api/ran-config "$ARTIFACT_DIR/ran-config.json" 2>/dev/null; then
  scrub_json_file "$ARTIFACT_DIR/ran-config.json"
fi
run_reconcile || true

JOURNEY_ID=""
if [ -n "$SCENARIO" ] || [ "$RAN_KIND" = "simulator" ]; then
  echo "[capture] triggering dashboard simulator"
  TTRC_STARTED_MS="$(now_ms)"
  trigger_simulator
  JOURNEY_ID="$(wait_simulator || true)"
else
  echo "[capture] watching for an external/bundled RAN journey"
  JOURNEY_ID="$(wait_passive_journey || true)"
fi

fetch_journey_artifacts "$JOURNEY_ID"
if [ -n "$JOURNEY_ID" ] && [ "$(capture_state)" = "failed" ]; then
  fetch_diagnosis "$JOURNEY_ID"
fi
write_timings "$STARTED_MS" "$TTRC_STARTED_MS"
write_summary

echo "[capture] wrote ${ARTIFACT_DIR}"
echo "::notice::QCore real-RAN evidence bundle: ${ARTIFACT_DIR}"
