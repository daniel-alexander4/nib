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

// TestTheCeremonyVerdictRefusesWhatItUsedToCallComplete — /pending 324.
//
// # Two defects, and the trap between them
//
// `Completeness` iterates the ROSTER and breaks on first match, so `signed <= obliged` always: an
// extra signature carrying a copy of the document's roster token was invisible to it, and the
// document read `valid (4 signer(s))` / `3 of 3` / `✓ Complete` / `"complete":true`, **exit 0**.
// Separately, `oneProceeding` was not in the exit condition at all, so a document whose signatures
// named different ceremonies printed exactly that and also exited 0.
//
// **The trap is the naive fix.** Adding a bare `!oneProc` to the exit condition was measured
// exiting 2 on a document whose only fault is that a counterparty updated Nib — the CLI never had
// the web's two D32 skew discriminators. That would break `nib verify x.pdf && echo ok`, the
// README's own idiom, over something no party did wrong. The skew rows below are why `disagrees()`
// carries a `skew == ""` clause, and they must stay exit-0 for this change to be worth shipping.
func TestTheCeremonyVerdictRefusesWhatItUsedToCallComplete(t *testing.T) {
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	t.Run("a complete ceremony still passes", func(t *testing.T) {
		dir := t.TempDir()
		path := convenedFixture(t, dir, 3, 3)
		cer := reportOf(t, path, now)
		if cer.refuses() {
			t.Fatalf("a complete, agreeing ceremony was refused (skew=%q unrostered=%v one=%v)",
				cer.skew, cer.unrostered, cer.oneProc)
		}
	})

	t.Run("an off-roster signature claiming the ceremony refuses", func(t *testing.T) {
		dir := t.TempDir()
		path := convenedFixture(t, dir, 3, 3)
		doc, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		// SETUP: it passes before the intruder is added, or the refusal below is about the fixture.
		if reportOf(t, path, now).refuses() {
			t.Fatal("setup: the base fixture already refuses")
		}
		rec, err := ceremony.CheckRecord(doc, now)
		if err != nil {
			t.Fatal(err)
		}
		rh, err := rec.RosterHash()
		if err != nil {
			t.Fatal(err)
		}
		// A stranger copies the document's own roster token — which is what defeats
		// `markOneProceeding`, and therefore the only case that needed a new check.
		sc, sk, err := sign.GenerateIdentity("Mallory")
		if err != nil {
			t.Fatal(err)
		}
		att := p2p.Attestation{Signer: "Mallory", When: now}
		att.RosterHash = hex.EncodeToString(rh)
		att.RosterVersion = ceremony.FormatVersion
		att.Intent = rec.Intent
		out, err := p2p.Contribute(doc, sc, sk, att, nil, p2p.Placement{})
		if err != nil {
			t.Fatal(err)
		}
		bad := filepath.Join(dir, "intruder.pdf")
		if err := os.WriteFile(bad, out, 0o600); err != nil {
			t.Fatal(err)
		}
		cer := reportOf(t, bad, now)
		if !cer.hasUnrostered() {
			t.Errorf("a signature claiming this ceremony from an identity the roster does not "+
				"name was not flagged. Completeness counts the OBLIGED signers and can never "+
				"exceed the roster, so the document still reads %d of %d — complete — while "+
				"carrying a signature nobody agreed to.", cer.signed, cer.obliged)
		}
		if !cer.refuses() {
			t.Error("…and it exited 0, under the README's own `nib verify x.pdf && echo intact`")
		}
		if j := cer.json(); j != nil && j.Complete {
			t.Error("the JSON still reports complete:true — the machine-readable channel " +
				"disagreeing with the human one is the divergence this condition exists to close")
		}
	})

	t.Run("signatures naming two ceremonies refuse", func(t *testing.T) {
		// Defect 2 driven at the artifact. **Both signers are ON the roster**, and that is the
		// whole point: the first cut of this arm used a stranger, so `refuses()` fired through
		// `hasUnrostered()` and dropping `disagrees()` from the door replayed GREEN. An arm that
		// two predicates can satisfy cannot tell you which one is missing.
		dir := t.TempDir()
		path, ps, rec := twoPartyCeremony(t, dir, now)
		_ = path
		doc := rec.doc

		// Party 1 signs, naming a DIFFERENT proceeding than the document carries.
		att := p2p.Attestation{Signer: "Bob Landlord", AcceptedPeer: ps[0].fp, When: now}
		att.RosterHash = strings.Repeat("ab", 32)
		att.RosterVersion = ceremony.FormatVersion
		att.Intent = rec.intent
		out, err := p2p.Contribute(doc, ps[1].cert, ps[1].key, att, nil, p2p.Placement{})
		if err != nil {
			t.Fatal(err)
		}
		bad := filepath.Join(dir, "twoceremonies.pdf")
		if err := os.WriteFile(bad, out, 0o600); err != nil {
			t.Fatal(err)
		}
		cer := reportOf(t, bad, now)
		// SETUP: a disagreement, not a skew, and NOT an unrostered signature — or this arm proves
		// a different clause than the one it names.
		if cer.skew != "" {
			t.Fatalf("setup: the fixture reads as a version skew (%q), not a disagreement", cer.skew)
		}
		if cer.hasUnrostered() {
			t.Fatalf("setup: a signer is off-roster (%v), so refuses() could fire without "+
				"disagrees() and this arm would not discriminate", cer.unrostered)
		}
		if !cer.disagrees() {
			t.Errorf("signatures naming two different ceremonies were not reported as disagreeing "+
				"(claimed=%d one=%v)", cer.claimed, cer.oneProc)
		}
		if !cer.refuses() {
			t.Error("…and it exited 0, which is defect 2: the text output said so and the exit " +
				"code did not, under the README's own `nib verify x.pdf && echo intact`")
		}
	})

	t.Run("a version skew must NOT refuse", func(t *testing.T) {
		// The anti-proof. `oneProc` is false here and every party agreed; only the skew clause
		// separates this from the row above.
		c := ceremonyReport{present: true, claimed: 2, oneProc: false,
			skew: "one or more signatures were written by a newer version of Nib"}
		if c.disagrees() {
			t.Error("a version skew was reported as a disagreement. The CLI never had the web's " +
				"D32 discriminators, so the naive fix exits 2 on a document whose counterparty " +
				"merely updated Nib — measured, and worse than the defect it closes.")
		}
		if c.refuses() {
			t.Error("…and it would have exited 2")
		}
	})

	t.Run("a convened document with no signatures says nothing about agreement", func(t *testing.T) {
		dir := t.TempDir()
		path := convenedFixture(t, dir, 3, 0)
		cer := reportOf(t, path, now)
		if cer.disagrees() {
			t.Error("a document with zero signatures was said to disagree — oneProc is false " +
				"because nothing claims the ceremony, which is not a disagreement")
		}
		for _, l := range cer.lines() {
			if strings.Contains(l, "do NOT all commit") {
				t.Errorf("it printed a disagreement over no signatures: %q", l)
			}
		}
	})
}

