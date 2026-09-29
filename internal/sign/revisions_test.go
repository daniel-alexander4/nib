package sign

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	dpdf "github.com/digitorus/pdf"
	"github.com/digitorus/pdfsign/verify"
	"github.com/digitorus/pkcs7"

	"nib/internal/testpdf"
)

// ---------------------------------------------------------------------------------------------
// Synthetic signed documents.
//
// pdfsign lays out every signature one way, so a fixture for "the gap is some other hex token" or
// "the dictionary lives in an object stream" cannot be made by signing and then editing: the edit
// moves offsets the signature covers. These are built from scratch instead — each object placed
// by hand, the xref written as an uncompressed xref stream (digitorus cannot follow a classic
// `xref` whose `/Prev` names a stream, and pdfsign writes streams), the `/ByteRange` filled into a
// fixed-width slot after layout, and the `/Contents` a real detached PKCS#7 over the honest ranges.
// ---------------------------------------------------------------------------------------------

const synthHexLen = 4096

type sobj struct {
	num   int
	body  string
	inStm bool
}

// brSlot is a /ByteRange placeholder `width` characters wide, filled by fillSig.
func brSlot(id string, width int) string { return fmt.Sprintf("%-*s", width, "@BR"+id+"@") }

// contentsSlot is a hex-string placeholder for a signature blob, filled by fillSig.
func contentsSlot(id string) string {
	m := "@C" + id + "@"
	return "<" + m + strings.Repeat("0", synthHexLen-len(m)) + ">"
}

// sigDict is a pdfsign-shaped signature dictionary: /ByteRange before /Contents, then extra.
func sigDict(id, extra string) string {
	return "<</Type/Sig/Filter/Adobe.PPKLite/SubFilter/adbe.pkcs7.detached/ByteRange[" + brSlot(id, 64) +
		"]/Contents" + contentsSlot(id) + extra + ">>"
}

// baseObjs is a one-page document whose AcroForm lists field 4, which points at signature 5.
func baseObjs(sig string) []sobj {
	return []sobj{
		{num: 1, body: "<</Type/Catalog/Pages 2 0 R/AcroForm<</SigFlags 3/Fields[4 0 R]>>>>"},
		{num: 2, body: "<</Type/Pages/Kids[3 0 R]/Count 1>>"},
		{num: 3, body: "<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]>>"},
		{num: 4, body: "<</FT/Sig/T(Signature1)/V 5 0 R>>"},
		{num: 5, body: sig},
	}
}

var reStartxrefTail = regexp.MustCompile(`startxref\s+(\d+)`)

func lastStartxref(t *testing.T, b []byte) int {
	t.Helper()
	all := reStartxrefTail.FindAllSubmatch(b, -1)
	if len(all) == 0 {
		t.Fatal("no startxref")
	}
	n, err := strconv.Atoi(string(all[len(all)-1][1]))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// synthRevision writes objs as one revision — the first when prev is nil, an incremental update of
// prev otherwise — with an uncompressed xref stream. Objects marked inStm go into one uncompressed
// object stream, so their bytes are literally in the file but have no header of their own.
func synthRevision(t *testing.T, prev []byte, objs []sobj, root int) []byte {
	t.Helper()
	var b bytes.Buffer
	prevX, prevSize := -1, 0
	if prev == nil {
		b.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	} else {
		prevX = lastStartxref(t, prev)
		r, err := dpdf.NewReader(bytes.NewReader(prev), int64(len(prev)))
		if err != nil {
			t.Fatalf("read previous revision: %v", err)
		}
		prevSize = int(r.Trailer().Key("Size").Int64())
		b.Write(prev)
		b.WriteString("\n")
	}
	type ent struct{ typ, a, c int }
	ents := map[int]ent{}
	next := prevSize
	var stm []sobj
	for _, o := range objs {
		if o.num >= next {
			next = o.num + 1
		}
		if o.inStm {
			stm = append(stm, o)
			continue
		}
		ents[o.num] = ent{1, b.Len(), 0}
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", o.num, o.body)
	}
	if len(stm) > 0 {
		stmNum := next
		next++
		var hdr, content strings.Builder
		for i, o := range stm {
			fmt.Fprintf(&hdr, "%d %d ", o.num, content.Len())
			content.WriteString(o.body)
			content.WriteString("\n")
			ents[o.num] = ent{2, stmNum, i}
		}
		data := hdr.String() + content.String()
		ents[stmNum] = ent{1, b.Len(), 0}
		fmt.Fprintf(&b, "%d 0 obj\n<</Type/ObjStm/N %d/First %d/Length %d>>\nstream\n%s\nendstream\nendobj\n",
			stmNum, len(stm), hdr.Len(), len(data), data)
	}
	xnum := next
	size := xnum + 1
	ents[xnum] = ent{1, b.Len(), 0}
	var nums []int
	if prev == nil {
		for n := 0; n < size; n++ {
			nums = append(nums, n)
		}
	} else {
		for n := range ents {
			nums = append(nums, n)
		}
		sort.Ints(nums)
	}
	var index strings.Builder
	var data bytes.Buffer
	for i := 0; i < len(nums); {
		j := i
		for j+1 < len(nums) && nums[j+1] == nums[j]+1 {
			j++
		}
		fmt.Fprintf(&index, "%d %d ", nums[i], j-i+1)
		for _, n := range nums[i : j+1] {
			e, ok := ents[n]
			if !ok {
				e = ent{0, 0, 0}
				if n == 0 {
					e.c = 65535
				}
			}
			data.Write([]byte{byte(e.typ), byte(e.a >> 24), byte(e.a >> 16), byte(e.a >> 8), byte(e.a), byte(e.c >> 8), byte(e.c)})
		}
		i = j + 1
	}
	prevKey := ""
	if prevX >= 0 {
		prevKey = fmt.Sprintf("/Prev %d", prevX)
	}
	fmt.Fprintf(&b, "%d 0 obj\n<</Type/XRef/Size %d/W[1 4 2]/Index[%s]/Root %d 0 R%s/Length %d>>\nstream\n",
		xnum, size, strings.TrimSpace(index.String()), root, prevKey, data.Len())
	b.Write(data.Bytes())
	fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", ents[xnum].a)
	return b.Bytes()
}

// geo is what a /ByteRange function may ask of the laid-out document.
type geo struct {
	gs, ge, size int64 // the contents slot's '<', one past its '>', the file length
	doc          []byte
}

// at is the offset of s in the document; the fixture fails if s is absent.
func (g geo) at(t *testing.T, s string) int64 {
	t.Helper()
	i := bytes.Index(g.doc, []byte(s))
	if i < 0 {
		t.Fatalf("fixture: %q not in document", s)
	}
	return int64(i)
}

func honestBR(g geo) string { return ints(0, g.gs, g.ge, g.size-g.ge) }

func ints(vs ...int64) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = strconv.FormatInt(v, 10)
	}
	return strings.Join(parts, " ")
}

// fillSig fills signature id's /ByteRange with br(geo) and its /Contents with blob(content), where
// content is the HONEST ranges — so the signature verifies exactly when the /ByteRange is the
// honest one or an equivalent (abutting) spelling of it.
func fillSig(t *testing.T, doc []byte, id string, br func(geo) string, blob func([]byte) []byte) []byte {
	t.Helper()
	out := append([]byte(nil), doc...)
	cm := []byte("@C" + id + "@")
	p := bytes.Index(out, cm)
	if p < 0 {
		t.Fatalf("fixture: no contents slot %q", cm)
	}
	gs := int64(p - 1)
	ge := gs + synthHexLen + 2
	copy(out[p:], bytes.Repeat([]byte("0"), len(cm)))
	g := geo{gs: gs, ge: ge, size: int64(len(out)), doc: out}
	if br == nil {
		br = honestBR
	}
	text := br(g)
	sm := []byte("@BR" + id + "@")
	q := bytes.Index(out, sm)
	if q < 0 {
		t.Fatalf("fixture: no byte-range slot %q", sm)
	}
	width := bytes.IndexByte(out[q:], ']')
	if width < 0 || len(text) > width {
		t.Fatalf("fixture: byte range %d chars does not fit its %d-char slot", len(text), width)
	}
	copy(out[q:q+width], fmt.Sprintf("%-*s", width, text))
	if blob != nil {
		content := append(append([]byte(nil), out[:gs]...), out[ge:]...)
		h := hex.EncodeToString(blob(content))
		if len(h) > synthHexLen {
			t.Fatalf("fixture: blob %d hex chars exceeds the %d-char slot", len(h), synthHexLen)
		}
		copy(out[gs+1:], h)
	}
	return out
}

type identity struct {
	certPEM, keyPEM []byte
	cert            *x509.Certificate
	signer          crypto.Signer
	fp              string
}

func newIdentity(t *testing.T, cn string) identity {
	t.Helper()
	c, k, err := GenerateIdentity(cn)
	if err != nil {
		t.Fatal(err)
	}
	cert, signer, err := ParseIdentity(c, k)
	if err != nil {
		t.Fatal(err)
	}
	return identity{c, k, cert, signer, fingerprintHex(t, c)}
}

// detached signs content as a detached PKCS#7 by id, carrying extra certificates after its own.
func detached(t *testing.T, id identity, extra ...*x509.Certificate) func([]byte) []byte {
	return func(content []byte) []byte {
		t.Helper()
		sd, err := pkcs7.NewSignedData(content)
		if err != nil {
			t.Fatal(err)
		}
		sd.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
		if err := sd.AddSigner(id.cert, id.signer, pkcs7.SignerInfoConfig{}); err != nil {
			t.Fatal(err)
		}
		for _, c := range extra {
			sd.AddCertificate(c)
		}
		sd.Detach()
		der, err := sd.Finish()
		if err != nil {
			t.Fatal(err)
		}
		return der
	}
}

// synthSigned is the honest one-signature synthetic document, signed by id.
func synthSigned(t *testing.T, id identity) []byte {
	t.Helper()
	return fillSig(t, synthRevision(t, nil, baseObjs(sigDict("1", "")), 1), "1", nil, detached(t, id))
}

func recordFor(t *testing.T, revs []Revision, obj uint32) Revision {
	t.Helper()
	for _, r := range revs {
		if r.Obj == obj {
			return r
		}
	}
	t.Fatalf("no record for object %d among %d records", obj, len(revs))
	return Revision{}
}

