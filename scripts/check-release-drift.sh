#!/bin/sh
# check-release-drift.sh — informational release-drift reporter (GAP-038).
#
# WHY THIS EXISTS
# ---------------
# This repo has shipped the same defect class six times: work lands on main, the
# published tag stays behind, and `go get github.com/get-h3/sdk-go@latest` keeps
# resolving a module that predates the fixes (GAP-010/025/031/033/036/038/047).
# Nothing here reported the distance between HEAD and the last tag, so the drift
# was only ever noticed by hand.
#
# WHY IT IS INFORMATIONAL
# -----------------------
# CI must never go red merely because main is ahead of the last tag. A busy main
# is normal, and a permanently red job gets ignored — which is exactly how the
# drift stayed invisible in the first place. So this script exits 0 always. The
# gate is manual / foreman-invoked (`make release-drift`); the ONLY path to a
# non-zero exit is an explicit `--fail-over N`, which fails when the
# non-bookkeeping drift exceeds N. The CI job that runs it is marked
# continue-on-error for the same reason: reporting, not gatekeeping.
#
# WHAT COUNTS AS DRIFT
# --------------------
# Every commit reachable from HEAD but not from the latest tag is drift; the
# subset that matters to consumers is the non-bookkeeping commits. Foreman tick
# ("chore(foreman)", "foreman tick") and board ("board:") commits carry no
# wire-facing change, so they are excluded from the warning comparison.
#
# USAGE
#   sh scripts/check-release-drift.sh                 # report; always exit 0
#   sh scripts/check-release-drift.sh 25              # warn above 25; exit 0
#   sh scripts/check-release-drift.sh --fail-over 25  # exit 1 above 25
#   sh scripts/check-release-drift.sh --fail-over=25  # same
#
# Threshold resolution: the first argument, or the value given to --fail-over,
# else DEFAULT_THRESHOLD below.
#
# Works from anywhere: the repo root is derived from this script's own location.

set -u

DEFAULT_THRESHOLD=20
THRESHOLD=$DEFAULT_THRESHOLD
FAIL_OVER=0

while [ "$#" -gt 0 ]; do
    case "$1" in
        --fail-over)
            FAIL_OVER=1
            if [ "$#" -ge 2 ]; then
                THRESHOLD=$2
                shift 2
            else
                shift
            fi
            ;;
        --fail-over=*)
            FAIL_OVER=1
            THRESHOLD=${1#--fail-over=}
            shift
            ;;
        *)
            THRESHOLD=$1
            shift
            ;;
    esac
done

case "$THRESHOLD" in
    '' | *[!0-9]*)
        echo "WARN: ignoring non-numeric drift threshold '$THRESHOLD' — using $DEFAULT_THRESHOLD" >&2
        THRESHOLD=$DEFAULT_THRESHOLD
        ;;
esac

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

if ! git -C "$ROOT" rev-parse --git-dir >/dev/null 2>&1; then
    echo "NOTICE: $ROOT is not a git work tree — drift not measured"
    exit 0
fi

TAG=$(git -C "$ROOT" describe --tags --abbrev=0 2>/dev/null || true)
if [ -z "$TAG" ]; then
    # Fallback for clones where describe finds no reachable tag (e.g. a tag that
    # only exists on a side branch) but tags are still present.
    TAG=$(git -C "$ROOT" tag -l --sort=-creatordate 2>/dev/null | head -n 1)
fi

if [ -z "$TAG" ]; then
    echo "NO TAGS"
    exit 0
fi

if ! git -C "$ROOT" rev-parse --verify --quiet "${TAG}^{commit}" >/dev/null 2>&1; then
    echo "NOTICE: latest tag '$TAG' does not resolve to a commit in this clone — drift not measured"
    exit 0
fi

TOTAL=$(git -C "$ROOT" rev-list --count "${TAG}..HEAD" 2>/dev/null || true)
if [ -z "$TOTAL" ]; then
    echo "NOTICE: cannot walk ${TAG}..HEAD in this clone (shallow checkout?) — drift not measured"
    exit 0
fi

# Bookkeeping = foreman tick and board writes. Non-merge subjects only: the
# per-commit list is what a consumer would actually receive on the next tag.
NON_BOOK=$(git -C "$ROOT" log --no-merges --format='%s' "${TAG}..HEAD" 2>/dev/null \
    | grep -v -E '^chore\(foreman\)|foreman tick|^board:' \
    | grep -c '.' || true)
case "$NON_BOOK" in
    '' | *[!0-9]*) NON_BOOK=0 ;;
esac

echo "RELEASE DRIFT: ${TOTAL} commits (${NON_BOOK} non-bookkeeping) since ${TAG}"

if [ "$NON_BOOK" -gt "$THRESHOLD" ]; then
    echo "WARN: release drift exceeds ${THRESHOLD} commits — cut the next tag"
    if [ "$FAIL_OVER" -eq 1 ]; then
        echo "FAIL: --fail-over ${THRESHOLD} given and ${NON_BOOK} non-bookkeeping commits are unreleased" >&2
        exit 1
    fi
fi

exit 0
