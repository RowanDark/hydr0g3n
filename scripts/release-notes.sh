#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/release-notes.sh [<from-tag>] [<to-ref>]

Generate a Markdown release note template from merge commits between two refs.
If <from-tag> is omitted, the script uses the latest reachable tag. <to-ref>
defaults to HEAD.
USAGE
}

if [[ ${1:-} == "-h" || ${1:-} == "--help" ]]; then
  usage
  exit 0
fi

if ! command -v git >/dev/null 2>&1; then
  echo "git is required" >&2
  exit 1
fi

from_ref="${1:-}"
to_ref="${2:-HEAD}"

if [[ -z "${from_ref}" ]]; then
  if ! from_ref=$(git describe --tags --abbrev=0 2>/dev/null); then
    echo "Could not determine the previous tag. Pass it explicitly." >&2
    exit 1
  fi
fi

if ! git rev-parse --verify "${from_ref}" >/dev/null 2>&1; then
  echo "Unknown start ref: ${from_ref}" >&2
  exit 1
fi

if ! git rev-parse --verify "${to_ref}" >/dev/null 2>&1; then
  echo "Unknown end ref: ${to_ref}" >&2
  exit 1
fi

mapfile -t subjects < <(git log "${from_ref}..${to_ref}" --merges --pretty=format:%b)

if [[ ${#subjects[@]} -eq 0 ]]; then
  mapfile -t subjects < <(git log "${from_ref}..${to_ref}" --pretty=format:%s)
fi

printf '# Release Notes for %s\n\n' "${to_ref}"
cat <<'TEMPLATE'
## Summary

- TODO

## Changes
TEMPLATE

if [[ ${#subjects[@]} -eq 0 ]]; then
  echo "- No pull requests merged in this range."
else
  for subject in "${subjects[@]}"; do
    clean_subject=$(echo "${subject}" | sed '/^$/d' | head -n1)
    if [[ -n "${clean_subject}" ]]; then
      printf -- '- %s\n' "${clean_subject}"
    fi
  done
fi
