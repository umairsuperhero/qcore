#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

DASHBOARD_URL="${QCORE_DASHBOARD_URL:-http://localhost:3000}"
OUTPUT_DIR="${QCORE_DEMO_OUTPUT_DIR:-docs/outreach/assets}"
TIMEOUT_SECONDS="${QCORE_DEMO_TIMEOUT_SECONDS:-300}"
POLL_SECONDS="${QCORE_DEMO_POLL_SECONDS:-2}"
PLAYWRIGHT_VERSION="${QCORE_DEMO_PLAYWRIGHT_VERSION:-1.56.1}"
START_STACK=1
KEEP_STACK=0
COLD_START=1

usage() {
  cat <<'EOF'
Usage: scripts/demo/record.sh [options]

Records the README demo from a real QCore dashboard run:
  make up-5g -> dashboard ready -> happy path -> wrong_ki -> diagnosis

Options:
  --dashboard-url URL   Dashboard URL (default: http://localhost:3000)
  --output DIR          Output directory (default: docs/outreach/assets)
  --timeout SECONDS     Max wait for dashboard readiness (default: 300)
  --no-up               Use an already-running QCore stack
  --keep-stack          Leave containers running after recording
  --no-cold             Do not run docker compose down before make up-5g
  -h, --help            Show this help

Environment:
  QCORE_DASHBOARD_URL, QCORE_DEMO_OUTPUT_DIR, QCORE_DEMO_TIMEOUT_SECONDS,
  QCORE_DEMO_PLAYWRIGHT_VERSION.

Notes:
  - Linux captures are the evidence-grade path for native SCTP/TUN.
  - macOS Docker Desktop can record the real dashboard/simulator path, but the
    evidence metadata will label it as non-native SCTP/TUN.
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --dashboard-url)
      DASHBOARD_URL="${2:-}"
      shift 2
      ;;
    --output)
      OUTPUT_DIR="${2:-}"
      shift 2
      ;;
    --timeout)
      TIMEOUT_SECONDS="${2:-}"
      shift 2
      ;;
    --no-up)
      START_STACK=0
      shift
      ;;
    --keep-stack)
      KEEP_STACK=1
      shift
      ;;
    --no-cold)
      COLD_START=0
      shift
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

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "$1 is required" >&2
    exit 1
  fi
}

need curl
need docker
need npm
need node
need python3

if ! command -v ffmpeg >/dev/null 2>&1; then
  cat >&2 <<'EOF'
ffmpeg is required to convert the captured browser video into README-ready mp4/gif.
Install it first, then rerun:
  macOS: brew install ffmpeg
  Ubuntu: sudo apt-get install -y ffmpeg
EOF
  exit 1
fi

mkdir -p "$OUTPUT_DIR"
TERMINAL_LOG="$OUTPUT_DIR/qcore-demo-terminal.txt"
EVIDENCE_JSON="$OUTPUT_DIR/qcore-demo-evidence.json"
RAW_WEBM="$OUTPUT_DIR/qcore-demo.webm"
MP4_OUT="$OUTPUT_DIR/qcore-demo.mp4"
GIF_OUT="$OUTPUT_DIR/qcore-demo.gif"
PW_DIR="$ROOT/.tmp/demo-playwright"

