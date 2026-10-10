package server

import (
	"encoding/json"
	"strings"
	"testing"

	"nib/internal/p2p"
)

// TestEachAttestationSaysWhetherItCommitsToThisCeremony — /pending 826.
//
// `oneProceeding` is a verdict on the whole document: one signature this build cannot read turns
// it false on every row, so the window had nothing to tell the readable signatures apart by and
// said nothing about them at all. The route now publishes the per-signature fact.
func TestEachAttestationSaysWhetherItCommitsToThisCeremony(t *testing.T) {
	record := strings.Repeat("c0", 32)
	proc := p2p.Proceeding{Commitment: record}
	for _, c := range []struct {
		name string
		a    p2p.SignerAttestation
		want bool
	}{
		{"a valid signature naming this record", p2p.SignerAttestation{Valid: true, RosterHash: strings.ToUpper(record)}, true},
		{"a plain signature naming no ceremony", p2p.SignerAttestation{Valid: true}, false},
		{"a signature naming another ceremony", p2p.SignerAttestation{Valid: true, RosterHash: strings.Repeat("dd", 32)}, false},
		{"a signature that does not verify", p2p.SignerAttestation{RosterHash: record}, false},
		{"a newer tag this build did not read", p2p.SignerAttestation{Valid: true, TagVersion: 99}, false},
	} {
		view := attestationViewOf(nil, c.a, proc)
		if view.Commits != c.want {
			t.Errorf("%s: commits = %v, want %v", c.name, view.Commits, c.want)
		}
		// What the window reads is the JSON, so the name is part of the contract.
		raw, err := json.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(string(raw), `"commits":true`); got != c.want {
			t.Errorf("%s: the answer carries commits=true: %v, want %v — %s", c.name, got, c.want, raw)
		}
	}
	// With no record to commit to, nothing commits: a signer's own token is not evidence that the
	// ceremony it names exists.
	if attestationViewOf(nil, p2p.SignerAttestation{Valid: true, RosterHash: record}, p2p.Proceeding{}).Commits {
		t.Error("a signature was reported as committing to a ceremony record the document does not carry")
	}
}
