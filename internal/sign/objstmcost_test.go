package sign

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/scaling"

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

func lookupCostOf(t *testing.T, doc []byte) (lookupCost, error) {
	t.Helper()
	r, err := dpdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatalf("STIMULUS: the reader refuses the fixture: %v", err)
	}
	return libraryLookupCost(r)
}

// stmSpec is one object stream of rawObjStmDoc: its header text and member text, `/N`, `/First`
// (the header's length when negative), `/Extends` (an object number, 0 for none), and whether it is
// flate-encoded; mangle, when set, rewrites the encoded bytes, and `/Length` is the result's.
type stmSpec struct {
	hdr, content string
	n, first     int
	extends      int
	flate        bool
	mangle       func([]byte) []byte
}

// rawObjStmDoc is an unsigned one-page document holding the given object streams (by object number)
// and an xref stream naming each member id as (stream, index).
func rawObjStmDoc(streams map[int]stmSpec, members map[int][2]int) []byte {
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
	top := 3
	for n := range streams {
		top = max(top, n)
	}
	for n := range members {
		top = max(top, n)
	}
	for n := 4; n <= top; n++ {
		s, ok := streams[n]
		if !ok {
			continue
		}
		data := []byte(s.hdr + s.content)
		filter := ""
		if s.flate {
			var z bytes.Buffer
			w := zlib.NewWriter(&z)
			w.Write(data)
			w.Close()
			data, filter = z.Bytes(), "/Filter/FlateDecode"
		}
		if s.mangle != nil {
			data = s.mangle(data)
		}
		first := s.first
		if first < 0 {
			first = len(s.hdr)
		}
		ext := ""
		if s.extends != 0 {
			ext = fmt.Sprintf("/Extends %d 0 R", s.extends)
		}
		off[n] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n<</Type/ObjStm/N %d/First %d%s%s/Length %d>>\nstream\n", n, s.n, first, ext, filter, len(data))
		b.Write(data)
		b.WriteString("\nendstream\nendobj\n")
	}
	x := top + 1
	off[x] = b.Len()
	var data bytes.Buffer
	for n := 0; n <= x; n++ {
		m, in := members[n]
		switch {
		case n == 0:
			data.Write([]byte{0, 0, 0, 0, 0, 0xff, 0xff})
		case in:
			data.Write([]byte{2, byte(m[0] >> 24), byte(m[0] >> 16), byte(m[0] >> 8), byte(m[0]), byte(m[1] >> 8), byte(m[1])})
		case off[n] != 0:
			o := off[n]
			data.Write([]byte{1, byte(o >> 24), byte(o >> 16), byte(o >> 8), byte(o), 0, 0})
		default:
			data.Write([]byte{0, 0, 0, 0, 0, 0, 0})
		}
	}
	fmt.Fprintf(&b, "%d 0 obj\n<</Type/XRef/Size %d/W[1 4 2]/Root 1 0 R/Length %d>>\nstream\n", x, x+1, data.Len())
	b.Write(data.Bytes())
	fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", off[x])
	return b.Bytes()
}

// chainDoc is k object streams, each extending the next, with m members listed only in the LAST —
// and named by the xref as in the FIRST, so every lookup walks the whole chain. cycle makes the last
// stream extend the first and adds a member no header lists.
func chainDoc(k, m int, cycle bool) []byte {
	streams := map[int]stmSpec{}
	for i := 0; i < k-1; i++ {
		streams[4+i] = stmSpec{content: " ", n: 0, first: 1, extends: 5 + i}
	}
	var hdr, content strings.Builder
	members := map[int][2]int{}
	for i := 0; i < m; i++ {
		fmt.Fprintf(&hdr, "%d %d ", 4+k+i, content.Len())
		content.WriteString("<</F 1>>\n")
		members[4+k+i] = [2]int{4, i}
	}
	last := stmSpec{hdr: hdr.String(), content: content.String(), n: m, first: -1, flate: true}
	if cycle {
		last.extends = 4
		members[4+k+m] = [2]int{4, m} // a phantom: listed by no header, so its lookup walks the cycle
	}
	streams[4+k-1] = last
	return rawObjStmDoc(streams, members)
}

// resolveMember resolves object id through r as the sweep does, reporting a panic as an error.
func resolveMember(r *dpdf.Reader, id int) (v dpdf.Value, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("%v", rec)
		}
	}()
	p := r.Xref()[id].Ptr()
	return r.Resolve(p, p), nil
}

// resolveEvery resolves every xref entry through one Reader — one pass of the sweep or the library.
func resolveEvery(t *testing.T, doc []byte) time.Duration {
	t.Helper()
	r, err := dpdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatalf("STIMULUS: the reader refuses the fixture: %v", err)
	}
	return scaling.TimeOnce(func() {
		for i := range r.Xref() {
			resolveMember(r, i)
		}
	})
}

