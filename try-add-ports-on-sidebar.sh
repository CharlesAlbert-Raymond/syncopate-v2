#!/usr/bin/env bash
# Try the add-ports-on-sidebar feature branch.
# Builds a temporary binary, runs it, and cleans up on exit.

set -euo pipefail

BINARY="/tmp/syncopate-add-ports-on-sidebar"
REPO_ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"

cleanup() {
    rm -f "$BINARY"
    echo "Cleaned up $BINARY"
}
trap cleanup EXIT

echo "Building syncopate (add-ports-on-sidebar)..."
go build -o "$BINARY" .

echo "Running..."
"$BINARY" --root "$REPO_ROOT" "$@"
