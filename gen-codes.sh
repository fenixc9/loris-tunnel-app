#!/usr/bin/env bash
set -euo pipefail

: "${LORIS_LICENSE_ADMIN_AUTHORIZATION:?Set the complete Authorization header value before running this script}"

generate_codes() {
  printf 'Authorization: %s\n' "$LORIS_LICENSE_ADMIN_AUTHORIZATION" |
    curl --fail-with-body --silent --show-error --header @- \
      --header 'Content-Type: application/json' \
      --data "$1" \
      'http://127.0.0.1:8000/api/v1/admin/license-codes/generate'
  printf '\n'
}

generate_codes '{"count":10,"duration_days":366,"prefix":"SH1Y-"}'
generate_codes '{"count":10,"duration_days":31,"prefix":"SH1M-"}'
generate_codes '{"count":10,"duration_days":1,"prefix":"SH1D-"}'
