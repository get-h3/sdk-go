package countguard

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The canonical counts live in exactly two machine-readable places: the
// repo-owned scripts/test-count.txt (battery + suite) and the live sources the
// guard derives them from (the Go test files, and the shim's canonical count
// when a sibling checkout is present). Nothing else policed the prose around
// them until H3-GAP-087 — the stale-count class re-offended here repeatedly
// (a CI grep hardcoded the battery total of the day and red-lined main while
// every test passed).
//
// These tests drive every documented outcome hermetically through the guard's
// env overrides, so a plain `go test ./...` catches drift before CI does:
//
//	exit 0 — canonical counts, the live suite and current-state prose agree;
//	exit 1 — drift (suite count moved, the battery moved upstream, a tracked
//	         current-state surface still quotes a retired count, or a dated
//	         report quotes one without a point-in-time banner);
//	exit 2 — the guard is misconfigured (missing / malformed canonical counts,
//	         an unreadable suite source, a non-numeric sibling count).
//	exit 0 with a BATTERY PARITY NOT VERIFIED caveat (GAP-053) — the sibling
//	         shim count is absent: the guard still passes (a fresh clone or a
//	         CI checkout has no ../shim) but the loud status AND the final
//	         summary say parity was not checked; H3_SDK_REQUIRE_SHIM_PARITY=1
//	         turns that absence into exit 2.
//
// Retired counts are assembled from digit fragments at runtime so this file's
// own source carries none of them: the guard sweeps tracked *.go files too, and
// a test that hardcoded them would fail the very sweep it is testing.

const (
	batteryKey = "battery"
	suiteKey   = "suite"
)

func repoRoot() string {
	// Tests run with the package directory as the working directory.
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		panic(err)
	}
	return root
}

func guardPath() string { return filepath.Join(repoRoot(), "scripts", "check-test-count.sh") }

func canonPath() string { return filepath.Join(repoRoot(), "scripts", "test-count.txt") }

// retiredDigits returns a retired battery total (a value the battery shipped
// before its current one) without writing the literal into this file.
func retiredBattery() string { return "4" + "5" }

// readCanonical returns the two canonical counts from scripts/test-count.txt.
func readCanonical(t *testing.T) map[string]int {
	t.Helper()
	raw, err := os.ReadFile(canonPath())
	if err != nil {
		t.Fatalf("read %s: %v", canonPath(), err)
	}
	found := map[string]int{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(value)
		if err != nil {
			t.Fatalf("%s: %q is not a bare number: %v", canonPath(), value, err)
		}
		if _, dup := found[key]; dup {
			t.Fatalf("%s declares %s= twice", canonPath(), key)
		}
		found[key] = n
	}
	for _, key := range []string{batteryKey, suiteKey} {
		if found[key] == 0 {
			t.Fatalf("%s must declare a positive %s=", canonPath(), key)
		}
	}
	return found
}

// runGuard runs the guard with a clean H3_SDK_* environment plus any overrides.
func runGuard(t *testing.T, override map[string]string) (int, string, string) {
	t.Helper()
	cmd := exec.Command("sh", guardPath())
	cmd.Dir = repoRoot()

	env := []string{}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "H3_SDK_") {
			continue
		}
		env = append(env, kv)
	}
	for key, value := range override {
		env = append(env, key+"="+value)
	}
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run guard: %v", err)
		}
		code = exitErr.ExitCode()
	}
	return code, stdout.String(), stderr.String()
}