// countingReaderAt counts the bytes read through it.
type countingReaderAt struct {
	r io.ReaderAt
	n int64
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	k, err := c.r.ReadAt(p, off)
	c.n += int64(k)
	return k, err
}

// fileBytesResolvingEvery is resolveEvery's pass, counting the bytes it reads from the file (opening the
// Reader excluded).
func fileBytesResolvingEvery(t *testing.T, doc []byte) int64 {
	t.Helper()
	f := &countingReaderAt{r: bytes.NewReader(doc)}
	r, err := dpdf.NewReader(f, int64(len(doc)))
	if err != nil {
		t.Fatalf("STIMULUS: the reader refuses the fixture: %v", err)
	}
	f.n = 0
	for i := range r.Xref() {
		resolveMember(r, i)
	}
	return f.n
}

// TestAnObjectStreamOfTwentyThousandMembersVerifies is /pending 751's own case, re-pinned by /pending
// 758. This 561 KB file — one honest signature, one 20,000-member stream — took the unpatched reader
// 129 s to verify (66 s a pass), so /pending 751 refused it `Invalid`. The patched reader decodes the
// stream and lexes its header once a pass, so it now costs the verdict milliseconds, reads `valid`,
// and is far under the ceiling.
func TestAnObjectStreamOfTwentyThousandMembersVerifies(t *testing.T) {
	doc := memberDoc(t, newIdentity(t, "Alice"), 20000)
	if err := pdfcpuCanRead(doc); err != nil || !scanForSignatureBlob(doc) {
		t.Fatalf("STIMULUS: pdfcpu err=%v, blob=%v — the file must reach the sweep", err, scanForSignatureBlob(doc))
	}
	c, err := lookupCostOf(t, doc)
	if err != nil || c.work() > maxLookupWork/10 {
		t.Errorf("the lookup cost is %d (%.3f of the ceiling), err=%v — the patched reader pays this stream once", c.work(), float64(c.work())/maxLookupWork, err)
	}
	calls := countingLibrary(t)
	t0 := time.Now()
	st := Verify(doc)
	took := time.Since(t0)
	if st.State != Valid || st.AddedAfter || len(st.Signers) != 1 || st.Signers[0].Fingerprint == "" {
		t.Errorf("state=%q addedAfter=%v(%q) signers=%d, want one valid signer and nothing added", st.State, st.AddedAfter, st.AddedAfterCause, len(st.Signers))
	}
	if *calls != 1 {
		t.Errorf("the library was called %d time(s), want 1", *calls)
	}
	if _, err := Revisions(doc); err != nil {
		t.Errorf("Revisions error %v, want none", err)
	}
	if took > 5*time.Second {
		t.Errorf("Verify took %v on a 20,000-member object stream; the patched reader reads it in well under a second", took)
	}
}

// TestVerifyIsNotQuadraticInObjectStreamMembers: four times the members must cost well under sixteen
// times the time; the pre-fix tree measured ~16x (2,000 → 8,000 members: 1.1 s → 17 s).
//
// Linear is x4 and the unpatched library's quadratic x16 (measured x14.3 on it); x8 is the midpoint on a
// log scale. Alone this measures x3.2-x4.3; best-of-N per size, one size after the other, tipped x6.1 under
// a loaded full suite, so the sizes are interleaved through `scaling` (/pending 799).
func TestVerifyIsNotQuadraticInObjectStreamMembers(t *testing.T) {
	a := newIdentity(t, "Alice")
	docs := map[int][]byte{}
	scaling.GrowsLinearly(t, "Verify over one object stream", 2000, 8000, 8, func(n int) time.Duration {
		if docs[n] == nil {
			docs[n] = memberDoc(t, a, n)
		}
		return scaling.TimeOnce(func() { Verify(docs[n]) })
	})
}

