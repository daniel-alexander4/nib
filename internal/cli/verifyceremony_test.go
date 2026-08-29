package cli

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// P07.S10: `nib verify` reports the ceremony.
//
// # The sentence the plan wrote it against
//
// "The CLI is the surface a dispute actually uses." A stranger handed a signed PDF and told to
// check it with Nib ran `nib verify` and got `valid (9 signer(s))` — true, and silent on the only
// questions a multi-party instrument raises: who was supposed to sign, whether they did, and
// whether this is one proceeding or several documents wearing one roster.
//
// # The fixture is CONVENED, and the first one was not
//
// The first drive of this used `PrepareCeremonyDocument`, which allocates the signature pages and
// embeds no record — that is `Convene`'s job. The document looked right, carried nine parties'
// worth of pages, and `nib verify` reported nothing at all, because there was no record to read.
// The unit tests would have passed against it. Live verification is what caught it, which is the
// step's whole argument.

// convenedFixture writes a real convened ceremony of n parties with `signedBy` of them signing.
func convenedFixture(t *testing.T, dir string, n, signedBy int) string {
	t.Helper()
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	type party struct {
		cert, key []byte
		fp        string
	}
	names := []string{"Alice Tenant", "Bob Landlord", "Carol Guarantor", "Dan Witness",
		"Erin Trustee", "Frank Director", "Grace Attorney", "Heidi Executor", "Ivan Surveyor"}
	if n > len(names) {
		t.Fatalf("fixture supports at most %d parties", len(names))
	}
	var ps []party
	var roster []ceremony.Party
	for i := 0; i < n; i++ {
		c, k, err := sign.GenerateIdentity("p")
		if err != nil {
			t.Fatal(err)
		}
		f, err := sign.Fingerprint(c)
		if err != nil {
			t.Fatal(err)
		}
		fp := hex.EncodeToString(f)
		ps = append(ps, party{c, k, fp})
		roster = append(roster, ceremony.Party{Fingerprint: fp, Label: names[i], Signs: true})
	}
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ceremony.Convene(base, ceremony.ConveneRequest{
		Intent:        "We agree to be bound by the lease of 14 Elm Row",
		Expires:       now.Add(96 * time.Hour),
		HopBudget:     30 * time.Minute,
		ConvenerSigns: true,
		Roster:        roster,
	}, ps[0].cert, ps[0].key, now)
	if err != nil {
		t.Fatal(err)
	}
	doc := got.Document

	rh, err := got.Record.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	pr := p2p.Roster{
		Commitment:        hex.EncodeToString(rh),
		CommitmentVersion: ceremony.FormatVersion,
		Intent:            got.Record.Intent,
	}
	for i := range ps {
		pr.Entries = append(pr.Entries, p2p.RosterEntry{
			Fingerprint: ps[i].fp, Signs: true, Label: names[i],
		})
	}
	for i := 0; i < signedBy; i++ {
		place, perr := p2p.PlacementFor(doc, pr, ps[i].fp)
		if perr != nil {
			t.Fatal(perr)
		}
		att := p2p.Attestation{AcceptedPeer: p2p.PredecessorOf(pr, ps[i].fp), When: now}
		p2p.StampCommitment(&att, pr, ps[i].fp)
		doc, err = p2p.Contribute(doc, ps[i].cert, ps[i].key, att, nil, place)
		if err != nil {
			t.Fatalf("party %d could not contribute: %v", i, err)
		}
	}
	// SETUP: the document really carries the signatures this fixture claims, or every assertion
	// about "5 of 9" is about a file that was never signed.
	if got := len(sign.Verify(doc).Signers); got != signedBy {
		t.Fatalf("setup: the fixture carries %d signature(s), want %d", got, signedBy)
	}
	path := filepath.Join(dir, "ceremony.pdf")
	if err := os.WriteFile(path, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVerifyNamesWhoHasNotSignedAndExitsNonZero(t *testing.T) {
	path := convenedFixture(t, t.TempDir(), 9, 5)

	out, code := captureStdout(t, func() int { return cmdVerify([]string{path}) })

	// STIMULUS before response: the ordinary signature line is still there, so the ceremony lines
	// below are an addition rather than a replacement.
	if !strings.Contains(out, "valid (5 signer(s))") {
		t.Fatalf("the signature line is gone or wrong:\n%s", out)
	}

	if !strings.Contains(out, "5 of 9 obliged signer(s) have signed") {
		t.Errorf("`nib verify` does not say how many of the roster have signed. A stranger told "+
			"to check a nine-party deed with Nib gets `valid (5 signer(s))`, which is true and "+
			"says nothing about the four who never signed:\n%s", out)
	}
	if !strings.Contains(out, "INCOMPLETE") {
		t.Errorf("an unfinished ceremony is not called unfinished:\n%s", out)
	}
	// NAMED, not counted. "4 have not signed" leaves a reader to work out which four from a
	// roster they may not have.
	for _, who := range []string{"Frank Director", "Grace Attorney", "Heidi Executor", "Ivan Surveyor"} {
		if !strings.Contains(out, who) {
			t.Errorf("the report does not name %q, who was obliged to sign and did not:\n%s", who, out)
		}
	}
	if !strings.Contains(out, "Alice Tenant") {
		t.Errorf("the report does not name the parties who DID sign, so a reader cannot tell "+
			"whose signatures they have:\n%s", out)
	}
	// The recital, which is what the parties actually agreed to.
	if !strings.Contains(out, "lease of 14 Elm Row") {
		t.Errorf("the report does not carry the ceremony's recital:\n%s", out)
	}

	// **Exit 2.** The README ships `nib verify contract.pdf && echo "signature intact"`, and a
	// nine-party deed four obliged parties never signed must not pass that. Same divergence
	// `AddedAfter` was added to this condition to close.
	if code != 2 {
		t.Errorf("`nib verify` exited %d on a ceremony four obliged parties never signed. Every "+
			"signature on it is valid, so a script gets 'fine' about a document the human report "+
			"calls INCOMPLETE — the machine-readable channel disagreeing with the human one, "+
			"which is the exact reason AddedAfter joined this condition", code)
	}
}

func TestVerifyOnACompleteCeremonySaysSoAndExitsZero(t *testing.T) {
	path := convenedFixture(t, t.TempDir(), 4, 4)

	out, code := captureStdout(t, func() int { return cmdVerify([]string{path}) })

	if !strings.Contains(out, "4 of 4 obliged signer(s) have signed") {
		t.Errorf("a complete ceremony is not reported as complete:\n%s", out)
	}
	if strings.Contains(out, "INCOMPLETE") {
		t.Errorf("a fully signed ceremony was called incomplete:\n%s", out)
	}
	if !strings.Contains(out, "every signature commits to this document's ceremony") {
		t.Errorf("the one-proceeding verdict is missing:\n%s", out)
	}
	if code != 0 {
		t.Errorf("a complete, valid ceremony exited %d — a verifier that refuses a good "+
			"document is worse than one that says too little", code)
	}
}

// TestVerifyOnAnOrdinaryDocumentSaysNothingAboutCeremonies is the control, and it is the one a
// naive fix breaks. Most documents Nib signs belong to no ceremony; describing them as a ceremony
// of nobody — "0 of 0 obliged signers" — would be a verdict on a proceeding that does not exist,
// and it would appear on every ordinary co-sign in the product.
func TestVerifyOnAnOrdinaryDocumentSaysNothingAboutCeremonies(t *testing.T) {
	dir := t.TempDir()
	base, err := testpdf.Text("a quotation")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := p2p.PrepareDocument(base)
	if err != nil {
		t.Fatal(err)
	}
	cert, key, err := sign.GenerateIdentity("Solo")
	if err != nil {
		t.Fatal(err)
	}
	place, err := p2p.NextPlacement(prepared)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := p2p.Contribute(prepared, cert, key, p2p.Attestation{When: time.Now()}, nil, place)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "quote.pdf")
	if err := os.WriteFile(path, signed, 0o600); err != nil {
		t.Fatal(err)
	}

	out, code := captureStdout(t, func() int { return cmdVerify([]string{path}) })

	if !strings.Contains(out, "valid (1 signer(s))") {
		t.Fatalf("setup: the ordinary co-signed document did not verify:\n%s", out)
	}
	if strings.Contains(out, "ceremony") || strings.Contains(out, "obliged") {
		t.Errorf("a document with no ceremony was described as having one:\n%s\n\nEvery ordinary "+
			"co-sign in the product would carry this line, and '0 of 0 obliged signers' is a "+
			"verdict on a proceeding that does not exist", out)
	}
	if code != 0 {
		t.Errorf("an ordinary co-signed document exited %d", code)
	}
}

func TestVerifyJSONCarriesTheCeremony(t *testing.T) {
	path := convenedFixture(t, t.TempDir(), 9, 5)

	out, _ := captureStdout(t, func() int { return cmdVerify([]string{"--json", path}) })

	var got struct {
		Ceremony *struct {
			Obliged       int      `json:"obliged"`
			Signed        int      `json:"signed"`
			Complete      bool     `json:"complete"`
			OneProceeding bool     `json:"oneProceeding"`
			Missing       []string `json:"missing"`
			Intent        string   `json:"intent"`
		} `json:"ceremony"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
		t.Fatalf("--json did not emit one JSON object: %v\n%s", err, out)
	}
	if got.Ceremony == nil {
		t.Fatal("--json carries no ceremony object, so a script has to parse the sentence")
	}
	if got.Ceremony.Obliged != 9 || got.Ceremony.Signed != 5 {
		t.Errorf("--json reports %d of %d, want 5 of 9", got.Ceremony.Signed, got.Ceremony.Obliged)
	}
	if got.Ceremony.Complete {
		t.Error("--json calls a five-of-nine ceremony complete")
	}
	if !got.Ceremony.OneProceeding {
		t.Error("--json says the signatures do not commit to one ceremony, on a document where they do")
	}
	if len(got.Ceremony.Missing) != 4 {
		t.Errorf("--json lists %d missing signers, want 4 — a script should not have to derive "+
			"the list by subtracting", len(got.Ceremony.Missing))
	}
	if got.Ceremony.Intent == "" {
		t.Error("--json carries no recital")
	}
}