func write(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// scratchTree builds a self-contained scan root the guard can police without
// touching the real repo: N synthetic Go tests, a canonical count file, and an
// absent sibling shim count (so battery parity is reported as NOT VERIFIED and
// every exit code below is driven by the check under test).
func scratchTree(t *testing.T, suite int) (root, canon string) {
	t.Helper()
	root = t.TempDir()
	var source strings.Builder
	source.WriteString("package sample\n\nimport \"testing\"\n\n")
	for i := 0; i < suite; i++ {
		fmt.Fprintf(&source, "func TestSynthetic%d(t *testing.T) {}\n", i)
	}
	write(t, filepath.Join(root, "sample_test.go"), source.String())
	canon = write(t, filepath.Join(root, "canon.txt"),
		fmt.Sprintf("%s=46\n%s=%d\n", batteryKey, suiteKey, suite))
	return root, canon
}

func scratchEnv(t *testing.T, root, canon string, extra map[string]string) map[string]string {
	t.Helper()
	env := map[string]string{
		"H3_SDK_SCAN_ROOT":       root,
		"H3_SDK_COUNT_FILE":      canon,
		"H3_SDK_LIVE":            "0",
		"H3_SDK_SHIM_COUNT_FILE": filepath.Join(root, "absent-shim-count.txt"),
	}
	for key, value := range extra {
		env[key] = value
	}
	return env
}

// --- the canonical inputs agree with reality --------------------------------

func TestCanonicalCountsAreWellFormed(t *testing.T) {
	counts := readCanonical(t)
	if counts[batteryKey] < 40 {
		t.Errorf("battery=%d looks wrong — the shim battery has been 40+ for a long time", counts[batteryKey])
	}
	if counts[suiteKey] < 1 {
		t.Errorf("suite=%d must be positive", counts[suiteKey])
	}
}

func TestLiveSuiteCountMatchesCanonical(t *testing.T) {
	// The guard derives the suite count from `go test ./... -list '^Test'`.
	// Prove that derivation agrees with the canonical file, so the number in
	// prose is the number the suite actually ships.
	counts := readCanonical(t)

	cmd := exec.Command("go", "test", "./...", "-list", "^Test")
	cmd.Dir = repoRoot()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go test -list: %v", err)
	}
	live := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Test") {
			live++
		}
	}
	if live != counts[suiteKey] {
		t.Fatalf("live suite has %d tests but scripts/test-count.txt says suite=%d", live, counts[suiteKey])
	}
}

func TestShimBatteryAgreesWhenCheckoutIsPresent(t *testing.T) {
	counts := readCanonical(t)

	t.Run("real sibling checkout agrees", func(t *testing.T) {
		// The only environment-dependent half: when the actual sibling checkout
		// sits at ../shim, its canonical count must agree with ours. In a
		// worktree (or a CI checkout) the default ../shim path resolves to
		// nothing, so this subtest skips — the branch behaviour below is driven
		// hermetically and runs everywhere.
		shimCanon := filepath.Join(repoRoot(), "..", "shim", "scripts", "test-count.txt")
		raw, err := os.ReadFile(shimCanon)
		if err != nil {
			t.Skipf("no sibling shim checkout at %s", shimCanon)
		}
		got := strings.TrimSpace(string(raw))
		want := strconv.Itoa(counts[batteryKey])
		if got != want {
			t.Fatalf("shim canonical battery is %q but this repo pins %q", got, want)
		}
	})

	// A shim count file that is guaranteed present, equal to our canonical
	// battery — drives the "sibling present and equal" branch on any host.
	presentShim := write(t, filepath.Join(t.TempDir(), "shim-count.txt"),
		strconv.Itoa(counts[batteryKey])+"\n")

	t.Run("present and equal: exit 0, agreement line printed", func(t *testing.T) {
		code, stdout, stderr := runGuard(t, map[string]string{
			"H3_SDK_SHIM_COUNT_FILE": presentShim,
		})
		if code != 0 {
			t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
		}
		if !strings.Contains(stdout, "battery agrees with the shim") {
			t.Errorf("guard did not report the agreement line:\n%s", stdout)
		}
		if !strings.Contains(stdout, "PASS — canonical battery") {
			t.Errorf("summary lost its plain PASS form:\n%s", stdout)
		}
	})

	t.Run("require knob: absent sibling exits 2 naming file and knob", func(t *testing.T) {
		// GAP-053: monorepo users and umbrella `make verify-counts` set
		// H3_SDK_REQUIRE_SHIM_PARITY=1 to fail fast when the sibling count is
		// absent — misconfiguration (exit 2), never a green pass.
		shim := filepath.Join(t.TempDir(), "absent-shim-count.txt")
		code, stdout, stderr := runGuard(t, map[string]string{
			"H3_SDK_SHIM_COUNT_FILE": shim,
			requireKnob:              "1",
		})
		if code != 2 {
			t.Fatalf("exit = %d, want 2 (stderr: %s)", code, stderr)
		}
		if !strings.Contains(stderr, "shim battery parity not verifiable") {
			t.Errorf("stderr does not name the failure:\n%s", stderr)
		}
		if !strings.Contains(stderr, shim) {
			t.Errorf("stderr does not name the missing file:\n%s", stderr)
		}
		if !strings.Contains(stderr, requireKnob) {
			t.Errorf("stderr does not name the env knob:\n%s", stderr)
		}
		if strings.Contains(stdout, "PASS") {
			t.Errorf("a demanded-but-missing parity must not PASS:\n%s", stdout)
		}
	})

	t.Run("present but wrong: still exit 1", func(t *testing.T) {
		shim := write(t, filepath.Join(t.TempDir(), "wrong-shim-count.txt"),
			strconv.Itoa(counts[batteryKey]-1)+"\n")
		code, _, stderr := runGuard(t, map[string]string{
			"H3_SDK_SHIM_COUNT_FILE": shim,
		})
		if code != 1 {
			t.Fatalf("exit = %d, want 1 (stderr: %s)", code, stderr)
		}
		if !strings.Contains(stderr, "battery drift") {
			t.Errorf("stderr does not name the drift:\n%s", stderr)
		}
	})

	t.Run("require knob with the sibling present is a plain pass", func(t *testing.T) {
		// The demand must not alter the outcome once parity IS verified.
		code, stdout, stderr := runGuard(t, map[string]string{
			"H3_SDK_SHIM_COUNT_FILE": presentShim,
			requireKnob:              "1",
		})
		if code != 0 {
			t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
		}
		if !strings.Contains(stdout, "battery agrees with the shim") {
			t.Errorf("agreement line missing with the knob set:\n%s", stdout)
		}
	})
}

