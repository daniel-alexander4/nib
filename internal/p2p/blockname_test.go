package p2p

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// P07.S07a: a signature block names the party the record names, in the capacity the record gives
// them, at their position in the proceeding — and not "Nib User", not one neighbour.
//
// # Why every assertion here is over NINE blocks and their DISTINCTNESS
//
// The defect this slice closes is nine identical blocks reading `Signer: Nib User`. Every
// assertion about one block passes against it: the block exists, it is on the right page, its
// rect is right, it carries an appearance, and the string it contains is a perfectly well-formed
// signer name. What fails is the comparison BETWEEN blocks, so that is what these drive.
//
// The same shape one level up is why the plan's own scope calls this "the axis nobody had
// checked": `Party.Label` sat inside the signed commitment with no display reader anywhere, and a
// criterion about naming the ceremony was satisfied by nine blocks that named nobody.

// labelledRoster builds an n-party ceremony roster with distinct labels, and a capacity on the
// parties whose index appears in `capacities`. It returns the roster and the identities in
// signing order, so a caller can contribute as each party in turn.
//
// The commitment is set because `StampCommitment` is a no-op without one — outside a ceremony
// there is no proceeding to name, which is exactly the branch these tests must not accidentally
// be measuring.
func labelledRoster(t *testing.T, n int, capacities map[int]string) (Roster, []l3Party) {
	t.Helper()
	r := Roster{Commitment: strings.Repeat("ab", 32), CommitmentVersion: 4}
	parties := make([]l3Party, 0, n)
	for i := 0; i < n; i++ {
		p := l3Identity(t, fmt.Sprintf("cert-cn-%d", i))
		parties = append(parties, p)
		r.Entries = append(r.Entries, RosterEntry{
			Fingerprint: p.fp,
			Signs:       true,
			Label:       fmt.Sprintf("Party Label %d", i),
			Capacity:    capacities[i],
		})
	}
	return r, parties
}

// TestNineBlocksNameNinePartiesAndNotOneOfThemIsNibUser is the slice's whole point.
func TestNineBlocksNameNinePartiesAndNotOneOfThemIsNibUser(t *testing.T) {
	const n = 9
	r, parties := labelledRoster(t, n, map[int]string{2: "as Director of Acme Ltd"})

	seen := map[string]int{}
	for i, p := range parties {
		// The attestation as a contribution door builds it: the CERTIFICATE's common name, which
		// is what shipped, and then the one door that stamps the ceremony's facts onto it.
		idCert, _, err := sign.ParseIdentity([]byte(p.cert), []byte(p.key))
		if err != nil {
			t.Fatal(err)
		}
		att := Attestation{Signer: idCert.Subject.CommonName, When: time.Now()}
		// SETUP: the constant really is what the door starts from, or "the roster overrode it" is
		// a claim about a value that was never there.
		if att.Signer != fmt.Sprintf("cert-cn-%d", i) {
			t.Fatalf("setup: party %d's certificate common name is %q; this test cannot show the "+
				"roster overriding a value it did not start with", i, att.Signer)
		}
		StampCommitment(&att, r, p.fp)

		if att.Signer != fmt.Sprintf("Party Label %d", i) {
			t.Errorf("party %d's block says %q; the record calls them %q — the label is inside "+
				"the signed commitment and the block is reading something else",
				i, att.Signer, r.Entries[i].Label)
		}
		if att.Position != i+1 || att.RosterSize != n {
			t.Errorf("party %d's block says Party %d of %d, want Party %d of %d",
				i, att.Position, att.RosterSize, i+1, n)
		}
		lines := strings.Join(att.AppearanceLines(), "\n")
		if strings.Contains(lines, "Nib User") {
			t.Errorf("party %d's block still says \"Nib User\":\n%s", i, lines)
		}
		// C09: a ceremony block may not name one neighbour. `Accepts:` is that line.
		if strings.Contains(lines, "Accepts:") {
			t.Errorf("party %d's block names one neighbour inside a ceremony:\n%s", i, lines)
		}
		if !strings.Contains(lines, fmt.Sprintf("Party %d of %d", i+1, n)) {
			t.Errorf("party %d's block does not say which party of how many:\n%s", i, lines)
		}
		// **Not a hex id.** P06 bans hex fingerprints from the primary flow and nothing banned a
		// hex ceremony id from the page a stranger reads. The commitment is 64 hex characters and
		// the block must not carry it.
		if strings.Contains(lines, r.Commitment) {
			t.Errorf("party %d's block names the ceremony by its hex commitment:\n%s", i, lines)
		}
		seen[lines]++
	}

	// **The comparison BETWEEN blocks, which is the one nine identical blocks fail.**
	if len(seen) != n {
		t.Errorf("nine parties produced %d distinct block(s), want %d — blocks that do not "+
			"differ per party are the defect this slice closes, and every check on a single "+
			"block passes against them", len(seen), n)
	}
}

