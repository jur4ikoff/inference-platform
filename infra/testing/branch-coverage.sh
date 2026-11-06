#!/usr/bin/env bash
set -uo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
results="$root/test-results/branch"
mkdir -p "$results"
if ! command -v gobco >/dev/null 2>&1; then
  printf '%s\n' 'gobco не найден: выполните make test-tools' >&2
  exit 1
fi
# Gobco copies the module to its own temporary directory. Resolve local module
# replacements to absolute paths in a separate copy; never edit the workspace.
stage=$(mktemp -d "${TMPDIR:-/tmp}/lab-branch-coverage.XXXXXXXX") || exit 1
trap 'rm -rf "$stage"' EXIT
for module in user-service model-service inference-service; do
  cp -R "$root/$module" "$stage/$module" || exit 1
  (cd "$stage/$module" && go mod edit "-replace=github.com/jur4ikoff/utils=$root/utils") || exit 1
  if [[ "$module" != user-service ]]; then
    (cd "$stage/$module" && go mod edit "-replace=github.com/jur4ikoff/internal-api=$root/internal-api") || exit 1
  fi
done
status=0
for package in user-service/internal/service user-service/internal/repository model-service/internal/service model-service/internal/repository inference-service/internal/service inference-service/internal/repository; do
  file=${package//\//-}
  printf '\n%s\n' "$package"
  if ! (cd "$stage/$package" && env -u USER_TEST_DATABASE_URL -u MODEL_TEST_DATABASE_URL -u INFERENCE_TEST_DATABASE_URL gobco -branch) > "$results/$file.txt" 2>&1; then
    status=1
  fi
  cat "$results/$file.txt"
done
exit "$status"
