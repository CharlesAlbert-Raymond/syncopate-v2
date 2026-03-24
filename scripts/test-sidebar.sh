#!/usr/bin/env bash
# Quick visual test of the sidebar UI.
# Builds and runs the sidebar view standalone (no tmux needed).
#
# Usage: ./scripts/test-sidebar.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$PROJECT_DIR" && git rev-parse --show-toplevel)"
BINARY="/tmp/syncopate-test-$$"

cleanup() { rm -f "$BINARY"; }
trap cleanup EXIT

echo "==> Building..."
(cd "$PROJECT_DIR" && go build -o "$BINARY" .)

echo "==> Launching sidebar view (press q to quit)"
echo ""
"$BINARY" --sidebar --root "$REPO_ROOT"