func mustSweep(t *testing.T, doc []byte) []Revision {
	t.Helper()
	revs, err := sweepRevisions(doc)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	return revs
}

// TestTheHonestSyntheticSignatureIsWellFormedAndVerified is the control every negative fixture
// below stands on: the builder's own honest output passes all eleven conjuncts, verifies, and
// names its signer. Without it, a refusal below could be the builder's fault.
func TestTheHonestSyntheticSignatureIsWellFormedAndVerified(t *testing.T) {
	a := newIdentity(t, "Alice")
	doc := synthSigned(t, a)
	st := Verify(doc)
	if st.State != Valid || len(st.Signers) != 1 || st.Signers[0].Fingerprint != a.fp || st.AddedAfter {
		t.Fatalf("honest synthetic: state=%s signers=%d fp=%q addedAfter=%v, want valid/1/%q/false",
			st.State, len(st.Signers), firstSignerFP(st), st.AddedAfter, a.fp)
	}
	revs, err := Revisions(doc)
	if err != nil {
		t.Fatal(err)
	}
	r := recordFor(t, revs, 5)
	if r.Cause != "" || r.conjunct != 0 || !r.Verified || r.Fingerprint != a.fp || r.CoverageEnd != int64(len(doc)) {
		t.Errorf("honest record = cause %q conjunct %d verified %v fp %q end %d, want well-formed, verified, %q, %d",
			r.Cause, r.conjunct, r.Verified, r.Fingerprint, r.CoverageEnd, a.fp, len(doc))
	}
	if r.Type != "Sig" || r.Filter != "Adobe.PPKLite" || r.SubFilter != "adbe.pkcs7.detached" || len(r.ByteRange) != 4 {
		t.Errorf("record names = %q/%q/%q, ByteRange %v", r.Type, r.Filter, r.SubFilter, r.ByteRange)
	}
}

// TestEachStructureConjunctRefusesItsOwnFixture — one negative fixture per conjunct of the
// eleven-conjunct rule (PLAN-returned-document P01.S01), each asserting WHICH conjunct fired, not
// only the cause: `malformed-byterange` lumps nine of them, so a fixture built for one and refused
// by another would pass on the cause alone.
//
// Each fixture's STIMULUS is asserted before its response: the byte range the fixture meant to
// write is the one in the record, so a builder that silently wrote an honest range cannot pass.
func TestEachStructureConjunctRefusesItsOwnFixture(t *testing.T) {
	a := newIdentity(t, "Alice")
	sign := detached(t, a)
	// written is the byte-range text the last fixture's builder wrote into its slot; `wrote`
	// turns it into the stimulus a case asserts, so a builder that silently wrote an honest range
	// cannot pass.
	var written string
	recording := func(br func(geo) string) func(geo) string {
		if br == nil {
			br = honestBR
		}
		return func(g geo) string { written = br(g); return written }
	}
	wrote := func() func([]byte) []int64 {
		fields := strings.Fields(written)
		want := make([]int64, len(fields))
		for i, f := range fields {
			n, err := strconv.ParseInt(f, 10, 64)
			if err != nil {
				t.Fatalf("fixture: byte range %q is not all integers", written)
			}
			want[i] = n
		}
		return func([]byte) []int64 { return want }
	}
	one := func(sig string, extra []sobj, br func(geo) string, post func([]byte) []byte) []byte {
		objs := append(baseObjs(sig), extra...)
		doc := fillSig(t, synthRevision(t, nil, objs, 1), "1", recording(br), sign)
		if post != nil {
			doc = post(doc)
		}
		return doc
	}
	std := sigDict("1", "")
	cases := []struct {
		name     string
		doc      []byte
		conjunct int
		cause    RefusalCause
		// wantBR is the stimulus: the record must carry exactly this byte range.
		wantBR func(doc []byte) []int64
	}{}
	add := func(name string, conj int, cause RefusalCause, doc []byte, want func(doc []byte) []int64) {
		cases = append(cases, struct {
			name     string
			doc      []byte
			conjunct int
			cause    RefusalCause
			wantBR   func(doc []byte) []int64
		}{name, doc, conj, cause, want})
	}
	slotGap := func(doc []byte) (gs, ge int64) {
		i := bytes.Index(doc, []byte("/Contents<"))
		return int64(i + len("/Contents")), int64(i+len("/Contents")) + synthHexLen + 2
	}
	size := func(doc []byte) int64 { return int64(len(doc)) }

	// (1) odd length, a real element, too short, and an indirect array.
	d := one(std, nil, func(g geo) string { return ints(0, g.gs, g.ge, g.size-g.ge, 7) }, nil)
	add("(1) five elements", 1, CauseMalformedByteRange, d, func(doc []byte) []int64 {
		gs, ge := slotGap(doc)
		return []int64{0, gs, ge, size(doc) - ge, 7}
	})
	d = one(std, nil, func(g geo) string { return fmt.Sprintf("0 %d %d %d.0", g.gs, g.ge, g.size-g.ge) }, nil)
	// The record reads a real as the library does (`Int64()` of a real is 0), so the stimulus is
	// the integers as read — and, below, that element 3 really is a real in an otherwise
	// well-shaped array, so conjunct (1) fires on the integer sub-condition and not on another.
	realDoc := d
	add("(1) a real where an integer belongs", 1, CauseMalformedByteRange, d, func(doc []byte) []int64 {
		gs, ge := slotGap(doc)
		return []int64{0, gs, ge, 0}
	})
	d = one(std, nil, func(g geo) string { return ints(0, g.size) }, nil)
	add("(1) one pair", 1, CauseMalformedByteRange, d, wrote())
	d = one(strings.Replace(std, "/ByteRange["+brSlot("1", 64)+"]", "/ByteRange 6 0 R", 1),
		[]sobj{{num: 6, body: "[" + brSlot("1", 64) + "]"}}, nil, nil)
	add("(1) an indirect array", 1, CauseMalformedByteRange, d, func(doc []byte) []int64 {
		gs, ge := slotGap(doc)
		return []int64{0, gs, ge, size(doc) - ge}
	})
	// (2) the first start is 1.
	d = one(std, nil, func(g geo) string { return ints(1, g.gs-1, g.ge, g.size-g.ge) }, nil)
	add("(2) first start 1", 2, CauseMalformedByteRange, d, func(doc []byte) []int64 {
		gs, ge := slotGap(doc)
		return []int64{1, gs - 1, ge, size(doc) - ge}
	})
	// (3) a zero-length pair: the tail after the gap is covered by nothing.
	d = one(std, nil, func(g geo) string { return ints(0, g.gs, g.ge, 0) }, nil)
	add("(3) zero-length pair", 3, CauseMalformedByteRange, d, func(doc []byte) []int64 {
		gs, ge := slotGap(doc)
		return []int64{0, gs, ge, 0}
	})
	// (4) starts that do not ascend.
	d = one(std, nil, func(g geo) string { return ints(0, g.gs, g.ge, g.size-g.ge, 0, 1) }, nil)
	add("(4) a start below its predecessor", 4, CauseMalformedByteRange, d, wrote())
	// (5) an overlapping pair, and a second gap.
	// The overlap sits beside one real gap, so it is the overlap alone that is wrong: without the
	// overlap check the gap count is still one.
	d = one(std, nil, func(g geo) string { return ints(0, g.gs, g.ge, 100, g.ge+90, g.size-g.ge-90) }, nil)
	add("(5) overlapping pair", 5, CauseMalformedByteRange, d, func(doc []byte) []int64 {
		gs, ge := slotGap(doc)
		return []int64{0, gs, ge, 100, ge + 90, size(doc) - ge - 90}
	})
	d = one(std, nil, func(g geo) string { return ints(0, g.gs, g.ge, 5, g.ge+10, g.size-g.ge-10) }, nil)
	add("(5) two gaps", 5, CauseMalformedByteRange, d, wrote())
	// (6) the last pair ends one byte past EOF.
	d = one(std, nil, func(g geo) string { return ints(0, g.gs, g.ge, g.size-g.ge+1) }, nil)
	add("(6) one byte past EOF", 6, CauseByteRangeOutsideFile, d, func(doc []byte) []int64 {
		gs, ge := slotGap(doc)
		return []int64{0, gs, ge, size(doc) - ge + 1}
	})
	// (7) a /Reason injected into the gap after an early '>': the hole now holds a key the
	// signature does not cover.
	inj := sigDict("1", "/Reason(injected)/X<00>")
	d = one(inj, nil, func(g geo) string {
		end := g.at(t, "/X<00>") + int64(len("/X<00>"))
		return ints(0, g.gs, end, g.size-end)
	}, nil)
	add("(7) key injected after an early '>'", 7, CauseMalformedByteRange, d, wrote())
	// (8) the gap is a hex token, but not this dictionary's /Contents.
	pad := sigDict("1", "/Pad<00112233>")
	d = one(pad, nil, func(g geo) string {
		s := g.at(t, "/Pad<") + int64(len("/Pad"))
		return ints(0, s, s+10, g.size-s-10)
	}, nil)
	add("(8) gap at another hex token", 8, CauseMalformedByteRange, d, wrote())
	// (9) /Contents is an indirect string: the gap is object 6's bytes.
	ind := "<</Type/Sig/Filter/Adobe.PPKLite/SubFilter/adbe.pkcs7.detached/ByteRange[" + brSlot("1", 64) + "]/Contents 6 0 R>>"
	d = one(ind, []sobj{{num: 6, body: contentsSlot("1")}}, nil, nil)
	add("(9) indirect /Contents", 9, CauseMalformedByteRange, d, wrote())
	// (10) the dictionary lives in an (uncompressed) object stream, so its hex is in the file but
	// has no header of its own.
	objs := baseObjs(std)
	objs[4].inStm = true
	d = fillSig(t, synthRevision(t, nil, objs, 1), "1", recording(nil), sign)
	add("(10) dictionary in an object stream", 10, CauseMalformedByteRange, d, wrote())
	// (11) the gap is a byte-identical copy of /Contents under another key.
	dup := sigDict("1", "/Foo"+contentsSlot("dup"))
	d = one(dup, nil, func(g geo) string {
		s := g.at(t, "/Foo<") + int64(len("/Foo"))
		return ints(0, s, s+synthHexLen+2, g.size-s-synthHexLen-2)
	}, func(doc []byte) []byte {
		i := bytes.Index(doc, []byte("/Contents<")) + len("/Contents")
		j := bytes.Index(doc, []byte("/Foo<")) + len("/Foo")
		copy(doc[j:j+synthHexLen+2], doc[i:i+synthHexLen+2])
		return doc
	})
	add("(11) the gap is preceded by another key", 11, CauseContentsElsewhere, d, wrote())
	// (11) header-shaped text between the header and /Contents names another object — the G10
	// residual's shape, refused (attributed to object 9, not 5).
	g10 := "<</Type/Sig/Filter/Adobe.PPKLite/SubFilter/adbe.pkcs7.detached/Reason(see 9 0 obj)/ByteRange[" +
		brSlot("1", 64) + "]/Contents" + contentsSlot("1") + ">>"
	d = one(g10, nil, nil, nil)
	add("(11) a nearer header names another object", 11, CauseContentsElsewhere, d, wrote())

	// STIMULUS for the real case: a direct four-element array whose last element is a real, so
	// the only (1) sub-condition it can fail is "all integers".
	{
		rr, err := dpdf.NewReader(bytes.NewReader(realDoc), int64(len(realDoc)))
		if err != nil {
			t.Fatal(err)
		}
		var v dpdf.Value
		for _, x := range rr.Xref() {
			if p := x.Ptr(); p.GetID() == 5 {
				v = rr.Resolve(p, p)
			}
		}
		br := v.Key("ByteRange")
		if br.Kind() != dpdf.Array || br.GetPtr() != v.GetPtr() || br.Len() != 4 || br.Index(3).Kind() != dpdf.Real {
			t.Fatalf("STIMULUS: the real fixture's /ByteRange is kind %v, %d elements, element 3 kind %v — not a direct 4-array ending in a real",
				br.Kind(), br.Len(), br.Index(3).Kind())
		}
	}

	for _, tc := range cases {
		revs := mustSweep(t, tc.doc)
		r := recordFor(t, revs, 5)
		if want := tc.wantBR(tc.doc); fmt.Sprint(r.ByteRange) != fmt.Sprint(want) {
			t.Errorf("%s: STIMULUS: record's byte range %v, the fixture meant %v", tc.name, r.ByteRange, want)
			continue
		}
		if r.libPos < 0 {
			t.Errorf("%s: STIMULUS: the record is outside the library's enumeration (PPKLite + parseable), "+
				"so a refusal would not be the structure rule's", tc.name)
		}
		if r.conjunct != tc.conjunct || r.Cause != tc.cause {
			t.Errorf("%s: conjunct %d cause %q, want conjunct %d cause %q", tc.name, r.conjunct, r.Cause, tc.conjunct, tc.cause)
		}
		if r.CoverageEnd != 0 {
			t.Errorf("%s: a refused record reports coverage to %d", tc.name, r.CoverageEnd)
		}
	}
}

