#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "== qcore fact-check =="
echo "branch: $(git branch --show-current 2>/dev/null || echo unknown)"
echo "commit: $(git rev-parse --short HEAD 2>/dev/null || echo unknown)"

echo
echo "== workspace =="
git status --short --branch

echo
echo "== whitespace =="
git diff --check

echo
echo "== stale current-state claim scan =="
scan_targets=(
  README.md
  AGENTS.md
  CLAUDE.md
  docs/ueransim-compat.md
  docs/wiki.md
  docs/v1-gap-closure-plan.md
  docs/3gpp-tracking.md
  .github/workflows
)
if rg -n --ignore-case \
  'T10.*not shipped|data-plane pending|Current T10 Gap|remaining T10 gate|Data-plane validation remains|PDU Session Accept and data-plane.*remain|CI UPF still uses DummyEgress' \
  "${scan_targets[@]}"; then
  echo "WRONG: stale current-state language found."
  exit 1
else
  echo "VERIFIED: no stale current-state T10/data-plane phrases in primary status surfaces."
fi

echo
echo "== PR/check evidence =="
if command -v gh >/dev/null 2>&1 && gh pr view --json number >/dev/null 2>&1; then
  pr_number="$(gh pr view --json number --jq '.number')"
  pr_title="$(gh pr view --json title --jq '.title')"
  pr_url="$(gh pr view --json url --jq '.url')"
  head_oid="$(gh pr view --json headRefOid --jq '.headRefOid')"
  echo "PR #$pr_number: $pr_title"
  echo "$pr_url"
  echo "head: $head_oid"
  if gh pr checks "$pr_number" --watch=false; then
    echo "VERIFIED: PR checks are currently successful."
  else
    echo "UNVERIFIABLE: PR checks are pending or failing; inspect GitHub before claiming green."
  fi
else
  echo "UNVERIFIABLE: no current PR detected or gh is unavailable."
fi

echo
echo "fact-check complete."
