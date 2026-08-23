#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd); cd "$ROOT"
TMP=${TMPDIR:-/tmp}/certlife-verify-$$; mkdir -p "$TMP"
PORT=$(python3 - <<'PY'
import socket
s=socket.socket(); s.bind(('127.0.0.1',0)); print(s.getsockname()[1]); s.close()
PY
)
BASE="http://127.0.0.1:$PORT"
trap 'kill ${PID:-0} 2>/dev/null || true; rm -rf "$TMP"' EXIT
go build -o "$TMP/certlife" ./cmd/certlife
CERTLIFE_DEPLOY_DIR="$TMP/deployments" "$TMP/certlife" -config /dev/null -db "$TMP/certlife.db" -addr "127.0.0.1:$PORT" -api-key dev-key >"$TMP/server.log" 2>&1 & PID=$!
for _ in $(seq 1 40); do curl -sf "$BASE/api/v1/health" >/dev/null && break; sleep .25; done
curl -sf "$BASE/api/v1/health" >/dev/null
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ); FUTURE=$(date -u -v+3d +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '+3 days' +%Y-%m-%dT%H:%M:%SZ)
curl -sf -X POST "$BASE/api/v1/certificates" -H 'X-API-Key: dev-key' -H 'Content-Type: application/json' -d "{\"name\":\"verify-cert\",\"domain\":\"verify.local\",\"sans\":[],\"issuer\":\"Mock CA\",\"serial_number\":\"AA01\",\"algorithm\":\"ECDSA\",\"key_bits\":256,\"valid_from\":\"$NOW\",\"valid_until\":\"$FUTURE\",\"environment\":\"prod\",\"notify_threshold_days\":30,\"auto_renew\":true,\"renew_threshold_days\":14,\"deploy_target_ids\":[1]}" >"$TMP/cert.json"
grep -q 'verify-cert' "$TMP/cert.json"
for JOB in expiry_checker renewal_runner notify_dispatcher; do curl -sf -X POST "$BASE/api/v1/tasks/run" -H 'X-API-Key: dev-key' -H 'Content-Type: application/json' -d "{\"name\":\"$JOB\"}" >"$TMP/$JOB.json"; grep -q '"code":0' "$TMP/$JOB.json"; done
curl -sf "$BASE/api/v1/renewals" -H 'X-API-Key: dev-key' | grep -q 'completed'
curl -sf "$BASE/api/v1/audit-logs" -H 'X-API-Key: dev-key' | grep -q 'cert.create'
bash scripts/count_lines.sh
go test ./...
go vet ./...
echo '端到端验证通过'
