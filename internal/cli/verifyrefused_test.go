package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	dpdf "github.com/digitorus/pdf"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

var startxrefRe = regexp.MustCompile(`startxref\s+(\d+)`)

// appendRevision appends objs (number → body) to a signed document as an incremental update with
// an uncompressed xref stream (pdfsign writes streams, and digitorus cannot follow a classic table
// whose /Prev names one). The sign package's fixture builders are test-only, so the CLI keeps its
// own; it is the smallest writer that reaches `cmdVerify` with each /pending 661 and 687 shape.
func appendRevision(t *testing.T, prev []byte, objs map[int]string) []byte {
	t.Helper()
	r, err := dpdf.NewReader(bytes.NewReader(prev), int64(len(prev)))
	if err != nil {
		t.Fatal(err)
	}
	size := int(r.Trailer().Key("Size").Int64())
	rp := r.Trailer().Key("Root").GetPtr()
	all := startxrefRe.FindAllSubmatch(prev, -1)
	if len(all) == 0 {
		t.Fatal("no startxref")
	}
	prevX, _ := strconv.Atoi(string(all[len(all)-1][1]))
	nums := make([]int, 0, len(objs))
	for n := range objs {
		nums = append(nums, n)
		size = max(size, n+1)
	}
	sort.Ints(nums)
	x := size
	var b bytes.Buffer
	b.Write(prev)
	b.WriteString("\n")
	offs := map[int]int{}
	for _, n := range nums {
		offs[n] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, objs[n])
	}
	offs[x] = b.Len()
	nums = append(nums, x)
	var data bytes.Buffer
	var index []string
	for _, n := range nums {
		o := offs[n]
		data.Write([]byte{1, byte(o >> 24), byte(o >> 16), byte(o >> 8), byte(o), 0, 0})
		index = append(index, fmt.Sprintf("%d 1", n))
	}
	fmt.Fprintf(&b, "%d 0 obj\n<</Type/XRef/Size %d/W[1 4 2]/Index[%s]/Root %d 0 R/Prev %d/Length %d>>\nstream\n",
		x, x+1, strings.Join(index, " "), rp.GetID(), prevX, data.Len())
	b.Write(data.Bytes())
	fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", offs[x])
	return b.Bytes()
}

// decoyBody is /pending 661's decoy: a signature-shaped dictionary claiming coverage to 999,999,999.
func decoyBody(filterClause string) string {
	return fmt.Sprintf("<</Type/Sig%s/ByteRange[0 10 20 999999999]/Contents<0001020304>>>", filterClause)
}

// appendDecoyRevision appends the decoy and a field pointing at it, NOT listed in `/Fields`.
func appendDecoyRevision(t *testing.T, signed []byte, filter string) (doc []byte, decoy int) {
	t.Helper()
	r, err := dpdf.NewReader(bytes.NewReader(signed), int64(len(signed)))
	if err != nil {
		t.Fatal(err)
	}
	decoy = int(r.Trailer().Key("Size").Int64())
	return appendRevision(t, signed, map[int]string{
		decoy:     decoyBody("/Filter/" + filter),
		decoy + 1: fmt.Sprintf("<</FT/Sig/T(decoy)/V %d 0 R>>", decoy),
	}), decoy
}

