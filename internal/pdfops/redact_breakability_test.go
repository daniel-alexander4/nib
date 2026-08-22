package pdfops

import (
	"bytes"
	"compress/zlib"
	"fmt"
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

// TestRedactLeavesNoResidualContentAnywhereInTheFile — the whole-file half of the doubt
// above, which the test above claims and does not check.
//
// `TestRedactLeavesNoResidualContent`'s second assertion reads "the secret must not survive
// anywhere else in the document either" and is implemented as `contentContains(redacted, "",
// secret)`. That walks **page content streams** — every page's, which is more than page 1's,
// but still only content streams. An object that is present in the file and referenced by no
// page is invisible to it, and "not reachable from a page" is exactly the state a
// half-removed object is in. The claim was wider than the check.
//
// This scans the file itself: every `stream`…`endstream` segment, inflated where it inflates,
// plus the raw bytes. That is a superset of the page walk and needs no page tree at all, so a
// leftover object in a dead branch of the xref is still in scope.
//
// **The ORIGINAL is the control, and it is the whole test.** Measured while writing this:
// `bytes.Contains(redacted, secret)` is false — and it is false of the ORIGINAL too, because
// testpdf's content stream is flate-compressed. So a raw-byte scan on its own is **vacuous
// here by construction**: it reports "clean" for a document that visibly contains the secret.
// The inflating scan finds it in exactly 1 stream of the original and 0 of the redacted,
// which is the discrimination this test is for. Anyone tempted to simplify this to a
// `bytes.Contains` should read that sentence again.
func TestRedactLeavesNoResidualContentAnywhereInTheFile(t *testing.T) {
	const secret = "CANARYSECRET98765"
	original, err := testpdf.Text(secret, "KEEPVISIBLE12345")
	if err != nil {
		t.Fatal(err)
	}
	redacted, err := RedactPages(original, map[int]RasterPage{1: rasterPage(t, 612, 792)})
	if err != nil {
		t.Fatal(err)
	}

	// SETUP / positive control: the scan can see the secret in a document that has it.
	// Without this the "0 in the redacted file" result below is equally consistent with a
	// scan that cannot see anything at all — which is what a raw-byte version of this test
	// would have been.
	if raw, n := fileCarries(original, secret); n == 0 {
		t.Fatalf("setup: the whole-file scan found the secret in 0 stream(s) of the "+
			"UNREDACTED document (raw-bytes=%v) — it cannot see what it is looking for, so "+
			"finding nothing after redaction would prove nothing", raw)
	}
	if raw, _ := fileCarries(original, secret); raw {
		t.Log("note: the secret is in the original's raw bytes, so this fixture is no longer " +
			"compressed; the inflating half of the scan is still what carries the assertion")
	}

	if raw, n := fileCarries(redacted, secret); n > 0 || raw {
		t.Errorf("after redaction the secret survives in the file: raw-bytes=%v, "+
			"stream(s)=%d. A rasterized page that leaves the original text recoverable "+
			"anywhere in the bytes is the classic fake-redaction leak — the thing this "+
			"path exists to prevent.", raw, n)
	}
}

// fileCarries scans a PDF's bytes for a needle: raw, and inside every stream segment that
// inflates. It deliberately does not use the page tree — an object nothing references is
// precisely what it is looking for.
func fileCarries(pdf []byte, needle string) (raw bool, streams int) {
	raw = bytes.Contains(pdf, []byte(needle))
	for i := 0; ; {
		j := bytes.Index(pdf[i:], []byte("stream"))
		if j < 0 {
			return raw, streams
		}
		start := i + j + len("stream")
		for start < len(pdf) && (pdf[start] == '\r' || pdf[start] == '\n') {
			start++
		}
		k := bytes.Index(pdf[start:], []byte("endstream"))
		if k < 0 {
			return raw, streams
		}
		seg := pdf[start : start+k]
		if zr, err := zlib.NewReader(bytes.NewReader(seg)); err == nil {
			if out, rerr := io.ReadAll(zr); rerr == nil && bytes.Contains(out, []byte(needle)) {
				streams++
			}
			zr.Close()
		} else if bytes.Contains(seg, []byte(needle)) {
			streams++
		}
		// **Past the whole `endstream` token, not to its start.** `endstream` CONTAINS
		// `stream`, so resuming at `start+k` finds that inner occurrence three bytes later
		// and treats everything up to the NEXT `endstream` as a segment — which desynchronises
		// the walk permanently: after the first object, every real payload is inside a
		// misaligned span that fails to inflate and is silently skipped.
		//
		// This shipped for the length of one test run and produced a **passing** whole-file
		// check, because the fixture's secret happens to live in the first stream (so the
		// control found it) and everything after was invisible (so the redacted file scanned
		// clean for the wrong reason). Caught by TestTheTwoResidueChecksDiffer…, which is the
		// argument for writing that test rather than trusting two agreeing green results.
		i = start + k + len("endstream")
	}
}

// TestTheTwoResidueChecksDifferAndTheDifferenceIsThePoint — why there are two.
//
// Without this, the whole-file check reads as a redundant restatement of the page-content
// one, and the obvious tidy-up is to delete it. This pins the case that separates them: a
// stream that is PRESENT in the file and referenced by no page.
//
// `contentContains(pdf, "", secret)` walks the page tree and extracts each page's content.
// An object outside that tree is not reachable, so it reports clean — correctly, for the
// question it asks. `fileCarries` scans the bytes and finds it. "Not reachable from a page"
// is the state a half-removed object is in, and it is the state a reader recovering redacted
// text is looking for, so the second question is the one the doubt actually poses.
//
// The dangling object is appended after %%EOF, which leaves the original xref authoritative —
// so the document still parses and the page-tree walk still runs. That is what makes the two
// results comparable rather than one of them being an error.
func TestTheTwoResidueChecksDifferAndTheDifferenceIsThePoint(t *testing.T) {
	const secret = "ORPHANEDSECRET4242"
	base, err := testpdf.Text("visible-one", "visible-two")
	if err != nil {
		t.Fatal(err)
	}
	// SETUP: the clean document is clean by BOTH checks, so anything below is caused by the
	// object we append and not by the fixture.
	if contentContains(t, base, "", secret) {
		t.Fatal("setup: the base fixture already contains the secret in its page content")
	}
	if raw, n := fileCarries(base, secret); raw || n > 0 {
		t.Fatalf("setup: the base fixture already carries the secret in its bytes (raw=%v, streams=%d)", raw, n)
	}

	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	if _, err := zw.Write([]byte("BT (" + secret + ") Tj ET")); err != nil {
		t.Fatal(err)
	}
	zw.Close()
	orphan := append([]byte("\n99 0 obj\n<< /Length "), []byte(fmt.Sprintf("%d /Filter /FlateDecode >>\nstream\n", z.Len()))...)
	orphan = append(orphan, z.Bytes()...)
	orphan = append(orphan, []byte("\nendstream\nendobj\n")...)
	polluted := append(append([]byte{}, base...), orphan...)

	// The page walk cannot see it — nothing references object 99.
	if contentContains(t, polluted, "", secret) {
		t.Error("the page-content check found an object no page references; if that is now " +
			"true, the two checks have converged and the second one may be redundant")
	}
	// The whole-file scan can. This is the entire reason it exists.
	raw, n := fileCarries(polluted, secret)
	if n == 0 && !raw {
		t.Error("the whole-file scan did NOT find a flate stream carrying the secret that " +
			"was appended to the file — it cannot see an unreferenced object, which is the " +
			"only thing it adds over the page-content check")
	}
	t.Logf("page-content check: clean; whole-file scan: raw=%v streams=%d", raw, n)
}
