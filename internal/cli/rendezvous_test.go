package cli

import (
	"errors"
	"strings"
	"testing"

	"nib/internal/rendezvous"
)

// The verdict's branches, table-tested.
//
// Every finding the slice's review raised about this logic was reachable only by
// constructing a Stats value, which the command itself made impossible — the verdict was
// buried inside a function that opens a socket and talks to the internet. Splitting it out
// is what makes these cases assertable at all.
func TestTheVerdictNeverContradictsTheReportAboveIt(t *testing.T) {
	indep := rendezvous.Class{
		Mapping: rendezvous.MappingEndpointIndependent,
		Agreed:  16, Sources: 16,
	}
	dependent := rendezvous.Class{Mapping: rendezvous.MappingEndpointDependent, Agreed: 1, Sources: 16}

	cases := []struct {
		name     string
		st       rendezvous.Stats
		self     rendezvous.SelfAddress
		aborted  bool
		wantCode int
		want     string
		notWant  string
	}{
		{
			name: "no routing table, nothing replied", st: rendezvous.Stats{Nodes: 0, Seeds: 5},
			wantCode: 1, want: "outbound UDP is blocked",
		},
		{
			// Measured on a real run: an empty table while replies WERE arriving. The
			// verdict used to name a blocked network as a cause when the evidence in its
			// own report ruled that out.
			name:     "no routing table, but replies reached us",
			st:       rendezvous.Stats{Nodes: 0, Seeds: 5, Responses: 2},
			wantCode: 1, want: "not blocking UDP", notWant: "outbound UDP is blocked",
		},
		{
			name: "reachable, nothing observed", st: rendezvous.Stats{Nodes: 20, Observed: 0},
			wantCode: 0, want: "nothing reported our address back",
		},
		{
			// The defect: Observed counts USABLE REPLIES, not agreement. Under a symmetric
			// NAT every responder reports a different address, so Observed is high and no
			// majority forms — and the verdict used to announce "16 nodes agreed" directly
			// beneath a line reading "1 of 16 sources agreed".
			name:     "symmetric NAT — replies but no agreement",
			st:       rendezvous.Stats{Nodes: 20, Observed: 16},
			self:     rendezvous.SelfAddress{V4: dependent},
			wantCode: 0, want: "did not agree", notWant: "16 of 16",
		},
		{
			name: "agreed", st: rendezvous.Stats{Nodes: 20, Observed: 16},
			self:     rendezvous.SelfAddress{V4: indep},
			wantCode: 0, want: "16 of 16 independent sources agreed",
		},
		{
			name: "interrupted", st: rendezvous.Stats{Nodes: 20, Observed: 16}, aborted: true,
			wantCode: 1, want: "interrupted",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			line, code := verdict(c.st, c.self, nil, nil, c.aborted)
			if code != c.wantCode {
				t.Errorf("exit code %d, want %d — %q", code, c.wantCode, line)
			}
			if !strings.Contains(line, c.want) {
				t.Errorf("verdict %q does not mention %q", line, c.want)
			}
			if c.notWant != "" && strings.Contains(line, c.notWant) {
				t.Errorf("verdict %q claims %q, which contradicts the classification above it",
					line, c.notWant)
			}
		})
	}
}

// A bootstrap error must reach the verdict, not just the transcript above it.
func TestTheVerdictNamesTheBootstrapFailure(t *testing.T) {
	line, code := verdict(rendezvous.Stats{Nodes: 0, Seeds: 5}, rendezvous.SelfAddress{},
		errors.New("context deadline exceeded"), nil, false)
	if code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	if !strings.Contains(line, "deadline") {
		t.Errorf("the verdict blames the network without mentioning that the bootstrap ran "+
			"out of time: %q", line)
	}
}

// classLine must never print Class.Addr for a mapping where it is deliberately unset.
func TestClassLineNeverPrintsAnUnsetAddress(t *testing.T) {
	for _, m := range []rendezvous.Mapping{
		rendezvous.MappingUnknown, rendezvous.MappingEndpointDependent,
	} {
		got := classLine(rendezvous.Class{Mapping: m, Agreed: 1, Sources: 3})
		if strings.Contains(got, "invalid AddrPort") {
			t.Errorf("classLine(%v) = %q — Addr is unset for this mapping and printing it "+
				"yields literal garbage on exactly the NAT class where remote co-signing is "+
				"hardest", m, got)
		}
	}
}
