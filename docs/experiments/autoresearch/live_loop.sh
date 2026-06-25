#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="/home/albert/git/maelstrom-v7"
SCRIPT="$REPO_ROOT/docs/experiments/autoresearch/live_loop.py"

python "$SCRIPT" "$@"
