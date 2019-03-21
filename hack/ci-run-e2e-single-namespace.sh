#!/usr/bin/env bash
set -eEuxo pipefail

dir="$(dirname "$0")"
./"${dir}"/ci-run-e2e.sh single-namespace
