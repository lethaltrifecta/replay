#!/usr/bin/env bash

set -euo pipefail

VERSION="${1:-0.29.0}"
SPEC_PATH="${2:-api/openapi.yaml}"
OUTPUT_DIR="${3:-ui/packages/api}"

npx -y "openapi-typescript-codegen@${VERSION}" \
  --input "${SPEC_PATH}" \
  --output "${OUTPUT_DIR}" \
  --client fetch
