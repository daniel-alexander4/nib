package server

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/digitorus/pdfsign/verify"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

// TestARefusedCopyIsNobodyToTheServersSignerReaders — P01.S03's server half (ADR-060), beside
// `internal/p2p`'s table: the two readers here are unexported and no one package can host all
// seven. A copied signature dictionary (/pending 687) is refused, and until S03 the library's
// second "signer" reached both: `unverifiedSigners` counted it — so a document signed only by a
// pinned counterparty read "1 from someone you have not verified" — and `signersSoFar` showed the
// consent screen a second row wearing the VICTIM's name with no fingerprint, the row the plan's pin
// forbids. Each must equal its answer on the untouched file.
func TestARefusedCopyIsNobodyToTheServersSignerReaders(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	aCert, aKey, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}
	afp, err := sign.Fingerprint(aCert)
	if err != nil {
		t.Fatal(err)
	}
	untouched, err := sign.SignApproval(base, aCert, aKey, sign.Options{Name: "Alice", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	ts, srv := startServerWith(t)
	authedClient(t, ts)
	srv.mu.Lock()
	v := srv.vault
	srv.mu.Unlock()
	if v == nil {
		t.Fatal("setup: the vault is not open")
	}
	if err := v.AddPinnedPeer(afp, "Alice"); err != nil {
		t.Fatal(err)
	}
	if _, pinned := pinnedLabel(v, afp); !pinned {
		t.Fatal("setup: the pin did not take, so a zero below would mean nothing")
	}

	outcomes := func(doc []byte) (unverified string, rows string) {
		u := unverifiedSigners(v, sign.Verify(doc))
		if u == nil {
			unverified = "nil"
		} else {
			unverified = fmt.Sprint(*u)
		}
		return unverified, fmt.Sprintf("%+v", firstOf(signersSoFar(doc)))
	}
	wantU, wantRows := outcomes(untouched)
	if wantU != "0" || len(firstOf(signersSoFar(untouched))) != 1 || firstOf(signersSoFar(untouched))[0].Fingerprint != hex.EncodeToString(afp) {
		t.Fatalf("setup: the untouched file reads unverified=%s rows=%s, want 0 and Alice alone", wantU, wantRows)
	}

	for _, tc := range []struct {
		name    string
		variant testpdf.CopyVariant
	}{
		{"victim's exact four under a new number", testpdf.CopyExact},
		{"victim's four plus 999999999 0", testpdf.CopyPastEOF},
		{"a copy the library fails", testpdf.CopyLibraryFails},
	} {
		doc, _, err := testpdf.CopiedSignatureDictionary(untouched, tc.variant)
		if err != nil {
			t.Fatal(err)
		}
		// STIMULUS: the library alone reports the copy as a second signer.
		resp, lerr := verify.Verify(bytes.NewReader(doc), int64(len(doc)))
		if lerr != nil || len(resp.Signers) != 2 {
			t.Fatalf("%s: STIMULUS: library err %v, %d signer(s); want 2", tc.name, lerr, len(resp.Signers))
		}
		u, rows := outcomes(doc)
		if u != wantU {
			t.Errorf("%s: unverifiedSigners %s on the copy, %s on the untouched file — the badge withholds Untampered over a refused copy", tc.name, u, wantU)
		}
		if rows != wantRows {
			t.Errorf("%s: signersSoFar %s on the copy, %s on the untouched file — the consent screen lists a refused copy", tc.name, rows, wantRows)
		}
		// /pending 738: not a signer, and not silent either. The badge, the details panel and `nib
		// verify` name the refusal; the consent screen is told it too, with its cause.
		_, refused := signersSoFar(doc)
		if len(refused) != 1 || refused[0].Cause == "" {
			t.Errorf("%s: the consent screen is told of %d refused signature(s) %+v, want the one copy with its cause — "+
				"a party consenting to co-sign is the one reader of the document's signatures left saying nothing about it", tc.name, len(refused), refused)
		}
	}
	if _, refused := signersSoFar(untouched); len(refused) != 0 {
		t.Errorf("the untouched file reports %d refused signature(s): %+v", len(refused), refused)
	}
}

// firstOf is signersSoFar's signer list, for the readers here that compare only that.
func firstOf(s []pendingSigner, _ []sign.RefusedSignature) []pendingSigner { return s }
