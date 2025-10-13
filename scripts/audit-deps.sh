#!/usr/bin/env bash
set -euo pipefail

if ! command -v go >/dev/null 2>&1; then
  echo "go is required to audit dependencies" >&2
  exit 1
fi

echo "Enumerating Go module dependencies..."
go list -m all

echo
if command -v govulncheck >/dev/null 2>&1; then
  echo "Running govulncheck for known vulnerabilities..."
  govulncheck ./...
else
  cat <<'MSG' >&2
[warning] govulncheck is not installed. Install govulncheck to perform
an automated vulnerability scan. See https://go.dev/blog/vuln for details.
MSG
fi
