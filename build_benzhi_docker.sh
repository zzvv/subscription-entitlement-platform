#!/bin/bash
set -euo pipefail
docker build --platform "${2:-linux/amd64}" -f benzhi.Dockerfile -t "${1:-subscription-entitlement-platform}" .
