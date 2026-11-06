#!/bin/sh
set -eu

if [ "$#" -eq 0 ]; then
  : "${BOOTSTRAP_ADMIN_EMAIL:?BOOTSTRAP_ADMIN_EMAIL is required}"
	: "${BOOTSTRAP_ADMIN_LOGIN:?BOOTSTRAP_ADMIN_LOGIN is required}"
  : "${BOOTSTRAP_ADMIN_FIRSTNAME:?BOOTSTRAP_ADMIN_FIRSTNAME is required}"
  : "${BOOTSTRAP_ADMIN_LASTNAME:?BOOTSTRAP_ADMIN_LASTNAME is required}"
  : "${BOOTSTRAP_ADMIN_PASSWORD:?BOOTSTRAP_ADMIN_PASSWORD is required}"
  printf '%s' "$BOOTSTRAP_ADMIN_PASSWORD" | go -C user-service run ./cmd/bootstrap-admin \
    --email "$BOOTSTRAP_ADMIN_EMAIL" \
		--login "$BOOTSTRAP_ADMIN_LOGIN" \
    --firstname "$BOOTSTRAP_ADMIN_FIRSTNAME" \
    --lastname "$BOOTSTRAP_ADMIN_LASTNAME" \
    --password-stdin
  exit 0
fi

if [ "$#" -ne 3 ]; then
  echo "usage: $0 [EMAIL FIRSTNAME LASTNAME]" >&2
  exit 2
fi

if [ -t 0 ]; then
  printf 'Password: ' >&2
  stty -echo
  IFS= read -r password
  stty echo
  printf '\nRepeat password: ' >&2
  stty -echo
  IFS= read -r confirm
  stty echo
  printf '\n' >&2
  if [ "$password" != "$confirm" ]; then
    echo 'passwords do not match' >&2
    exit 2
  fi
  printf '%s' "$password" | go -C user-service run ./cmd/bootstrap-admin --email "$1" --firstname "$2" --lastname "$3" --password-stdin
else
  go -C user-service run ./cmd/bootstrap-admin --email "$1" --firstname "$2" --lastname "$3" --password-stdin
fi
