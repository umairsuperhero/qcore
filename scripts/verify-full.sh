#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "== qcore verify-full =="
echo "branch: $(git branch --show-current 2>/dev/null || echo unknown)"
echo "commit: $(git rev-parse --short HEAD 2>/dev/null || echo unknown)"

echo
echo "== whitespace =="
git diff --check

echo
echo "== go build/vet/test in golang:1.23 =="
docker run --rm -v "$PWD":/src -w /src -v qcore-gomod:/go/pkg/mod golang:1.23 \
  sh -c "go build ./... && go vet ./... && go test ./..."

echo
echo "== go race tests in golang:1.23 =="
docker run --rm -v "$PWD":/src -w /src -v qcore-gomod:/go/pkg/mod golang:1.23 \
  sh -c "go test -race ./..."

echo
echo "== dashboard typecheck + build =="
cd "$ROOT/pkg/dashboard/web"
npm ci --no-audit --no-fund
npx tsc --noEmit --pretty false
npm run build

echo
echo "verify-full passed."
