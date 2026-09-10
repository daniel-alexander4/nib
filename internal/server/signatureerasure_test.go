package server

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// signedFixture is a real four-page document with a real approval signature on it.
func signedFixture(t *testing.T) []byte {
	t.Helper()
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		other, terr := testpdf.Text("another page")
		if terr != nil {
			t.Fatal(terr)
		}
		if base, err = pdfops.Append(base, other); err != nil {
			t.Fatal(err)
		}
	}
	cert, key, gerr := sign.GenerateIdentity("Signer")
	if gerr != nil {
		t.Fatal(gerr)
	}
	signed, serr := sign.SignApproval(base, cert, key, sign.Options{Name: "A", Reason: "r"})
	if serr != nil {
		t.Fatal(serr)
	}
	return signed
}

// TestACommitThatErasesASignatureIsRefusedUnlessAccepted — /pending 455.
//
// The defect: which pdfcpu primitive a route happened to reach for decided whether an edited signed
// document came out `invalid` — a broken signature a reader can point at — or `unsigned`, carrying
// no evidence it was ever signed at all. Nothing on the server refused either way; the only guard
// was a browser `confirm`, which the CLI and the harness bypass entirely.
//
// **Driven through the doors rather than through a route, deliberately.** `commitMutation` and
// `commitBarrier` are where the rule lives, and there are ten call sites between them. A test
// pointed at one route would prove the rule for that route and say nothing about the other nine.
func TestACommitThatErasesASignatureIsRefusedUnlessAccepted(t *testing.T) {
	signed := signedFixture(t)

	// The two outcomes, measured here rather than assumed, because the whole rule turns on the
	// difference and `internal/pdfops`' table is a different package's business.
	erasing, err := pdfops.Collect(signed, []string{"2", "1"})
	if err != nil {
		t.Fatal(err)
	}
	breaking, err := pdfops.Rotate(signed, []string{"1"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	if sign.HasSignatureBlob(erasing) {
		t.Fatal("setup: Collect kept the signature blob, so the erasing case is not erasing and " +
			"this test would pass against a build that refuses nothing")
	}
	if !sign.HasSignatureBlob(breaking) {
		t.Fatal("setup: Rotate dropped the signature blob, so the ALLOWED case is not allowed and " +
			"the third assertion below cannot tell a working rule from a blanket refusal")
	}

	holding := func(t *testing.T) (*Server, *document) {
		t.Helper()
		srv := openTestServer(t, signed)
		return srv, srv.activeDoc()
	}
	for _, c := range []struct {
		name   string
		commit func(s *Server, doc *document, result []byte, accept bool) error
	}{
		{"commitMutation", func(s *Server, doc *document, result []byte, accept bool) error {
			return s.commitMutation(doc, signed, result, accept)
		}},
		{"commitBarrier", func(s *Server, doc *document, result []byte, accept bool) error {
			return s.commitBarrier(doc, result, accept)
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Refused when it erases and nobody said yes.
			s, doc := holding(t)
			if err := c.commit(s, doc, erasing, false); err == nil {
				t.Error("a commit that leaves no trace of the document's signature was allowed " +
					"with no acknowledgement — the CLI and every non-browser caller reach this " +
					"door, and the only thing that ever warned about it was a browser confirm")
			} else if !strings.Contains(err.Error(), "no record it was ever signed") {
				t.Errorf("refused, but not as a signature erasure: %v", err)
			}
			// And the document is untouched, which is the half that makes the refusal worth having.
			if !sign.HasSignatureBlob(s.docBytes(doc)) {
				t.Error("the refusal still replaced the document — it refused after committing, " +
					"which is no refusal at all")
			}

			// Allowed once the caller says yes.
			s2, doc2 := holding(t)
			if err := c.commit(s2, doc2, erasing, true); err != nil {
				t.Errorf("an acknowledged erasure was still refused: %v", err)
			}

			// And an operation that leaves the signature as EVIDENCE is never refused. Without
			// this the rule could be "refuse every commit on a signed document", which would
			// break rotate, insert and append — all of which keep the blob.
			s3, doc3 := holding(t)
			if err := c.commit(s3, doc3, breaking, false); err != nil {
				t.Errorf("a commit that leaves the signature in place as a BROKEN signature was "+
					"refused: %v. That outcome is the loud one and is what this rule exists to "+
					"prefer, not to prevent", err)
			}
		})
	}
}

// TestTheSignatureErasureRefusalSaysWhatTheClientListensFor holds the server's sentence and the
// client's matcher together.
//
// The client cannot know in advance whether an operation erases a signature — measured,
// `api.MergeRaw` preserves it and `Collect` does not — so it discovers it from the refusal and
// offers the retry. It recognises the refusal by a phrase. Reword the Go error without the JS and
// the product silently stops offering the way through: every erasure becomes a dead 409.
func TestTheSignatureErasureRefusalSaysWhatTheClientListensFor(t *testing.T) {
	const token = "no record it was ever signed"
	if !strings.Contains(errSignatureWouldBeErased.Error(), token) {
		t.Errorf("the server's refusal no longer contains %q, which web/app.js matches on. Every "+
			"signature-erasing operation now returns a 409 the client cannot act on", token)
	}
	js, err := os.ReadFile(filepath.Join("..", "..", "web", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), "SIGNATURE_ERASURE_TOKEN = '"+token+"'") {
		t.Errorf("web/app.js no longer declares SIGNATURE_ERASURE_TOKEN as %q — the two copies "+
			"have drifted, and the one that goes quiet is the client", token)
	}
	// The field name is the other half of the same contract, and /pending 447's guard checks the
	// routes. This checks the spelling agrees at all.
	if !strings.Contains(string(js), "'acceptSignatureLoss'") {
		t.Error("web/app.js never appends acceptSignatureLoss, so the retry cannot carry the " +
			"acknowledgement and the refusal is a dead end")
	}
}

// TestUnverifiedSignersCountsOnlyIdentitiesThisMachineCannotVouchFor — /pending 390's server half.
//
// The badge's rule is "may this say Untampered", and this is the fact it turns on. The three states
// are not decoration: **absent** means the question was not asked (nothing is signed, or the vault
// is locked so the pinned set cannot be read), **zero** means every signature is by a known
// identity, and a positive count is the attack — a signature by somebody this user has never seen.
func TestUnverifiedSignersCountsOnlyIdentitiesThisMachineCannotVouchFor(t *testing.T) {
	theirs := strings.Repeat("bb", 32)
	stranger := strings.Repeat("cc", 32)
	sig := func(fps ...string) sign.Status {
		st := sign.Status{State: sign.Valid}
		for _, f := range fps {
			st.Signers = append(st.Signers, sign.SignerInfo{Valid: true, Fingerprint: f})
		}
		return st
	}

	// A locked vault cannot answer, and "cannot answer" must not read as "all known".
	if got := unverifiedSigners(nil, sig(theirs)); got != nil {
		t.Errorf("a locked vault reported %d unverified signers — it has read no pinned set at "+
			"all, and any number here is invented. The badge treats absent as 'do not vouch'; a "+
			"zero here would make it claim verification that never happened", *got)
	}

	ts, srv := startServerWith(t)
	// The vault is opened by authenticating, exactly as the ceremony-arm tests do — `startServerWith`
	// alone leaves it locked, which is the FIRST case above and not the ones below.
	authedClient(t, ts)
	srv.mu.Lock()
	v := srv.vault
	srv.mu.Unlock()
	if v == nil {
		t.Fatal("setup: the vault is not open, so every case below is the locked one")
	}
	// An unsigned document is never asked about.
	if got := unverifiedSigners(v, sign.Status{State: sign.Unsigned}); got != nil {
		t.Errorf("a document with no signatures reported %d unverified signers", *got)
	}

	theirsFP, err := hex.DecodeString(theirs)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.AddCeremonyPeer(theirsFP, "A counterparty", "c"); err != nil {
		t.Fatal(err)
	}
	// The stimulus floor: the pin actually took, or "0 unverified" below is a pin that never
	// happened being read as a signer that was never checked.
	if _, pinned := pinnedLabel(v, theirsFP); !pinned {
		t.Fatal("setup: the pin did not take, so a zero below would mean nothing")
	}

	if got := unverifiedSigners(v, sig(theirs)); got == nil || *got != 0 {
		t.Errorf("a document signed only by a PINNED peer reported %v unverified — the badge "+
			"would withhold Untampered from a counterparty the user has already verified out of "+
			"band, which is the pairing this product asks them to do", got)
	}
	if got := unverifiedSigners(v, sig(theirs, stranger)); got == nil || *got != 1 {
		t.Errorf("a document co-signed by a stranger reported %v unverified, want 1 — this is "+
			"/pending 390 verbatim: the stranger's own self-signed signature is valid over its "+
			"own byte range, and nothing else distinguishes it", got)
	}
	// This machine's own signature is not a stranger's, and treating it as one would put a
	// warning on every document the user signed alone.
	//
	// **`identity(v)` and not `v.Identity()`, and the first cut of this used the latter and
	// SKIPPED.** A fresh test vault holds no identity until something asks for one, so the self
	// case never ran — and a probe that removed the self branch from production stayed green,
	// which is a vacuous assertion of exactly the shape this repo grades hardest. `identity`
	// generates and stores one, which is what any real signing flow on this machine has already
	// done by the time a document carries this user's signature.
	cert, _, ierr := identity(v)
	if ierr != nil {
		t.Fatal(ierr)
	}
	selfFP, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	if got := unverifiedSigners(v, sig(hex.EncodeToString(selfFP))); got == nil || *got != 0 {
		t.Errorf("a document this machine signed ITSELF reported %v unverified — a user is not a "+
			"pinned peer of themselves, so without that case every solo signature warns", got)
	}
}

// TestNoCloseOutPathProducesTheExpiredState — /pending 421, and it is what makes D28's amendment
// stick rather than being a paragraph.
//
// D28 said a ceremony ends by completing, declining, EXPIRING or being abandoned, and the close-out
// can conclude three of those. `closeOutReason`'s `past` branch tests `Expires` plus
// `closeOutGrace` — word for word `abandoned`'s own definition, the deadline and the grace both
// passed with nothing said — so at the only moment a close-out runs, both are true and `abandoned`
// is the more specific claim. There is no window in which only expiry holds.
//
// **And the obvious fix is refused by the mechanism it would run inside.** Firing a close-out at
// the deadline to mint `expired` would archive proceedings the grace exists to let finish:
// `closeOutGrace`'s own doc says it covers "the delivery round starting AFTER the deadline … plus
// the machine being off".
//
// So this asserts the ABSENCE, and the absence is the decision. If a producer ever appears, D28's
// amendment, `outcomes.test.mjs`'s header and its PRODUCIBLE list are all stale together, and this
// is the thing that says so.
func TestNoCloseOutPathProducesTheExpiredState(t *testing.T) {
	src, err := os.ReadFile("closeout.go")
	if err != nil {
		t.Fatal(err)
	}
	body := funcBodyFrom(string(src), strings.Index(string(src), "func closeOutReason("))
	// The stimulus floor: this scan is reading the right function at all. A body that does not
	// mention the state it DOES produce is a body this scan failed to find, and an empty string
	// contains no "StateExpired" either — which would read as a clean pass forever.
	if !strings.Contains(body, "StateAbandoned") {
		t.Fatal("closeOutReason's body could not be read, or no longer mentions StateAbandoned — " +
			"an empty body contains no StateExpired either, so this check would pass over nothing")
	}
	if strings.Contains(body, "StateExpired") {
		t.Error("closeOutReason can now produce ceremony.StateExpired. That may well be right — " +
			"but three other places are written on the assumption that it cannot, and they are " +
			"stale the moment it does: D28's 2026-09-10 amendment in PLAN-signing-ceremony.md, " +
			"the header of test/jsdom/outcomes.test.mjs, and that file's PRODUCIBLE list. Read " +
			"closeOutGrace's doc before deciding — the grace exists so a late delivery round can " +
			"finish, and a close-out that fires at the deadline archives proceedings mid-round.")
	}
}
