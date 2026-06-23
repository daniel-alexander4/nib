package pdfops

import (
	"bytes"
	"io"
	"testing"

	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// contentContains reports whether the decoded content stream of the given pages
// ("" = all) holds needle. It extracts via pdfcpu, which decompresses the
// streams first, so a FlateDecode'd copy of the text can't hide from the scan.
func contentContains(t *testing.T, pdf []byte, pages, needle string) bool {
	t.Helper()
	var sel []string
	if pages != "" {
		sel = []string{pages}
	}
	// pdfcpu v0.13 ExtractContent hands each page's decoded content stream to a
	// callback; accumulate them all and scan the lot for the needle.
	var buf bytes.Buffer
	digest := func(r io.Reader, _ int) error { _, err := io.Copy(&buf, r); return err }
	if err := api.ExtractContent(bytes.NewReader(pdf), sel, digest, model.NewDefaultConfiguration()); err != nil {
		t.Fatalf("extract content: %v", err)
	}
	return bytes.Contains(buf.Bytes(), []byte(needle))
}

// TestRedactLeavesNoResidualContent proves nib's "replace the page with a flat
// image" redaction is not breakable by recovering the original content: after
// redaction the secret text must be gone from the page's content stream and from
// everywhere else in the file, while non-redacted pages keep their text. The
// scan runs against decoded streams, so a compressed leftover can't slip past.
func TestRedactLeavesNoResidualContent(t *testing.T) {
	const secret = "CANARYSECRET98765"
	const keep = "KEEPVISIBLE12345"

	original, err := testpdf.Text(secret, keep)
	if err != nil {
		t.Fatal(err)
	}

	// Positive control: the secret must be detectable in the unredacted page-1
	// content. Without this, a later "secret is absent" check could pass simply
	// because the scan can't see the text — a false sense of safety.
	if !contentContains(t, original, "1", secret) {
		t.Fatal("detection sanity failed: secret not found in the original page-1 content")
	}

	redacted, err := RedactPages(original, map[int]RasterPage{1: rasterPage(t, 612, 792)})
	if err != nil {
		t.Fatal(err)
	}

	// The redacted page was rasterized — its original text must be gone.
	if contentContains(t, redacted, "1", secret) {
		t.Error("redacted page still carries the original text in its content stream")
	}
	// And the secret must not survive anywhere else in the document either.
	if contentContains(t, redacted, "", secret) {
		t.Error("secret text leaked into another page or object of the redacted PDF")
	}
	// A non-redacted page keeps its vector text untouched.
	if !contentContains(t, redacted, "2", keep) {
		t.Error("a non-redacted page lost its content during redaction")
	}
}