cleanup() {
  if [ "$START_STACK" -eq 1 ] && [ "$KEEP_STACK" -eq 0 ]; then
    docker compose -f deployments/docker/docker-compose.yml --profile 5g down >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

wait_for_dashboard() {
  local deadline=$((SECONDS + TIMEOUT_SECONDS))
  echo "[demo] waiting for dashboard at ${DASHBOARD_URL}/api/health"
  while [ "$SECONDS" -lt "$deadline" ]; do
    if curl -fsS --max-time 5 "${DASHBOARD_URL}/api/health" | python3 -c '
import json, sys
try:
    body = json.load(sys.stdin)
except Exception:
    raise SystemExit(1)
raise SystemExit(0 if body.get("ready_for_use") is True else 1)
' >/dev/null 2>&1; then
      echo "[demo] dashboard ready"
      return 0
    fi
    sleep "$POLL_SECONDS"
  done
  echo "dashboard did not become ready within ${TIMEOUT_SECONDS}s" >&2
  return 1
}

ensure_playwright() {
  if [ ! -d "$PW_DIR/node_modules/playwright" ]; then
    mkdir -p "$PW_DIR"
    (
      cd "$PW_DIR"
      if [ ! -f package.json ]; then
        npm init -y >/dev/null
      fi
      npm install --no-audit --no-fund "playwright@${PLAYWRIGHT_VERSION}"
      npx playwright install chromium
    )
  fi
}

if [ "$START_STACK" -eq 1 ]; then
  {
    echo "$ make up-5g"
    echo "[capture] real command output follows; long Docker build/startup may be time-compressed in the video."
  } > "$TERMINAL_LOG"
  if [ "$COLD_START" -eq 1 ]; then
    docker compose -f deployments/docker/docker-compose.yml --profile 5g down >>"$TERMINAL_LOG" 2>&1 || true
  fi
  make up-5g 2>&1 | tee -a "$TERMINAL_LOG"
else
  {
    echo "$ make up-5g"
    echo "[capture] --no-up was used; recording attached to an already-running QCore stack."
  } > "$TERMINAL_LOG"
fi

wait_for_dashboard
ensure_playwright

QCORE_DASHBOARD_URL="$DASHBOARD_URL" \
QCORE_DEMO_OUTPUT_DIR="$OUTPUT_DIR" \
QCORE_DEMO_TERMINAL_LOG="$TERMINAL_LOG" \
QCORE_DEMO_EVIDENCE_JSON="$EVIDENCE_JSON" \
NODE_PATH="$PW_DIR/node_modules" \
  node scripts/demo/record-dashboard.mjs

if [ ! -f "$RAW_WEBM" ]; then
  echo "expected raw video not found: $RAW_WEBM" >&2
  exit 1
fi

echo "[demo] converting $RAW_WEBM -> $MP4_OUT"
ffmpeg -hide_banner -loglevel error -y -i "$RAW_WEBM" \
  -movflags +faststart -pix_fmt yuv420p -vf "scale=1280:-2" "$MP4_OUT"

echo "[demo] converting $RAW_WEBM -> $GIF_OUT"
PALETTE="$(mktemp "${TMPDIR:-/tmp}/qcore-demo-palette.XXXXXX.png")"
ffmpeg -hide_banner -loglevel error -y -i "$RAW_WEBM" \
  -vf "fps=6,scale=760:-1:flags=lanczos,palettegen=max_colors=96" "$PALETTE"
ffmpeg -hide_banner -loglevel error -y -i "$RAW_WEBM" -i "$PALETTE" \
  -lavfi "fps=6,scale=760:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5" "$GIF_OUT"
rm -f "$PALETTE"

GIF_BYTES="$(wc -c < "$GIF_OUT" | tr -d ' ')"
if [ "$GIF_BYTES" -gt 10485760 ]; then
  echo "gif exceeds README hard cap: ${GIF_BYTES} bytes > 10485760" >&2
  exit 1
fi

python3 - "$EVIDENCE_JSON" "$MP4_OUT" "$GIF_OUT" <<'PY'
import json
import os
import sys

evidence_path, mp4_path, gif_path = sys.argv[1:4]
with open(evidence_path, "r", encoding="utf-8") as f:
    evidence = json.load(f)
evidence["outputs"] = {
    "webm": "qcore-demo.webm",
    "mp4": os.path.basename(mp4_path),
    "gif": os.path.basename(gif_path),
    "gif_bytes": os.path.getsize(gif_path),
    "mp4_bytes": os.path.getsize(mp4_path),
}
with open(evidence_path, "w", encoding="utf-8") as f:
    json.dump(evidence, f, indent=2, sort_keys=True)
    f.write("\n")
PY

echo "[demo] wrote:"
echo "  $MP4_OUT"
echo "  $GIF_OUT"
echo "  $EVIDENCE_JSON"