// VERIFIED_ASSEMBLED and REQUIRE_KNOB are assembled from fragments because the
// guard sweeps tracked *.go files: a literal here would either trip check (d)
// itself or (for the knob) leak its name into a file checked only for counts.
// GAP-053: keep the loud status and the knob name stable — the tests below pin
// both, and the acceptance runs grep for them.
const verifiedAssembled = "VER" + "IFIED"

const requireKnob = "H3_SDK_REQUIRE" + "_SHIM_PARITY"

func TestGuardPassesOnTheCurrentTree(t *testing.T) {
	code, stdout, stderr := runGuard(t, nil)
	if code != 0 {
		t.Fatalf("guard exited %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "PASS") {
		t.Fatalf("guard did not report PASS:\n%s", stdout)
	}
	counts := readCanonical(t)
	for _, want := range []string{strconv.Itoa(counts[batteryKey]), strconv.Itoa(counts[suiteKey])} {
		if !strings.Contains(stdout, want) {
			t.Errorf("guard summary does not name the canonical count %s:\n%s", want, stdout)
		}
	}

	t.Run("absent sibling is loud, not a silent green", func(t *testing.T) {
		// GAP-053: with no sibling shim count, the guard still exits 0 (CI
		// checkouts have no ../shim) but the output must be impossible to
		// mistake for "parity checked and agreed".
		shim := filepath.Join(t.TempDir(), "absent-shim-count.txt")
		code, stdout, stderr := runGuard(t, map[string]string{
			"H3_SDK_SHIM_COUNT_FILE": shim,
		})
		if code != 0 {
			t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
		}
		if !strings.Contains(stdout, "parity NOT "+verifiedAssembled) {
			t.Errorf("stdout lacks the loud NOT VERIFIED status:\n%s", stdout)
		}
		if !strings.Contains(stdout, shim) {
			t.Errorf("the status does not name the path that was checked:\n%s", stdout)
		}
		if strings.Contains(stdout, "battery agrees") {
			t.Errorf("guard claims parity agreement with an absent sibling:\n%s", stdout)
		}
		if !strings.Contains(stdout, "battery parity NOT "+verifiedAssembled) {
			t.Errorf("final PASS line does not carry the NOT VERIFIED caveat:\n%s", stdout)
		}
	})
}

func TestGuardStaticDerivationMatchesLiveDerivation(t *testing.T) {
	// H3_SDK_LIVE=0 forces the static `^func Test` fallback. It must produce the
	// same number as the live toolchain, or the fallback silently lies.
	live, stdout, stderr := runGuard(t, nil)
	if live != 0 {
		t.Fatalf("live mode exited %d\n%s\n%s", live, stdout, stderr)
	}
	static, stdout, stderr := runGuard(t, map[string]string{"H3_SDK_LIVE": "0"})
	if static != 0 {
		t.Fatalf("static mode exited %d\n%s\n%s", static, stdout, stderr)
	}
	counts := readCanonical(t)
	if !strings.Contains(stdout, strconv.Itoa(counts[suiteKey])) {
		t.Fatalf("static derivation did not agree with suite=%d:\n%s", counts[suiteKey], stdout)
	}
}

// --- exit 2: guard misconfigured -------------------------------------------

func TestGuardExitsTwoWhenCanonicalFileIsMissing(t *testing.T) {
	code, _, stderr := runGuard(t, map[string]string{
		"H3_SDK_COUNT_FILE": filepath.Join(t.TempDir(), "absent.txt"),
	})
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, "canonical count file missing") {
		t.Errorf("stderr does not name the problem:\n%s", stderr)
	}
}

