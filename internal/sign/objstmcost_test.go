package sign

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dpdf "github.com/digitorus/pdf"
)

// memberDoc is the honest synthetic signed document with n more objects, each a tiny dictionary, in
// ONE object stream — the /pending 751 shape.
func memberDoc(t *testing.T, id identity, n int) []byte {
	t.Helper()
	objs := baseObjs(sigDict("1", ""))
	for i := 0; i < n; i++ {
		objs = append(objs, sobj{num: 6 + i, body: "<</F 1>>", inStm: true})
	}
	return fillSig(t, synthRevision(t, nil, objs, 1), "1", nil, detached(t, id))
}

// streamDoc is an unsigned one-page document whose objects 4..4+m-1 sit in one flate object stream,
// member 4 a pad array of padLen bytes. claimN is added to the header's true `/N`, and phantom adds
// an xref entry naming the stream for an id its header does not list.
func streamDoc(padLen, m int, claimN int, phantom bool) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	off := map[int]int{}
	for _, o := range []struct {
		n int
		s string
	}{{1, "<</Type/Catalog/Pages 2 0 R>>"}, {2, "<</Type/Pages/Kids[3 0 R]/Count 1>>"}, {3, "<</Type/Page/Parent 2 0 R/MediaBox[0 0 9 9]>>"}} {
		off[o.n] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", o.n, o.s)
	}
	var hdr, content strings.Builder
	for i := 0; i < m; i++ {
		fmt.Fprintf(&hdr, "%d %d ", 4+i, content.Len())
		if i == 0 {
			content.WriteString("[" + strings.Repeat("0 ", padLen/2) + "]\n")
		} else {
			content.WriteString("<</F 1>>\n")
		}
	}
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	w.Write([]byte(hdr.String() + content.String()))
	w.Close()
	stm := 4 + m
	if phantom {
		stm++ // object 4+m is the phantom: listed in the xref as in the stream, absent from its header
	}
	off[stm] = b.Len()
	fmt.Fprintf(&b, "%d 0 obj\n<</Type/ObjStm/N %d/First %d/Filter/FlateDecode/Length %d>>\nstream\n", stm, m+claimN, hdr.Len(), z.Len())
	b.Write(z.Bytes())
	b.WriteString("\nendstream\nendobj\n")
	x := stm + 1
	off[x] = b.Len()
	var data bytes.Buffer
	for n := 0; n <= x; n++ {
		switch {
		case n == 0:
			data.Write([]byte{0, 0, 0, 0, 0, 0xff, 0xff})
		case n >= 4 && n < stm:
			i := n - 4
			data.Write([]byte{2, 0, 0, byte(stm >> 8), byte(stm), byte(i >> 8), byte(i)})
		default:
			o := off[n]
			data.Write([]byte{1, byte(o >> 24), byte(o >> 16), byte(o >> 8), byte(o), 0, 0})
		}
	}
	fmt.Fprintf(&b, "%d 0 obj\n<</Type/XRef/Size %d/W[1 4 2]/Root 1 0 R/Length %d>>\nstream\n", x, x+1, data.Len())
	b.Write(data.Bytes())
	fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", off[x])
	return b.Bytes()
}

func lookupCostOf(t *testing.T, doc []byte) (pairs, read int64, err error) {
	t.Helper()
	r, err := dpdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatalf("STIMULUS: the reader refuses the fixture: %v", err)
	}
	return libraryLookupCost(r)
}

// TestAnObjectStreamTheLibraryWouldReadQuadraticallyNeverReachesIt is /pending 751's own case. The
// signature reader re-decodes an object stream and re-lexes its header for every member it resolves,
// and the sweep and the library each resolve every xref object: measured on the pre-fix tree, this
// 561 KB file — one honest signature, one 20,000-member stream — took `Verify` 129 s and read `valid`.
// It must be refused before either pass pays it: `Invalid`, `could-not-check`, the library never
// called, `Revisions` naming the ceiling, and in well under a second of the reader's time.
func TestAnObjectStreamTheLibraryWouldReadQuadraticallyNeverReachesIt(t *testing.T) {
	doc := memberDoc(t, newIdentity(t, "Alice"), 20000)
	if err := pdfcpuCanRead(doc); err != nil || !scanForSignatureBlob(doc) {
		t.Fatalf("STIMULUS: pdfcpu err=%v, blob=%v — the file must reach the sweep", err, scanForSignatureBlob(doc))
	}
	calls := countingLibrary(t)
	t0 := time.Now()
	st := Verify(doc)
	took := time.Since(t0)
	if st.State != Invalid || !st.AddedAfter || st.AddedAfterCause != AddedAfterCouldNotCheck {
		t.Errorf("state=%q addedAfter=%v(%q), want invalid/could-not-check — a verdict nib took %v to reach over a reader it cannot afford",
			st.State, st.AddedAfter, st.AddedAfterCause, took)
	}
	if *calls != 0 {
		t.Errorf("the library was called %d time(s) on a document whose object stream costs it quadratically", *calls)
	}
	if _, err := Revisions(doc); !errors.Is(err, errLookupCostCeiling) {
		t.Errorf("Revisions error %v, want errLookupCostCeiling", err)
	}
	if took > 5*time.Second {
		t.Errorf("Verify took %v on a 20,000-member object stream; refused, it costs milliseconds", took)
	}
}

