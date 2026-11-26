#!/usr/bin/env bash
set -euo pipefail
scripts=/src/infra/testing/lab2
event() { python3 "$scripts/report.py" event "$@"; }
case "${1:-pipeline}" in
  setup) exec bash "$scripts/setup.sh" ;;
  report) exec python3 "$scripts/report.py" generate ;;
  event) shift; event "$@"; exit ;;
  ready)
    for ((attempt=0; attempt<60; attempt++)); do
      if curl -fsS --max-time 2 http://nginx/.well-known/jwks.json >/dev/null &&
         curl -fsS --max-time 2 http://model-service:8081/api/v1/openapi.json >/dev/null &&
         curl -fsS --max-time 2 http://inference-service:8080/metrics >/dev/null; then exit 0; fi
      sleep 1
    done
    exit 1 ;;
  pipeline) ;;
  *) exit 2 ;;
esac
stage=
cp /usr/local/share/lab2-snapshot-revision /results/snapshot-revision.txt
finalize() {
  local code=$?
  trap - EXIT
  if [[ -n "$stage" ]]; then event "$stage" broken "runner interrupted or aborted (exit $code)"; fi
  exit "$code"
}
trap finalize EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
export TEST_ALLURE_BY_MODULE=1
for next in unit integration e2e; do
  if [[ "${LAB2_MODE:-full}" == integration && "$next" != integration ]]; then
    event "$next" skipped 'integration-only invocation'
    continue
  fi
  stage=$next
  event "$stage" running
  if [[ "$stage" == integration && "${LAB2_PAUSE_STAGE:-}" == integration ]]; then touch /results/pause-ready; sleep 300; fi
  status=0
  case "$stage" in
    unit) bash /src/infra/testing/run-go-tests.sh unit /results/unit || status=$? ;;
    integration) USER_TEST_DATABASE_URL="$DATABASE_URL" MODEL_TEST_DATABASE_URL="$DATABASE_URL" INFERENCE_TEST_DATABASE_URL="$DATABASE_URL" bash /src/infra/testing/run-go-tests.sh service-integration /results/integration || status=$? ;;
    e2e) bash "$scripts/e2e.sh" || status=$? ;;
  esac
  if [[ "${LAB2_FAIL_STAGE:-}" == "$stage" ]]; then status=1; fi
  if [[ "$stage" == integration && "$status" == 0 ]]; then bash "$scripts/setup.sh" verify > /results/integration-restoration.log 2>&1 || status=$?; fi
  if [[ "$status" != 0 ]]; then
    event "$stage" failed "stage returned $status (injected failure: ${LAB2_FAIL_STAGE:-none})"
    stage=
    exit "$status"
  fi
  event "$stage" passed
  stage=
done
