// Package lintguard holds the tests for scripts/lint.sh, the lint driver the
// Makefile's lint target invokes (DF-H3-SDK-GO-FOREMAN-1).
//
// The bug class: the lint target was a single `||` chain ending in `echo`, so
// `make lint` exited 0 on any host with neither golangci-lint nor staticcheck
// installed — green lint that linted nothing. The chain now lives in a script
// with explicit availability checks, and this package drives every branch
// hermetically with stub linters through the script's H3_LINT_* env overrides,
// so a plain `go test ./...` catches regressions of the shell semantics
// without requiring any real linter on PATH.
//
// The contract proven here:
//
//	exit 0     — an installed linter ran and passed (golangci-lint directly, or
//	             the staticcheck fallback when golangci-lint failed);
//	exit != 0  — neither linter installed (with a clear diagnostic), or
//	             golangci-lint failed with no staticcheck installed, or the
//	             only installed linter reported problems.
//
// Like the countguard, the script itself carries the behaviour; this package
// only drives it.
package lintguard