// TestAnUnlabelledPartyFallsBackToItsFingerprintAndNeverToAConstant is the acceptance clause's
// second half — "`Party.Label`, falling back to the fingerprint".
//
// It is a separate test because the fixture that drives the first half cannot drive this one:
// `labelledRoster` labels every party, so the fallback branch is never taken there and a change
// that dropped it would leave every assertion above green.
//
// **The fallback is the fingerprint and not a constant, which is the whole point.** An unlabelled
// party is one the convener did not name; the honest block says which KEY signed rather than
// inventing a person, and inventing a person is precisely what `"Nib User"` did nine times.
func TestAnUnlabelledPartyFallsBackToItsFingerprintAndNeverToAConstant(t *testing.T) {
	r, parties := labelledRoster(t, 3, nil)
	r.Entries[1].Label = "" // the convener named two of three

	att := Attestation{Signer: "Nib User", When: time.Now()}
	StampCommitment(&att, r, parties[1].fp)

	if att.Signer == "Nib User" {
		t.Fatal(`an unlabelled party's block still says "Nib User" — the fallback is the ` +
			"constant, so a ceremony where the convener named nobody is nine identical blocks " +
			"again")
	}
	// The fingerprint, shortened for the eye the way every other fingerprint on a block is.
	if want := shortFingerprint(parties[1].fp); att.Signer != want {
		t.Errorf("an unlabelled party's block says %q, want %q — the fallback names the key that "+
			"signed", att.Signer, want)
	}
	// And the labelled parties either side are unaffected, so the fallback is per party rather
	// than a mode the whole roster falls into.
	for _, i := range []int{0, 2} {
		var other Attestation
		StampCommitment(&other, r, parties[i].fp)
		if other.Signer != fmt.Sprintf("Party Label %d", i) {
			t.Errorf("party %d is labelled %q and its block says %q — one unlabelled party "+
				"changed what the others are called", i, r.Entries[i].Label, other.Signer)
		}
	}
}

// TestACapacityRendersOnlyForThePartyThatHasOne is D20's amendment: capacity is per-party, and an
// empty one renders NOTHING rather than an empty field.
//
// The two halves fail differently on purpose. Reading the capacity off the wrong roster entry
// gives every party party 2's capacity — caught by the second half; rendering an empty capacity as
// a blank line makes a ceremony that needs none look misconfigured — caught by the first.
func TestACapacityRendersOnlyForThePartyThatHasOne(t *testing.T) {
	r, parties := labelledRoster(t, 4, map[int]string{1: "as Guarantor"})
	for i, p := range parties {
		att := Attestation{When: time.Now()}
		StampCommitment(&att, r, p.fp)
		lines := att.AppearanceLines()
		joined := strings.Join(lines, "\n")
		has := strings.Contains(joined, "Capacity:")
		if i == 1 {
			if !has {
				t.Errorf("party 1 signs %q and the block does not say so:\n%s",
					r.Entries[1].Capacity, joined)
			}
			if !strings.Contains(joined, "as Guarantor") {
				t.Errorf("party 1's block carries a Capacity line without their capacity:\n%s", joined)
			}
		} else {
			if has {
				t.Errorf("party %d has no capacity and their block carries a Capacity line "+
					"anyway — a ceremony that needs no capacities must not look "+
					"misconfigured:\n%s", i, joined)
			}
			if strings.Contains(joined, "as Guarantor") {
				t.Errorf("party %d's block carries party 1's capacity — the entry is being read "+
					"by the wrong index:\n%s", i, joined)
			}
		}
	}
}

