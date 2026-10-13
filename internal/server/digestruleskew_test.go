package server

import (
	"errors"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// documentUnderDigestRule is an unsigned document carrying a ceremony record that VERIFIES and names
// content-digest rule `rule` — what a party holds when the ceremony was convened by a build whose rule this
// one does not compute. The record is re-signed by its own convener, because `DigestVersion` is inside the
// signed preimage: an edited copy would fail as a bad signature and prove the wrong thing.
func documentUnderDigestRule(t *testing.T, rule int) ([]byte, ceremony.Record) {
	t.Helper()
	convened, cert, key := convenedDocument(t)
	rec := extractRecord(t, convened)
	rec.DigestVersion = rule
	if err := rec.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	if rec.DigestVersion != rule {
		t.Fatal("setup: Sign overwrote the record's digest rule")
	}
	if pdfops.ContentDigestRuleReadable(rule) {
		t.Fatalf("setup: this build computes rule %d, so a record under it is not a skew", rule)
	}
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ceremony.Embed(base, rec)
	if err != nil {
		t.Fatal(err)
	}
	return doc, rec
}

// TestTheArrivalGateRefusesADigestRuleSkewByName — ADR-143, /pending 691: `ceremony.ErrDigestVersion` had no
// caller that could put it on the wire. The arrival gate refused a document whose record names a rule this
// build does not compute, `p2p.refusalCode` answered 0, nothing was written, and the dialler read a bare EOF —
// a 502 "could not connect to peer" with a D19 network cause, for a peer that connected and said no. The
// refusal now carries the wire code for its DIRECTION, and the sentence names the right remedy.
func TestTheArrivalGateRefusesADigestRuleSkewByName(t *testing.T) {
	_, invText := convenedFor(t)
	inv, err := ceremony.ParseInvitation(invText)
	if err != nil {
		t.Fatal(err)
	}
	cer := &ceremonyID{inv: inv}
	for _, c := range []struct {
		name         string
		rule         int
		is, isNot    error
		says, remedy string
	}{
		{"a rule this build retired", pdfops.ContentDigestVersion - 2, p2p.ErrCeremonyRuleRetired,
			p2p.ErrCeremonyRuleTooNew, "started with an older Nib", "convene it again"},
		{"a rule this build does not know yet", pdfops.ContentDigestVersion + 1, p2p.ErrCeremonyRuleTooNew,
			p2p.ErrCeremonyRuleRetired, "started with a newer Nib", "update Nib"},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc, _ := documentUnderDigestRule(t, c.rule)
			err := cer.checkArrival(doc, time.Now(), nil)
			if !errors.Is(err, ceremony.ErrDigestVersion) {
				t.Fatalf("the arrival gate answered %v — want the digest-rule skew", err)
			}
			if !errors.Is(err, c.is) || errors.Is(err, c.isNot) {
				t.Errorf("the refusal is not the wire refusal for its direction, and only that one: %v", err)
			}
			if !p2p.IsContributionRefusal(err) {
				t.Errorf("the refusal has no wire code, so it reaches the dialler as a bare EOF: %v", err)
			}
			if got := connectFailure(err); strings.Contains(got, "could not connect to peer") {
				t.Errorf("a version difference is reported as a connect failure: %q", got)
			}
			if msg := err.Error(); !strings.Contains(msg, c.says) || !strings.Contains(msg, c.remedy) ||
				strings.Contains(msg, "not the same document") {
				t.Errorf("the sentence does not say %q with the remedy %q, or it accuses: %s", c.says, c.remedy, msg)
			}
		})
	}
}
