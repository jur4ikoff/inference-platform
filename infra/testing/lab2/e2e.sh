#!/usr/bin/env bash
set -euo pipefail
cd /results
exec > >(tee e2e.log) 2>&1
base=http://nginx
token=
request() {
  local method=$1 path=$2 expected=$3 data=${4:-} code
  local args=(--silent --show-error --connect-timeout 3 --max-time 20 -X "$method" -H 'Content-Type: application/json' -o response.json -w '%{http_code}')
  [[ -z "$token" ]] || args+=(-H "Authorization: Bearer $token")
  [[ -z "$data" ]] || args+=(--data "$data")
  [[ "$method:$path" != POST:*/deployment ]] || args+=(-H 'Idempotency-Key: lab2-lifecycle')
  printf 'curl -X %s %s%s -> ' "$method" "$base" "$path"
  code=$(curl "${args[@]}" "$base$path")
  printf '%s\n' "$code"
  cat response.json
  printf '\n'
  [[ " $expected " == *" $code "* ]]
}
field() { python3 -c 'import json,sys; value=json.load(open("response.json"))[sys.argv[1]]; assert value; print(value)' "$1"; }
finish_capture() {
  local status=$?
  trap - EXIT
  kill -INT "$capture" 2>/dev/null || true
  wait "$capture" || status=1
  tcpdump -nn -A -r traffic.pcap > traffic.txt 2>capture-read.log || status=1
  if ! grep -q 'POST /api/v1/auth/login' traffic.txt; then status=1; fi
  if ! grep -q 'HTTP/1.1 200' traffic.txt; then status=1; fi
  exit "$status"
}
tcpdump --immediate-mode -i eth0 -U -s 0 -w traffic.pcap 'tcp port 80' 2>tcpdump.log &
capture=$!
trap finish_capture EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
sleep 1
kill -0 "$capture"
request POST /api/v1/auth/login 200 '{"login":"labadmin","password":"lab2-password"}'
token=$(field access_token)
request POST /api/v1/projects 201 '{"name":"lab2-e2e","description":"Lab 2 lifecycle"}'
request POST /api/v1/auth/project-token 200 '{"project_name":"lab2-e2e"}'
token=$(field access_token)
request POST /api/v1/projects/lab2-e2e/models 201 '{"name":"lab2-model","spec":{"provider_type":"HUGGING_FACE","provider_hugging_face":{"repo":"lab2/fixture"}}}'
model=$(field id)
path=/api/v1/projects/lab2-e2e/models/$model
request POST "$path/deployment" 202
deployment=$(field id)
for ((attempt=0; attempt<45; attempt++)); do
  request GET "$path/deployment" 200
  [[ "$(field id)" == "$deployment" ]]
  status=$(field status)
  [[ "$status" != failed ]]
  [[ "$status" != running ]] || break
  sleep 1
done
[[ "$status" == running ]]
# Manual failure/interruption controls exercise restoration with live data/resources.
if [[ "${LAB2_PAUSE_STAGE:-}" == e2e ]]; then
  touch /results/pause-ready
  sleep 300
fi
[[ "${LAB2_FAIL_STAGE:-}" != e2e ]]
request DELETE "$path/deployment" '202 204'
stopped=0
for ((attempt=0; attempt<45; attempt++)); do
  request GET "$path/deployment" '200 404'
  state=$(python3 -c 'import json; print(json.load(open("response.json")).get("status", "missing"))')
  if [[ "$state" == stopped || "$state" == 404 ]]; then stopped=1; break; fi
  sleep 1
done
[[ "$stopped" == 1 ]]
request GET /api/v1/projects/lab2-e2e/deployments 200
python3 -c 'import json; assert json.load(open("response.json"))["deployments"] == []'
request DELETE "$path" 204
request GET "$path" 404
echo 'E2E lifecycle passed'
