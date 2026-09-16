package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAShortEnderMarkerIsRefusedNotPanicked — /pending 500's info item, `markEndedBy`'s refusal
// sentence sliced `prior[:12]` and `partyFP[:12]`.
//
// `endedBy` returns whatever the marker file holds, trimmed, and `wasDelivered`'s comment already
// names a planted value as in scope for "suppresses a leg, never authorises one". A planted or
// truncated marker shorter than twelve bytes made the SECOND decline's refusal panic while building
// its own error sentence: on the p2p goroutine, inside `endCeremony`, where a recovered panic reaches
// nobody and the ender-not-recorded notice is never raised.
func TestAShortEnderMarkerIsRefusedNotPanicked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	const id = "dddddddddddddddddddddddddddddddd"
	path, err := endedByPath(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// STIMULUS: the planted marker is what the reader returns, and it is shorter than the slice.
	if got := endedBy(id); got != "abc" {
		t.Fatalf("setup: the planted marker reads %q, want \"abc\"", got)
	}

	var merr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("recording a different ender over a short marker PANICKED (%v) while writing "+
					"its own refusal sentence", r)
			}
		}()
		merr = markEndedBy(id, "7a"+strings.Repeat("8b", 31))
	}()
	if merr == nil && !t.Failed() {
		t.Error("a different ender was recorded over an existing marker; the rule is write-once")
	}
	// And the short PARTY side of the same sentence: a caller handing a short fingerprint.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("a short party fingerprint PANICKED the refusal sentence (%v)", r)
			}
		}()
		_ = markEndedBy(id, "7a")
	}()
}