// TestAnAbuttingSixElementByteRangeEndsAtItsLastPair — D5: coverage ends at Index(n-2)+Index(n-1),
// not at Index(2)+Index(3), and abutting pairs are one gap, not three.
func TestAnAbuttingSixElementByteRangeEndsAtItsLastPair(t *testing.T) {
	a := newIdentity(t, "Alice")
	doc := fillSig(t, synthRevision(t, nil, baseObjs(sigDict("1", "")), 1), "1", func(g geo) string {
		return ints(0, g.gs, g.ge, 100, g.ge+100, g.size-g.ge-100)
	}, detached(t, a))
	revs, err := Revisions(doc)
	if err != nil {
		t.Fatal(err)
	}
	r := recordFor(t, revs, 5)
	br := r.ByteRange
	// STIMULUS: six elements, and a reader of elements 2 and 3 would stop 100 bytes past the gap.
	if len(br) != 6 || br[2]+br[3] >= br[4]+br[5] {
		t.Fatalf("STIMULUS: byte range %v is not a six-element range whose last pair ends later", br)
	}
	if r.Cause != "" || !r.Verified {
		t.Fatalf("abutting range: cause %q (conjunct %d) verified %v, want well-formed and verified", r.Cause, r.conjunct, r.Verified)
	}
	if r.CoverageEnd != br[4]+br[5] || r.CoverageEnd != int64(len(doc)) {
		t.Errorf("coverage end %d, want Index(4)+Index(5) = %d", r.CoverageEnd, br[4]+br[5])
	}
}

// TestAKidsNestedSignatureIsSeen — D6, met by construction: the sweep reads the xref, never
// `/Fields`, so a signature under a parent field's `/Kids` is a record like any other.
func TestAKidsNestedSignatureIsSeen(t *testing.T) {
	a := newIdentity(t, "Alice")
	objs := baseObjs(sigDict("1", ""))
	objs[3] = sobj{num: 4, body: "<</T(parent)/Kids[6 0 R]>>"}
	objs = append(objs, sobj{num: 6, body: "<</FT/Sig/T(child)/Parent 4 0 R/V 5 0 R>>"})
	doc := fillSig(t, synthRevision(t, nil, objs, 1), "1", nil, detached(t, a))
	// STIMULUS: /Fields does not list the signature's field, and the remaining /Fields walk finds
	// no signature in it. (Before P01.S02 this also asserted the deleted /Fields ByteRange walk
	// returned saw=false — green at 11490690, the pre-S01 proof S04 records.)
	if listedInFields(t, doc, 5) {
		t.Fatal("STIMULUS: /Fields lists the nested signature directly")
	}
	if signatureBlobPresent(doc) {
		t.Fatal("STIMULUS: the /Fields blob walk found the nested signature")
	}
	revs, err := Revisions(doc)
	if err != nil {
		t.Fatal(err)
	}
	r := recordFor(t, revs, 5)
	if r.Cause != "" || !r.Verified || r.Fingerprint != a.fp {
		t.Errorf("nested signature record: cause %q verified %v fp %q, want well-formed, verified, %q", r.Cause, r.Verified, r.Fingerprint, a.fp)
	}
}

// victimDict returns the text of the first PPKLite signature dictionary in doc, header excluded.
func victimDict(t *testing.T, doc []byte) (num int, body string) {
	t.Helper()
	vi := bytes.Index(doc, []byte("/Adobe.PPKLite"))
	if vi < 0 {
		t.Fatal("no PPKLite dictionary")
	}
	hs := bytes.LastIndex(doc[:vi], []byte(" 0 obj"))
	ns := bytes.LastIndexAny(doc[:hs], "\n\r ") + 1
	num, err := strconv.Atoi(string(doc[ns:hs]))
	if err != nil {
		t.Fatalf("victim header: %v", err)
	}
	oe := hs + bytes.Index(doc[hs:], []byte("endobj"))
	return num, strings.TrimSpace(string(doc[hs+len(" 0 obj") : oe]))
}

