#!/usr/bin/env bash
set -euo pipefail

cleanup() {
  code=$?
  if [[ $code -ne 0 ]]; then
    echo "== Monitoring diagnostics =="
    docker compose --profile monitoring ps || true
    docker compose --profile monitoring logs --no-color --tail 100 || true
  fi
  docker compose --profile monitoring down -v --remove-orphans || true
  exit "$code"
}
trap cleanup EXIT

docker compose --profile monitoring up -d --build --scale worker=2
ready=0
for i in $(seq 1 100); do
  if curl -fsS http://127.0.0.1:8080/health/live >/dev/null 2>&1 &&
    curl -fsS http://127.0.0.1:9090/-/ready >/dev/null 2>&1 &&
    curl -fsS http://127.0.0.1:3000/api/health >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 2
done
if [[ $ready != 1 ]]; then echo "Monitoring services did not become ready"; exit 1; fi

echo "== Verify provisioned dashboard =="
curl -fsS -u admin:fiapx-monitoring-local \
  http://127.0.0.1:3000/api/dashboards/uid/fiapx-operations |
  python3 -c 'import json,sys; d=json.load(sys.stdin)["dashboard"]; assert d["uid"]=="fiapx-operations"; assert len(d["panels"])>=10'
echo "Grafana dashboard is provisioned"

query_metric() {
  curl -fsSG --data-urlencode "query=$1" \
    http://127.0.0.1:9090/api/v1/query |
    python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["status"]=="success"; data=d["data"]["result"]; assert data, "metric not scraped yet"; print(float(data[0]["value"][1]))'
}

echo "== Verify Prometheus scrape targets =="
targets_ready=0
for i in $(seq 1 50); do
  api="$(query_metric 'up{job="fiapx-api"}' 2>/dev/null || echo 0)"
  rabbit="$(query_metric 'up{job="rabbitmq-queues"}' 2>/dev/null || echo 0)"
  if [[ "$api" == "1.0" && "$rabbit" == "1.0" ]]; then targets_ready=1; break; fi
  sleep 2
done
if [[ $targets_ready != 1 ]]; then echo "Scrape target missing: API=$api RabbitMQ=$rabbit"; exit 1; fi
echo "API and RabbitMQ exporters are healthy"

echo "== Submit a failed job and inspect live metrics =="
token="$(curl --fail-with-body -fsS -X POST http://127.0.0.1:8080/api/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"observability-test@example.com","password":"strong-monitoring-demo-password-2026"}' |
  python3 -c 'import json,sys; print(json.load(sys.stdin)["accessToken"])')"
printf 'invalid video to test failure metrics' > /tmp/fiapx-monitor-invalid.mp4
job_id="$(curl --fail-with-body -fsS -X POST http://127.0.0.1:8080/api/jobs \
  -H "Authorization: Bearer $token" \
  -F 'video=@/tmp/fiapx-monitor-invalid.mp4;filename=monitor-invalid.mp4' |
  python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["id"])')"
observed=0
for i in $(seq 1 75); do
  failed="$(query_metric 'fiapx_jobs{status="failed"}' 2>/dev/null || echo 0)"
  notices="$(query_metric 'fiapx_notifications_unread' 2>/dev/null || echo 0)"
  consumers="$(query_metric 'rabbitmq_detailed_queue_consumers{queue="fiapx.jobs"}' 2>/dev/null || echo 0)"
  if [[ "$failed" == "1.0" && "$notices" == "1.0" && "$consumers" != "0" && "$consumers" != "0.0" ]]; then
    observed=1
    break
  fi
  sleep 2
done
if [[ $observed != 1 ]]; then
  echo "Live metrics incomplete: failed=$failed unread=$notices consumers=$consumers"
  exit 1
fi
echo "OK: Grafana provisioned, Prometheus scraping, job failures and worker consumers visible"