// TestTheReaderResolvesAnObjectStreamInLinearTime is the patch's own scaling case, below Verify (whose
// fixed costs would hide a quadratic at small sizes): one pass over a 5,000- and a 20,000-member stream.
// Unpatched, measured: 3.2 s → 66 s (21x). Four times the members must cost under eight times the time —
// it was six, best of three per size one size after the other, and failed at 6.1x under load against
// good code (/pending 799); x8 is the log midpoint of x4 and x16.
//
// **Counted first, timed second.** The decode half of the patch is WORK the reader does on the file, so it is
// counted: the bytes one pass reads from the file, which a reader that re-decodes the stream per lookup (as
// upstream did) multiplies by the members and the patched reader does not. That count is the machine's to
// neither help nor hinder; the clock under it stays for a CPU-only quadratic the file never sees (a header
// re-lexed per lookup from cached bytes), and the clock alone passed the re-decode at load ~30 (/pending 799).
func TestTheReaderResolvesAnObjectStreamInLinearTime(t *testing.T) {
	a := newIdentity(t, "Alice")
	docs := map[int][]byte{}
	read := func(n int) int64 {
		if docs[n] == nil {
			docs[n] = memberDoc(t, a, n)
		}
		return fileBytesResolvingEvery(t, docs[n])
	}
	small, large := read(1000), read(4000)
	t.Logf("one pass reads %d file bytes at 1,000 members and %d at 4,000 (x%.2f)", small, large, float64(large)/float64(small))
	if small <= 0 || float64(large)/float64(small) > 8 {
		t.Fatalf("4x the object-stream members read x%.1f the file bytes (%d → %d) — the reader decodes the stream again per lookup",
			float64(large)/float64(small), small, large)
	}
	scaling.GrowsLinearly(t, "resolving every member", 5000, 20000, 8, func(n int) time.Duration {
		if docs[n] == nil {
			docs[n] = memberDoc(t, a, n)
		}
		return resolveEvery(t, docs[n])
	})
}

// TestAMemberBeforeACorruptTailStillResolves: the patched reader decodes an object stream only as far
// as a lookup needs, as upstream did, so damage after a member never reaches that member's lookup. An
// eager decode (read the whole stream, then serve members from it) turns every such member into a
// panic — the unfaithful prototype this patch replaced did exactly that.
func TestAMemberBeforeACorruptTailStillResolves(t *testing.T) {
	pad := "[" + strings.Repeat("0 ", 100_000) + "]\n"
	var hdr, content strings.Builder
	members := map[int][2]int{}
	for i := 0; i < 4; i++ {
		fmt.Fprintf(&hdr, "%d %d ", 5+i, content.Len())
		if i < 3 {
			content.WriteString(fmt.Sprintf("<</F %d>>\n", i))
		} else {
			content.WriteString(pad)
		}
		members[5+i] = [2]int{4, i}
	}
	for _, c := range []struct {
		name   string
		mangle func([]byte) []byte
	}{
		{"a short /Length", func(b []byte) []byte { return b[:len(b)/2] }},
		{"a corrupt zlib tail", func(b []byte) []byte {
			b = bytes.Clone(b)
			for i := len(b) / 2; i < len(b); i++ {
				b[i] = 0xff
			}
			return b
		}},
	} {
		doc := rawObjStmDoc(map[int]stmSpec{4: {hdr: hdr.String(), content: content.String(), n: 4, first: -1, flate: true, mangle: c.mangle}}, members)
		r, err := dpdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
		if err != nil {
			t.Fatalf("%s: STIMULUS: %v", c.name, err)
		}
		if _, err := resolveMember(r, 8); err == nil {
			t.Fatalf("%s: STIMULUS: the member past the damage resolved — the damage is not after it", c.name)
		}
		for i := 0; i < 3; i++ {
			v, err := resolveMember(r, 5+i)
			if err != nil || v.Key("F").Int64() != int64(i) {
				t.Errorf("%s: member %d before the damage: %v, err=%v — want <</F %d>>, as the unpatched reader read it", c.name, 5+i, v, err, i)
			}
		}
		if _, err := resolveMember(r, 8); err == nil {
			t.Errorf("%s: the member past the damage resolved on a second lookup — the damage was not raised again", c.name)
		}
	}
}

// TestAnOverclaimedHeaderStillResolvesTheMembersItListsBeforeABadToken: the patched reader lexes the
// header only as far as the wanted id, as upstream did. An /N claiming five pairs over a header whose
// third pair is a lexer error still yields the two members before it; a lookup that needs the header
// past the error raises it, before and after the members are read.
func TestAnOverclaimedHeaderStillResolvesTheMembersItListsBeforeABadToken(t *testing.T) {
	doc := rawObjStmDoc(map[int]stmSpec{4: {hdr: "5 0 6 9 ) 1 ", content: "<</F 0>>\n<</F 1>>\n", n: 5, first: -1}},
		map[int][2]int{5: {4, 0}, 6: {4, 1}, 7: {4, 2}})
	r, err := dpdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatalf("STIMULUS: %v", err)
	}
	members := func(when string) {
		for i := 0; i < 2; i++ {
			v, err := resolveMember(r, 5+i)
			if err != nil || v.Key("F").Int64() != int64(i) {
				t.Errorf("%s: member %d, listed before the bad token: %v, err=%v — want <</F %d>>", when, 5+i, v, err, i)
			}
		}
	}
	// On a fresh Reader first: a lexer that read the whole header up front fails here.
	members("first")
	if _, err := resolveMember(r, 7); err == nil || !strings.Contains(err.Error(), "unexpected delimiter") {
		t.Fatalf("STIMULUS: the id past the bad token: err=%v, want the lexer's delimiter panic", err)
	}
	members("after the error")
	if _, err := resolveMember(r, 7); err == nil {
		t.Errorf("the id past the bad token resolved on a second lookup — the lexer's error was not raised again")
	}
}

