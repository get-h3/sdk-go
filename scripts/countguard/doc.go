// Package countguard holds the tests for this repo's compliance/test-count
// guard, scripts/check-test-count.sh (H3-GAP-087).
//
// The guard is a shell script because it must run before/without the Go
// toolchain is set up (CI, pre-commit, a fresh clone) and because the shim —
// the sibling repo this pattern is propagated from — polices its own prose the
// same way. Its tests live in the repo's own toolchain (go test) so
// `make test` / `make all` / CI exercise them with everything else.
//
// The script itself carries the count contract; this package only drives it.
package countguard