// TestACopiedSignatureDictionaryIsRefused — /pending 687. A NEW dictionary carrying the victim's
// `/Contents` and `/ByteRange` verifies as a second signer under the victim's fingerprint. Both
// shapes: the victim's four numbers plus `999999999 0` (which a last-pair rule alone would let end
// coverage anywhere), and the victim's exact four under a new object number (which only (11) sees).
func TestACopiedSignatureDictionaryIsRefused(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a := newIdentity(t, "Alice")
	signed, err := SignApproval(base, a.certPEM, a.keyPEM, Options{Name: "Alice", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	vnum, vbody := victimDict(t, signed)
	r, err := dpdf.NewReader(bytes.NewReader(signed), int64(len(signed)))
	if err != nil {
		t.Fatal(err)
	}
	copyNum := int(r.Trailer().Key("Size").Int64())
	root := int(func() uint32 { p := r.Trailer().Key("Root").GetPtr(); return p.GetID() }())
	brRe := regexp.MustCompile(`/ByteRange\s*\[([^\]]*)\]`)
	m := brRe.FindStringSubmatch(vbody)
	if m == nil {
		t.Fatal("victim has no /ByteRange")
	}
	for _, tc := range []struct {
		name     string
		body     string
		conjunct int
		cause    RefusalCause
	}{
		// None of the first three reads a byte past the victim's own ranges, so none is the K-pair
		// gate's: each is a refused RECORD and the document keeps its signer and its verdict (the
		// S03 plan-review pin). `999999999 0` starts past EOF, `-5 0` is an empty section before
		// the file, and the odd fifth element's missing length is null, read as 0.
		{"victim's four plus 999999999 0", brRe.ReplaceAllString(vbody, "/ByteRange ["+m[1]+" 999999999 0]"), 3, CauseMalformedByteRange},
		{"victim's four plus -5 0", brRe.ReplaceAllString(vbody, "/ByteRange ["+m[1]+" -5 0]"), 3, CauseMalformedByteRange},
		{"victim's four plus an odd 0", brRe.ReplaceAllString(vbody, "/ByteRange ["+m[1]+" 0]"), 1, CauseMalformedByteRange},
		{"victim's four under a new number", vbody, 11, CauseContentsElsewhere},
	} {
		doc := synthRevision(t, signed, []sobj{{num: copyNum, body: tc.body}}, root)
		// STIMULUS: the library alone takes the copy for a second valid signature by the victim —
		// the attack is live, so a refusal below is doing work.
		resp, lerr := verify.Verify(bytes.NewReader(doc), int64(len(doc)))
		if lerr != nil || len(resp.Signers) != 2 || !resp.Signers[0].ValidSignature || !resp.Signers[1].ValidSignature {
			t.Fatalf("%s: STIMULUS: library err %v; the copy is not a verifying second signer", tc.name, lerr)
		}
		c := recordFor(t, mustSweep(t, doc), uint32(copyNum))
		if c.conjunct != tc.conjunct || c.Cause != tc.cause {
			t.Errorf("%s: copy conjunct %d cause %q, want %d %q", tc.name, c.conjunct, c.Cause, tc.conjunct, tc.cause)
		}
		st := Verify(doc)
		revs, err := Revisions(doc)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		// A structural refusal is a refused record, never a refused document: the victim's
		// signature still verifies, so the document is still valid.
		if st.State != Valid {
			t.Errorf("%s: document state %s, want valid — the refusal is the copy's, not the document's", tc.name, st.State)
		}
		c = recordFor(t, revs, uint32(copyNum))
		// The library verified the copy, and still no fingerprint: a refused record names nobody,
		// because P02 selects records by fingerprint and would select the copy as the victim.
		if !c.Verified || c.Fingerprint != "" {
			t.Errorf("%s: the refused copy's record: verified %v fingerprint %q, want verified and NO fingerprint", tc.name, c.Verified, c.Fingerprint)
		}
		if len(st.Signers) != 2 || st.Signers[1].Fingerprint != "" {
			t.Errorf("%s: Verify reports the copy's signer as %+v, want no fingerprint", tc.name, st.Signers)
		}
		// P01.S02: the copy's revision is covered by no valid, well-formed signature, and the reason
		// a reader is given is the refusal, naming the copy.
		if !st.AddedAfter || st.AddedAfterCause != AddedAfterRefusedSignature {
			t.Errorf("%s: addedAfter %v cause %q, want true %q", tc.name, st.AddedAfter, st.AddedAfterCause, AddedAfterRefusedSignature)
		}
		if len(st.Refused) != 1 || st.Refused[0].Obj != uint32(copyNum) || st.Refused[0].Cause != tc.cause || st.Refused[0].Filter != ppkLite {
			t.Errorf("%s: refused %+v, want exactly object %d, filter %q, cause %q", tc.name, st.Refused, copyNum, ppkLite, tc.cause)
		}
		v := recordFor(t, revs, uint32(vnum))
		if v.Cause != "" || !v.Verified || v.Fingerprint != a.fp {
			t.Errorf("%s: the victim's own record: cause %q verified %v fp %q, want well-formed, verified, %q", tc.name, v.Cause, v.Verified, v.Fingerprint, a.fp)
		}
	}
}

// appendDecoy661 appends /pending 661's decoy — a signature-shaped dictionary claiming coverage to
// 999,999,999 — and a field pointing at it. **The field is NOT listed in `/Fields`**: the catalog's
// AcroForm is not rewritten, so the `/Fields` walk never saw this decoy and a document built here
// warned even before P01.S02 (the S02 grill measured it). `appendListedDecoy661` is the shape that
// defeated the walk.
func appendDecoy661(t *testing.T, signed []byte, filter string) (doc []byte, decoy int) {
	t.Helper()
	r, err := dpdf.NewReader(bytes.NewReader(signed), int64(len(signed)))
	if err != nil {
		t.Fatal(err)
	}
	size := int(r.Trailer().Key("Size").Int64())
	root := int(func() uint32 { p := r.Trailer().Key("Root").GetPtr(); return p.GetID() }())
	decoy = size
	body := fmt.Sprintf("<</Type/Sig %s/ByteRange[0 10 20 999999999]/Contents<0001020304>>>", filter)
	return synthRevision(t, signed, []sobj{{num: decoy, body: body}, {num: decoy + 1, body: fmt.Sprintf("<</FT/Sig/T(decoy)/V %d 0 R>>", decoy)}}, root), decoy
}

// TestThe661DecoysAreRecordsWithCauses — /pending 661's two decoy shapes each hid an appended
// revision behind a byte range claiming coverage to EOF. They are records naming their object and
// cause, never silent skips; and the second (PPKLite, unparseable) is NOT refused by the library
// gate, because the library drops it before reading any range.
func TestThe661DecoysAreRecordsWithCauses(t *testing.T) {
	a := newIdentity(t, "Alice")
	signed := synthSigned(t, a)
	for _, tc := range []struct {
		filter string
		cause  RefusalCause
	}{
		{"", CauseUnsupportedFilter},
		{"/Filter/Adobe.PPKLite", CauseUnparseableContents},
	} {
		doc, decoy := appendDecoy661(t, signed, tc.filter)
		revs, err := Revisions(doc)
		if err != nil {
			t.Fatalf("decoy %q: %v", tc.filter, err)
		}
		d := recordFor(t, revs, uint32(decoy))
		// STIMULUS: the decoy claims coverage past the appended revision.
		if len(d.ByteRange) != 4 || d.ByteRange[3] != 999999999 {
			t.Fatalf("decoy %q: STIMULUS: byte range %v", tc.filter, d.ByteRange)
		}
		if d.Cause != tc.cause || d.Verified || d.libPos != -1 || d.CoverageEnd != 0 {
			t.Errorf("decoy %q: cause %q verified %v libPos %d end %d, want %q, unverified, outside the library, no coverage",
				tc.filter, d.Cause, d.Verified, d.libPos, d.CoverageEnd, tc.cause)
		}
		if st := Verify(doc); st.State != Valid || len(st.Signers) != 1 {
			t.Errorf("decoy %q: Verify state %s signers %d, want valid/1 — the gate must not refuse a record the library never reads",
				tc.filter, st.State, len(st.Signers))
		}
	}
}

// appendListedDecoy661 is /pending 661's decoy as the reproduction built it: the appended revision
// also rewrites the AcroForm holder — inline in the catalog, or its own object — so `/Fields` lists
// the decoy field beside the real ones, with `/SigFlags 3`. This is the shape the pre-S02 `/Fields`
// ByteRange walk measured coverage from, and so the one whose red proof is not vacuous.
func appendListedDecoy661(t *testing.T, signed []byte, filter string) (doc []byte, decoy int) {
	t.Helper()
	r, err := dpdf.NewReader(bytes.NewReader(signed), int64(len(signed)))
	if err != nil {
		t.Fatal(err)
	}
	size := int(r.Trailer().Key("Size").Int64())
	rootV := r.Trailer().Key("Root")
	rp := rootV.GetPtr()
	root := int(rp.GetID())
	acro := rootV.Key("AcroForm")
	ap := acro.GetPtr()
	var refs []string
	for i := 0; i < acro.Key("Fields").Len(); i++ {
		fp := acro.Key("Fields").Index(i).GetPtr()
		refs = append(refs, fmt.Sprintf("%d 0 R", fp.GetID()))
	}
	decoy = size
	refs = append(refs, fmt.Sprintf("%d 0 R", decoy+1))
	acroBody := fmt.Sprintf("<</Fields[%s]/SigFlags 3>>", strings.Join(refs, " "))
	objs := []sobj{
		{num: decoy, body: fmt.Sprintf("<</Type/Sig %s/ByteRange[0 10 20 999999999]/Contents<0001020304>>>", filter)},
		{num: decoy + 1, body: fmt.Sprintf("<</FT/Sig/T(decoy)/V %d 0 R>>", decoy)},
	}
	if ap.GetID() != 0 && int(ap.GetID()) != root {
		objs = append(objs, sobj{num: int(ap.GetID()), body: acroBody})
	} else {
		pp := rootV.Key("Pages").GetPtr()
		objs = append(objs, sobj{num: root, body: fmt.Sprintf("<</Type/Catalog/Pages %d 0 R/AcroForm%s>>", pp.GetID(), acroBody)})
	}
	return synthRevision(t, signed, objs, root), decoy
}

// listedInFields reports whether `/Fields` lists a field whose `/V` is object obj.
func listedInFields(t *testing.T, doc []byte, obj int) bool {
	t.Helper()
	r, err := dpdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatal(err)
	}
	fields := r.Trailer().Key("Root").Key("AcroForm").Key("Fields")
	for i := 0; i < fields.Len(); i++ {
		if p := fields.Index(i).Key("V").GetPtr(); int(p.GetID()) == obj {
			return true
		}
	}
	return false
}

// TestTheListedDecoysWarn — /pending 661, the S02 acceptance. Each decoy claims coverage past an
// appended revision and is LISTED in `/Fields`, which is what the deleted `/Fields` ByteRange walk
// took coverage from: on the pre-S02 tree (`11490690`) both read `valid, addedAfter=false` — the red
// proof, recorded in `docs/red-proofs.md`. `AddedAfter` now measures only verified, well-formed
// records, so each warns, names the refused decoy, and says why.
func TestTheListedDecoysWarn(t *testing.T) {
	a := newIdentity(t, "Alice")
	signed := synthSigned(t, a)
	for _, tc := range []struct {
		filter string
		cause  RefusalCause
	}{
		{"", CauseUnsupportedFilter},
		{"/Filter/Adobe.PPKLite", CauseUnparseableContents},
	} {
		doc, decoy := appendListedDecoy661(t, signed, tc.filter)
		// STIMULUS: the decoy is listed in /Fields, claims coverage past the file, and the real
		// signature still verifies — so only the coverage rule can make this warn.
		if !listedInFields(t, doc, decoy) {
			t.Fatalf("decoy %q: STIMULUS: /Fields does not list the decoy", tc.filter)
		}
		d := recordFor(t, mustSweep(t, doc), uint32(decoy))
		if len(d.ByteRange) != 4 || d.ByteRange[2]+d.ByteRange[3] <= int64(len(doc)) {
			t.Fatalf("decoy %q: STIMULUS: byte range %v does not claim the appended revision", tc.filter, d.ByteRange)
		}
		st := Verify(doc)
		if st.State != Valid || len(st.Signers) != 1 || st.Signers[0].Fingerprint != a.fp {
			t.Fatalf("decoy %q: STIMULUS: state %s signers %d, want valid/1 naming Alice", tc.filter, st.State, len(st.Signers))
		}
		if !st.AddedAfter {
			t.Errorf("decoy %q: addedAfter=false — a decoy /Fields lists hid the appended revision (/pending 661)", tc.filter)
		}
		if st.AddedAfterCause != AddedAfterRefusedSignature {
			t.Errorf("decoy %q: cause %q, want %q", tc.filter, st.AddedAfterCause, AddedAfterRefusedSignature)
		}
		if len(st.Refused) != 1 || st.Refused[0].Obj != uint32(decoy) || st.Refused[0].Cause != tc.cause {
			t.Errorf("decoy %q: refused %+v, want exactly object %d with cause %q", tc.filter, st.Refused, decoy, tc.cause)
		}
	}
}

// TestAParseFailingSignatureBeforeAValidOneLeavesTheJoinAligned — the library silently skips a
// PPKLite dictionary whose PKCS#7 does not parse, so the positional join must skip it too; counting
// it would shift every later signer onto the wrong record.
func TestAParseFailingSignatureBeforeAValidOneLeavesTheJoinAligned(t *testing.T) {
	a := newIdentity(t, "Alice")
	objs := baseObjs("<</Type/Sig/Filter/Adobe.PPKLite/ByteRange[0 1 2 3]/Contents<0102030405>>>")
	objs[3].body = "<</FT/Sig/T(Signature1)/V 6 0 R>>"
	objs = append(objs, sobj{num: 6, body: sigDict("1", "")})
	doc := fillSig(t, synthRevision(t, nil, objs, 1), "1", nil, detached(t, a))
	revs, err := Revisions(doc)
	if err != nil {
		t.Fatal(err)
	}
	bad, good := recordFor(t, revs, 5), recordFor(t, revs, 6)
	// STIMULUS: the bad record precedes the good one in the library's walk order.
	if !(bad.Obj < good.Obj) || bad.Filter != ppkLite {
		t.Fatal("STIMULUS: the unparseable PPKLite dictionary does not precede the valid one")
	}
	if bad.Cause != CauseUnparseableContents || bad.Verified || bad.libPos != -1 {
		t.Errorf("bad record: cause %q verified %v libPos %d", bad.Cause, bad.Verified, bad.libPos)
	}
	if good.libPos != 0 || !good.Verified || good.Fingerprint != a.fp || good.Cause != "" {
		t.Errorf("good record: libPos %d verified %v fp %q cause %q, want 0/true/%q/well-formed", good.libPos, good.Verified, good.Fingerprint, good.Cause, a.fp)
	}
}

// hybrid builds a classic-table file whose catalog lives in an object stream listed only by the
// trailer's /XRefStm — the hybrid-reference shape (ISO 32000-1 7.5.8.4).
func hybrid() []byte {
	var b bytes.Buffer
	off := map[int]int{}
	b.WriteString("%PDF-1.5\n%\xe2\xe3\xcf\xd3\n")
	off[2] = b.Len()
	b.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	off[3] = b.Len()
	b.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>\nendobj\n")
	cat := "<< /Type /Catalog /Pages 2 0 R /AcroForm << /SigFlags 3 /Fields [] >> >>"
	hdr := "1 0 "
	strm := hdr + cat
	off[4] = b.Len()
	fmt.Fprintf(&b, "4 0 obj\n<< /Type /ObjStm /N 1 /First %d /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(hdr), len(strm), strm)
	b.WriteString("% /ByteRange token so the pre-parse byte scan admits this file\n")
	off[5] = b.Len()
	var data bytes.Buffer
	data.Write([]byte{2, 0, 0, 0, 4, 0, 0}) // obj 1 in objstm 4 index 0
	fmt.Fprintf(&b, "5 0 obj\n<< /Type /XRef /Size 6 /W [1 4 2] /Index [1 1] /Length %d >>\nstream\n", data.Len())
	b.Write(data.Bytes())
	b.WriteString("\nendstream\nendobj\n")
	xo := b.Len()
	fmt.Fprintf(&b, "xref\n0 6\n0000000000 65535 f \n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n", off[2], off[3], off[4], off[5])
	fmt.Fprintf(&b, "trailer\n<< /Size 6 /Root 1 0 R /XRefStm %d >>\nstartxref\n%d\n%%%%EOF\n", off[5], xo)
	return b.Bytes()
}

// TestAHybridReferenceFileYieldsNoRecord pins today's blindness so fixing /pending 733 flips it on
// purpose: digitorus/pdf never reads `/XRefStm`, so an object reachable only through it is invisible
// to the sweep and to the library alike.
func TestAHybridReferenceFileYieldsNoRecord(t *testing.T) {
	h := hybrid()
	// STIMULUS: the catalog carrying /SigFlags exists, and pdfcpu reads it.
	if !bytes.Contains(h, []byte("/SigFlags 3")) || pdfcpuCanRead(h) != nil {
		t.Fatal("STIMULUS: the hybrid fixture has no readable catalog carrying /SigFlags")
	}
	revs, err := Revisions(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 0 {
		t.Errorf("the hybrid-reference file yields %d records — /pending 733 (digitorus reads /XRefStm) "+
			"may have been fixed; if so this assertion flips on purpose", len(revs))
	}
}

// TestTheGapScanIsBoundedPerRecord — conjunct (11)'s scan never reads more than its window, however
// many near-miss headers the window holds. Measured shape: a `/Reason` of 8,000 `x1 1 obj ` — each
// an `obj` keyword whose walk-back reaches digits and white space and then fails at `x` — pushing
// the dictionary's own header past the 64 KiB window, so the record is refused and the scan stops.
func TestTheGapScanIsBoundedPerRecord(t *testing.T) {
	a := newIdentity(t, "Alice")
	reason := strings.Repeat("x1 1 obj ", 8000)
	sig := "<</Type/Sig/Filter/Adobe.PPKLite/SubFilter/adbe.pkcs7.detached/Reason(" + reason + ")/ByteRange[" +
		brSlot("1", 64) + "]/Contents" + contentsSlot("1") + ">>"
	doc := fillSig(t, synthRevision(t, nil, baseObjs(sig), 1), "1", nil, detached(t, a))
	revs, st, err := sweep(doc)
	if err != nil {
		t.Fatal(err)
	}
	r := recordFor(t, revs, 5)
	// STIMULUS: the window is full of candidates, and the own header is outside it.
	if strings.Count(string(doc[r.gapStart-gapScanWindow:r.gapStart]), " obj ") < 5000 ||
		bytes.Contains(doc[r.gapStart-gapScanWindow:r.gapStart], []byte("5 0 obj")) {
		t.Fatal("STIMULUS: the scan window is not full of near-miss headers, or holds the own header")
	}
	if r.conjunct != 11 {
		t.Errorf("record conjunct %d (%q), want 11 — its own header is beyond the window", r.conjunct, r.Cause)
	}
	if st.scanned > 2*gapScanWindow+64 {
		t.Errorf("the scan examined %d bytes for one record, want at most %d (twice its window)", st.scanned, 2*gapScanWindow+64)
	}
}

// TestTheGapScanCountsEveryByteItWalks — `examined` is the budget tests' evidence, so it must count
// the walk-back of a candidate that fails early as well as one that fails late. Each `x5 obj ` walks
// back over its space and its digit and then fails at `x` (not a second white-space run): two bytes a
// count taken only on the late exit would miss.
func TestTheGapScanCountsEveryByteItWalks(t *testing.T) {
	const r = 1000
	data := []byte("5 0 obj\n<</A[" + strings.Repeat("x5 obj ", r) + "]/Contents<00>>>")
	gs := bytes.Index(data, []byte("<00>"))
	keyAt := gs - len("/Contents")
	obj, ok, examined := gapOwner(data, gs, 0)
	// STIMULUS: the owner is found, past r near-miss candidates.
	if !ok || obj != 5 || bytes.Count(data, []byte("x5 obj ")) != r {
		t.Fatalf("STIMULUS: owner %d ok %v over %d candidates", obj, ok, bytes.Count(data, []byte("x5 obj ")))
	}
	// The keyword loop visits every position from keyAt-3 down to the header's `o` at 4; each
	// candidate walks back two bytes; the header's own walk-back is `5 0 ` (four bytes).
	want := (keyAt - 3 - 4 + 1) + 2*r + 4
	if examined != want {
		t.Errorf("gapOwner examined %d bytes, want %d — a walk-back that exits early went uncounted", examined, want)
	}
}

// TestTheGapScanIsLinearAcrossRecords — many records cost O(len(data)), not O(records × window).
//
// The hostile shape: object 9 is one array holding N byte-identical `/Contents<H>` tokens, then 60
// KB of text, then one more. N signature dictionaries each carry `/Contents<H>` and point their gap
// at a different token; M more all point at the last one. Every token passes (1)-(10) — it IS a hex
// string decoding to the record's `/Contents` — and every scan must walk back to object 9's header
// to refuse it. Without the floor at the previous gap's end, token i scans min(64 KiB, i tokens)
// back, O(N × window); without the per-gap memo, the M records sharing the last token each re-scan
// its 60 KB. With both, the scans together examine less than the file.
func TestTheGapScanIsLinearAcrossRecords(t *testing.T) {
	a := newIdentity(t, "Alice")
	h := strings.ToUpper(hex.EncodeToString(detached(t, a)([]byte("x"))))
	token := "/Contents<" + h + ">"
	const n, m = 300, 300
	arr := "[" + strings.Repeat(token+" ", n) + "(" + strings.Repeat("a", 60000) + ") " + token + "]"
	objs := baseObjs(sigDict("0", ""))
	objs = append(objs, sobj{num: 9, body: arr})
	for i := 0; i < n+m; i++ {
		objs = append(objs, sobj{num: 10 + i, body: "<</Type/Sig/Filter/Adobe.PPKLite/ByteRange[" +
			brSlot(strconv.Itoa(i+1), 48) + "]/Contents<" + h + ">>>"})
	}
	doc := fillSig(t, synthRevision(t, nil, objs, 1), "0", nil, detached(t, a))
	// The token offsets inside object 9, in file order.
	start := bytes.Index(doc, []byte("9 0 obj"))
	var at []int64
	for off := start; ; {
		i := bytes.Index(doc[off:], []byte(token))
		if i < 0 || len(at) == n+1 {
			break
		}
		at = append(at, int64(off+i+len("/Contents")))
		off += i + len(token)
	}
	if len(at) != n+1 {
		t.Fatalf("fixture: found %d tokens in object 9, want %d", len(at), n+1)
	}
	for i := 0; i < n+m; i++ {
		gs := at[min(i, n)]
		ge := gs + int64(len(h)) + 2
		slot := []byte("@BR" + strconv.Itoa(i+1) + "@")
		q := bytes.Index(doc, slot)
		copy(doc[q:q+48], fmt.Sprintf("%-48s", ints(0, gs, ge, int64(len(doc))-ge)))
	}
	revs, st, err := sweep(doc)
	if err != nil {
		t.Fatal(err)
	}
	passedLocal, elsewhere := 0, 0
	for _, r := range revs {
		if r.Obj < 10 {
			continue
		}
		if r.conjunct == 11 {
			elsewhere++
		}
		if r.conjunct == 0 || r.conjunct == 11 {
			passedLocal++
		}
	}
	// STIMULUS: every hostile record reached conjunct (11) — its gap passed the ten local checks —
	// and (11) refused it, so every one of them was scanned.
	if passedLocal != n+m || elsewhere != n+m {
		t.Fatalf("STIMULUS: %d of %d hostile records reached (11) and %d were refused by it", passedLocal, n+m, elsewhere)
	}
	if st.scanned > len(doc) {
		t.Errorf("the scans examined %d bytes over a %d-byte document with %d records — not linear", st.scanned, len(doc), len(revs))
	}
	t.Logf("records=%d size=%d scanned=%d", len(revs), len(doc), st.scanned)
}

// tinyPNG is a 4x4 black image for a visible signature's appearance.
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := 0; i < 16; i++ {
		img.Set(i%4, i/4, color.Black)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// fingerprintWriters counts, per enclosing function, every write of a field named Fingerprint in
// the package's non-test files: an assignment to `x.Fingerprint` or a `Fingerprint:` composite key.
func fingerprintWriters(t *testing.T, dir string) map[string]int {
	t.Helper()
	fset := token.NewFileSet()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.AssignStmt:
					for _, l := range n.Lhs {
						if sel, ok := l.(*ast.SelectorExpr); ok && sel.Sel.Name == "Fingerprint" {
							out[fn.Name.Name]++
						}
					}
				case *ast.KeyValueExpr:
					if id, ok := n.Key.(*ast.Ident); ok && id.Name == "Fingerprint" {
						out[fn.Name.Name]++
					}
				}
				return true
			})
		}
	}
	return out
}