func TestGuardExitsTwoWhenCanonicalCountsAreMalformed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    string
	}{
		{"suite_not_a_number", "battery=46\nsuite=one hundred\n", "exactly one 'suite="},
		{"suite_missing", "battery=46\n", "exactly one 'suite="},
		{"battery_missing", "suite=110\n", "exactly one 'battery="},
		{"suite_duplicated", "battery=46\nsuite=110\nsuite=111\n", "exactly one 'suite="},
	} {
		t.Run(tc.name, func(t *testing.T) {
			canon := write(t, filepath.Join(t.TempDir(), "canon.txt"), tc.content)
			code, _, stderr := runGuard(t, map[string]string{"H3_SDK_COUNT_FILE": canon})
			if code != 2 {
				t.Fatalf("exit = %d, want 2 (stderr: %s)", code, stderr)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr does not name the problem %q:\n%s", tc.want, stderr)
			}
		})
	}
}

func TestGuardExitsTwoWhenSuiteSourceIsUnreadable(t *testing.T) {
	root := t.TempDir() // no *_test.go anywhere
	canon := write(t, filepath.Join(root, "canon.txt"), "battery=46\nsuite=3\n")
	code, _, stderr := runGuard(t, scratchEnv(t, root, canon, nil))
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, "cannot derive the Go suite count") {
		t.Errorf("stderr does not name the problem:\n%s", stderr)
	}
}

func TestGuardExitsTwoWhenSiblingCountIsNotANumber(t *testing.T) {
	counts := readCanonical(t)
	canon := write(t, filepath.Join(t.TempDir(), "canon.txt"),
		fmt.Sprintf("%s=%d\n%s=%d\n", batteryKey, counts[batteryKey], suiteKey, counts[suiteKey]))
	shim := write(t, filepath.Join(t.TempDir(), "shim-count.txt"), "forty-six\n")
	code, _, stderr := runGuard(t, map[string]string{
		"H3_SDK_COUNT_FILE":      canon,
		"H3_SDK_SHIM_COUNT_FILE": shim,
	})
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, "is not a bare number") {
		t.Errorf("stderr does not name the problem:\n%s", stderr)
	}
}

// --- exit 1: drift ---------------------------------------------------------

func TestGuardExitsOneWhenTheSuiteMoved(t *testing.T) {
	counts := readCanonical(t)
	canon := write(t, filepath.Join(t.TempDir(), "canon.txt"),
		fmt.Sprintf("%s=%d\n%s=%d\n", batteryKey, counts[batteryKey], suiteKey, counts[suiteKey]+1))
	code, _, stderr := runGuard(t, map[string]string{"H3_SDK_COUNT_FILE": canon})
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, "suite drift") {
		t.Errorf("stderr does not name the drift:\n%s", stderr)
	}
}

func TestGuardExitsOneWhenTheBatteryMovedUpstream(t *testing.T) {
	counts := readCanonical(t)
	canon := write(t, filepath.Join(t.TempDir(), "canon.txt"),
		fmt.Sprintf("%s=%d\n%s=%d\n", batteryKey, counts[batteryKey], suiteKey, counts[suiteKey]))
	shim := write(t, filepath.Join(t.TempDir(), "shim-count.txt"), strconv.Itoa(counts[batteryKey]+1)+"\n")
	code, _, stderr := runGuard(t, map[string]string{
		"H3_SDK_COUNT_FILE":      canon,
		"H3_SDK_SHIM_COUNT_FILE": shim,
	})
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, "battery drift") {
		t.Errorf("stderr does not name the drift:\n%s", stderr)
	}
}

func TestGuardFlagsARetiredCountInACurrentStateSurface(t *testing.T) {
	root, canon := scratchTree(t, 100)
	stale := retiredBattery() + "/" + retiredBattery()
	write(t, filepath.Join(root, "drift.md"), "the battery reports "+stale+" PASSED\n")

	code, stdout, stderr := runGuard(t, scratchEnv(t, root, canon, nil))
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "drift.md:1") {
		t.Errorf("the report does not name file:line:\n%s", stdout)
	}
	if !strings.Contains(stderr, "stale count literal") {
		t.Errorf("stderr does not name the class:\n%s", stderr)
	}
}

