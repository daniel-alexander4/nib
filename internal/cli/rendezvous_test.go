package cli

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"nib/internal/rendezvous"
	"os"
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
			// Exit 1, changed at v1.116.12. The diagnostic could not establish the fact it
			// was run to establish, and 0 reported that to a script as a pass. The
			// self-test arm already treats the same shape as non-zero, in terms: "the mode
			// whose entire purpose is to prove the publish path works must be able to say
			// that it didn't". This row had pinned the old code.
			name: "reachable, nothing observed", st: rendezvous.Stats{Nodes: 20, Observed: 0},
			wantCode: 1, want: "nothing reported our address back",
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

// The banner is the disclosure surface, and it had no guard.
//
// Twice in one arc a prose claim about network behaviour went stale in this repo: the
// README said Nib made only one call on its own, and then said this command published
// nothing on the day --self-test started publishing. Prose rots exactly like a doc comment.
func TestTheBannerSaysWhetherItPublishes(t *testing.T) {
	plain := banner(false)
	if !strings.Contains(plain, "nothing is published here") {
		t.Errorf("the plain banner does not say it publishes nothing:\n%s", plain)
	}
	if strings.Contains(plain, "PUBLISHES") {
		t.Errorf("the plain banner claims to publish:\n%s", plain)
	}

	self := banner(true)
	if !strings.Contains(self, "PUBLISHES") {
		t.Errorf("the --self-test banner does not say it publishes:\n%s", self)
	}
	if strings.Contains(self, "nothing is published here") {
		t.Errorf("the --self-test banner still claims it publishes nothing — this is the "+
			"exact staleness that has already happened twice:\n%s", self)
	}
	// The irreversibility, which is the part a user cannot get back.
	for _, must := range []string{"no recall", "Ctrl-C"} {
		if !strings.Contains(self, must) {
			t.Errorf("the --self-test banner omits %q — it is the branch with the "+
				"irreversible side effect, so it is the one that needs both:\n%s", must, self)
		}
	}
}

// The banner must be printed before anything opens a socket or publishes.
//
// **This test reaches the public BitTorrent DHT, and until v1.116.8 it did so in tier 1.**
// Its own comment claimed "a budget too small to do anything", and the arithmetic refutes
// that: `probeShare = max(budget/3, 8s)` is 8 s and `bootShare = max(budget-8s, 1s)` is
// 1 s, so `time.Millisecond` buys a nine-second live run. `go test ./...` is the tier
// CONTRIBUTING says a fresh clone runs unaided, so a contributor's first test run performed
// the exact act `banner()` exists to disclose and invite Ctrl-C on. It measured
// sub-second only on machines with no route out, which is also why it stayed unnoticed.
//
// Gated on `-short` being absent AND the same opt-in the live DHT tests use, so the
// property (ordering of the output) is still asserted hermetically below while the network
// half is opt-in. See build/dhtlive.sh.
func TestTheBannerPrecedesTheSocket(t *testing.T) {
	if os.Getenv("NIB_LIVE_DHT") == "" {
		// The hermetic half: banner() alone, which is what the assertions are about.
		got := banner(false)
		if !strings.HasPrefix(got, "nib rendezvous diagnostic") {
			t.Fatalf("the first thing printed was not the disclosure:\n%.200s", got)
		}
		if i := strings.Index(got, "local socket"); i >= 0 && i < strings.Index(got, "Ctrl-C") {
			t.Error("the socket line precedes the Ctrl-C invitation")
		}
		return
	}
	var buf strings.Builder
	runRendezvous(&buf, io.Discard, time.Millisecond, false)
	got := buf.String()
	if !strings.HasPrefix(got, "nib rendezvous diagnostic") {
		t.Fatalf("the first thing printed was not the disclosure:\n%.200s", got)
	}
	if i := strings.Index(got, "local socket"); i >= 0 && i < strings.Index(got, "Ctrl-C") {
		t.Error("the socket line precedes the Ctrl-C invitation — the disclosure must " +
			"come before anything opens")
	}
}

// The eclipse note is the disclosure half of the seed work, and it had no test.
func TestTheInvitationSeedNoteSaysWhichWayItWent(t *testing.T) {
	if got := invitationSeedNote(rendezvous.Stats{}); got != "" {
		t.Errorf("no seeds supplied and it said %q", got)
	}
	unused := invitationSeedNote(rendezvous.Stats{InvitationSeeds: 3})
	if !strings.Contains(unused, "unused") {
		t.Errorf("seeds supplied and not needed, but the note does not say so: %q", unused)
	}
	used := invitationSeedNote(rendezvous.Stats{InvitationSeeds: 3, InvitationSeedsUsed: true})
	if !strings.Contains(used, "sender chose") {
		t.Errorf("the table came from the invitation's addresses and the note does not say "+
			"so — that is the eclipse fact the acceptance asks to be readable: %q", used)
	}
	if strings.Contains(unused, "sender chose") {
		t.Error("unused seeds are reported as though the table came from them")
	}
}

