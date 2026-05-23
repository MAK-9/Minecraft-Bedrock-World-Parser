#!/usr/bin/env bash
set -euo pipefail

echo "==> Generating fixtures..."
go run testdata/generate_fixtures.go

echo "==> Running tests..."
go test ./... -v -count=1

echo "==> Running vet..."
go vet ./...

echo ""
echo "All checks passed."
