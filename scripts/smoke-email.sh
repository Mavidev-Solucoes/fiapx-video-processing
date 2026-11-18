#!/usr/bin/env bash
set -euo pipefail
dc() {
  if [[ -n "${FIAPX_COVERAGE_DIR:-}" ]]; then
    docker compose -f docker-compose.yml -f docker-compose.mail.yml -f docker-compose.coverage.yml "$@"
  else
    docker compose -f docker-compose.yml -f docker-compose.mail.yml "$@"
  fi
}
cleanup() {
  code=$?
  if [[ $code -ne 0 ]]; then
    echo "== Email delivery diagnostics =="
    dc ps || true
    dc logs --no-color --tail 130 || true
  fi
  dc down -v --remove-orphans || true
  exit "$code"
}
trap cleanup EXIT
dc up -d --build --scale worker=1
ready=0
for i in $(seq 1 80); do
  if curl -fsS http://127.0.0.1:8080/health/live >/dev/null 2>&1 &&
    curl -fsS http://127.0.0.1:8025/api/v1/messages >/dev/null 2>&1; then
    ready=1; break
  fi
  sleep 2
done
if [[ $ready != 1 ]]; then echo "API or Mailpit did not become ready"; exit 1; fi
token="$(curl -fsS -X POST http://127.0.0.1:8080/api/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"fiapx-mail-test@example.test","password":"a-strong-demo-password-2026"}' |
  python3 -c 'import json,sys; print(json.load(sys.stdin)["accessToken"])')"
printf 'not a video' > /tmp/fiapx-invalid-email.mp4
upload() {
  curl -fsS http://127.0.0.1:8080/api/jobs \
    -H "Authorization: Bearer $token" \
    -F "video=@/tmp/fiapx-invalid-email.mp4;filename=$1" |
    python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["status"]=="queued"; print(d["id"])'
}
wait_failed() {
  local id="$1" status=""
  for i in $(seq 1 50); do
    status="$(curl -fsS -H "Authorization: Bearer $token" \
      "http://127.0.0.1:8080/api/jobs/$id" |
      python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')"
    if [[ "$status" == "failed" ]]; then return 0; fi
    sleep 2
  done
  echo "Video $id not marked failed; status=$status"; return 1
}
email_sent() {
  dc exec -T postgres psql -U fiapx -d fiapx -Atc \
    "SELECT COUNT(*) FROM notification_email_outbox e
     JOIN user_notifications n ON n.id=e.notification_id
     WHERE n.job_id='$1' AND e.sent_at IS NOT NULL"
}
email_pending() {
  dc exec -T postgres psql -U fiapx -d fiapx -Atc \
    "SELECT COUNT(*) FROM notification_email_outbox e
     JOIN user_notifications n ON n.id=e.notification_id
     WHERE n.job_id='$1' AND e.sent_at IS NULL AND e.abandoned_at IS NULL"
}
await_sent() {
  for i in $(seq 1 60); do
    if [[ "$(email_sent "$1")" == 1 ]]; then return 0; fi
    sleep 2
  done
  echo "Email delivery failed for job $1"; return 1
}
check_email_body() {
  for i in $(seq 1 20); do
    if curl -fsS http://127.0.0.1:8025/view/latest.txt 2>/dev/null | grep -Fq "$1"; then
      return 0
    fi
    sleep 2
  done
  echo "Captured Mailpit message is missing job ID $1"; return 1
}
echo "== Verify failure generates a real SMTP message =="
first="$(upload invalid-email-one.mp4)"
wait_failed "$first"
await_sent "$first"
check_email_body "$first"
echo "OK: terminal failure generated an email captured by Mailpit"
echo "== Verify a temporary SMTP outage does not lose the email =="
dc stop mailpit
second="$(upload invalid-email-two.mp4)"
wait_failed "$second"
if [[ "$(email_pending "$second")" != 1 ]]; then
  echo "Email intent disappeared while SMTP was down"; exit 1
fi
dc start mailpit
await_sent "$second"
check_email_body "$second"
echo "OK: pending email was delivered after Mailpit recovered"
echo "== Verify single notification intent per failed job =="
count="$(dc exec -T postgres psql -U fiapx -d fiapx -Atc \
  "SELECT COUNT(*) FROM notification_email_outbox e
   JOIN user_notifications n ON n.id=e.notification_id
   WHERE n.job_id='$second'")"
if [[ "$count" != 1 ]]; then echo "Duplicate email intents: $count"; exit 1; fi
echo "OK: SMTP capture, retry, failure ownership and idempotent email outbox"
