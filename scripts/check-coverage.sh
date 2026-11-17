#!/usr/bin/env bash
set -euo pipefail

# Enforce statement coverage of ALL Go packages collected by ./...
# Do not exclude packages or relax the minimum based on CI configuration.
readonly minimum="80.0"
readonly report="${1:-coverage-summary.txt}"

if [[ ! -s "$report" ]]; then
  echo "::error title=Missing Go coverage report::Expected a non-empty $report" >&2
  exit 1
fi

# go tool cover -func emits one 'total:' line ending in a percentage.
measured="$(awk '$1 == "total:" {value=$NF; count++} END {if(count == 1) print value}' "$report")"
if [[ ! "$measured" =~ ^[0-9]+([.][0-9]+)?%$ ]]; then
  echo "::error title=Invalid Go coverage report::No unique numeric total percentage found in $report" >&2
  exit 1
fi

percentage="${measured%\%}"
if LC_ALL=C awk -v measured="$percentage" -v minimum="$minimum"     'BEGIN { exit !(measured + 0 >= minimum + 0) }'; then
  echo "PASS: Go statement coverage is $measured (minimum ${minimum}%)."
else
  echo "::error title=Go coverage below 80%::Measured $measured; required at least ${minimum}% across all Go packages." >&2
  exit 1
fi
