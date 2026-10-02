package sign

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pkcs7"

	"nib/internal/testpdf"
)

// The P02 phase-close review's fixes to the screens and the boundary walk (`code-reviews/…p02-phase-close…`, R1).

// bareBlob is a SignedData with NO signed attributes that encapsulates content, signed with key and naming cert —
// the shape the screens had admitted on "it encapsulates content" alone.
func bareBlob(t *testing.T, content []byte, cert identity, key identity) []byte {
	t.Helper()
	sd, err := pkcs7.NewSignedData(content)
	if err != nil {
		t.Fatal(err)
	}
	if err := sd.SignWithoutAttr(cert.cert, key.signer, pkcs7.SignerInfoConfig{}); err != nil {
		t.Fatal(err)
	}
	b, err := sd.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestAForgedKeyPassesNeitherScreen — the signer's certificate is in every document they signed, so anyone can write a
// SignerInfo that NAMES it, signed with their own key. Both screens admitted one with no signed attributes because it
// encapsulated content; both now call one function, which checks it against the named key.
func TestAForgedKeyPassesNeitherScreen(t *testing.T) {
	alice, mallory := newIdentity(t, "Alice"), newIdentity(t, "Mallory")
	forged := bareBlob(t, []byte("anything"), alice, mallory)
	p7, err := pkcs7.Parse(forged)
	if err != nil {
		t.Fatal(err)
	}
	pr := proofOf(p7)
	if pr == nil || hex.EncodeToString(fingerprintOf(pr.cert)) != alice.fp || pr.signedAttrs != nil || !pr.encapsulated || pr.content == nil {
		t.Fatal("stimulus: the forged blob is not a bare, encapsulating SignerInfo naming Alice with its content kept")
	}
	pdf := []byte("%PDF-1.7\n" + strings.Repeat("%pad\n", 20) + "7 0 obj\n<< /Type /Sig /ByteRange [0 10 20 30] /Contents <" +
		hex.EncodeToString(forged) + "> >>\nendobj\n")
	lits := rawByteRangeEnds(pdf)[50]
	if len(lits) != 1 {
		t.Fatalf("stimulus: %d literals end at 50", len(lits))
	}
	budget := int64(1 << 30)
	if prescreen(pdf, lits, alice.fp, &budget) {
		t.Error("the prescreen admitted a SignerInfo naming Alice and signed with Mallory's key")
	}
	rec := Revision{proof: pr, ByteRange: []int64{0, 10, 20, 30}}
	if (candidate{end: 50, proposers: []*Revision{&rec}}).screen(pdf, &budget) {
		t.Error("the candidate screen admitted a SignerInfo naming Alice and signed with Mallory's key")
	}
	// The control: the same shape made with Alice's OWN key, over what the library verifies, passes — the screen
	// refuses the key, not the shape.
	r := selected(nil, pdf, rec.ByteRange)
	honest, err := pkcs7.Parse(bareBlob(t, append([]byte("anything"), r...), alice, alice))
	if err != nil {
		t.Fatal(err)
	}
	honest.Content = []byte("anything") // the encapsulated content; the library appends the ranges' bytes to it
	rec.proof = proofOf(honest)
	if !(candidate{end: 50, proposers: []*Revision{&rec}}).screen(pdf, &budget) {
		t.Error("the same shape signed with Alice's own key over the content and the ranges was refused")
	}
}

// TestTheScreenChecksAnEncapsulatingSignerAsTheLibraryDoes — the library verifies a SignerInfo with no signed attributes
// over the encapsulated content FOLLOWED BY the ranges' bytes (pdfsign `verify/signature.go` appends them), never over
// the content alone and never by comparing the content with a hash of the ranges. So the `adbe.pkcs7.sha1` shape —
// content = SHA-1 of the ranges, signed — does not verify there, and the screen must not admit it either.
func TestTheScreenChecksAnEncapsulatingSignerAsTheLibraryDoes(t *testing.T) {
	alice, mallory := newIdentity(t, "Alice"), newIdentity(t, "Mallory")
	pdf := bytes.Repeat([]byte("signed bytes "), 50)
	br := []int64{0, 100, 200, 300}
	r := selected(nil, pdf, br)
	digest := sha1.Sum(r)
	big := bytes.Repeat([]byte{'c'}, maxEncapsulatedContent+1)
	for _, row := range []struct {
		name          string
		signed, carry []byte // what the signature covers; what the SignedData encapsulates
		key           identity
	}{
		{"the content then the ranges, the signer's own key", append([]byte("head"), r...), []byte("head"), alice},
		{"the content then the ranges, another key", append([]byte("head"), r...), []byte("head"), mallory},
		{"the adbe.pkcs7.sha1 shape: the ranges' SHA-1, signed", digest[:], digest[:], alice},
		{"the content alone", []byte("head"), []byte("head"), alice},
		{"the ranges alone, over too much content to keep", r, big, alice},
	} {
		p7, err := pkcs7.Parse(bareBlob(t, row.signed, alice, row.key))
		if err != nil {
			t.Fatal(err)
		}
		p7.Content = append(append([]byte(nil), row.carry...), r...)
		lib := p7.Verify() == nil
		p7.Content = row.carry
		rec := Revision{proof: proofOf(p7), ByteRange: br}
		budget := int64(1 << 30)
		if got := (candidate{proposers: []*Revision{&rec}}).screen(pdf, &budget); got != lib {
			t.Errorf("%s: the library says %v, the screen %v", row.name, lib, got)
		}
		if row.name == "the content then the ranges, the signer's own key" && !lib {
			t.Fatal("stimulus: the library does not verify the shape it is said to, so agreement proves nothing")
		}
	}
}

// TestForgedFakeSectionsCannotCrowdOutAVersion — the review's construction: 17 fake xref sections, each holding a
// SignerInfo naming the signer signed with another key, ranges selecting 11 bytes and a padded file so the budget is
// never the bound (each fake's object runs to the stream's one `endobj`, and its scans are charged). Admitted, they
// spent the walk's 16 pdfcpu reads and the version in the file read `could-not-check`.
func TestForgedFakeSectionsCannotCrowdOutAVersion(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a, m := newIdentity(t, "Alice"), newIdentity(t, "Mallory")
	sA := signAs(t, base, a, "original")
	vnum, _ := victimDict(t, sA)
	doc := synthRevision(t, sA, []sobj{{num: vnum, body: "<< /Foo 2 >>"}}, catalogOf(t, sA))
	contents := "/Contents <" + hex.EncodeToString(bareBlob(t, []byte("anything"), a, m)) + ">\n"
	k := maxBoundaryScreens + 1
	pad := strings.Repeat("% padding line .......................................\n", (1<<20)/56)
	build := func(ends []int64) []byte {
		var sb strings.Builder
		sb.WriteString(pad)
		for i := 0; i < k; i++ {
			fmt.Fprintf(&sb, "%06d 0 obj\n<</Type/XRef>>%s", 2000+i, contents)
			fmt.Fprintf(&sb, "/ByteRange [0 10 %012d 1]\nstartxref\n%012d\n%%%%EOF\n", ends[i]-1, 0)
		}
		out := synthRevision(t, doc, []sobj{{num: 990, body: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", sb.Len(), sb.String())}}, catalogOf(t, doc))
		for i, at := 0, 0; i < k; i++ { // each marker names its own fake header
			h := bytes.Index(out[at:], []byte(fmt.Sprintf("%06d 0 obj", 2000+i))) + at
			sx := bytes.Index(out[h:], []byte("startxref\n")) + h + 10
			copy(out[sx:sx+12], fmt.Sprintf("%012d", h))
			at = sx
		}
		return out
	}
	first := build(make([]int64, k))
	var ends []int64
	for i := len(doc) + len(pad); len(ends) < k; {
		j := bytes.Index(first[i:], []byte("%%EOF\n")) + i
		ends = append(ends, int64(j+6))
		i = j + 6
	}
	hostile := build(ends)
	all := rawByteRangeEnds(hostile)
	for _, e := range ends {
		if len(all[e]) != 1 {
			t.Fatalf("stimulus: %d literals end at fake boundary %d, want one", len(all[e]), e)
		}
	}
	if n := len(boundariesOf(hostile)); n < k {
		t.Fatalf("stimulus: %d boundaries in the hostile file, fewer than the %d fakes", n, k)
	}
	got := SignedRevisionFor(hostile, a.fp)
	if !bytes.Equal(got.Prefix, sA) {
		t.Fatalf("cause %q, %d bytes: %d fake sections with a forged SignerInfo crowded out the version", got.Cause, len(got.Prefix), k)
	}
	verifies, budget := 0, int64(16*len(hostile))
	if _, _, screens := walkBoundariesCounted(hostile, a.fp, &verifies, &budget); screens != 1 {
		t.Errorf("%d pdfcpu reads; want 1 — the forged fakes fail the prescreen, the signer's version does not", screens)
	}
}

// TestTheBoundaryWalkHoldsASmallMultipleOfTheFile — the walk's two whole-file scans kept every literal ByteRange and
// every marker uncharged (P02's phase-close review): measured 1.7 GB allocated, ~907 MB peak, for 64 MB of literals,
// and 790 MB allocated for 64 MB of distinct xref markers. Allocation is counted (deterministic, unlike a peak or a
// time under load), on the two shapes that cost most, each against its own stimulus.
func TestTheBoundaryWalkHoldsASmallMultipleOfTheFile(t *testing.T) {
	const size = 8 << 20
	const ceiling = 8 // × the file; measured ~4× (literals) and ~5.3× (markers), against ~27× and ~12× before
	head := "%PDF-1.7\n1 0 obj\n<<>>\nendobj\n"
	xref := head + "xref\n0 1\n0000000000 65535 f \ntrailer\n<<>>\nstartxref\n" + strconv.Itoa(len(head)) + "\n%%EOF\n"
	lit := fmt.Sprintf("/ByteRange[0 1 2 %d]", len(xref)-2)
	literals := []byte(xref + strings.Repeat(lit, size/len(lit)))
	var mb bytes.Buffer
	mb.WriteString("%PDF-1.7\n")
	for mb.Len() < size {
		fmt.Fprintf(&mb, "xref\nstartxref\n%d\n%%%%EOF\n", mb.Len())
	}
	markers := mb.Bytes()
	if n := len(rawByteRangeEnds(literals)[int64(len(xref))]); n < size/len(lit) {
		t.Fatalf("stimulus: %d literals end at the boundary, not every one", n)
	}
	if n := len(boundariesOf(markers)); n < size/32 {
		t.Fatalf("stimulus: %d boundaries among the markers", n)
	}
	for name, doc := range map[string][]byte{"literals ending at a boundary": literals, "distinct xref markers": markers} {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		verifies, budget := 0, int64(screenBudgetFactor)*int64(len(doc))
		t0 := time.Now()
		walkBoundariesCounted(doc, "ab", &verifies, &budget)
		d := time.Since(t0)
		runtime.ReadMemStats(&after)
		if got := after.TotalAlloc - before.TotalAlloc; got > ceiling*uint64(len(doc)) {
			t.Errorf("%s: the walk allocated %d MB over a %d MB file, more than %d× (%v)", name, got>>20, len(doc)>>20, ceiling, d)
		}
	}
	// Every byte the scans look at is charged to the shared budget, as the prescreen's own bytes are. One boundary, one
	// literal ending at it, and a long tail neither scan finds anything in: both passes must be paid for.
	one := []byte(xref + "2 0 obj\n<< " + lit + " >>\nendobj\n" + strings.Repeat(" ", size))
	if len(rawByteRangeEnds(one)[int64(len(xref))]) != 1 || len(boundariesOf(one)) == 0 {
		t.Fatal("stimulus: the file has no boundary with a literal ending at it, so the literal scan never runs")
	}
	verifies, budget := 0, int64(1<<40)
	walkBoundariesCounted(one, "ab", &verifies, &budget)
	if spent := int64(1<<40) - budget; spent < 2*int64(len(one)) {
		t.Errorf("the walk charged %d bytes for two passes over a %d-byte file", spent, len(one))
	}
	// Markers naming one ordinary object each re-read its first KB (the review: 64 MB of them, 3.0 s uncharged). Charged,
	// they spend the budget, and the walk says it could not finish rather than that nothing was there.
	var nb bytes.Buffer
	nb.WriteString("%PDF-1.7\n1 0 obj\n<<" + strings.Repeat("A", 1100))
	for nb.Len() < size {
		nb.WriteString("startxref\n9\n%%EOF\n")
	}
	nonXref := nb.Bytes()
	verifies, budget = 0, int64(screenBudgetFactor)*int64(len(nonXref))
	if _, cut, _ := walkBoundariesCounted(nonXref, "ab", &verifies, &budget); !cut {
		t.Error("markers each re-reading an object's first KB did not spend the budget: the look at the offset is not charged")
	}
}
