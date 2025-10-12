#!/usr/bin/env bash
# Demonstration helper that authenticates once and prints a JSON payload that
# Hydro can consume via --pre-hook. The emitted cookie will be attached to every
# subsequent fuzzing request.
set -euo pipefail

LOGIN_URL="${HYDRO_LOGIN_URL:-https://example.com/api/login}"
USERNAME="${HYDRO_USERNAME:-demo}"
PASSWORD="${HYDRO_PASSWORD:-demo}"

# Capture response headers so we can extract the issued session cookie.
tmp_headers=$(mktemp)
trap 'rm -f "${tmp_headers}"' EXIT

curl -sS -D "${tmp_headers}" -o /dev/null \
  -X POST "${LOGIN_URL}" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}"

session_cookie=$(awk 'BEGIN {IGNORECASE=1} /^Set-Cookie:/ {print $2; exit}' "${tmp_headers}")
session_cookie="${session_cookie%%;*}"

if [[ -z "${session_cookie}" ]]; then
  echo "Failed to capture session cookie" >&2
  exit 1
fi

printf '{"cookie":"%s"}\n' "${session_cookie}"
