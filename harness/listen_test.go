package harness

import (
	"os"
	"strings"
	"testing"
)

// clearPort unsets PORT for the duration of the calling test and restores
// whatever the host had. t.Setenv cannot express "unset", and the fallback rule
// only exists in relation to an unset/blank variable — the state a fresh
// terminal has.
func clearPort(t *testing.T) {
	t.Helper()
	saved, had := os.LookupEnv(PortEnv)
	if err := os.Unsetenv(PortEnv); err != nil {
		t.Fatalf("os.Unsetenv(%s): %v", PortEnv, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(PortEnv, saved)
			return
		}
		_ = os.Unsetenv(PortEnv)
	})
}

// TestPortFromEnv pins the ONE port-resolution rule every example harness shares
// (H3-SDK-GO-FOREMAN-4): PORT wins when it carries a value, and an unset, empty
// or blank PORT falls back to DefaultPort. The four examples used to hardcode
// ":9191", so a second one could never be started; this table is what keeps
// "PORT=9293 go run ./examples/<name>/" honest for all of them.
func TestPortFromEnv(t *testing.T) {
	tests := []struct {
		name string
		port string
		set  bool
		want string
	}{
		{name: "unset falls back to the default", set: false, want: DefaultPort},
		{name: "empty falls back to the default", set: true, port: "", want: DefaultPort},
		{name: "whitespace-only falls back to the default", set: true, port: " \t\n ", want: DefaultPort},
		{name: "explicit value wins", set: true, port: "9293", want: "9293"},
		{name: "surrounding whitespace is trimmed", set: true, port: " 9293\n", want: "9293"},
		{name: "explicit default value is returned as-is", set: true, port: DefaultPort, want: "9191"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearPort(t)
			if tt.set {
				t.Setenv(PortEnv, tt.port)
			}
			if got := PortFromEnv(); got != tt.want {
				t.Errorf("PortFromEnv() = %q, want %q (PORT=%q, set=%v)", got, tt.want, tt.port, tt.set)
			}
		})
	}
}

// TestListenAddrTracksPortOverride asserts the value the examples hand to
// http.ListenAndServe: it must carry the resolved PORT and the ":" separator, so
// a PORT override really moves the listener rather than only changing a log line.
func TestListenAddrTracksPortOverride(t *testing.T) {
	tests := []struct {
		name string
		port string
		set  bool
		want string
	}{
		{name: "unset PORT listens on the default", set: false, want: ":" + DefaultPort},
		{name: "blank PORT listens on the default", set: true, port: "  ", want: ":" + DefaultPort},
		{name: "PORT override moves the listener", set: true, port: "9293", want: ":9293"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearPort(t)
			if tt.set {
				t.Setenv(PortEnv, tt.port)
			}
			if got := ListenAddr(); got != tt.want {
				t.Errorf("ListenAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAddrInUseMessage pins the collision hint byte-for-byte. The wording is the
// one examples/echo printed before the hint was shared, so pinning it keeps all
// four examples saying what a reader of echo already knows — and proving the
// message names both the address and the PORT override.
func TestAddrInUseMessage(t *testing.T) {
	got := AddrInUseMessage(":9293")
	want := "address already in use on :9293 — is another harness running? set PORT to override"
	if got != want {
		t.Errorf("AddrInUseMessage(\":9293\") = %q, want %q", got, want)
	}
	for _, needle := range []string{":9293", PortEnv} {
		if !strings.Contains(got, needle) {
			t.Errorf("AddrInUseMessage(\":9293\") = %q, want it to name %q", got, needle)
		}
	}
}
