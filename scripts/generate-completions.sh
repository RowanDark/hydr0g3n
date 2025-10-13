#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out_dir="${repo_root}/completions"
mkdir -p "${out_dir}"

for shell in bash zsh fish; do
  out_file="${out_dir}/hydro.${shell}"
  echo "Generating ${shell} completions -> ${out_file}" >&2
  go run ./cmd/hydro --completion-script "${shell}" >"${out_file}"
  chmod 644 "${out_file}"
done
