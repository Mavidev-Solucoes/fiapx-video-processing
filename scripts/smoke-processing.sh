#!/usr/bin/env bash
set -euo pipefail
cleanup() {
  code=$?
  if [[ $code -ne 0 ]]; then
    docker compose ps || true
    docker compose logs --no-color --tail 120 || true
  fi
  docker compose down -v --remove-orphans || true
  exit "$code"
}
trap cleanup EXIT
docker compose up -d --build --scale worker=2
for i in $(seq 1 90); do
  if curl -fsS http://127.0.0.1:8080/health/live >/dev/null 2>&1; then break; fi
  if [[ "$i" == 90 ]]; then echo "API not ready"; exit 1; fi
  sleep 2
done

register() {
  curl --fail-with-body -fsS http://127.0.0.1:8080/api/auth/register \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"$1\",\"password\":\"a-strong-local-password-2026\"}" |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["accessToken"])'
}
token_a="$(register "process-a@example.com")"
token_b="$(register "process-b@example.com")"

echo "Generating a real FFmpeg test video"
docker compose exec -T --index 1 worker ffmpeg -hide_banner -nostdin -loglevel error \
  -f lavfi -i "color=c=red:s=160x90:r=2:d=3" \
  -c:v mpeg4 -q:v 5 -y /tmp/fiapx-real.mp4
docker compose cp --index 1 worker:/tmp/fiapx-real.mp4 /tmp/fiapx-real.mp4
printf 'invalid media content' > /tmp/fiapx-invalid.mp4

upload() {
  curl --fail-with-body -fsS http://127.0.0.1:8080/api/jobs \
    -H "Authorization: Bearer $token_a" \
    -F "video=@$1;filename=$2" |
    python3 -c 'import json,sys; j=json.load(sys.stdin); assert j["status"]=="queued"; print(j["id"])'
}
first="$(upload /tmp/fiapx-real.mp4 one.mp4)"
second="$(upload /tmp/fiapx-real.mp4 two.mp4)"
wait_status() {
  local id="$1" expected="$2" status=""
  for i in $(seq 1 90); do
    status="$(curl --fail-with-body -fsS \
      -H "Authorization: Bearer $token_a" \
      "http://127.0.0.1:8080/api/jobs/$id" |
      python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')"
    if [[ "$status" == "$expected" ]]; then return; fi
    if [[ "$status" == "failed" && "$expected" != "failed" ]]; then
      echo "Video failed unexpectedly: $id"; exit 1
    fi
    sleep 2
  done
  echo "$id did not reach $expected (last $status)"; exit 1
}
wait_status "$first" completed
wait_status "$second" completed
echo "Both videos processed"

curl --fail-with-body -fsS \
  -H "Authorization: Bearer $token_a" \
  "http://127.0.0.1:8080/api/jobs/$first/download" \
  -o /tmp/fiapx-result.zip
python3 - <<'PY'
import zipfile
with zipfile.ZipFile("/tmp/fiapx-result.zip") as z:
    images = [x for x in z.namelist() if x.endswith(".png")]
    assert len(images) >= 2, images
    assert all(z.getinfo(x).file_size > 0 for x in images)
print("ZIP contains extracted PNG images")
PY

code="$(curl -sS -o /dev/null -w '%{http_code}' \
  -H "Authorization: Bearer $token_b" \
  "http://127.0.0.1:8080/api/jobs/$first/download")"
if [[ "$code" != "404" ]]; then echo "Cross-user download allowed: $code"; exit 1; fi

echo "Duplicate RabbitMQ delivery"
before="$(docker compose exec -T postgres psql -U fiapx -d fiapx -Atc \
  "SELECT attempt_count FROM video_jobs WHERE id='$first'")"
