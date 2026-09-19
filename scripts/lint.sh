#!/bin/sh
# lint.sh — run whichever Go linters are installed; fail loudly when none are.
#
# Why this exists (DF-H3-SDK-GO-FOREMAN-1): the Makefile's lint target used to
# be `golangci-lint run ./... || staticcheck ./... || echo "..."`, so on a host
# with NEITHER linter installed the final `echo` succeeded and `make lint`
# exited 0 while nothing was checked. Availability, the fallback chain and the
# diagnostics now live here, where scripts/lintguard can drive every branch
# hermetically instead of the behaviour hiding inside a Makefile one-liner.
#
# Contract (exit codes):
#   0 — a linter ran and passed: golangci-lint directly, or the staticcheck
#       fallback exactly as the old `||` chain behaved (golangci-lint failed,
#       staticcheck is installed and clean);
#   non-zero — no linter is installed at all (clear diagnostic on stderr), or
#   golangci-lint failed with no staticcheck installed, or the linter(s) that
#   ran reported problems.
#
# Dependencies: POSIX sh + coreutils. No network, no venv.
#
# Overrides (driven by scripts/lintguard/lint_test.go):
#   H3_LINT_GOLANGCI     binary name/path standing in for golangci-lint
#   H3_LINT_STATICCHECK  binary name/path standing in for staticcheck
#   H3_LINT_DIR          directory to lint (default: this repo's root)

set -eu

GOLANGCI=${H3_LINT_GOLANGCI:-golangci-lint}
STATICCHECK=${H3_LINT_STATICCHECK:-staticcheck}
TARGET_DIR=${H3_LINT_DIR:-$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)}

have() { command -v "$1" >/dev/null 2>&1; }

# The whole point of the fix: silence is not success. If nothing is installed,
# fail with a diagnostic that names both tools and how to get them.
if ! have "$GOLANGCI" && ! have "$STATICCHECK"; then
    echo "lint: FAIL — no linter available; refusing to report success." >&2
    echo "lint: neither '${GOLANGCI}' nor '${STATICCHECK}' was found on PATH." >&2
    echo "lint: install one, e.g.:" >&2
    echo "lint:   go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" >&2
    echo "lint:   go install honnef.co/go/tools/cmd/staticcheck@latest" >&2
    echo "lint: then re-run: make lint" >&2
    exit 1
fi

cd "$TARGET_DIR"

if have "$GOLANGCI"; then
    if "$GOLANGCI" run ./...; then
        exit 0
    fi
    # golangci-lint ran and did not pass. Fall back to staticcheck when it is
    # installed (the old `||` chain semantics); otherwise the failure stands.
    if ! have "$STATICCHECK"; then
        echo "lint: FAIL — golangci-lint reported problems and staticcheck is not installed to cross-check." >&2
        exit 1
    fi
    echo "lint: NOTE — golangci-lint failed; falling back to staticcheck." >&2
    "$STATICCHECK" ./...
    exit 0
fi

# Only staticcheck is installed: its result governs.
"$STATICCHECK" ./...
exit 0
