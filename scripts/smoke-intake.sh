#!/usr/bin/env bash
set -euo pipefail

cleanup() {
  code=$?
  if [[ $code -ne 0 ]]; then
    echo "== Container diagnostics =="
    docker compose ps || true
    docker compose logs --no-color --tail 120 || true
  fi
  docker compose down -v --remove-orphans || true
  exit "$code"
}
trap cleanup EXIT

docker compose up -d --build
ready=0
for i in $(seq 1 75); do
  if curl -fsS http://127.0.0.1:8080/health/live >/dev/null 2>&1; then ready=1; break; fi
  sleep 2
done
if [[ $ready != 1 ]]; then echo "API did not become ready"; exit 1; fi

register() {
  curl --fail-with-body -fsS http://127.0.0.1:8080/api/auth/register \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"$1\",\"password\":\"a-very-strong-local-password-2026\"}" |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["accessToken"])'
}

user_a="$(register "smoke-a@example.com")"
user_b="$(register "smoke-b@example.com")"
printf 'video-upload-smoke-file' > /tmp/fiapx-smoke.mp4

upload() {
  curl --fail-with-body -fsS \
    -X POST http://127.0.0.1:8080/api/jobs \
    -H "Authorization: Bearer $1" \
    -F 'video=@/tmp/fiapx-smoke.mp4;filename=sample.mp4'
}

job_response="$(upload "$user_a")"
job_id="$(printf '%s' "$job_response" | python3 -c 'import json,sys; j=json.load(sys.stdin); assert j["status"]=="queued"; print(j["id"])')"
echo "accepted queued job: $job_id"

code="$(curl -sS -o /tmp/fiapx-private.json -w '%{http_code}' \
  -H "Authorization: Bearer $user_b" \
  "http://127.0.0.1:8080/api/jobs/$job_id")"
if [[ "$code" != "404" ]]; then echo "cross-user isolation failed: $code"; exit 1; fi

echo "== Wait for initial outbox dispatch =="
published=0
for i in $(seq 1 30); do
  published="$(docker compose exec -T postgres psql -U fiapx -d fiapx -Atc \
    "SELECT count(*) FROM video_job_outbox WHERE published_at IS NOT NULL")"
  if [[ "$published" -ge 1 ]]; then break; fi
  sleep 2
done
if [[ "$published" -lt 1 ]]; then echo "initial outbox dispatch failed"; exit 1; fi

echo "== Stop broker and accept second job =="
docker compose stop rabbitmq
second_response="$(upload "$user_a")"
second_id="$(printf '%s' "$second_response" | python3 -c 'import json,sys; j=json.load(sys.stdin); assert j["status"]=="queued"; print(j["id"])')"
echo "accepted while broker offline: $second_id"

pending="$(docker compose exec -T postgres psql -U fiapx -d fiapx -Atc \
  "SELECT count(*) FROM video_job_outbox WHERE job_id = '$second_id' AND published_at IS NULL")"
if [[ "$pending" != "1" ]]; then echo "missing durable outbox entry for second job"; exit 1; fi

echo "== Restart broker and verify recovery =="
docker compose start rabbitmq
recovered=0
for i in $(seq 1 50); do
  recovered="$(docker compose exec -T postgres psql -U fiapx -d fiapx -Atc \
    "SELECT count(*) FROM video_job_outbox WHERE published_at IS NOT NULL")"
  if [[ "$recovered" -ge 2 ]]; then break; fi
  sleep 2
done
if [[ "$recovered" -lt 2 ]]; then echo "outbox recovery failed"; exit 1; fi

echo "OK: uploads accepted; ownership enforced; offline request recovered from outbox."
