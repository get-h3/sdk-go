package harness

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"syscall"
)

// This file is the ONE place that decides which port the examples in examples/
// serve on (H3-SDK-GO-FOREMAN-4). Before it existed each example hardcoded
// ":9191": starting a second one while the first was running died with a bare
// "address already in use", only examples/echo explained how to move the
// listener, and the four examples disagreed about whether a port could be
// chosen at all. Resolution rules, the listen address and the collision message
// live here so every example behaves identically.

// DefaultPort is the TCP port every example harness serves on when the PORT
// environment variable is unset, empty or blank.
const DefaultPort = "9191"

// PortEnv is the environment variable every example harness reads to move its
// listener off DefaultPort — the documented fix for "address already in use"
// when another harness (or a sibling test run) already owns the default port.
const PortEnv = "PORT"

// PortFromEnv returns the TCP port an example harness should listen on: the
// PORT environment variable with surrounding whitespace trimmed, or DefaultPort
// when PORT is unset, empty or blank.
func PortFromEnv() string {
	if port := strings.TrimSpace(os.Getenv(PortEnv)); port != "" {
		return port
	}
	return DefaultPort
}

// ListenAddr returns the ":<port>" listen address for an example harness — the
// value to hand to http.ListenAndServe. It is derived from PortFromEnv, so
// "PORT=9293 go run ./examples/conformance/" serves on :9293 while an unset PORT
// serves on DefaultPort (:9191).
func ListenAddr() string {
	return ":" + PortFromEnv()
}

// AddrInUseMessage is the shared, user-facing explanation for a failed bind on
// addr. It names the address and the PortEnv override, so a reader learns how to
// move the listener instead of only reading "address already in use".
func AddrInUseMessage(addr string) string {
	return fmt.Sprintf("address already in use on %s — is another harness running? set %s to override", addr, PortEnv)
}

// Serve starts h on addr and blocks. It never returns: a bind collision is
// reported with AddrInUseMessage (naming the PortEnv override) and any other
// listen error is reported as-is — both are fatal with a non-zero exit, so a
// start-up failure can never be mistaken for a running harness.
//
// Callers are the main() functions of the examples, which pass ListenAddr() as
// addr to get the shared PORT default/override.
func Serve(addr string, h http.Handler) {
	err := http.ListenAndServe(addr, h)
	if errors.Is(err, syscall.EADDRINUSE) {
		log.Fatal(AddrInUseMessage(addr))
	}
	log.Fatal(err)
}
