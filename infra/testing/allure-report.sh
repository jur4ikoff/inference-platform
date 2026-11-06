#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
mode=${1:-unit}
case "$mode" in unit|integration|shuffle|coverage|processes) ;; *) printf 'Неизвестный набор результатов: %s\n' "$mode" >&2; exit 2;; esac
if ! command -v allure >/dev/null 2>&1; then
  printf '%s\n' 'Allure CLI не найден. Установите Allure Report 2 (на macOS: brew install allure).' >&2
  exit 1
fi
results="$root/test-results/$mode"
shopt -s nullglob
xml=()
for file in "$results"/*.xml; do
  [[ "$file" == *.cobertura.xml ]] || xml+=("$file")
done
if (( ${#xml[@]} == 0 )); then
  printf 'JUnit XML не найден в %s. Установите gotestsum и заново запустите соответствующие тесты.\n' "$results" >&2
  exit 1
fi
stage=$(mktemp -d "${TMPDIR:-/tmp}/lab-allure.XXXXXXXX")
trap 'rm -rf "$stage"' EXIT
cp "${xml[@]}" "$stage/"
# Allure 2 reads JUnit XML with the bundled JUnit XML plugin.
allure generate "$stage" --clean -o "$root/test-results/allure/$mode"
printf 'Allure: %s\n' "$root/test-results/allure/$mode/index.html"
