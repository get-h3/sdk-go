// Conformance server — serves the ConformanceHarness via HTTP for h3-test validation.
//
// Usage:
//
//	go run ./examples/conformance/
//	PORT=9293 go run ./examples/conformance/   # when another harness holds :9191
//
// Then run: h3-test --endpoint http://localhost:9191
package main

import (
	"log"

	"github.com/get-h3/sdk-go/harness"
	"github.com/get-h3/sdk-go/testbed"
)

func main() {
	addr := harness.ListenAddr()
	srv := harness.NewHTTPServer(testbed.NewConformanceHarness())
	log.Printf("h3 conformance harness listening on %s (set %s to override)", addr, harness.PortEnv)
	// Serve never returns: it reports a bind collision with the shared hint
	// naming the PORT override, and every other listen error is fatal.
	harness.Serve(addr, srv)
}
