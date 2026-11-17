#!/usr/bin/env bash
set -euo pipefail

folder="$(mktemp -d)"
trap 'rm -rf "$folder"' EXIT
report="$folder/summary.txt"

expect_result() {
  local percentage="$1"
  local expected_status="$2"
  printf 'github.com/example/pkg/file.go:1:\tFoo\t80.0%%\ntotal:\t(statements)\t%s\n' "$percentage" > "$report"
  if bash scripts/check-coverage.sh "$report" >"$folder/output.txt" 2>&1; then
    result=0
  else
    result=1
  fi
  if [[ "$result" != "$expected_status" ]]; then
    echo "Unexpected gate result: ${percentage}, expected exit ${expected_status}"
    cat "$folder/output.txt"
    exit 1
  fi
}

expect_result "79.9%" 1
expect_result "39.6%" 1
expect_result "80.0%" 0
expect_result "80.1%" 0
expect_result "100.0%" 0
expect_result "0.0%" 1
expect_result "not-a-percent" 1

printf 'total:\t(statements)\t85.0%%\ntotal:\t(statements)\t85.0%%\n' > "$report"
if bash scripts/check-coverage.sh "$report" >"$folder/output.txt" 2>&1; then
  echo "A report with duplicate total lines was accepted"
  exit 1
fi
: > "$report"
if bash scripts/check-coverage.sh "$report" >"$folder/output.txt" 2>&1; then
  echo "An empty report was accepted"
  exit 1
fi
echo "PASS: coverage gate boundary, malformed input, and missing report cases"