// appendListedDecoy is the decoy as /pending 661's reproduction built it: the appended revision also
// rewrites the AcroForm holder so `/Fields` lists the decoy field beside the real ones — the shape
// the deleted `/Fields` ByteRange walk took coverage from (on `11490690` it read `valid` with no
// warning, so the CLI exited 0).
func appendListedDecoy(t *testing.T, signed []byte, filterClause string) (doc []byte, decoy int) {
	t.Helper()
	r, err := dpdf.NewReader(bytes.NewReader(signed), int64(len(signed)))
	if err != nil {
		t.Fatal(err)
	}
	decoy = int(r.Trailer().Key("Size").Int64())
	rootV := r.Trailer().Key("Root")
	rp := rootV.GetPtr()
	root := int(rp.GetID())
	acro := rootV.Key("AcroForm")
	var refs []string
	for i := 0; i < acro.Key("Fields").Len(); i++ {
		fp := acro.Key("Fields").Index(i).GetPtr()
		refs = append(refs, fmt.Sprintf("%d 0 R", fp.GetID()))
	}
	refs = append(refs, fmt.Sprintf("%d 0 R", decoy+1))
	acroBody := fmt.Sprintf("<</Fields[%s]/SigFlags 3>>", strings.Join(refs, " "))
	objs := map[int]string{
		decoy:     decoyBody(filterClause),
		decoy + 1: fmt.Sprintf("<</FT/Sig/T(decoy)/V %d 0 R>>", decoy),
	}
	app := acro.GetPtr()
	if ap := int(app.GetID()); ap != 0 && ap != root {
		objs[ap] = acroBody
	} else {
		pp := rootV.Key("Pages").GetPtr()
		objs[root] = fmt.Sprintf("<</Type/Catalog/Pages %d 0 R/AcroForm%s>>", pp.GetID(), acroBody)
	}
	return appendRevision(t, signed, objs), decoy
}

// appendCopiedDictionary is /pending 687: a NEW object carrying the victim's signature dictionary,
// its `/ByteRange` rewritten by br (nil keeps it). The library alone verifies it as a second signer.
func appendCopiedDictionary(t *testing.T, signed []byte, br func(string) string) (doc []byte, obj int) {
	t.Helper()
	vi := bytes.Index(signed, []byte("/Adobe.PPKLite"))
	if vi < 0 {
		t.Fatal("no PPKLite dictionary")
	}
	hs := bytes.LastIndex(signed[:vi], []byte(" 0 obj"))
	oe := hs + bytes.Index(signed[hs:], []byte("endobj"))
	body := strings.TrimSpace(string(signed[hs+len(" 0 obj") : oe]))
	if br != nil {
		re := regexp.MustCompile(`/ByteRange\s*\[([^\]]*)\]`)
		m := re.FindStringSubmatch(body)
		if m == nil {
			t.Fatal("victim has no /ByteRange")
		}
		body = re.ReplaceAllString(body, "/ByteRange ["+br(m[1])+"]")
	}
	r, err := dpdf.NewReader(bytes.NewReader(signed), int64(len(signed)))
	if err != nil {
		t.Fatal(err)
	}
	obj = int(r.Trailer().Key("Size").Int64())
	return appendRevision(t, signed, map[int]string{obj: body}), obj
}