// TestTheLookupCostChargesWhatThePatchedReaderReads pins the figure to the patched reader: a stream's
// header pairs and decoded bytes are charged ONCE, as far as the furthest lookup reached, and every
// lookup is charged a dictionary read for each stream it visits.
func TestTheLookupCostChargesWhatThePatchedReaderReads(t *testing.T) {
	c, err := lookupCostOf(t, memberDoc(t, newIdentity(t, "Alice"), 100))
	if err != nil || c.pairs < 100 || c.pairs > 110 || c.visits < 100 || c.visits > 110 {
		t.Errorf("100 members: %+v err=%v, want ~100 pairs (once each) and ~100 visits", c, err)
	}
	doc := streamDoc(64, 3, 0, true)
	c, err = lookupCostOf(t, doc)
	if err != nil || c.pairs != 3 || c.bytes < 64 || c.visits != 4 {
		t.Errorf("a phantom member of a 3-member stream: %+v err=%v, want 3 pairs (lexed once, to the end), the whole stream, 4 visits", c, err)
	}
	// /N of two billion over a three-member header: past the header's end the patched reader reads the
	// rest as one (0, 0) pair, so the claim is charged no more pairs than the decoded bytes can hold.
	c, err = lookupCostOf(t, streamDoc(64, 3, 2_000_000_000, true))
	if err != nil || c.pairs > c.bytes/2+1 {
		t.Errorf("a phantom member under a /N of two billion: %+v err=%v, want admitted and at most one pair per two decoded bytes", c, err)
	}
	// A pad the members after it read past: decoded once, not once per member.
	c, err = lookupCostOf(t, streamDoc(16<<20, 50, 0, false))
	if err != nil || c.bytes < 16<<20 || c.bytes > 17<<20 {
		t.Errorf("49 members behind a 16 MiB pad: %+v err=%v, want the pad charged once", c, err)
	}
}

// TestAnExtendsChainTimesItsMembersIsRefused is the quadratic the patch did NOT remove: each lookup
// re-reads the dictionary of every stream on its `/Extends` chain. 2,000 streams in a chain, 2,000
// members at its end: four million dictionary reads a pass. And a chain that cycles never terminates.
func TestAnExtendsChainTimesItsMembersIsRefused(t *testing.T) {
	c, err := lookupCostOf(t, chainDoc(2000, 2000, false))
	if !errors.Is(err, errLookupCostCeiling) {
		t.Errorf("a 2,000-stream chain under 2,000 members: %+v err=%v, want errLookupCostCeiling", c, err)
	}
	if c, err := lookupCostOf(t, chainDoc(20, 20, false)); err != nil || c.visits != 20*20 {
		t.Errorf("a 20-stream chain under 20 members: %+v err=%v, want 400 visits and admitted", c, err)
	}
	if _, err := lookupCostOf(t, chainDoc(3, 2, true)); !errors.Is(err, errLookupCostCeiling) {
		t.Errorf("an /Extends cycle: err=%v, want errLookupCostCeiling", err)
	}
}

// TestADecodedObjectStreamPastTheMemoryCeilingIsRefused: the patched reader HOLDS what it decodes for
// the pass, so the decoded bytes are capped as memory (the byte term alone, ~57 MiB). A ~100 KB flate
// stream of blanks decodes to 100 MiB; a lookup that misses its header decodes all of it.
func TestADecodedObjectStreamPastTheMemoryCeilingIsRefused(t *testing.T) {
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	w.Write([]byte("5 0 "))
	blank := bytes.Repeat([]byte{' '}, 1<<20)
	for i := 0; i < 100; i++ {
		w.Write(blank)
	}
	w.Close()
	enc := z.Bytes()
	doc := rawObjStmDoc(map[int]stmSpec{4: {hdr: "5 0 ", content: "", n: 1, first: 4, flate: true, mangle: func([]byte) []byte { return enc }}},
		map[int][2]int{5: {4, 0}, 6: {4, 1}})
	if c, err := lookupCostOf(t, doc); !errors.Is(err, errLookupCostCeiling) {
		t.Errorf("a phantom member of a stream decoding to 100 MiB: %+v err=%v, want errLookupCostCeiling", c, err)
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
		c, err := lookupCostOf(t, doc)
		if err != nil {
			t.Errorf("%s: %v — an honest producer's file is refused", f, err)
		}
		if w := c.work(); w > worst {
			worst, worstName = w, filepath.Base(f)
		}
	}
	t.Logf("%d files; the dearest is %s at %.4f of the ceiling", len(files), worstName, float64(worst)/maxLookupWork)
}
