package lintguard

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These tests drive scripts/lint.sh end-to-end with stub linter binaries wired
// in through the script's H3_LINT_* env overrides, so every branch of the
// availability / fallback / fail-loud contract is exercised deterministically
// on a host that has (or hasn't) any real linter installed.
//
// DF-H3-SDK-GO-FOREMAN-1: the Makefile lint target was a bare
// `golangci-lint ... || staticcheck ... || echo ...` chain — when neither tool
// existed, the `echo` made the recipe succeed and `make lint` printed green
// while linting nothing.

func repoRoot() string {
	// Tests run with the package directory as the working directory.
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		panic(err)
	}
	return root
}

func lintScript() string { return filepath.Join(repoRoot(), "scripts", "lint.sh") }

// stub is a fake linter: records every invocation (one line per call) and
// exits with a fixed code.
type stub struct {
	path   string
	record string
}

func newStub(t *testing.T, dir, name string, exitCode int) stub {
	t.Helper()
	record := filepath.Join(dir, name+".calls")
	path := filepath.Join(dir, name)
	body := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$*\" >>%q\nexit %d\n", record, exitCode)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write stub %s: %v", name, err)
	}
	return stub{path: path, record: record}
}

// missing returns a path guaranteed not to resolve, simulating "not installed".
func missing(dir, name string) string { return filepath.Join(dir, name+"-NOT-INSTALLED") }

func calls(t *testing.T, s stub) []string {
	t.Helper()
	raw, err := os.ReadFile(s.record)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("read stub record: %v", err)
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// runLint executes scripts/lint.sh with the given linter overrides against an
// empty scratch directory, returning exit code, stdout and stderr.
func runLint(t *testing.T, golangci, staticcheck string) (int, string, string) {
	t.Helper()
	if _, err := os.Stat(lintScript()); err != nil {
		t.Fatalf("lint script missing: %v", err)
	}
	cmd := exec.Command("sh", lintScript())
	cmd.Dir = repoRoot()
	cmd.Env = append(os.Environ(),
		"H3_LINT_GOLANGCI="+golangci,
		"H3_LINT_STATICCHECK="+staticcheck,
		"H3_LINT_DIR="+t.TempDir(),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run lint.sh: %v", err)
		}
		code = exitErr.ExitCode()
	}
	return code, stdout.String(), stderr.String()
}

// --- the contract branches ---------------------------------------------------

func TestNoLinterFailsWithDiagnostic(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runLint(t, missing(dir, "golangci-lint"), missing(dir, "staticcheck"))
	if code == 0 {
		t.Fatal("lint.sh exited 0 with NO linter available — the exact DF-H3-SDK-GO-FOREMAN-1 bug (silence read as success)")
	}
	for _, want := range []string{"no linter available", "golangci-lint", "staticcheck", "make lint"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr diagnostic missing %q; got:\n%s", want, stderr)
		}
	}
}

func TestGolangciPassesSkipsStaticcheck(t *testing.T) {
	dir := t.TempDir()
	gc := newStub(t, dir, "golangci-lint", 0)
	sc := newStub(t, dir, "staticcheck", 0)
	code, _, stderr := runLint(t, gc.path, sc.path)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (installed linter passed); stderr:\n%s", code, stderr)
	}
	if got := calls(t, gc); len(got) != 1 || got[0] != "run ./..." {
		t.Errorf("golangci-lint calls = %q, want one 'run ./...'", got)
	}
	if got := calls(t, sc); len(got) != 0 {
		t.Errorf("staticcheck must not run when golangci-lint passes, got %q", got)
	}
}

func TestGolangciFailsFallsBackToStaticcheck(t *testing.T) {
	dir := t.TempDir()
	gc := newStub(t, dir, "golangci-lint", 1)
	sc := newStub(t, dir, "staticcheck", 0)
	code, _, stderr := runLint(t, gc.path, sc.path)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (clean staticcheck fallback); stderr:\n%s", code, stderr)
	}
	if got := calls(t, gc); len(got) != 1 {
		t.Errorf("golangci-lint calls = %q, want exactly one attempt", got)
	}
	if got := calls(t, sc); len(got) != 1 || got[0] != "./..." {
		t.Errorf("staticcheck calls = %q, want one './...'", got)
	}
	if !strings.Contains(stderr, "falling back to staticcheck") {
		t.Errorf("fallback not announced on stderr; got:\n%s", stderr)
	}
}

