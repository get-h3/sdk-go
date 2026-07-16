// Conformance server — serves the ConformanceHarness via HTTP for h3-test validation.
//
// Usage:
//
//	go run ./examples/conformance/
//
// Then run: h3-test --endpoint http://localhost:9191
package main

import (
	"net/http"

	"github.com/get-h3/sdk-go/harness"
	"github.com/get-h3/sdk-go/testbed"
)

func main() {
	h := testbed.NewConformanceHarness()
	srv := harness.NewHTTPServer(h)
	http.ListenAndServe(":9191", srv)
}
