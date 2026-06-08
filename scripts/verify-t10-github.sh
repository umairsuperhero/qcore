#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

branch="${1:-$(git branch --show-current)}"
if [ -z "$branch" ]; then
  echo "Cannot determine branch. Pass one explicitly: scripts/verify-t10-github.sh <branch>"
  exit 1
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "gh CLI is required for GitHub T10 verification."
  exit 1
fi

echo "Triggering ueransim-interop for branch: $branch"
gh workflow run ueransim-interop.yml --ref "$branch"

echo "Waiting for GitHub to create the run..."
run_id=""
for _ in $(seq 1 30); do
  run_id="$(gh run list --workflow ueransim-interop.yml --branch "$branch" --limit 1 --json databaseId,status \
    --jq '.[] | select(.status != "completed") | .databaseId' 2>/dev/null | head -1 || true)"
  if [ -n "$run_id" ]; then
    break
  fi
  sleep 2
done

if [ -z "$run_id" ]; then
  run_id="$(gh run list --workflow ueransim-interop.yml --branch "$branch" --limit 1 --json databaseId \
    --jq '.[0].databaseId' 2>/dev/null || true)"
fi

if [ -z "$run_id" ]; then
  echo "Could not find a ueransim-interop run for $branch."
  exit 1
fi

echo "Watching run: $run_id"
gh run watch "$run_id" --exit-status

echo
echo "Checking for T10 data-plane pass annotation/log line..."
if gh run view "$run_id" --log | grep -q "T10 DATA PLANE PASS"; then
  echo "T10 DATA PLANE PASS found in run $run_id."
else
  echo "Run $run_id passed, but the expected T10 DATA PLANE PASS line was not found."
  exit 1
fi
