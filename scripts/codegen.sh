#!/usr/bin/env bash
# Refresh the vendored OpenAPI specs and the preprocessed copies embedded by
# the CLI. The CLI is spec-driven (cmd/twoctl/cli/operations.go walks the
# embedded spec at startup), so no Go client code is generated here.
#
# Pipeline per spec:
#   openapi/<api>-api.yaml
#     -> .build/<api>-api.processed.yaml          (3.1 -> 3.0 rewrite)
#     -> cmd/twoctl/cli/specs/<api>-api.processed.yaml  (embedded)
#
# Run from the repo root.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

APIS=(
  checkout billing-account repay recourse company limits
  autofill business-registration marketplace
  trade-account-v2 trade-account-v3 webhooks
)

mkdir -p .build cmd/twoctl/cli/specs

for api in "${APIS[@]}"; do
  spec="openapi/${api}-api.yaml"
  processed=".build/${api}-api.processed.yaml"

  echo "==> ${api}"
  go run ./internal/preprocess "${spec}" "${processed}"
  cp "${processed}" "cmd/twoctl/cli/specs/${api}-api.processed.yaml"
done

echo "==> go build"
go build ./...

echo "done."