// TestNoProducerSignatureIsRefused — the precondition the whole rule stands on (G11), counted PER
// SOURCE, one subtest each: nib's own signing paths by name, and each real-producer corpus file. A
// missing corpus file SKIPS its subtest with `SKIP (not a pass)` — never a logged continue that
// reads as green.
func TestNoProducerSignatureIsRefused(t *testing.T) {
	when := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	a, b := newIdentity(t, "Alice"), newIdentity(t, "Bob")
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	text, err := testpdf.Text("the lease", "page two")
	if err != nil {
		t.Fatal(err)
	}
	type source struct {
		name string
		doc  []byte
		sigs int
		path string // a real-producer file, read inside its own subtest
	}
	var sources []source
	fin, err := Sign(text, a.certPEM, a.keyPEM, Options{Name: "Alice", Reason: "Finalized in Nib", When: when})
	if err != nil {
		t.Fatal(err)
	}
	sources = append(sources, source{name: "Finalize (Sign, certification)", doc: fin, sigs: 1})
	ext, err := SignExternal(form, makeP12(t, "Jane External", "pw"), "pw", Options{Name: "Jane", Reason: "r", When: when})
	if err != nil {
		t.Fatal(err)
	}
	sources = append(sources, source{name: "nib sign / Finalize with a .p12 (SignExternal)", doc: ext, sigs: 1})
	a1, err := SignApproval(form, a.certPEM, a.keyPEM, Options{Name: "Alice", Reason: "co-sign", When: when})
	if err != nil {
		t.Fatal(err)
	}
	a2, err := SignApproval(a1, b.certPEM, b.keyPEM, Options{Name: "Bob", Reason: "co-sign", When: when,
		Appearance: &Appearance{Image: tinyPNG(t), Page: 1, Rect: [4]float64{50, 50, 150, 100}}})
	if err != nil {
		t.Fatal(err)
	}
	a3, err := SignApproval(a2, a.certPEM, a.keyPEM, Options{Name: "Alice", Reason: "third", When: when})
	if err != nil {
		t.Fatal(err)
	}
	sources = append(sources, source{name: "co-sign / ceremony hop (SignApproval x3, 2nd visible)", doc: a3, sigs: 3})
	home, homeErr := os.UserHomeDir()
	for _, f := range []string{"irs-f1040.pdf", "irs-fw9.pdf"} {
		sources = append(sources, source{name: "real producer " + f, sigs: 1, path: filepath.Join(home, "nib", "producers", "designer", f)})
	}
	for _, s := range sources {
		t.Run(s.name, func(t *testing.T) {
			doc := s.doc
			if s.path != "" {
				if homeErr != nil {
					t.Skipf("SKIP (not a pass): no home directory to find the corpus in: %v", homeErr)
				}
				b, err := os.ReadFile(s.path)
				if err != nil {
					t.Skipf("SKIP (not a pass): the real-producer corpus is absent: %v", err)
				}
				doc = b
			}
			// The sweep itself must finish on every honest document: an error there is `Invalid`
			// before the library is called, so one on a producer's output would refuse it whole.
			if _, _, err := sweep(doc); err != nil {
				t.Fatalf("the sweep errored on a producer's own output: %v", err)
			}
			revs, err := Revisions(doc)
			if err != nil {
				t.Fatalf("Revisions: %v", err)
			}
			verified := 0
			for _, r := range revs {
				if r.Verified {
					verified++
				}
			}
			// STIMULUS: the source's signatures were all found, and at least one verified — a
			// source with nothing in it would pass the refusal check below vacuously.
			if s.sigs <= 0 || len(revs) != s.sigs || verified == 0 {
				t.Fatalf("STIMULUS: %d records (want %d, more than none), %d verified (want more than none)", len(revs), s.sigs, verified)
			}
			for _, r := range revs {
				if r.Cause != "" {
					t.Errorf("object %d refused %q (conjunct %d) — a real producer fails the rule; park the slice", r.Obj, r.Cause, r.conjunct)
				}
			}
			t.Logf("records=%d verified=%d", len(revs), verified)
		})
	}
}

