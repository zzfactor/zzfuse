#!/bin/bash
set -e

# Usage:
#   ./script/macos-run.sh [--skip-build|-S] [--help|-h] [collector-args...]
#
# Options:
#   --skip-build, -S   Skip building the binary; run the currently built one.
#   --help, -h         Show this help and exit.

project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_root"

DO_BUILD=true
PASS_THROUGH_ARGS=()

for arg in "$@"; do
  case "$arg" in
    --skip-build|-S)
      DO_BUILD=false
      ;;
    --help|-h)
      sed -n '1,15p' "$0"
      exit 0
      ;;
    *)
      PASS_THROUGH_ARGS+=("$arg")
      ;;
  esac
done

if [ "$DO_BUILD" = true ]; then
  ./script/macos-build.sh
else
  echo "[macos-run] Skipping build step (using existing ./trazr-otel/trazr-otel)" >&2
fi

# Supply authentication and database settings through your environment.
exec ./trazr-otel/trazr-otel --feature-gates=+service.profilesSupport --config "${TRAZR_CONFIG:-config.yaml}" "${PASS_THROUGH_ARGS[@]}"