func TestGuardFlagsAStaleSuiteClaim(t *testing.T) {
	root, canon := scratchTree(t, 100)
	claim := strconv.Itoa(100+1) + " tests"
	write(t, filepath.Join(root, "suite-claim.md"), "Run the suite: "+claim+" green.\n")

	code, stdout, stderr := runGuard(t, scratchEnv(t, root, canon, nil))
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "suite-claim.md:1") {
		t.Errorf("the report does not name file:line:\n%s", stdout)
	}
	if !strings.Contains(stdout, "suite claim") {
		t.Errorf("the report does not name the claim class:\n%s", stdout)
	}
}

func TestGuardExemptsALineMarkedHistorical(t *testing.T) {
	root, canon := scratchTree(t, 100)
	stale := retiredBattery() + "/" + retiredBattery()
	write(t, filepath.Join(root, "narration.md"),
		"first run scored "+stale+" (count-ok-historical: the 2026-08 era).\n")

	code, stdout, stderr := runGuard(t, scratchEnv(t, root, canon, nil))
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "PASS") {
		t.Errorf("guard did not report PASS:\n%s", stdout)
	}
}

func TestGuardExemptsADocumentThatDeclaresItselfHistorical(t *testing.T) {
	root, canon := scratchTree(t, 100)
	stale := retiredBattery() + "/" + retiredBattery()
	write(t, filepath.Join(root, "docs", "dogfood", "diagnostics.md"),
		"# Trail\n\n> **Historical (2026-08-04):** point-in-time record.\n\nthe battery scored "+stale+"\n")

	code, stdout, stderr := runGuard(t, scratchEnv(t, root, canon, nil))
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

func TestGuardRequiresABannerOnDatedRecords(t *testing.T) {
	root, canon := scratchTree(t, 100)
	stale := retiredBattery() + "/" + retiredBattery()
	report := filepath.Join(root, "docs", "dogfood", "2026-01-01-integration.md")
	write(t, report, "# Report\n\nthe battery scored "+stale+"\n")

	code, _, stderr := runGuard(t, scratchEnv(t, root, canon, nil))
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, "no point-in-time banner") {
		t.Errorf("stderr does not ask for a banner:\n%s", stderr)
	}
	if !strings.Contains(stderr, "2026-01-01-integration.md") {
		t.Errorf("stderr does not name the record:\n%s", stderr)
	}

	write(t, report, "# Report\n\n"+
		"> **Historical (2026-01-01):** point-in-time record — the counts below are\n"+
		"> not live status.\n\nthe battery scored "+stale+"\n")

	code, stdout, stderr := runGuard(t, scratchEnv(t, root, canon, nil))
	if code != 0 {
		t.Fatalf("bannered record still fails: exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

func TestGuardFailsOnAPathWithWhitespace(t *testing.T) {
	root, canon := scratchTree(t, 100)
	// A file list that word-splits would silently scan the wrong paths.
	write(t, filepath.Join(root, "odd name.md"), "nothing here\n")
	code, _, stderr := runGuard(t, scratchEnv(t, root, canon, nil))
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, "whitespace in a tracked path") {
		t.Errorf("stderr does not name the problem:\n%s", stderr)
	}
}

// --- the guard is wired into normal verification ---------------------------

func TestGuardIsWiredIntoMakeAndCI(t *testing.T) {
	makefile, err := os.ReadFile(filepath.Join(repoRoot(), "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	if !regexp.MustCompile(`(?m)^\.PHONY:.*\bverify-counts\b`).Match(makefile) {
		t.Error("Makefile .PHONY does not list verify-counts")
	}
	if !regexp.MustCompile(`(?m)^verify-counts:\n\tsh scripts/check-test-count\.sh$`).Match(makefile) {
		t.Error("Makefile has no `verify-counts: sh scripts/check-test-count.sh` target")
	}
	if !regexp.MustCompile(`(?m)^all:.*\bverify-counts\b`).Match(makefile) {
		t.Error("Makefile `all:` target does not run verify-counts")
	}

	workflow, err := os.ReadFile(filepath.Join(repoRoot(), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}
	if !strings.Contains(string(workflow), "sh scripts/check-test-count.sh") {
		t.Error("ci.yml does not run the count guard")
	}

	if _, err := os.Stat(guardPath()); err != nil {
		t.Errorf("guard script is missing: %v", err)
	}
	if _, err := os.Stat(canonPath()); err != nil {
		t.Errorf("canonical count file is missing: %v", err)
	}
}