// countingLibrary replaces libraryVerify for one test and counts the calls.
func countingLibrary(t *testing.T) *int {
	t.Helper()
	n := new(int)
	orig := libraryVerify
	libraryVerify = func(f io.ReaderAt, size int64) (*verify.Response, error) {
		*n++
		return orig(f, size)
	}
	t.Cleanup(func() { libraryVerify = orig })
	return n
}

// TestAByteRangeAskingForMoreThanTheFileNeverReachesTheLibrary — the K-pair gate. The library
// copies every pair a /ByteRange names (`processByteRange`), so `[0 S 0 S … ×K]` allocates K×S and a
// Go out-of-memory is fatal. K = 10,000 here, S = the file's length: the library would copy K×S. The
// same door refuses K pairs of NEGATIVE length (`io.NewSectionReader` reads one as "to the end", so
// each pair copies the file with no sum to exceed) and an INDIRECT /ByteRange (re-parsed three times
// per pair). Shapes that read nothing are NOT this door's — see
// TestAByteRangeThatReadsNothingIsARefusedRecordNotARefusedDocument.
func TestAByteRangeAskingForMoreThanTheFileNeverReachesTheLibrary(t *testing.T) {
	a := newIdentity(t, "Alice")
	const k = 10000
	sig := "<</Type/Sig/Filter/Adobe.PPKLite/SubFilter/adbe.pkcs7.detached/ByteRange[" + brSlot("1", k*16) +
		"]/Contents" + contentsSlot("1") + ">>"
	doc := fillSig(t, synthRevision(t, nil, baseObjs(sig), 1), "1", func(g geo) string {
		vs := make([]int64, 0, 2*k)
		for i := 0; i < k; i++ {
			vs = append(vs, 0, g.size)
		}
		return ints(vs...)
	}, detached(t, a))
	// A negative length: `io.NewSectionReader` reads it as "to the end of the file", so each pair
	// copies the whole file — the K-pair attack with no sum to exceed.
	neg := fillSig(t, synthRevision(t, nil, baseObjs(sig), 1), "1", func(g geo) string {
		vs := make([]int64, 0, 2*k)
		for i := 0; i < k; i++ {
			vs = append(vs, 0, -1)
		}
		return ints(vs...)
	}, detached(t, a))
	// A pair starting past EOF reads nothing, so it must add nothing to the sum — and must not
	// SUBTRACT: `size-off` is negative there, and a clamp without the `off < size` guard would buy
	// headroom for the K pairs of the whole file that follow it.
	const kp = 1000
	pastSig := "<</Type/Sig/Filter/Adobe.PPKLite/SubFilter/adbe.pkcs7.detached/ByteRange[" + brSlot("1", kp*16+24) +
		"]/Contents" + contentsSlot("1") + ">>"
	pastThenK := fillSig(t, synthRevision(t, nil, baseObjs(pastSig), 1), "1", func(g geo) string {
		vs := []int64{999999999, 999999999}
		for i := 0; i < kp; i++ {
			vs = append(vs, 0, g.size)
		}
		return ints(vs...)
	}, detached(t, a))
	// An INDIRECT /ByteRange: `processByteRange` calls `v.Key("ByteRange")` three times per pair and
	// an indirect array is re-parsed on each, so K=4,000 empty pairs after an honest four took 7.86 s
	// on a 29 KB file and still ended `valid`. Nothing here reads past the file — the indirection is
	// the only stimulus.
	const ki = 4000
	indSig := "<</Type/Sig/Filter/Adobe.PPKLite/SubFilter/adbe.pkcs7.detached/ByteRange 6 0 R/Contents" + contentsSlot("1") + ">>"
	indirect := fillSig(t, synthRevision(t, nil, append(baseObjs(indSig), sobj{num: 6, body: "[" + brSlot("1", ki*6) + "]"}), 1), "1", func(g geo) string {
		vs := []int64{0, g.gs, g.ge, g.size - g.ge}
		for i := 0; i < ki; i++ {
			vs = append(vs, 1, 0)
		}
		return ints(vs...)
	}, detached(t, a))

	calls := countingLibrary(t)
	// STIMULUS: the seam is the library's door — an honest document reaches it once.
	if Verify(synthSigned(t, a)); *calls != 1 {
		t.Fatalf("STIMULUS: an honest document made %d library calls, want 1 — the seam is not the door", *calls)
	}
	sumOf := func(br []int64) (sum int64, negative bool) {
		for i := 1; i < len(br); i += 2 {
			sum += br[i]
			negative = negative || br[i] < 0
		}
		return
	}
	for _, tc := range []struct {
		name string
		doc  []byte
		// stimulus is what makes the byte range one the library must not see.
		stimulus func(br []int64, size int64) bool
	}{
		{"K=10,000 pairs of the whole file", doc, func(br []int64, size int64) bool {
			sum, _ := sumOf(br)
			return len(br) == 2*k && sum > size
		}},
		{"K=10,000 pairs of negative length", neg, func(br []int64, size int64) bool {
			sum, negative := sumOf(br)
			// Every length is negative, so the sum alone reads as far INSIDE the file.
			return len(br) == 2*k && negative && sum < 0
		}},
		{"a pair past EOF, then K=1,000 pairs of the whole file", pastThenK, func(br []int64, size int64) bool {
			return len(br) == 2+2*kp && br[0] > size && br[3] == size
		}},
		{"K=4,000 empty pairs in an INDIRECT array", indirect, func(br []int64, size int64) bool {
			sum, negative := sumOf(br)
			// Resolved through the indirection, summing to the file, and nothing negative: only the
			// indirection makes it the gate's.
			return len(br) == 4+2*ki && !negative && sum == size-(br[2]-br[1]) &&
				bytes.Contains(indirect, []byte("/ByteRange 6 0 R"))
		}},
	} {
		revs := mustSweep(t, tc.doc)
		r := recordFor(t, revs, 5)
		sum, _ := sumOf(r.ByteRange)
		// STIMULUS: the record is one the library would process, and its byte range is the hostile one.
		if r.libPos < 0 || !tc.stimulus(r.ByteRange, int64(len(tc.doc))) {
			t.Fatalf("%s: STIMULUS: libPos %d, %d elements summing %d over a %d-byte file", tc.name, r.libPos, len(r.ByteRange), sum, len(tc.doc))
		}
		*calls = 0
		var m0, m1 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m0)
		st := Verify(tc.doc)
		runtime.ReadMemStats(&m1)
		if st.State != Invalid {
			t.Errorf("%s: state %s, want invalid", tc.name, st.State)
		}
		if *calls != 0 {
			t.Errorf("%s: the library was called %d times on a byte range it must not be handed", tc.name, *calls)
		}
		// Bounded: the gate's own cost is the pdfcpu parse and one sweep — measured 5.5 MB on the
		// 165 KB K-pair file — against the library's K×S (here 1.6 GB).
		if alloc := m1.TotalAlloc - m0.TotalAlloc; alloc > 16<<20 {
			t.Errorf("%s: Verify allocated %d bytes on a %d-byte file, want under 16 MiB", tc.name, alloc, len(tc.doc))
		} else {
			t.Logf("%s: file %d bytes, Verify allocated %d bytes", tc.name, len(tc.doc), alloc)
		}
		if _, err := Revisions(tc.doc); !errors.Is(err, errLibraryWouldOverread) {
			t.Errorf("%s: Revisions err %v, want errLibraryWouldOverread", tc.name, err)
		}
	}
}