// And the verdict must not blame the shipped list for a table built from an invitation.
func TestTheVerdictDoesNotBlameTheShippedListForInvitationSeeds(t *testing.T) {
	shipped, _ := verdict(rendezvous.Stats{Nodes: 0, Seeds: 5, Responses: 2},
		rendezvous.SelfAddress{}, nil, nil, false)
	if !strings.Contains(shipped, "shipped seed addresses") {
		t.Errorf("a shipped-list failure is not named as one: %q", shipped)
	}
	viaInv, _ := verdict(rendezvous.Stats{Nodes: 0, Seeds: 5, Responses: 2, InvitationSeedsUsed: true},
		rendezvous.SelfAddress{}, nil, nil, false)
	if strings.Contains(viaInv, "shipped seed addresses answered") {
		t.Errorf("the invitation's addresses were tried too, and the verdict sends the user "+
			"to fix the shipped list: %q", viaInv)
	}
}

// TestTheNoteDistinguishesTriedFromUnused.
//
// The two-branch note collapsed three states into two: the default arm said "the shipped
// list worked" about a machine where nothing worked at all. Tried-and-failed is the state
// an operator most needs named, because it is the only one where the answer is neither
// "fine" nor "your ceremony partner chose your view of the DHT".
func TestTheNoteDistinguishesTriedFromUnused(t *testing.T) {
	unused := invitationSeedNote(rendezvous.Stats{InvitationSeeds: 3})
	tried := invitationSeedNote(rendezvous.Stats{InvitationSeeds: 3, InvitationSeedsTried: true})
	used := invitationSeedNote(rendezvous.Stats{
		InvitationSeeds: 3, InvitationSeedsTried: true, InvitationSeedsUsed: true,
	})

	if unused == tried || tried == used || unused == used {
		t.Fatalf("two of the three states print the same line:\nunused=%q\ntried=%q\nused=%q",
			unused, tried, used)
	}
	if !strings.Contains(unused, "the shipped list worked") {
		t.Errorf("the unused note stopped saying the shipped list worked: %q", unused)
	}
	if strings.Contains(tried, "worked") {
		t.Errorf("the tried-and-failed note claims something worked: %q", tried)
	}
	if !strings.Contains(used, "sender chose") {
		t.Errorf("the used note stopped naming whose list built the table: %q", used)
	}
}

// TestTheProbeVerdictCannotReportSuccessWithoutObservingAnything.
//
// The `Observed == 0` arm — "the DHT is reachable but nothing reported our address back" —
// returned exit **0**. That is the diagnostic failing to establish the fact it was run to
// establish, reported as a pass to any script or pasted `echo $?`. The self-test arm one
// screen above already treats the same shape as non-zero, with the comment "the mode whose
// entire purpose is to prove the publish path works must be able to say that it didn't".
func TestTheProbeVerdictCannotReportSuccessWithoutObservingAnything(t *testing.T) {
	// STIMULUS/control: a run that DID observe must still exit 0, or this is just a
	// diagnostic that always fails.
	good := rendezvous.Stats{Nodes: 40, Observed: 16, Responses: 40}
	if _, code := verdict(good, rendezvous.SelfAddress{
		V4: rendezvous.Class{Mapping: rendezvous.MappingEndpointIndependent, Agreed: 16, Sources: 16},
	}, nil, nil, false); code != 0 {
		t.Fatalf("a healthy probe exited %d, want 0 — the control does not hold", code)
	}

	bad := rendezvous.Stats{Nodes: 40, Observed: 0, Responses: 40}
	msg, code := verdict(bad, rendezvous.SelfAddress{}, nil, nil, false)
	if code == 0 {
		t.Errorf("a probe that observed NOTHING exited 0:\n%s", msg)
	}
}

// TestTheDisclosureSaysHowMuchOfTheTableCameFromTheStranger pins InvitationBootstrapped's
// only reader. The counter was maintained, subtracted from `bootstrapped`, and carried in an
// exported struct that nothing but a test ever read — so the fact it exists to state was
// computed and then dropped.
func TestTheDisclosureSaysHowMuchOfTheTableCameFromTheStranger(t *testing.T) {
	note := invitationSeedNote(rendezvous.Stats{
		InvitationSeeds: 3, InvitationSeedsUsed: true, InvitationBootstrapped: 17,
	})
	if !strings.Contains(note, "17") {
		t.Errorf("the disclosure does not say how many nodes came from the invitation:\n%s", note)
	}
	// And the figure belongs ONLY to the branch that earned it. The untried branch says
	// "unused (the shipped list worked)" — printing a contributed-node count there would
	// state a contribution that by definition did not happen.
	if n := invitationSeedNote(rendezvous.Stats{InvitationSeeds: 3, InvitationBootstrapped: 17}); strings.Contains(n, "17") {
		t.Errorf("the untried branch printed a contribution count:\n%s", n)
	}
}
