// Command gen-types regenerates Go protocol types from H3 JSON Schema files.
// Stub: validates schema files exist and are parseable JSON.
// Full implementation will generate Go structs with JSON tags matching the wire format.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: gen-types <schema_glob>...\n")
		os.Exit(1)
	}

	// Collect schema files from globs
	var files []string
	for _, glob := range os.Args[1:] {
		matches, err := filepath.Glob(glob)
		if err != nil {
			fmt.Fprintf(os.Stderr, "glob error %q: %v\n", glob, err)
			os.Exit(1)
		}
		files = append(files, matches...)
	}

	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "no schema files found\n")
		os.Exit(1)
	}

	fmt.Printf("gen-types: validating %d schema files\n", len(files))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", f, err)
			os.Exit(1)
		}
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			fmt.Fprintf(os.Stderr, "invalid JSON in %s: %v\n", f, err)
			os.Exit(1)
		}
		fmt.Printf("  ✓ %s\n", strings.TrimPrefix(f, "schemas/"))
	}

	fmt.Println("gen-types: all schemas valid (stub — full code generation pending)")
}
