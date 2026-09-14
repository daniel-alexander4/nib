package tagwrite

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

func signedPDF(t *testing.T) []byte {
	t.Helper()
	certPEM, keyPEM, err := sign.GenerateIdentity("Nib Test")
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := testpdf.Text("a signed page of text")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := sign.Sign(pdf, certPEM, keyPEM, sign.Options{Name: "Nib Test", When: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if st := sign.Verify(signed).State; st != sign.Valid {
		t.Fatalf("fixture verifies as %q — it is not signed", st)
	}
	return signed
}

// TestTheDoorRefusesASignedDocumentBeforeWriting — both writes, each naming what it was asked to do. The
// review and the edit are ones the unsigned document takes, so the refusal is the signature's and nothing else.
func TestTheDoorRefusesASignedDocumentBeforeWriting(t *testing.T) {
	plain, err := testpdf.Text("a signed page of text")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := pdfops.ProposeTags(plain)
	if err != nil || len(prop.Elements) == 0 {
		t.Fatalf("setup: %+v %v", prop, err)
	}
	var review []pdfops.TagReview
	for _, e := range prop.Elements {
		review = append(review, pdfops.TagReview{ID: e.ID, Role: e.Role, Text: e.Text})
	}
	if _, err := Commit(plain, review); err != nil {
		t.Fatalf("setup: the unsigned document refuses the review: %v", err)
	}

	signed := signedPDF(t)
	sprop, err := pdfops.ProposeTags(signed)
	if err != nil {
		t.Fatal(err)
	}
	review = review[:0]
	for _, e := range sprop.Elements {
		review = append(review, pdfops.TagReview{ID: e.ID, Role: e.Role, Text: e.Text})
	}
	keep := append([]byte{}, signed...)
	if out, err := Commit(signed, review); !errors.Is(err, ErrSigned) || out != nil || !strings.Contains(err.Error(), "adding structure") {
		t.Errorf("commit on a signed document: %v — want ErrSigned naming the commit", err)
	}
	if out, err := Edit(signed, []pdfops.StructureEdit{{Kind: "alt", Element: 1, Value: "x", Index: -1}}); !errors.Is(err, ErrSigned) || out != nil || !strings.Contains(err.Error(), "correcting its structure") {
		t.Errorf("edit on a signed document: %v — want ErrSigned naming the correction", err)
	}
	if !bytes.Equal(keep, signed) {
		t.Error("a refused write changed the input")
	}
}

// TestTheDoorPassesThroughTheWritersOwnErrors — stale and malformed stay what pdfops says they are.
func TestTheDoorPassesThroughTheWritersOwnErrors(t *testing.T) {
	plain, err := testpdf.Text("some text")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Commit(plain, nil); !errors.Is(err, pdfops.ErrTagsStale) {
		t.Errorf("an empty review against a proposal: %v, want ErrTagsStale", err)
	}
	if _, err := Edit(plain, []pdfops.StructureEdit{{Kind: "paint"}}); !errors.Is(err, pdfops.ErrTagsReview) {
		t.Errorf("an edit that is not one: %v, want ErrTagsReview", err)
	}
	if _, err := Edit(plain, []pdfops.StructureEdit{{Kind: "alt", Element: 5, Index: -1}}); !errors.Is(err, pdfops.ErrTagsStale) {
		t.Errorf("an edit on a document with no tree: %v, want ErrTagsStale", err)
	}
}

// TestAResultThatDoesNotValidateIsNotReturned — the door's last rule. No review or edit nib accepts is
// known to write an invalid document, so the rule is driven directly with bytes that are not one.
func TestAResultThatDoesNotValidateIsNotReturned(t *testing.T) {
	out, err := validated([]byte("%PDF-1.7\nnot a document\n"))
	if !errors.Is(err, ErrInvalid) || out != nil {
		t.Errorf("an invalid result: %v, %d byte(s) — want ErrInvalid and nothing to write", err, len(out))
	}
	plain, err := testpdf.Text("valid")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := validated(plain); err != nil || !bytes.Equal(out, plain) {
		t.Errorf("a valid result: %v — want it returned unchanged", err)
	}
}

// TestDecodingReadsTheRequestShapes — an absent index appends, a present one (0 included) is kept, the
// proposal's own JSON is a review, and a body that is not JSON is ErrMalformed.
func TestDecodingReadsTheRequestShapes(t *testing.T) {
	edits, err := DecodeEdits(strings.NewReader(`{"edits":[{"kind":"move","element":7},{"kind":"move","element":8,"parent":3,"index":0},{"kind":"alt","element":9,"value":"v"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []pdfops.StructureEdit{{Kind: "move", Element: 7, Index: -1}, {Kind: "move", Element: 8, Parent: 3, Index: 0}, {Kind: "alt", Element: 9, Value: "v", Index: -1}}
	if len(edits) != len(want) {
		t.Fatalf("decoded %+v", edits)
	}
	for i := range want {
		if edits[i] != want[i] {
			t.Errorf("edit %d decoded %+v, want %+v", i, edits[i], want[i])
		}
	}
	reviews, err := DecodeReview(strings.NewReader(`{"elements":[{"id":2,"role":"H1","page":1,"text":"T","pageBox":[0,0,1,1]},{"id":3,"role":"P","ignore":true,"text":"U"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 2 || reviews[0] != (pdfops.TagReview{ID: 2, Role: "H1", Text: "T"}) || reviews[1] != (pdfops.TagReview{ID: 3, Role: "P", Ignore: true, Text: "U"}) {
		t.Errorf("decoded %+v", reviews)
	}
	for _, bad := range []string{`not json`, `{"edits": 5}`} {
		if _, err := DecodeEdits(strings.NewReader(bad)); !errors.Is(err, ErrMalformed) {
			t.Errorf("DecodeEdits(%q) = %v, want ErrMalformed", bad, err)
		}
		if _, err := DecodeReview(strings.NewReader(bad)); !errors.Is(err, ErrMalformed) && bad == `not json` {
			t.Errorf("DecodeReview(%q) = %v, want ErrMalformed", bad, err)
		}
	}
}