payload="$(python3 -c 'import json,sys; print(json.dumps({"properties":{"delivery_mode":2},"routing_key":"fiapx.jobs","payload":json.dumps({"jobId":sys.argv[1]}),"payload_encoding":"string"}))' "$first")"
curl --fail-with-body -fsS -u 'fiapx:fiapx-local-only' \
  -H 'Content-Type: application/json' -d "$payload" \
  'http://127.0.0.1:15672/api/exchanges/%2F/amq.default/publish' |
  python3 -c 'import json,sys; assert json.load(sys.stdin)["routed"]'
sleep 4
after="$(docker compose exec -T postgres psql -U fiapx -d fiapx -Atc \
  "SELECT attempt_count FROM video_jobs WHERE id='$first'")"
if [[ "$before" != "$after" ]]; then echo "Duplicate reprocessed video"; exit 1; fi

invalid="$(upload /tmp/fiapx-invalid.mp4 invalid.mp4)"
wait_status "$invalid" failed
echo "== Verify terminal-failure notifications and ownership =="
notices="$(curl --fail-with-body -fsS -H "Authorization: Bearer $token_a" \
  http://127.0.0.1:8080/api/notifications)"
notice_id="$(printf '%s' "$notices" | python3 -c 'import json,sys; d=json.load(sys.stdin); n=[x for x in d["notifications"] if x["jobId"]==sys.argv[1]]; assert len(n)==1, n; assert not n[0].get("readAt"); print(n[0]["id"])' "$invalid")"
count_b="$(curl --fail-with-body -fsS -H "Authorization: Bearer $token_b" \
  http://127.0.0.1:8080/api/notifications | \
  python3 -c 'import json,sys; print(len(json.load(sys.stdin)["notifications"]))')"
if [[ "$count_b" != "0" ]]; then echo "Notifications leaked to another account"; exit 1; fi
code="$(curl -sS -o /dev/null -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $token_b" \
  "http://127.0.0.1:8080/api/notifications/$notice_id/read")"
if [[ "$code" != "404" ]]; then echo "Other user marked notification read: $code"; exit 1; fi
curl --fail-with-body -fsS -X POST -H "Authorization: Bearer $token_a" \
  "http://127.0.0.1:8080/api/notifications/$notice_id/read"
curl --fail-with-body -fsS -H "Authorization: Bearer $token_a" \
  http://127.0.0.1:8080/api/notifications |
  python3 -c 'import json,sys; d=json.load(sys.stdin); n=[x for x in d["notifications"] if str(x["id"])==sys.argv[1]]; assert len(n)==1 and n[0].get("readAt")' "$notice_id"
echo "OK: failure notification was delivered, private, and marked read"

echo "== Simulate an expired worker lease and verify recovery =="
docker compose stop worker
recovery="$(upload /tmp/fiapx-real.mp4 recovery.mp4)"
dispatched=0
for i in $(seq 1 40); do
  dispatched="$(docker compose exec -T postgres psql -U fiapx -d fiapx -Atc \
    "SELECT count(*) FROM video_job_outbox WHERE job_id='$recovery' AND published_at IS NOT NULL")"
  if [[ "$dispatched" == 1 ]]; then break; fi
  sleep 2
done
if [[ "$dispatched" != 1 ]]; then echo "Recovery job was not enqueued"; exit 1; fi
docker compose exec -T postgres psql -U fiapx -d fiapx -v ON_ERROR_STOP=1 -c \
  "UPDATE video_jobs SET status='processing',
   lease_token='916fb91d-b331-45e3-a640-ef5f8c9b15c7',
   lease_expires_at=NOW()-INTERVAL '1 minute', attempt_count=1
   WHERE id='$recovery' AND status='queued'"
docker compose start worker
wait_status "$recovery" completed
attempt="$(docker compose exec -T postgres psql -U fiapx -d fiapx -Atc \
  "SELECT attempt_count FROM video_jobs WHERE id='$recovery'")"
if [[ "$attempt" != "2" ]]; then echo "Expired lease did not recover correctly: $attempt"; exit 1; fi
echo "OK: expired lease was reclaimed and video processed"

echo "OK: two workers, ZIP, private download, duplicate suppression, notifications and lease recovery"

