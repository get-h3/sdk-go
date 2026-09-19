// Command gen-types validates the H3 protocol JSON Schema files.
//
// It is a schema validator, not a code generator: for each file matching the
// given globs it checks that the file exists and parses as JSON. The Go wire
// types in protocol/types.go are maintained by hand against these schemas;
// the //go:generate directive in types.go runs this command to keep the
// schema inputs valid, not to emit Go code (GAP-045).
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const usageText = `gen-types — validate H3 protocol JSON Schema files

usage: gen-types <schema_glob>...

For every file matching the globs, gen-types checks that it exists and parses
as JSON, printing one line per file. It does not generate Go code: the wire
types in protocol/types.go are maintained by hand against the schemas.

exit codes: 0 = all schemas valid (or help requested), 1 = usage error or
invalid/missing schema
`

// run implements the command for args (os.Args[1:]) and returns the process
// exit code. Output goes to stdout/stderr so tests can drive it directly.
func run(args []string, stdout, stderr io.Writer) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			fmt.Fprint(stdout, usageText)
			return 0
		}
	}

	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return 1
	}

	// Collect schema files from globs
	var files []string
	for _, glob := range args {
		matches, err := filepath.Glob(glob)
		if err != nil {
			fmt.Fprintf(stderr, "glob error %q: %v\n", glob, err)
			return 1
		}
		files = append(files, matches...)
	}

	if len(files) == 0 {
		fmt.Fprintln(stderr, "no schema files found")
		return 1
	}

	fmt.Fprintf(stdout, "gen-types: validating %d schema files\n", len(files))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintf(stderr, "read %s: %v\n", f, err)
			return 1
		}
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			fmt.Fprintf(stderr, "invalid JSON in %s: %v\n", f, err)
			return 1
		}
		fmt.Fprintf(stdout, "  ✓ %s\n", strings.TrimPrefix(f, "schemas/"))
	}

	fmt.Fprintln(stdout, "gen-types: all schemas valid")
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