func TestGolangciFailsNoStaticcheckFails(t *testing.T) {
	dir := t.TempDir()
	gc := newStub(t, dir, "golangci-lint", 1)
	code, _, stderr := runLint(t, gc.path, missing(dir, "staticcheck"))
	if code == 0 {
		t.Fatal("golangci-lint failed with no staticcheck installed — must not exit 0")
	}
	if !strings.Contains(stderr, "staticcheck is not installed") {
		t.Errorf("stderr must name the missing fallback; got:\n%s", stderr)
	}
	if got := calls(t, gc); len(got) != 1 {
		t.Errorf("golangci-lint calls = %q, want exactly one attempt", got)
	}
}

func TestStaticcheckOnlyPasses(t *testing.T) {
	dir := t.TempDir()
	sc := newStub(t, dir, "staticcheck", 0)
	code, _, stderr := runLint(t, missing(dir, "golangci-lint"), sc.path)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", code, stderr)
	}
	if got := calls(t, sc); len(got) != 1 || got[0] != "./..." {
		t.Errorf("staticcheck calls = %q, want one './...'", got)
	}
}

func TestStaticcheckOnlyFails(t *testing.T) {
	dir := t.TempDir()
	sc := newStub(t, dir, "staticcheck", 1)
	code, _, _ := runLint(t, missing(dir, "golangci-lint"), sc.path)
	if code == 0 {
		t.Fatal("the only installed linter reported problems — must not exit 0")
	}
	if got := calls(t, sc); len(got) != 1 {
		t.Errorf("staticcheck calls = %q, want exactly one attempt", got)
	}
}

func TestBothInstalledGolangciFailsStaticcheckFails(t *testing.T) {
	dir := t.TempDir()
	gc := newStub(t, dir, "golangci-lint", 1)
	sc := newStub(t, dir, "staticcheck", 1)
	code, _, _ := runLint(t, gc.path, sc.path)
	if code == 0 {
		t.Fatal("both linters ran and both reported problems — must not exit 0")
	}
	if got := calls(t, sc); len(got) != 1 {
		t.Errorf("staticcheck fallback calls = %q, want exactly one", got)
	}
}

// --- negative control + wiring ----------------------------------------------

// TestOldChainShapeSilentlySucceeded pins WHY the fix was needed: the exact
// pre-fix recipe shape (two missing commands rescued by a trailing `echo`)
// exits 0. lint.sh with the same "nothing installed" inputs must NOT. If
// someone reintroduces the echo-terminated chain, the tests above flip red.
func TestOldChainShapeSilentlySucceeded(t *testing.T) {
	dir := t.TempDir()
	old := "golangci-lint-ABSENT run ./... 2>/dev/null || staticcheck-ABSENT ./... 2>/dev/null || echo \"lint: no linter available (install golangci-lint or staticcheck)\""
	cmd := exec.Command("sh", "-c", old)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		if !strings.Contains(stdout.String(), "no linter available") {
			t.Fatalf("control lost its premise: old chain exited 0 without the echo; stdout:\n%s", stdout.String())
		}
	} else {
		t.Fatalf("premise broken: old echo-terminated chain exited non-zero (err=%v)", err)
	}

	// The fixed script under the same "nothing installed" condition fails.
	code, _, _ := runLint(t, missing(dir, "golangci-lint"), missing(dir, "staticcheck"))
	if code == 0 {
		t.Fatal("lint.sh reproduced the old bug: exit 0 with no linter")
	}
}

func TestMakefileLintTargetDelegatesToScript(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(), "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	recipe := lintRecipeFromMakefile(string(raw))
	if !strings.Contains(recipe, "scripts/lint.sh") {
		t.Errorf("Makefile lint target must delegate to scripts/lint.sh; recipe was:\n%s", recipe)
	}
	if strings.Contains(recipe, "|| echo") {
		t.Errorf("Makefile lint recipe still carries an `|| echo` terminator (the exit-0 mask); recipe:\n%s", recipe)
	}
}

// lintRecipeFromMakefile returns the tab-indented body of the top-level
// `lint:` target (best-effort line scan; the Makefile is small and regular).
func lintRecipeFromMakefile(text string) string {
	inLint := false
	var body []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "lint:") {
			inLint = true
			continue
		}
		if inLint {
			if strings.HasPrefix(line, "\t") {
				body = append(body, line)
				continue
			}
			break
		}
	}
	return strings.Join(body, "\n")
}
