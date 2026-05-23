#!/usr/bin/env bash
# Run every lint that Go Report Card runs, plus go vet. Any failure here is
# a failure on goreportcard.com.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

# Files to inspect: every .go file except generated clients and the build
# scratch directory.
mapfile -t GO_FILES < <(find . -name "*.go" -not -name "zz_generated.go" -not -path "./.build/*")

echo "==> gofmt -s"
unformatted="$(gofmt -s -l "${GO_FILES[@]}" || true)"
if [[ -n "${unformatted}" ]]; then
  echo "gofmt -s needs to run on:"
  echo "${unformatted}"
  exit 1
fi

echo "==> go vet"
go vet ./...

echo "==> ineffassign"
ineffassign ./...

echo "==> misspell"
misspell -error "${GO_FILES[@]}" README.md docs/

echo "==> gocyclo (threshold 15)"
gocyclo -over 15 -ignore "zz_generated|\.build" . | tee /tmp/gocyclo.out
if [[ -s /tmp/gocyclo.out ]]; then
  exit 1
fi

echo "==> golint"
out="$(golint ./... | grep -v "zz_generated\|\.build" || true)"
if [[ -n "${out}" ]]; then
  echo "${out}"
  exit 1
fi

echo "all checks passed"