// TestOutsideACeremonyTheBlockStillNamesItsCounterparty is the control, and it is not decoration:
// every assertion above is about the ceremony branch, and a change that satisfied all of them by
// deleting the two-party appearance would leave an ordinary co-sign naming nobody.
func TestOutsideACeremonyTheBlockStillNamesItsCounterparty(t *testing.T) {
	att := Attestation{
		Signer:            "Nib User",
		AcceptedPeer:      strings.Repeat("cd", 32),
		AcceptedPeerLabel: "Test Counterparty",
		When:              time.Now(),
	}
	StampCommitment(&att, Roster{}, strings.Repeat("ef", 32)) // no roster: a no-op
	joined := strings.Join(att.AppearanceLines(), "\n")
	if !strings.Contains(joined, "Accepts: Test Counterparty") {
		t.Errorf("a two-party co-sign's block no longer names its counterparty:\n%s", joined)
	}
	if strings.Contains(joined, "Party ") {
		t.Errorf("a co-sign with no ceremony claims a position in one:\n%s", joined)
	}
	if att.Position != 0 || att.RosterSize != 0 {
		t.Errorf("no roster produced Position=%d RosterSize=%d, want 0/0 — those two fields are "+
			"what selects the ceremony appearance", att.Position, att.RosterSize)
	}
}

// TestThePartyNameReachesTheSIGNATURE drives it end to end on a real nine-party document: the
// label does not merely reach `AppearanceLines`, it reaches the signature's own common name, which
// is what `Attestations` reports to the panel and to `nib verify`.
//
// `AppearanceLines` is rendered by the CLIENT, so a test that only asserts those lines is
// asserting a string this package hands out. `Attestation.Signer` also becomes `sign.Options.Name`
// inside `Contribute` — the signed identity a reader gets back — and that is the half no client
// can be wrong about.
func TestThePartyNameReachesTheSIGNATURE(t *testing.T) {
	const n = 9
	r, parties := labelledRoster(t, n, nil)
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := PrepareCeremonyDocument(base, CeremonyID{1, 2, 3}, []byte("convener"), n)
	if err != nil {
		t.Fatal(err)
	}
	// SETUP: nothing has signed yet, so every name read back below is one this test produced.
	if got := len(sign.Verify(doc).Signers); got != 0 {
		t.Fatalf("setup: the convened document already carries %d signature(s)", got)
	}

	for i, p := range parties {
		place, err := PlacementFor(doc, r, p.fp)
		if err != nil {
			t.Fatalf("party %d has no placement: %v", i, err)
		}
		idCert, _, err := sign.ParseIdentity([]byte(p.cert), []byte(p.key))
		if err != nil {
			t.Fatal(err)
		}
		att := Attestation{
			Signer:       idCert.Subject.CommonName,
			AcceptedPeer: PredecessorOf(r, p.fp),
			When:         time.Now(),
		}
		StampCommitment(&att, r, p.fp)
		doc, err = Contribute(doc, []byte(p.cert), []byte(p.key), att, nil, place)
		if err != nil {
			t.Fatalf("party %d could not contribute: %v", i, err)
		}
	}

	st := sign.Verify(doc)
	if len(st.Signers) != n {
		t.Fatalf("the finished document carries %d signature(s), want %d", len(st.Signers), n)
	}
	names := map[string]int{}
	for i, s := range st.Signers {
		names[s.Name]++
		if s.Name == "Nib User" {
			t.Errorf("signature %d reports its signer as \"Nib User\" — the constant reached the "+
				"document", i)
		}
		if !strings.HasPrefix(s.Name, "Party Label ") {
			t.Errorf("signature %d reports its signer as %q; the record labels every party "+
				"\"Party Label <n>\"", i, s.Name)
		}
	}
	if len(names) != n {
		t.Errorf("the finished document's %d signatures report %d distinct signer name(s): %v — "+
			"a document on which every party is called the same thing is what this closes",
			n, len(names), names)
	}
	// The document is still a document: the blocks did not cost it its pages.
	if _, err := pdfops.PageCount(doc); err != nil {
		t.Errorf("the nine-party document no longer parses: %v", err)
	}
}
