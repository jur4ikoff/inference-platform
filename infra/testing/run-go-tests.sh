#!/usr/bin/env bash
set -uo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
mode=${1:?expected unit, shuffle, processes, integration or coverage}
results="$root/test-results/$mode"
mkdir -p "$results"
if ! command -v gotestsum >/dev/null 2>&1; then
  printf '%s\n' 'gotestsum не найден: вывод отображается в консоли и сохраняется в формате go test -json' >&2
fi
if [[ "$mode" == processes ]]; then
  export TEST_PROCESS_LOG="$results/processes.tsv"
  : > "$TEST_PROCESS_LOG"
fi
status=0
for module in user-service model-service inference-service; do
  case "$mode:$module" in
    unit:*|shuffle:*|processes:*) flags=(-count=1 -skip '^(TestUserRepositoryIntegration|TestModelsAreIdentifiedByIDWithinProject|TestDeploymentIntegration)' ./...);;
    coverage:*) flags=(-count=1 -skip '^(TestUserRepositoryIntegration|TestModelsAreIdentifiedByIDWithinProject|TestDeploymentIntegration)' -coverprofile="$results/$module.cover" ./...);;
    integration:user-service) flags=(-count=1 -run '^TestUserRepositoryIntegration$' ./internal/repository);;
    integration:model-service) flags=(-count=1 -run '^TestModelsAreIdentifiedByIDWithinProject$' ./internal/repository);;
    integration:inference-service) flags=(-count=1 -run '^TestDeploymentIntegration' ./internal/repository);;
    *) printf 'Неизвестный режим: %s\n' "$mode" >&2; exit 2;;
  esac
  workdir="$root/$module"
  if [[ "$mode" == shuffle ]]; then flags=(-shuffle="${TEST_SHUFFLE_SEED:-on}" "${flags[@]}"); fi
  if [[ "$mode" == processes ]]; then export TEST_PROCESS_MODULE="$module"; flags=(-exec="$root/infra/testing/trace-test-process.sh" "${flags[@]}"); fi
  printf '\n%s: %s\n' "$mode" "$module"
  # Remove previous generated results so an interrupted run cannot be reported as current.
  rm -f "$results/$module.xml" "$results/$module.json" "$results/$module.tests.html"
  if [[ "$mode" == coverage ]]; then
    rm -f "$results/$module.cover" "$results/$module.html" "$results/$module.txt" "$results/$module.cobertura.xml"
  fi
  if command -v gotestsum >/dev/null 2>&1; then
    if ! (cd "$workdir" && gotestsum --junitfile "$results/$module.xml" --jsonfile "$results/$module.json" -- "${flags[@]}"); then status=1; fi
  else
    if ! (cd "$workdir" && go test -json "${flags[@]}" | tee "$results/$module.json" | python3 -u "$root/infra/testing/print-go-test-output.py"); then status=1; fi
    printf 'Результаты: %s\n' "$results/$module.json"
  fi
  if [[ -s "$results/$module.json" ]]; then
    python3 "$root/infra/testing/render-go-report.py" "$results/$module.json" "$results/$module.tests.html" || status=1
  fi
  if [[ "$mode" == coverage && -s "$results/$module.cover" ]]; then
    (cd "$workdir" && go tool cover -html="$results/$module.cover" -o "$results/$module.html") || status=1
    (cd "$workdir" && go tool cover -func="$results/$module.cover" > "$results/$module.txt") || status=1
    if command -v gocover-cobertura >/dev/null 2>&1; then
      if (cd "$workdir" && gocover-cobertura < "$results/$module.cover" > "$results/$module.cobertura.xml"); then
        python3 -c 'import sys, xml.etree.ElementTree as ET; root=ET.parse(sys.argv[1]).getroot(); print("Line coverage:", root.get("lines-covered", "?"), "/", root.get("lines-valid", "?"), "(rate:", root.get("line-rate", "?"), ")")' "$results/$module.cobertura.xml" || status=1
      else
        rm -f "$results/$module.cobertura.xml"
        status=1
      fi
    else
      printf '%s\n' 'gocover-cobertura не найден: покрытие строк не рассчитано; выполните make test-tools' >&2
      status=1
    fi
  fi
done
if [[ "$mode" == processes ]]; then
  awk -F '\t' '{count[$1]++; total++} END {for (name in count) printf "%s: %d test binaries\n", name, count[name]; printf "total: %d test binaries\n", total}' "$TEST_PROCESS_LOG" | sort > "$results/summary.txt"
  cat "$results/summary.txt"
fi
exit "$status"
