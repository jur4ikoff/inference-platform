#!/usr/bin/env bash
set -eu
: "${TEST_PROCESS_LOG:?TEST_PROCESS_LOG must be set}"
printf '%s\t%s\t%s\t%s\n' "${TEST_PROCESS_MODULE:-unknown}" "$$" "$PPID" "$1" >> "$TEST_PROCESS_LOG"
exec "$@"
