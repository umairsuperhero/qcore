#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "== qcore verify-fast =="
echo "branch: $(git branch --show-current 2>/dev/null || echo unknown)"
echo "commit: $(git rev-parse --short HEAD 2>/dev/null || echo unknown)"

changed="$(
  {
    git diff --name-only --diff-filter=ACMR HEAD -- || true
    git diff --cached --name-only --diff-filter=ACMR -- || true
    git ls-files --others --exclude-standard || true
  } | sort -u
)"

if [ -z "$changed" ]; then
  echo "No local file changes detected; running lightweight status checks only."
else
  echo "Changed files:"
  echo "$changed" | sed 's/^/  /'
fi

echo
echo "== whitespace =="
git diff --check

if echo "$changed" | grep -qE '\.go$'; then
  echo
  echo "== go changed: gofmt + targeted package tests in golang:1.23 =="
  go_files="$(echo "$changed" | grep -E '\.go$' || true)"
  if [ -n "$go_files" ]; then
    not_formatted="$(docker run --rm -v "$PWD":/src -w /src golang:1.23 sh -c "gofmt -l $go_files" | tr '\n' ' ')"
    if [ -n "$not_formatted" ]; then
      echo "Go files need gofmt: $not_formatted"
      exit 1
    fi
    packages="$(echo "$go_files" | xargs -n1 dirname | sort -u | sed 's#^#./#' | tr '\n' ' ')"
    docker run --rm -v "$PWD":/src -w /src -v qcore-gomod:/go/pkg/mod golang:1.23 sh -c "go test $packages"
  fi
else
  echo
  echo "== go =="
  echo "No Go changes detected."
fi

if echo "$changed" | grep -qE '(^pkg/dashboard/web/|package-lock\.json$|package\.json$)'; then
  echo
  echo "== dashboard changed: typecheck + build =="
  cd "$ROOT/pkg/dashboard/web"
  if [ ! -d node_modules ]; then
    npm install --no-audit --no-fund
  fi
  npx tsc --noEmit --pretty false
  npm run build
  cd "$ROOT"
else
  echo
  echo "== dashboard =="
  echo "No dashboard web changes detected."
fi

if echo "$changed" | grep -qE '(^README\.md$|^AGENTS\.md$|^CLAUDE\.md$|^docs/|^\.github/workflows/)'; then
  echo
  echo "== status-doc stale claim scan =="
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
    echo "Found stale current-state language. Update or move it to explicitly historical context."
    exit 1
  fi
else
  echo
  echo "== status-doc stale claim scan =="
  echo "No status/docs/workflow changes detected."
fi

echo
echo "verify-fast passed."