// TestAByteRangeThatReadsNothingIsARefusedRecordNotARefusedDocument — the other half of the K-pair
// gate: an odd count, a negative offset and a pair running past EOF look hostile but cost the
// library nothing beyond the file (`libraryWouldOverread`'s comment cites why), so they reach the
// library and are refused as RECORDS by the structure rule. Gating them would make the document
// `Invalid` and blank its co-signers (the S03 plan-review pin). The past-EOF pair starts inside the
// file, so its read is clamped to `size-off` and the sum stays within the file: the rule, not a
// special case, is what keeps it out of the gate — and conjunct (6) is what refuses it.
func TestAByteRangeThatReadsNothingIsARefusedRecordNotARefusedDocument(t *testing.T) {
	a := newIdentity(t, "Alice")
	build := func(tail func(g geo) []int64) []byte {
		return fillSig(t, synthRevision(t, nil, baseObjs(sigDict("1", "")), 1), "1", func(g geo) string {
			return ints(tail(g)...)
		}, detached(t, a))
	}
	calls := countingLibrary(t)
	for _, tc := range []struct {
		name     string
		doc      []byte
		conjunct int
		cause    RefusalCause
		stimulus func(br []int64, size int64) bool
	}{
		{"odd element count", build(func(g geo) []int64 { return []int64{0, g.gs, g.ge, g.size - g.ge, 1} }), 1, CauseMalformedByteRange,
			func(br []int64, size int64) bool { return len(br) == 5 }},
		{"a negative offset", build(func(g geo) []int64 { return []int64{0, g.gs, g.ge, g.size - g.ge, -1, 1} }), 4, CauseMalformedByteRange,
			func(br []int64, size int64) bool { return len(br) == 6 && br[4] < 0 && br[5] > 0 }},
		// `bytes.Reader.ReadAt` refuses a negative offset before copying, whatever the length.
		{"a negative offset with a length past the file", build(func(g geo) []int64 { return []int64{0, g.gs, g.ge, g.size - g.ge, -1, 999999999} }), 4, CauseMalformedByteRange,
			func(br []int64, size int64) bool { return len(br) == 6 && br[4] < 0 && br[5] > size }},
		{"a pair ending one byte past EOF", build(func(g geo) []int64 { return []int64{0, g.gs, g.ge, g.size - g.ge + 1} }), 6, CauseByteRangeOutsideFile,
			func(br []int64, size int64) bool { return len(br) == 4 && br[2] < size && br[2]+br[3] == size+1 }},
		// The raw lengths sum far past the file; the CLAMPED read (min(l, size-off)) does not, and
		// the clamped read is what the library copies.
		{"a last length of 999,999,999", build(func(g geo) []int64 { return []int64{0, g.gs, g.ge, 999999999} }), 6, CauseByteRangeOutsideFile,
			func(br []int64, size int64) bool { return len(br) == 4 && br[2] < size && br[1]+br[3] > size }},
	} {
		r := recordFor(t, mustSweep(t, tc.doc), 5)
		if r.libPos < 0 || !tc.stimulus(r.ByteRange, int64(len(tc.doc))) {
			t.Fatalf("%s: STIMULUS: libPos %d, byte range %v over a %d-byte file", tc.name, r.libPos, r.ByteRange, len(tc.doc))
		}
		*calls = 0
		Verify(tc.doc)
		if *calls != 1 {
			t.Errorf("%s: the library was called %d times, want 1 — a range that reads nothing is not the gate's", tc.name, *calls)
		}
		revs, err := Revisions(tc.doc)
		if errors.Is(err, errLibraryWouldOverread) {
			t.Errorf("%s: Revisions answered errLibraryWouldOverread, want a refused record", tc.name)
		}
		if r := recordFor(t, revs, 5); r.conjunct != tc.conjunct || r.Cause != tc.cause || r.Fingerprint != "" {
			t.Errorf("%s: record conjunct %d cause %q fp %q, want %d %q and no fingerprint", tc.name, r.conjunct, r.Cause, r.Fingerprint, tc.conjunct, tc.cause)
		}
	}
}

// TestASweepThatCannotFinishNeverReachesTheLibrary — the K-pair gate has nothing to judge when the
// sweep errors, so an error must be `Invalid` before the library runs, not a licence to call it.
// Measured before the fix: an indirect `/SubFilter` naming a mis-headed object made the sweep fail
// while pdfcpu read the file, and the library then copied the K pairs (25 MB from a 9 KB file).
func TestASweepThatCannotFinishNeverReachesTheLibrary(t *testing.T) {
	a := newIdentity(t, "Alice")
	const k = 300
	sig := "<</Type/Sig/Filter/Adobe.PPKLite/SubFilter 6 0 R/ByteRange[" + brSlot("1", k*16) +
		"]/Contents" + contentsSlot("1") + ">>"
	objs := append(baseObjs(sig), sobj{num: 6, body: "/adbe.pkcs7.detached"})
	doc := fillSig(t, synthRevision(t, nil, objs, 1), "1", func(g geo) string {
		vs := make([]int64, 0, 2*k)
		for i := 0; i < k; i++ {
			vs = append(vs, 0, g.size)
		}
		return ints(vs...)
	}, detached(t, a))
	i := bytes.Index(doc, []byte("\n6 0 obj"))
	if i < 0 {
		t.Fatal("fixture: no object 6")
	}
	doc[i+1] = '8' // the xref still points here; the header now names another object
	// STIMULUS: pdfcpu reads the file (so the readability gate admits it) and the sweep cannot finish.
	if err := pdfcpuCanRead(doc); err != nil {
		t.Fatalf("STIMULUS: pdfcpu refuses the fixture (%v), so the readability gate answers first", err)
	}
	if _, err := sweepRevisions(doc); err == nil {
		t.Fatal("STIMULUS: the sweep finished on the mis-headed fixture")
	}
	calls := countingLibrary(t)
	st := Verify(doc)
	if st.State != Invalid || !st.AddedAfter || len(st.Signers) != 0 {
		t.Errorf("state %s addedAfter %v signers %d, want invalid, warned, none", st.State, st.AddedAfter, len(st.Signers))
	}
	if *calls != 0 {
		t.Errorf("the library was called %d times on a document the sweep could not read", *calls)
	}
	if revs, err := Revisions(doc); err == nil || revs != nil {
		t.Errorf("Revisions: %d records, err %v; want none and an error", len(revs), err)
	}
}

