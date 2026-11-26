#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../../.." && pwd)
export LAB2_MODE=${1:-full}
[[ "$LAB2_MODE" == full || "$LAB2_MODE" == integration || "$LAB2_MODE" == finalize ]] || { echo 'expected full, integration or finalize' >&2; exit 2; }
if [[ "$LAB2_MODE" == finalize ]]; then : "${LAB2_PROJECT:?finalize requires the original LAB2_PROJECT}"; fi
export LAB2_PROJECT=${LAB2_PROJECT:-lab2-$(date +%Y%m%d%H%M%S)-$$-$RANDOM}
[[ "$LAB2_PROJECT" =~ ^lab2-[a-z0-9-]+$ ]] || { echo 'LAB2_PROJECT must start with lab2- and contain lowercase letters, digits or hyphens' >&2; exit 2; }
export LAB2_RESULTS_DIR=${LAB2_RESULTS_DIR:-$root/test-results/lab2/$LAB2_PROJECT}
export LAB2_HISTORY_DIR=${LAB2_HISTORY_DIR:-$root/test-results/lab2/history}
export LAB2_MODEL_IMAGE=$LAB2_PROJECT-model:local
export LAB2_RUNNER_IMAGE=${LAB2_RUNNER_IMAGE:-$LAB2_PROJECT-runner:local}
export LAB2_SOURCE_REVISION=$(git -C "$root" rev-parse HEAD)
mkdir -p "$LAB2_RESULTS_DIR" "$LAB2_HISTORY_DIR"
export LAB2_RESULTS_DIR=$(cd "$LAB2_RESULTS_DIR" && pwd)
export LAB2_HISTORY_DIR=$(cd "$LAB2_HISTORY_DIR" && pwd)
if [[ "$LAB2_MODE" != finalize ]]; then
  [[ -z "$(ls -A "$LAB2_RESULTS_DIR")" ]] || { echo 'results directory must be empty' >&2; exit 2; }
  if docker network inspect "$LAB2_PROJECT"_default >/dev/null 2>&1; then echo 'project already exists; choose a fresh LAB2_PROJECT' >&2; exit 2; fi
