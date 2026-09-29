package p2p

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/digitorus/pdfsign/verify"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

// TestARefusedCopyMovesNoCeremonyReader — P01.S03's acceptance table (ADR-060). A copied signature
// dictionary (/pending 687) is a record nib refuses, and until S03 it was still a SIGNER: the library
// enumerates it as a second valid signature by the victim, and every ceremony reader that counts or
// positions signatures took it as one. Measured on `939c7986`: `ContributionProgress` halted the
// ceremony (`ErrPrefixMismatch`, "signature 2 is (none)"), `NextPlacement` moved the next block
// 136 → 232, and the library-FAILS variant made the prefix unprovable.
//
// Each reader's outcome on each copy must EQUAL its outcome on the untouched file — the copy is not a
// signer, so it may change nothing a signer count feeds. Tiers 4 and 6 run one binary on every side
// and cannot see this, so this table is the acceptance and they are the backstop.
func TestARefusedCopyMovesNoCeremonyReader(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	aCert, aKey, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}
	bCert, _, err := sign.GenerateIdentity("Bob")
	if err != nil {
		t.Fatal(err)
	}
	afp, err := sign.Fingerprint(aCert)
	if err != nil {
		t.Fatal(err)
	}
	bfp, err := sign.Fingerprint(bCert)
	if err != nil {
		t.Fatal(err)
	}
	a, b := hex.EncodeToString(afp), hex.EncodeToString(bfp)
	untouched, err := sign.SignApproval(base, aCert, aKey, sign.Options{Name: "Alice", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	roster := Roster{Entries: []RosterEntry{{Fingerprint: a, Signs: true}, {Fingerprint: b, Signs: true}}}
	proc := Proceeding{Signing: []string{a, b}}

	// outcomes is every reader's answer on one document, as one comparable string per reader.
	outcomes := func(doc []byte) map[string]string {
		pl, perr := NextPlacement(doc)
		pr, prErr := ContributionProgress(doc, roster)
		signed, obliged := Completeness(Attestations(sign.Verify(doc), proc), proc)
		return map[string]string{
			"NextPlacement":        fmt.Sprintf("%v %v", pl.Rect, perr),
			"ContributionProgress": fmt.Sprintf("done=%d complete=%v err=%v", pr.Done, pr.Complete, prErr),
			"Completeness":         fmt.Sprintf("%d/%d", signed, obliged),
			"confirmCoSigned":      fmt.Sprint(confirmCoSigned(doc, afp, bfp, true)),
			"ReadAttestations":     fmt.Sprint(len(ReadAttestations(doc))),
		}
	}
	want := outcomes(untouched)
	// The untouched file's own answers are the ones a one-signer, two-party ceremony should give,
	// or "equal to the untouched file" is equality with a broken baseline.
	if want["ContributionProgress"] != "done=1 complete=false err=<nil>" || want["Completeness"] != "1/2" || want["ReadAttestations"] != "1" {
		t.Fatalf("setup: the untouched file's own readers are %v", want)
	}

	for _, tc := range []struct {
		name    string
		variant testpdf.CopyVariant
		valid   int // how many signatures the library alone verifies on the copy
	}{
		{"victim's exact four under a new number", testpdf.CopyExact, 2},
		{"victim's four plus 999999999 0", testpdf.CopyPastEOF, 2},
		{"a copy the library fails", testpdf.CopyLibraryFails, 1},
	} {
		doc, _, err := testpdf.CopiedSignatureDictionary(untouched, tc.variant)
		if err != nil {
			t.Fatal(err)
		}
		// STIMULUS: the library alone reports TWO signers on the copy — what `Signers` was before
		// S03 — so a reader agreeing with the untouched file is the exclusion's work.
		resp, lerr := verify.Verify(bytes.NewReader(doc), int64(len(doc)))
		if lerr != nil || len(resp.Signers) != 2 {
			t.Fatalf("%s: STIMULUS: library err %v, %d signer(s); want the copy as a second", tc.name, lerr, len(resp.Signers))
		}
		valid := 0
		for _, s := range resp.Signers {
			if s.ValidSignature {
				valid++
			}
		}
		if valid != tc.valid {
			t.Fatalf("%s: STIMULUS: the library verifies %d, want %d", tc.name, valid, tc.valid)
		}
		got := outcomes(doc)
		for reader, w := range want {
			if got[reader] != w {
				t.Errorf("%s: %s reads %q on the copy and %q on the untouched file — a refused copy moved a ceremony reader", tc.name, reader, got[reader], w)
			}
		}
		// The failure the table exists for, named: the copy must not halt the ceremony.
		if _, prErr := ContributionProgress(doc, roster); errors.Is(prErr, ErrPrefixMismatch) || errors.Is(prErr, ErrPrefixUnproven) {
			t.Errorf("%s: the copy halts the ceremony: %v", tc.name, prErr)
		}
	}
}