// TestVerifyPrintsEachRefusedSignature — P01.S02's CLI reader. A refused decoy makes `nib verify`
// exit 2, name the object and its cause on its own line, and say which fact set the warning; the
// decoy's `/Filter` is attacker-typed, and a PDF name decodes `#1B` to an escape byte, so it is
// printed quoted and never reaches the terminal raw.
func TestVerifyPrintsEachRefusedSignature(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	cert, key, _ := sign.GenerateIdentity("Signer")
	signed, err := sign.SignApproval(base, cert, key, sign.Options{Name: "Signer", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	doc, decoy := appendDecoyRevision(t, signed, "Evil#1B#5B31mRED")
	path := filepath.Join(t.TempDir(), "decoy.pdf")
	if err := os.WriteFile(path, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	// STIMULUS: the signature still verifies, the decoy is refused, and its filter carries an
	// escape byte once decoded — so every line below is the reader's work.
	st := sign.Verify(doc)
	if st.State != sign.Valid || len(st.Refused) != 1 || st.Refused[0].Obj != uint32(decoy) || !strings.Contains(st.Refused[0].Filter, "\x1b") {
		t.Fatalf("STIMULUS: state %s refused %+v; want valid with object %d refused under a filter holding ESC", st.State, st.Refused, decoy)
	}
	out, code := captureStdout(t, func() int { return cmdVerify([]string{path}) })
	if code != 2 {
		t.Errorf("exit %d, want 2 — a document carrying a refused signature is not wholly signed", code)
	}
	want := fmt.Sprintf(`refused: object %d (filter "Evil\x1b[31mRED"): %s`, decoy, sign.CauseUnsupportedFilter)
	if !strings.Contains(out, want) {
		t.Errorf("output does not carry %q:\n%s", want, out)
	}
	if !strings.Contains(out, "a signature Nib refused is present") {
		t.Errorf("the status line does not say a refused signature set the warning:\n%s", out)
	}
	if strings.ContainsRune(out, 0x1b) {
		t.Errorf("an escape byte from the document's /Filter reached the terminal raw: %q", out)
	}
}

// TestVerifyRefusesEveryDecoyAndCopy — the S02 acceptance through the binary's own command: both
// LISTED /pending 661 decoys and both /pending 687 copied dictionaries exit 2, print the refused
// object and its cause, and say both facts in the status line. The listed decoys are the shapes the
// deleted `/Fields` walk measured coverage from; the unlisted one above warned on HEAD already.
func TestVerifyRefusesEveryDecoyAndCopy(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	cert, key, _ := sign.GenerateIdentity("Signer")
	signed, err := sign.SignApproval(base, cert, key, sign.Options{Name: "Signer", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	type fixture struct {
		name    string
		build   func() ([]byte, int)
		filter  string
		cause   sign.RefusalCause
		signers int
	}
	for _, tc := range []fixture{
		{"listed decoy, no /Filter", func() ([]byte, int) { return appendListedDecoy(t, signed, "") }, "", sign.CauseUnsupportedFilter, 1},
		{"listed decoy, PPKLite", func() ([]byte, int) { return appendListedDecoy(t, signed, "/Filter/Adobe.PPKLite") }, "Adobe.PPKLite", sign.CauseUnparseableContents, 1},
		{"copied dictionary, victim's four plus 999999999 0", func() ([]byte, int) {
			return appendCopiedDictionary(t, signed, func(br string) string { return br + " 999999999 0" })
		}, "Adobe.PPKLite", sign.CauseMalformedByteRange, 2},
		{"copied dictionary under a new number", func() ([]byte, int) { return appendCopiedDictionary(t, signed, nil) }, "Adobe.PPKLite", sign.CauseContentsElsewhere, 2},
	} {
		doc, obj := tc.build()
		path := filepath.Join(t.TempDir(), "doc.pdf")
		if err := os.WriteFile(path, doc, 0o600); err != nil {
			t.Fatal(err)
		}
		// STIMULUS: the real signature still verifies (and a copy verifies beside it), so the
		// document is Valid and only the coverage rule can make the command exit 2.
		st := sign.Verify(doc)
		if st.State != sign.Valid || len(st.Signers) != tc.signers {
			t.Fatalf("%s: STIMULUS: state %s signers %d, want valid/%d", tc.name, st.State, len(st.Signers), tc.signers)
		}
		if !st.AddedAfter || st.AddedAfterCause != sign.AddedAfterRefusedSignature {
			t.Errorf("%s: addedAfter %v cause %q, want true %q", tc.name, st.AddedAfter, st.AddedAfterCause, sign.AddedAfterRefusedSignature)
		}
		out, code := captureStdout(t, func() int { return cmdVerify([]string{path}) })
		if code != 2 {
			t.Errorf("%s: exit %d, want 2", tc.name, code)
		}
		if want := fmt.Sprintf("refused: object %d (filter %q): %s", obj, tc.filter, tc.cause); !strings.Contains(out, want) {
			t.Errorf("%s: output does not carry %q:\n%s", tc.name, want, out)
		}
		if !strings.Contains(out, "content added after the last signature") || !strings.Contains(out, "a signature Nib refused is present") {
			t.Errorf("%s: the status line does not name both facts:\n%s", tc.name, out)
		}
	}
}
