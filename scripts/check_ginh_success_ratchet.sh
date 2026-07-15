#!/usr/bin/env bash
# =============================================================================
# check_ginh_success_ratchet.sh — no NEW gin.H success bodies
# =============================================================================
# Success responses (200/201) must come from named, JSON-tagged structs in the
# service's handler apitypes.go — anonymous gin.H literals are un-generatable
# contracts and a classic source of client<->backend drift. gin.H remains
# fine for error bodies.
#
# This is a RATCHET: each module has a baseline count of pre-existing
# occurrences. The check fails when a module EXCEEDS its baseline (a new
# gin.H success body was added). When you migrate a surface and the count
# drops, LOWER the baseline here in the same PR so it can't creep back up.
# Migrated modules (baseline 0) stay at zero.
#
# Run locally: scripts/check_ginh_success_ratchet.sh
# CI: enforced in the "API Types Drift" job (lint.yml).
# =============================================================================
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# Modules not listed default to 0.
# (plain function, not declare -A: macOS ships bash 3.2)
baseline_for() {
    case "$1" in
        *)                   echo 0 ;;
    esac
}

PATTERN='c\.JSON\(http\.Status(OK|Created|Accepted), gin\.H'
fail=0

for dir in services/*/; do
    mod="$(basename "$dir")"
    count=$(grep -rEn "$PATTERN" "$dir" --include='*.go' 2>/dev/null | grep -v '_test\.go' | grep -cv '/testhelper/' || true)
    base="$(baseline_for "$mod")"
    if [ "$count" -gt "$base" ]; then
        echo "FAIL: services/$mod has $count gin.H success bodies (baseline $base)."
        echo "      New 200/201 responses must be named structs in internal/handler/apitypes.go:"
        grep -rEn "$PATTERN" "$dir" --include='*.go' | grep -v '_test\.go' | sed 's/^/        /'
        fail=1
    elif [ "$count" -lt "$base" ]; then
        echo "NOTE: services/$mod is at $count (baseline $base) — lower its baseline in scripts/check_ginh_success_ratchet.sh to lock in the progress."
    fi
done

if [ "$fail" -ne 0 ]; then
    exit 1
fi
echo "gin.H success-body ratchet: OK"