fi
compose=(docker compose -p "$LAB2_PROJECT" -f "$root/infra/testing/lab2/compose.yaml")
printf '%s\n' "$LAB2_PROJECT" > "$LAB2_RESULTS_DIR/project.txt"
git -C "$root" rev-parse HEAD > "$LAB2_RESULTS_DIR/commit.txt"
git -C "$root" status --short --untracked-files=all > "$LAB2_RESULTS_DIR/worktree.txt"
echo "Lab 2 project: $LAB2_PROJECT; results: $LAB2_RESULTS_DIR"
runner_ready=0
postgres_ready=0
runner_id=
event() { "${compose[@]}" run --rm --no-deps -T runner event "$@"; }
finish() {
  local status=$? cleanup=0
  trap - EXIT INT TERM
  set +e
  if [[ "$runner_ready" == 0 ]] && docker image inspect "$LAB2_RUNNER_IMAGE" >/dev/null 2>&1; then runner_ready=1; fi
  if [[ -n "$runner_id" ]]; then
    docker stop --time 10 "$runner_id" >/dev/null 2>&1
    docker rm -f "$runner_id" >/dev/null 2>&1 || cleanup=1
  fi
  if [[ "$postgres_ready" == 1 ]]; then
    "${compose[@]}" logs --no-color > "$LAB2_RESULTS_DIR/stand.log" 2>&1
    "${compose[@]}" stop -t 20 nginx user-service model-service inference-service >> "$LAB2_RESULTS_DIR/cleanup.log" 2>&1 || cleanup=1
    # Runtime uses a shared project label; ownership is the isolated test network.
    # Enumerate network endpoints as well as Compose resources, including stopped models.
    docker network inspect "$LAB2_PROJECT"_default --format '{{range $id, $container := .Containers}}{{$id}}{{"\n"}}{{end}}' > "$LAB2_RESULTS_DIR/network-containers.txt" || cleanup=1
    while IFS= read -r id; do
      [[ -n "$id" ]] || continue
      label=$(docker inspect "$id" --format '{{index .Config.Labels "com.docker.compose.project"}}') || cleanup=1
      if [[ "$label" == inference-models ]]; then docker rm -f "$id" >> "$LAB2_RESULTS_DIR/cleanup.log" 2>&1 || cleanup=1; fi
    done < "$LAB2_RESULTS_DIR/network-containers.txt"
    # Stopped runtime containers may no longer be active network endpoints.
    docker ps -aq --filter label=com.docker.compose.project=inference-models > "$LAB2_RESULTS_DIR/model-containers.txt" || cleanup=1
    while IFS= read -r id; do
      [[ -n "$id" ]] || continue
      network=$(docker inspect "$id" --format '{{.HostConfig.NetworkMode}}') || cleanup=1
      if [[ "$network" == "$LAB2_PROJECT"_default ]]; then docker rm -f "$id" >> "$LAB2_RESULTS_DIR/cleanup.log" 2>&1 || cleanup=1; fi
    done < "$LAB2_RESULTS_DIR/model-containers.txt"
    "${compose[@]}" run --rm --no-deps -T runner setup >> "$LAB2_RESULTS_DIR/cleanup.log" 2>&1 || cleanup=1
  elif [[ "$runner_ready" == 1 ]]; then
    cleanup=1
  fi
  "${compose[@]}" down --volumes --remove-orphans >> "$LAB2_RESULTS_DIR/cleanup.log" 2>&1 || cleanup=1
  if [[ -n "$runner_id" ]]; then docker rm -f "$runner_id" >/dev/null 2>&1 || true; fi
  if docker image inspect "$LAB2_MODEL_IMAGE" >/dev/null 2>&1; then docker image rm "$LAB2_MODEL_IMAGE" >> "$LAB2_RESULTS_DIR/cleanup.log" 2>&1 || cleanup=1; fi
  if [[ "$runner_ready" == 1 ]]; then
    local reporter=(docker run --rm -v "$LAB2_RESULTS_DIR:/results" -v "$LAB2_HISTORY_DIR:/history" "$LAB2_RUNNER_IMAGE")
    if [[ "$status" != 0 && ! -f "$LAB2_RESULTS_DIR/stages/setup.json" ]]; then
      "${reporter[@]}" event setup failed "stand preparation aborted (exit $status)"
    fi
    if [[ "$cleanup" == 0 ]]; then
      "${reporter[@]}" event cleanup passed 'S0 restored and verified; own stand removed' || status=1
    else
      "${reporter[@]}" event cleanup failed 'restoration or resource removal failed; inspect cleanup.log'
      status=1
    fi
    "${reporter[@]}" report > "$LAB2_RESULTS_DIR/report.log" 2>&1 || status=1
    if [[ "$LAB2_RUNNER_IMAGE" == "$LAB2_PROJECT-runner:local" ]] && ! docker image rm "$LAB2_RUNNER_IMAGE" >> "$LAB2_RESULTS_DIR/cleanup.log" 2>&1; then
      "${reporter[@]}" event cleanup failed 'runner image removal failed; inspect cleanup.log'
      "${reporter[@]}" report >> "$LAB2_RESULTS_DIR/report.log" 2>&1
      status=1
    fi
  fi
  echo "Lab 2 finished (exit $status): $LAB2_RESULTS_DIR"
  exit "$status"
}
trap finish EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
if [[ "$LAB2_MODE" == finalize ]]; then
  if ! docker image inspect "$LAB2_RUNNER_IMAGE" >/dev/null 2>&1; then
    docker image tag inference-lab2-runner:local "$LAB2_RUNNER_IMAGE"
  fi
  runner_ready=1
  runner_id=$(docker inspect "$LAB2_PROJECT-runner" --format '{{.Id}}' 2>/dev/null || true)
  if [[ -n "$("${compose[@]}" ps -q postgres)" ]]; then postgres_ready=1; fi
  exit 0
fi
if [[ "${LAB2_SKIP_BUILD:-0}" != 1 ]]; then
  "${compose[@]}" build runner model-fixture
  docker image tag "$LAB2_RUNNER_IMAGE" inference-lab2-runner:local
else
  # Each run still tags its own fixture image; reuse the runner build only.
  docker image tag inference-lab2-runner:local "$LAB2_RUNNER_IMAGE"
  "${compose[@]}" build model-fixture
fi
runner_ready=1
"${compose[@]}" up -d --wait postgres
postgres_ready=1
event setup running
if ! "${compose[@]}" run --rm --no-deps -T runner setup > "$LAB2_RESULTS_DIR/setup.log" 2>&1; then event setup failed 'see setup.log'; exit 1; fi
if [[ "$LAB2_MODE" == full ]]; then
  "${compose[@]}" up -d nginx user-service model-service inference-service
  if ! "${compose[@]}" run --rm --no-deps -T runner ready > "$LAB2_RESULTS_DIR/ready.log" 2>&1; then event setup failed 'service readiness timed out; see ready.log and stand.log'; exit 1; fi
fi
event setup passed
runner_id=$("${compose[@]}" run -d --no-deps --name "$LAB2_PROJECT-runner" runner pipeline)
docker logs --follow "$runner_id" > "$LAB2_RESULTS_DIR/runner.log" 2>&1 &
logs_pid=$!
while [[ "$(docker inspect "$runner_id" --format '{{.State.Running}}')" == true ]]; do sleep 1; done
wait "$logs_pid" || true
exit "$(docker inspect "$runner_id" --format '{{.State.ExitCode}}')"
