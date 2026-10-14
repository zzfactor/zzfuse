#!/bin/sh
# install-ocb-local.sh
# Installs the OpenTelemetry Collector Builder (OCB) binary for your local OS (macOS or Linux)
# Usage: ./install-ocb-local.sh

set -eu

project_root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
task_ocb_bin=$(mktemp -d)
trap 'rm -rf "$task_ocb_bin"' EXIT HUP INT TERM

# Specify the OCB version to install
OCB_VERSION=v0.163.0

# Install OCB for the current platform
GOBIN="$task_ocb_bin" GOTOOLCHAIN=go1.27.2 GO111MODULE=on go install go.opentelemetry.io/collector/cmd/builder@${OCB_VERSION}

# Copy the binary to the project root as 'ocb'
cp "$task_ocb_bin/builder" "$project_root/ocb"

echo "OCB installed as ./ocb for $(uname -s) ($(go env GOARCH))"
