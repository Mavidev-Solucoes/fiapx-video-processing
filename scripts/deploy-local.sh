#!/usr/bin/env bash
set -euo pipefail
# Run from repository root; two worker replicas by default.
docker compose --profile monitoring up -d --build --scale worker="${WORKERS:-2}"
docker compose --profile monitoring ps
printf "Grafana dashboard: http://localhost:3000/d/fiapx-operations (admin / local default password)\n"
echo 'FIAP X is available on http://localhost:8080/'