// reportOf builds the ceremony report the CLI would print for a file.
func reportOf(t *testing.T, path string, now time.Time) ceremonyReport {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return ceremonyReportOf(b, sign.Verify(b), now)
}

type ceremonyParty324 struct {
	cert, key []byte
	fp        string
}

type ceremonyDoc324 struct {
	doc    []byte
	intent string
}

// twoPartyCeremony convenes a two-party ceremony and has party 0 sign it honestly, returning both
// parties' keys so a caller can make party 1 sign DISHONESTLY while still being on the roster.
// `convenedFixture` cannot serve this: it does not expose the identities it generates.
func twoPartyCeremony(t *testing.T, dir string, now time.Time) (string, []ceremonyParty324, ceremonyDoc324) {
	t.Helper()
	var ps []ceremonyParty324
	var roster []ceremony.Party
	for _, name := range []string{"Alice Tenant", "Bob Landlord"} {
		c, k, err := sign.GenerateIdentity("p")
		if err != nil {
			t.Fatal(err)
		}
		f, err := sign.Fingerprint(c)
		if err != nil {
			t.Fatal(err)
		}
		fp := hex.EncodeToString(f)
		ps = append(ps, ceremonyParty324{c, k, fp})
		roster = append(roster, ceremony.Party{Fingerprint: fp, Label: name, Signs: true})
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
			Fingerprint: ps[i].fp, Signs: true, Label: roster[i].Label,
		})
	}
	place, perr := p2p.PlacementFor(got.Document, pr, ps[0].fp)
	if perr != nil {
		t.Fatal(perr)
	}
	att := p2p.Attestation{AcceptedPeer: p2p.PredecessorOf(pr, ps[0].fp), When: now}
	p2p.StampCommitment(&att, pr, ps[0].fp)
	doc, err := p2p.Contribute(got.Document, ps[0].cert, ps[0].key, att, nil, place)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "twoparty.pdf")
	if err := os.WriteFile(path, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, ps, ceremonyDoc324{doc: doc, intent: got.Record.Intent}
}
