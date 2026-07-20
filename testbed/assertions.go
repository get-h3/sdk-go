// Package testbed provides MockHermes and assertion helpers for unit-testing
// harness implementations against the H3 protocol.
package testbed

import (
	"testing"

	"github.com/get-h3/sdk-go/protocol"
)

// AssertDecisionType fails the test if d.Decision != expected.
func AssertDecisionType(t *testing.T, d *protocol.Decision, expected protocol.DecisionType) {
	t.Helper()
	if d == nil {
		t.Fatal("decision is nil")
	}
	if d.Decision != expected {
		t.Errorf("expected decision type %q, got %q", expected, d.Decision)
	}
}

// AssertTextContent fails the test if d.Text.Content != expected or d.Text.Finished != finished.
func AssertTextContent(t *testing.T, d *protocol.Decision, content string, finished bool) {
	t.Helper()
	if d == nil {
		t.Fatal("decision is nil")
	}
	if d.Text == nil {
		t.Fatal("Text field is nil — expected a text decision")
	}
	if d.Text.Content != content {
		t.Errorf("expected text content %q, got %q", content, d.Text.Content)
	}
	if d.Text.Finished != finished {
		t.Errorf("expected finished=%v, got %v", finished, d.Text.Finished)
	}
}

// AssertEndReason fails the test if d.End.Reason != expected.
func AssertEndReason(t *testing.T, d *protocol.Decision, expected protocol.EndReason) {
	t.Helper()
	if d == nil {
		t.Fatal("decision is nil")
	}
	if d.End == nil {
		t.Fatal("End field is nil — expected an end decision")
	}
	if d.End.Reason != expected {
		t.Errorf("expected end reason %q, got %q", expected, d.End.Reason)
	}
}

// AssertNoError fails the test if err != nil.
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// AssertDecisionValid fails the test if d.Validate() returns an error.
func AssertDecisionValid(t *testing.T, d *protocol.Decision) {
	t.Helper()
	if d == nil {
		t.Fatal("decision is nil")
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("decision validation failed: %v", err)
	}
}