// TestTheGapTokenIsReadOncePerGap — conjuncts (7) and (8) walk a gap's token once, however many
// dictionaries name it. M records pointing at one G-byte token cost M×G without the memo (measured
// 1.44 s at M=400, G=4 MiB). Two shapes: a token of G hex digits that decodes to the wrong LENGTH
// (refused at (8) on the length alone), and a token of G spaces around the right bytes (equal length,
// compared once per record against one decoding — never re-walking the spaces).
func TestTheGapTokenIsReadOncePerGap(t *testing.T) {
	a := newIdentity(t, "Alice")
	h := strings.ToUpper(hex.EncodeToString(detached(t, a)([]byte("x"))))
	const g, m = 1 << 20, 64
	for _, tc := range []struct {
		name     string
		token    string
		conjunct int
	}{
		{"G hex digits, the wrong length", "<" + strings.Repeat("A", g) + ">", 8},
		{"G spaces around the right bytes", "<" + strings.Repeat(" ", g) + h + ">", 11},
	} {
		objs := baseObjs(sigDict("0", ""))
		objs = append(objs, sobj{num: 9, body: "[" + tc.token + "]"})
		for i := 0; i < m; i++ {
			objs = append(objs, sobj{num: 10 + i, body: "<</Type/Sig/Filter/Adobe.PPKLite/ByteRange[" +
				brSlot(strconv.Itoa(i+1), 48) + "]/Contents<" + h + ">>>"})
		}
		doc := fillSig(t, synthRevision(t, nil, objs, 1), "0", nil, detached(t, a))
		gs := int64(bytes.Index(doc, []byte("["+tc.token[:2])) + 1)
		ge := gs + int64(len(tc.token))
		for i := 0; i < m; i++ {
			q := bytes.Index(doc, []byte("@BR"+strconv.Itoa(i+1)+"@"))
			copy(doc[q:q+48], fmt.Sprintf("%-48s", ints(0, gs, ge, 1)))
		}
		revs, st, err := sweep(doc)
		if err != nil {
			t.Fatal(err)
		}
		// STIMULUS: every hostile record names the big token and got past (7) — so without the memo
		// each would have walked it — and was refused where the shape says.
		hit := 0
		for _, r := range revs {
			if r.Obj >= 10 && r.gapStart == gs && r.gapEnd == ge && r.conjunct == tc.conjunct {
				hit++
			}
		}
		if hit != m {
			t.Fatalf("%s: STIMULUS: %d of %d records named the %d-byte token and were refused at (%d)", tc.name, hit, m, len(tc.token), tc.conjunct)
		}
		// The bound: each token walked once and decoded at most once (2× the file), plus the
		// compared `/Contents` bytes, which are half their own hex in the file (another ½×).
		if st.forward > 5*len(doc)/2 {
			t.Errorf("%s: (7) and (8) examined %d bytes over a %d-byte document with %d records naming one token — once per record, not once per gap",
				tc.name, st.forward, len(doc), m)
		}
		t.Logf("%s: size=%d forward=%d", tc.name, len(doc), st.forward)
	}
}

// twoSigners returns a document Alice signed and Bob then co-signed.
func twoSigners(t *testing.T, a, b identity) []byte {
	t.Helper()
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	one, err := SignApproval(base, a.certPEM, a.keyPEM, Options{Name: "Alice", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	two, err := SignApproval(one, b.certPEM, b.keyPEM, Options{Name: "Bob", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return two
}

// TestTheJoinToleratesAFailedSignatureAndNamesTheValidOne — the grill's crack. The library returns
// an EMPTY bag on every failure path, so the join's rule is "empty library bag ⇒ not valid", never
// "both empty": the literal rule would make every tampered document a join error and blank its
// valid co-signer.
func TestTheJoinToleratesAFailedSignatureAndNamesTheValidOne(t *testing.T) {
	a, b := newIdentity(t, "Alice"), newIdentity(t, "Bob")
	doc := twoSigners(t, a, b)
	// Tamper a byte only Bob's signature covers: his /Name, written after his gap.
	i := bytes.LastIndex(doc, []byte("(Bob)"))
	if i < 0 {
		t.Fatal("no (Bob) to tamper")
	}
	doc[i+2] = 'x'
	resp, err := verify.Verify(bytes.NewReader(doc), int64(len(doc)))
	// STIMULUS: the library reports Alice valid and Bob invalid with an empty bag.
	if err != nil || len(resp.Signers) != 2 || !resp.Signers[0].ValidSignature || resp.Signers[1].ValidSignature || len(resp.Signers[1].Certificates) != 0 {
		t.Fatalf("STIMULUS: library err %v; want [valid, invalid with an empty bag]", err)
	}
	st := Verify(doc)
	if st.State != Invalid || len(st.Signers) != 2 {
		t.Fatalf("state %s signers %d, want invalid/2", st.State, len(st.Signers))
	}
	if st.Signers[0].Fingerprint != a.fp {
		t.Errorf("Alice's valid signature reports fingerprint %q, want %q — a failed co-signer blanked her", st.Signers[0].Fingerprint, a.fp)
	}
	if st.Signers[1].Fingerprint != "" {
		t.Errorf("Bob's FAILED signature reports fingerprint %q — nothing established it", st.Signers[1].Fingerprint)
	}
	// Bob's failed revision is covered by no VALID signature, so AddedAfter warns (P01.S02) — and it
	// must say it was APPENDED, never that it could not check: the join did not disagree.
	if !st.AddedAfter || st.AddedAfterCause != AddedAfterAppended {
		t.Errorf("addedAfter %v cause %q, want true %q: the join reported a disagreement on an ordinary tampered document, or Bob's failed signature bounded coverage",
			st.AddedAfter, st.AddedAfterCause, AddedAfterAppended)
	}
}

// TestTheJoinRefusesEveryDisagreement drives the join's three refusals at its own door, from a real
// two-signer document: a record deleted before the join (the shift), a bag that is not the
// record's, and a signer that verified with no certificates.
func TestTheJoinRefusesEveryDisagreement(t *testing.T) {
	a, b := newIdentity(t, "Alice"), newIdentity(t, "Bob")
	doc := twoSigners(t, a, b)
	resp, err := verify.Verify(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatal(err)
	}
	fresh := func() []Revision { return mustSweep(t, doc) }
	// STIMULUS: the unmodified pair joins, each signer onto its own record.
	if at, err := joinLibrary(fresh(), resp.Signers); err != nil || len(at) != 2 {
		t.Fatalf("STIMULUS: the honest join failed: %v", err)
	}
	shifted := fresh()[1:]
	for i := range shifted {
		shifted[i].libPos--
	}
	if _, err := joinLibrary(shifted, resp.Signers); !errors.Is(err, errJoin) || !strings.Contains(err.Error(), "reported 2 signers") {
		t.Errorf("a record deleted before the join: err %v, want the count disagreement", err)
	}
	swapped := []verify.Signer{resp.Signers[1], resp.Signers[0]}
	if _, err := joinLibrary(fresh(), swapped); !errors.Is(err, errJoin) || !strings.Contains(err.Error(), "certificates are not") {
		t.Errorf("signers out of order: err %v, want the bag disagreement", err)
	}
	bare := append([]verify.Signer(nil), resp.Signers...)
	bare[0].Certificates = nil
	if _, err := joinLibrary(fresh(), bare); !errors.Is(err, errJoin) || !strings.Contains(err.Error(), "no certificates") {
		t.Errorf("a valid signer with no bag: err %v, want the empty-bag disagreement", err)
	}
}

// TestAJoinErrorNamesNobodyAndWarns — a disagreement between the library and the records blanks
// every fingerprint and routes to AddedAfter's fail-closed arm (Default 1 of the S01 grill).
func TestAJoinErrorNamesNobodyAndWarns(t *testing.T) {
	a := newIdentity(t, "Alice")
	doc := synthSigned(t, a)
	// STIMULUS: undisturbed, the document names Alice and does not warn.
	if st := Verify(doc); st.Signers[0].Fingerprint != a.fp || st.AddedAfter {
		t.Fatalf("STIMULUS: undisturbed fp %q addedAfter %v", firstSignerFP(st), st.AddedAfter)
	}
	orig := libraryVerify
	libraryVerify = func(f io.ReaderAt, size int64) (*verify.Response, error) {
		resp, err := orig(f, size)
		if err == nil && resp != nil {
			resp.Signers = append(resp.Signers, resp.Signers[0]) // one more than the document holds
		}
		return resp, err
	}
	t.Cleanup(func() { libraryVerify = orig })
	st := Verify(doc)
	for i, s := range st.Signers {
		if s.Fingerprint != "" {
			t.Errorf("signer %d reports %q after the join disagreed", i, s.Fingerprint)
		}
	}
	if !st.AddedAfter || st.AddedAfterCause != AddedAfterCouldNotCheck {
		t.Errorf("addedAfter %v cause %q after the join disagreed, want true %q — the warning went quiet independently of the verdict",
			st.AddedAfter, st.AddedAfterCause, AddedAfterCouldNotCheck)
	}
}

// TestALibraryThatReportsNothingForARealSignatureWarns — the zero-signer branch: when the library
// errors or reports no signer for a document whose sweep found one it should have processed, the join
// disagrees, and that must be as loud as every other disagreement — `Invalid`, `AddedAfter` set, nobody
// named. The claims pass (P01.S01's commit gate) found this branch returning the error with no warning.
func TestALibraryThatReportsNothingForARealSignatureWarns(t *testing.T) {
	a := newIdentity(t, "Alice")
	doc := synthSigned(t, a)
	// STIMULUS: undisturbed, the document is valid, names Alice, and does not warn.
	if st := Verify(doc); st.State != Valid || st.AddedAfter || firstSignerFP(st) != a.fp {
		t.Fatalf("STIMULUS: undisturbed state %v addedAfter %v fp %q", st.State, st.AddedAfter, firstSignerFP(st))
	}
	orig := libraryVerify
	libraryVerify = func(io.ReaderAt, int64) (*verify.Response, error) {
		return nil, errors.New("the library could not read what the sweep read")
	}
	t.Cleanup(func() { libraryVerify = orig })
	st := Verify(doc)
	if st.State != Invalid {
		t.Errorf("state %v, want Invalid: a signature nobody checked is not an unsigned document", st.State)
	}
	if !st.AddedAfter || st.AddedAfterCause != AddedAfterCouldNotCheck {
		t.Errorf("addedAfter %v cause %q after the library dropped a signature the sweep found, want true %q — the warning went quiet",
			st.AddedAfter, st.AddedAfterCause, AddedAfterCouldNotCheck)
	}
	if len(st.Signers) != 0 {
		t.Errorf("%d signers named after the library reported none", len(st.Signers))
	}
}

// TestTheSignerFingerprintHasOneWriter — ADR-058's door: `SignerInfo.Fingerprint` is written in
// `signerInfo` from the record, and `Revision.Fingerprint` in `joinLibrary` where the record
// verified. A third writer of a `Fingerprint` field in this package is a second home for "who
// signed", which is what ADR-051's hole and ADR-058's supersession were both about.
func TestTheSignerFingerprintHasOneWriter(t *testing.T) {
	sites := fingerprintWriters(t, ".")
	want := map[string]int{"signerInfo": 1, "joinLibrary": 1}
	if fmt.Sprint(sites) != fmt.Sprint(want) {
		t.Errorf("Fingerprint fields are written in %v, want exactly %v", sites, want)
	}
}