// TestVerifyIsNotQuadraticInObjectStreamMembers: four times the members must cost well under sixteen
// times the time. Best of three per size, so a loaded machine cannot fake a pass or a failure; the
// pre-fix tree measured ~16x (2,000 → 8,000 members: 1.1 s → 17 s).
func TestVerifyIsNotQuadraticInObjectStreamMembers(t *testing.T) {
	a := newIdentity(t, "Alice")
	best := func(n int) time.Duration {
		doc := memberDoc(t, a, n)
		var b time.Duration
		for i := 0; i < 3; i++ {
			t0 := time.Now()
			Verify(doc)
			if d := time.Since(t0); i == 0 || d < b {
				b = d
			}
		}
		return b
	}
	small, large := best(2000), best(8000)
	if ratio := float64(large) / float64(small); ratio > 6 {
		t.Errorf("4x the object-stream members cost %.1fx the time (%v → %v) — the reader's lookup is quadratic and nothing bounds it", ratio, small, large)
	}
}

// TestTheLookupCostChargesWhatTheLibraryReads pins the figure to the library's loop (`read.go:891-898`):
// a member at header position p costs p+1 lexed pairs and the bytes up to it, and a member the header
// does not list costs every pair `/N` CLAIMS — the library lexes that many, at EOF or not — and the
// whole stream. A `/N` of two billion over a three-member header is the attacker's version of the miss.
func TestTheLookupCostChargesWhatTheLibraryReads(t *testing.T) {
	pairs, _, err := lookupCostOf(t, memberDoc(t, newIdentity(t, "Alice"), 100))
	if err != nil || pairs != 100*101/2 {
		t.Errorf("100 members: pairs=%d err=%v, want %d and no error", pairs, err, 100*101/2)
	}
	pairs, read, err := lookupCostOf(t, streamDoc(64, 3, 0, true))
	if err != nil || pairs != (1+2+3)+3 || read < 64 {
		t.Errorf("a phantom member of a 3-member stream: pairs=%d read=%d err=%v, want %d pairs, the whole stream read, no error", pairs, read, err, 1+2+3+3)
	}
	if _, _, err := lookupCostOf(t, streamDoc(64, 3, 2_000_000_000, true)); !errors.Is(err, errLookupCostCeiling) {
		t.Errorf("a phantom member under a /N of two billion: err=%v, want errLookupCostCeiling", err)
	}
	// A pad the members after it must read past, times the members: the byte half of the figure.
	if _, _, err := lookupCostOf(t, streamDoc(16<<20, 50, 0, false)); !errors.Is(err, errLookupCostCeiling) {
		t.Errorf("49 members behind a 16 MiB pad: err=%v, want errLookupCostCeiling", err)
	}
}

// TestNoProducerDocumentReachesTheLookupCeiling holds the ceiling above every real producer's output:
// a refusal is `Invalid`, so a ceiling below an honest document would call it tampered.
func TestNoProducerDocumentReachesTheLookupCeiling(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("SKIP (not a pass): no home directory: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(home, "nib", "producers", "*", "*.pdf"))
	if len(files) == 0 {
		t.Skip("SKIP (not a pass): the real-producer corpus is absent")
	}
	var worst int64
	worstName := ""
	for _, f := range files {
		doc, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		pairs, read, err := lookupCostOf(t, doc)
		if err != nil {
			t.Errorf("%s: %v — an honest producer's file is refused", f, err)
		}
		if w := pairs*lookupPairWeight + read*lookupByteWeight; w > worst {
			worst, worstName = w, filepath.Base(f)
		}
	}
	t.Logf("%d files; the dearest is %s at %.2f of the ceiling", len(files), worstName, float64(worst)/maxLookupWork)
}
