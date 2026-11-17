#!/usr/bin/env bash
set -euo pipefail
# Run from repository root; two worker replicas by default.
docker compose up -d --build --scale worker="${WORKERS:-2}"
docker compose ps
echo 'FIAP X is available on http://localhost:8080/'
