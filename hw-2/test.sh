#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

go vet ./...
go test -race -count=1 ./...

if command -v golangci-lint >/dev/null; then
	golangci-lint run ./...
fi
