#!/usr/bin/env bash
set -euo pipefail
# Usage: FIAPX_TAG=<full commit sha> WORKERS=2 bash scripts/deploy-release.sh
# Published images must exist. Run docker login ghcr.io for private packages.
export FIAPX_IMAGE_BASE="${FIAPX_IMAGE_BASE:-ghcr.io/mavidev-solucoes/fiapx-video-processing}"
export FIAPX_TAG="${FIAPX_TAG:-latest}"
docker compose -f docker-compose.yml -f docker-compose.release.yml --profile monitoring pull api worker prometheus grafana
docker compose -f docker-compose.yml -f docker-compose.release.yml --profile monitoring up -d --no-build --scale worker="${WORKERS:-2}"
docker compose -f docker-compose.yml -f docker-compose.release.yml --profile monitoring ps
echo 'Published FIAP X images are running on http://localhost:8080/'
