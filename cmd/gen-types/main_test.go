package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// GAP-045: gen-types is a schema validator, not a code generator. Pin the
// user-facing text so no surface can drift back to stub language or claim the
// command emits Go types.
func TestGenTypesOutputDescribesSchemaValidation(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "a.json")
	if err := os.WriteFile(good, []byte(`{"type":"object"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run([]string{good}, &out, &errOut); code != 0 {
		t.Fatalf("valid schema: exit %d, stderr: %s", code, errOut.String())
	}
	stdout := out.String()
	if !strings.Contains(stdout, "gen-types: validating 1 schema files") {
		t.Errorf("missing validation banner in %q", stdout)
	}
	if !strings.Contains(stdout, "gen-types: all schemas valid") {
		t.Errorf("missing success line in %q", stdout)
	}
	for _, banned := range []string{"stub", "pending", "generates", "generated"} {
		if strings.Contains(stdout, banned) {
			t.Errorf("stdout claims %q; command only validates", banned)
		}
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{bad}, &out, &errOut); code != 1 {
		t.Fatalf("invalid schema: exit %d, want 1 (stderr: %s)", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "invalid JSON") {
		t.Errorf("missing invalid-JSON diagnostic: %q", errOut.String())
	}

	out.Reset()
	errOut.Reset()
	if code := run([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("help exit %d, want 0 (stderr: %s)", code, errOut.String())
	}
	text := out.String()
	if !strings.Contains(text, "validate H3 protocol JSON Schema files") {
		t.Errorf("help does not identify schema validation: %q", text)
	}
	if !strings.Contains(text, "does not generate Go code") {
		t.Errorf("help does not disclaim code generation: %q", text)
	}

	out.Reset()
	errOut.Reset()
	if code := run(nil, &out, &errOut); code != 1 {
		t.Fatalf("no-args exit %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "usage: gen-types") {
		t.Errorf("missing usage line: %q", errOut.String())
	}
}
