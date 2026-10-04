package pdfops

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// lawOneCorpus is one population `PLAN-text-reflow.md` P05 names: its documents, or why it is absent.
type lawOneCorpus struct {
	name   string
	docs   []runCorpusDoc
	absent string
}

// externalPDFs reads every PDF under the directory env names, else under def. An absent directory is a reason,
// never an empty corpus that passes.
func externalPDFs(t *testing.T, name, env, def string) lawOneCorpus {
	t.Helper()
	root := os.Getenv(env)
	if root == "" {
		root = def
	}
	if root == "" {
		return lawOneCorpus{name: name, absent: "no root; set " + env}
	}
	if _, err := os.Stat(root); err != nil {
		return lawOneCorpus{name: name, absent: fmt.Sprintf("%s is not on this machine; set %s", root, env)}
	}
	var files []string
	if err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".pdf") {
			files = append(files, p)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	c := lawOneCorpus{name: name}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, f)
		c.docs = append(c.docs, runCorpusDoc{filepath.ToSlash(rel), b})
	}
	if len(c.docs) == 0 {
		c.absent = "no PDFs under " + root
	}
	return c
}

// lawOneCensus counts what a corpus exercised, so a green round trip is never read as coverage of a construct
// the corpus did not contain.
type lawOneCensus struct {
	docs, unread, pages, pageStreams, forms, arrayPages, joins, separated int
	seen                                                                  map[string]int
}

// constructs are the exit criterion's list, by the name the census reports.
var lawOneConstructs = []string{"inline image", "literal string", "hex string", "dictionary operand",
	"marked content", "a form XObject a page reaches"}

// pdfOperators is ISO 32000-1 Annex A, Table A.1 — every operator a content stream may carry.
var pdfOperators = func() map[string]bool {
	m := map[string]bool{}
	for _, o := range strings.Fields(`b b* B B* BDC BI BMC BT BX c cm CS cs d d0 d1 Do DP EI EMC ET EX f F f* G g gs h ` +
		`i ID j J K k l m M MP n q Q re RG rg ri s S SC sc SCN scn sh T* Tc Td TD Tf Tj TJ TL Tm Tr Ts Tw Tz v w W W* y ' "` +
		// Not operators: `Tokenize` emits these operand keywords as Operator tokens (/pending 632), and a boolean in a
		// marked-content property list is legal.
		` true false null`) {
		m[o] = true
	}
	return m
}()

// unknownOperator is the first operator token outside Table A.1 that no `BX`…`EX` compatibility section excuses.
//
// **This is what makes law 1 see a MIS-SCAN.** `WriteTokens` copies spans, so ANY tokenization that covers the stream
// round-trips — an inline image ended one byte early still writes back byte-identically, leaving its `I` as the next
// "operator". The review measured all three inline-image end paths surviving the round trip alone; a mis-scan
// surfaces as an operator no producer writes. Measured at the same time: 0 unknown operators across 330 documents.
func unknownOperator(src []byte, toks []contentstream.Token) (string, bool) {
	compat := 0
	for _, tk := range toks {
		if tk.Kind != contentstream.Operator {
			continue
		}
		op := string(tk.Bytes(src))
		switch {
		case op == "BX":
			compat++
		case op == "EX" && compat > 0:
			compat--
		case !pdfOperators[op] && compat == 0:
			return op, true
		}
	}
	return "", false
}

func (c *lawOneCensus) tally(src []byte, toks []contentstream.Token) {
	for _, tk := range toks {
		switch tk.Kind {
		case contentstream.InlineImage:
			c.seen["inline image"]++
		case contentstream.LiteralString:
			c.seen["literal string"]++
		case contentstream.HexString:
			c.seen["hex string"]++
		case contentstream.DictOpen:
			c.seen["dictionary operand"]++
		case contentstream.Operator:
			switch string(tk.Bytes(src)) {
			case "BMC", "BDC", "EMC":
				c.seen["marked content"]++
			}
		}
	}
}

// lawOneRoundTrip is law 1 on one stream: the tokens cover it completely and without overlap, and writing them
// back gives the same bytes. It returns the first failure, naming the offset.
func lawOneRoundTrip(src []byte) ([]contentstream.Token, error) {
	toks := contentstream.Tokenize(src)
	prev := 0
	for i, tk := range toks {
		if tk.Start != prev || tk.End <= tk.Start {
			return nil, fmt.Errorf("token %d spans [%d,%d) after the previous ended at %d — the tokens do not "+
				"cover the stream", i, tk.Start, tk.End, prev)
		}
		prev = tk.End
	}
	if prev != len(src) {
		return nil, fmt.Errorf("the tokens cover %d of %d bytes", prev, len(src))
	}
	out, err := contentstream.WriteTokens(src, toks)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(out, src) {
		at := 0
		for at < len(out) && at < len(src) && out[at] == src[at] {
			at++
		}
		return nil, fmt.Errorf("the round trip differs at byte %d of %d", at, len(src))
	}
	return toks, nil
}

// formsOf is every form XObject reachable from res through `/XObject`, recursively, each object once.
func formsOf(ctx *model.Context, res types.Dict, done map[int]bool, visit func(name string, body []byte)) {
	if res == nil {
		return
	}
	xobjs, err := ctx.DereferenceDict(res["XObject"])
	if err != nil || xobjs == nil {
		return
	}
	names := make([]string, 0, len(xobjs))
	for n := range xobjs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		o := xobjs[n]
		if ir, ok := o.(types.IndirectRef); ok {
			if done[ir.ObjectNumber.Value()] {
				continue
			}
			done[ir.ObjectNumber.Value()] = true
		}
		sd, _, err := ctx.DereferenceStreamDict(o)
		if err != nil || sd == nil {
			continue
		}
		if st := sd.Dict.NameEntry("Subtype"); st == nil || *st != "Form" {
			continue
		}
		body := streamContent(sd)
		if body == nil {
			continue
		}
		visit(n, body)
		inner, _ := ctx.DereferenceDict(sd.Dict["Resources"])
		formsOf(ctx, inner, done, visit)
	}
}

// TestLawOneHoldsOverRealDocuments — `PLAN-text-reflow.md` P05.S01, law 1: *a rewrite that changes nothing
// changes nothing*. Every page's content, read through the door (ADR-056), and every form XObject its resources
// reach, tokenizes with complete coverage and writes back byte-identically — over the generated corpus, the
// real-producer corpus and veraPDF's PDF/UA-1 corpus.
//
// **Byte-identical is the DECODED stream**, per the phase-open pin: the file cannot be, because pdfcpu's writer
// re-encodes (the document-level half is `ContentDigest`, P05.S02).
//
// **The census is the assertion that makes the round trip mean something**: each construct the exit criterion
// names must have been SEEN, or a green run over a corpus without inline images would read as proof they
// survive. With an external corpus absent, an unseen construct is a SKIP that names it, never a pass.
//
// **What it does not see**: annotation appearance streams and Type 3 glyph procedures (content streams nothing in
// reflow rewrites), and a form reached only through a pattern or a soft mask. **Nor two inline-image end paths**:
// no corpus document carries an ASCII-filtered or `/L`-declared inline image (their end mutations stay green here),
// so those are held by `contentstream`'s `TestTextAfterAnASCIIEncodedInlineImageSurvives` and
// `TestADeclaredLengthCarriesAnInlineImagePastAFalseEI` (both probed red); and no document uses `BX`/`EX`.
func TestLawOneHoldsOverRealDocuments(t *testing.T) {
	gen := lawOneCorpus{name: "generated", docs: runCorpus(t)}
	for _, s := range []testpdf.JoinShape{testpdf.JoinRegular, testpdf.JoinComment, testpdf.JoinSafe} {
		pdf, _, err := testpdf.SplitContents("a divided page", s)
		if err != nil {
			t.Fatal(err)
		}
		gen.docs = append(gen.docs, runCorpusDoc{fmt.Sprintf("divided page, shape %d", s), pdf})
	}
	corpora := []lawOneCorpus{
		gen,
		externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers")),
		externalPDFs(t, "veraPDF PDF/UA-1", externalCorpusEnv, defaultExternalCorpus()),
	}
	total := lawOneCensus{seen: map[string]int{}}
	var absent []string
	for _, corp := range corpora {
		if corp.absent != "" {
			absent = append(absent, corp.name+": "+corp.absent)
			continue
		}
		c := lawOneCensus{seen: map[string]int{}}
		for _, doc := range corp.docs {
			ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
			if err != nil {
				c.unread++ // a document pdfcpu will not read is not the walker's to round-trip
				t.Logf("%s / %s: not read (%v)", corp.name, doc.name, err)
				continue
			}
			c.docs++
			done := map[int]bool{}
			for p := 1; p <= ctx.PageCount; p++ {
				d, _, attrs, err := ctx.PageDict(p, false)
				if err != nil || d == nil {
					continue
				}
				c.pages++
				if o, _ := ctx.Dereference(d["Contents"]); o != nil {
					if arr, ok := o.(types.Array); ok {
						c.arrayPages++
						if len(arr) > 1 {
							c.joins += len(arr) - 1
						}
						raw, rerr := ctx.PageContent(d, p) //pagecontent:exempt test — the join this door replaces
						door, derr := pdfread.PageContent(ctx, d, p)
						if rerr == nil && derr == nil && !bytes.Equal(raw, door) {
							c.separated++
						}
					}
				}
				src, err := pdfread.PageContent(ctx, d, p)
				if err == nil {
					c.pageStreams++
					toks, rerr := lawOneRoundTrip(src)
					if rerr != nil {
						t.Errorf("%s / %s page %d: %v", corp.name, doc.name, p, rerr)
					} else {
						c.tally(src, toks)
						if op, bad := unknownOperator(src, toks); bad {
							t.Errorf("%s / %s page %d: the operator %.24q is not in ISO 32000-1 Table A.1 — a "+
								"mis-scan the round trip cannot see", corp.name, doc.name, p, op)
						}
					}
				} else if err != model.ErrNoContent {
					t.Logf("%s / %s page %d: content not decoded (%v)", corp.name, doc.name, p, err)
				}
				if attrs == nil {
					continue
				}
				formsOf(ctx, attrs.Resources, done, func(name string, body []byte) {
					c.forms++
					c.seen["a form XObject a page reaches"]++
					toks, rerr := lawOneRoundTrip(body)
					if rerr != nil {
						t.Errorf("%s / %s page %d form /%s: %v", corp.name, doc.name, p, name, rerr)
						return
					}
					c.tally(body, toks)
					if op, bad := unknownOperator(body, toks); bad {
						t.Errorf("%s / %s page %d form /%s: the operator %.24q is not in ISO 32000-1 Table A.1",
							corp.name, doc.name, p, name, op)
					}
				})
			}
		}
		// Criterion 3's stimulus: the generated corpus carries both fused shapes, and the door must have had to
		// separate exactly those two — a door that stopped separating still round-trips, so only this count sees it.
		if corp.name == gen.name && c.separated != 2 {
			t.Errorf("generated: the door separated %d joins, want the 2 fused fixtures", c.separated)
		}
		if c.pageStreams == 0 {
			t.Errorf("%s: %d documents and not one page stream walked — the corpus exercised nothing", corp.name, len(corp.docs))
		}
		t.Logf("%s: %d documents read (%d not), %d pages, %d page streams, %d form streams, %d array pages, "+
			"%d joins, %d needed a separator; constructs %v",
			corp.name, c.docs, c.unread, c.pages, c.pageStreams, c.forms, c.arrayPages, c.joins, c.separated, c.seen)
		total.pageStreams += c.pageStreams
		total.forms += c.forms
		for k, v := range c.seen {
			total.seen[k] += v
		}
	}
	var unseen []string
	for _, k := range lawOneConstructs {
		if total.seen[k] == 0 {
			unseen = append(unseen, k)
		}
	}
	switch {
	case len(unseen) > 0 && len(absent) == 0:
		t.Errorf("every corpus is present and none exercised %s — the round trip says nothing about it",
			strings.Join(unseen, ", "))
	case len(unseen) > 0:
		t.Skipf("SKIP (not a pass): %s unexercised because %s", strings.Join(unseen, ", "), strings.Join(absent, "; "))
	case len(absent) > 0:
		t.Logf("NOTE (a narrower population, not a pass over it): %s", strings.Join(absent, "; "))
	}
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

// noOpRewrite reads pdf, writes every page's content back through the walker and `setPageContent` unchanged, and
// returns the written document with the number of pages rewritten and of those whose door join pdfcpu's differs from.
func noOpRewrite(t *testing.T, pdf []byte) (out []byte, rewritten, separated int, err error) {
	t.Helper()
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return nil, 0, 0, err
	}
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, derr := ctx.PageDict(p, false)
		if derr != nil || d == nil {
			continue
		}
		src, cerr := pdfread.PageContent(ctx, d, p)
		if cerr != nil {
			continue
		}
		raw, rerr := ctx.PageContent(d, p) //pagecontent:exempt test — the join ContentDigest hashes
		if rerr == nil && !bytes.Equal(raw, src) {
			separated++
		}
		walked, werr := contentstream.WriteTokens(src, contentstream.Tokenize(src))
		if werr != nil {
			t.Fatalf("page %d: %v", p, werr)
		}
		if err := setPageContent(ctx, d, walked); err != nil {
			t.Fatalf("page %d: %v", p, err)
		}
		rewritten++
	}
	var buf bytes.Buffer
	if err := api.WriteContext(ctx, &buf); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), rewritten, separated, nil
}

// annotsReachAPage reports whether anything reachable from a page's `/Annots` — an annotation's `/P`, a link's
// `/Dest`, an action's `/D` — references a page object. `ContentDigest` hashes `/Annots` by following references and
// hashes a stream's DICTIONARY with its body, so under rule 4 such a document's digest covers its pages' content-stream
// encoding (`/Length`, `/Filter`) as well as their content (/pending 720). Rule 5 hashes a reached page as its position.
func annotsReachAPage(ctx *model.Context) bool {
	pages := map[int]bool{}
	for i := 1; i <= ctx.PageCount; i++ {
		if ir, err := ctx.PageDictIndRef(i); err == nil && ir != nil {
			pages[ir.ObjectNumber.Value()] = true
		}
	}
	seen := map[int]bool{}
	var reach func(o types.Object, depth int) bool
	reach = func(o types.Object, depth int) bool {
		if depth > 16 {
			return false
		}
		if ir, ok := o.(types.IndirectRef); ok {
			n := ir.ObjectNumber.Value()
			if pages[n] {
				return true
			}
			if seen[n] {
				return false
			}
			seen[n] = true
			o, _ = ctx.Dereference(ir)
		}
		switch v := o.(type) {
		case types.Dict:
			for _, e := range v {
				if reach(e, depth+1) {
					return true
				}
			}
		case types.StreamDict:
			for _, e := range v.Dict {
				if reach(e, depth+1) {
					return true
				}
			}
		case types.Array:
			for _, e := range v {
				if reach(e, depth+1) {
					return true
				}
			}
		}
		return false
	}
	for i := 1; i <= ctx.PageCount; i++ {
		if d, _, _, err := ctx.PageDict(i, false); err == nil && d != nil && reach(d["Annots"], 0) {
			return true
		}
	}
	return false
}

// TestANoOpWalkKeepsTheDigest — `PLAN-text-reflow.md` P05.S02, law 1 at the DOCUMENT level: a walk that changes
// nothing, written back through the page write door (`setPageContent`) and pdfcpu's writer, leaves `ContentDigest`
// unchanged. The file's bytes cannot be kept — pdfcpu re-encodes, and the deepdive measured a no-op write at
// `bytes_equal=false, digest_equal=true` — so the digest is the identity the document keeps.
//
// **The stimulus is asserted first**: every document must have had pages rewritten AND its bytes must have moved,
// or "the digest held" is a statement about a write that never happened.
//
// **Under rule 5 the digest holds on EVERY document** (ADR-080). Under rule 4 it did not, in two measured ways, and
// the test still pins both against the legacy arm — that arm checks every v4 record for as long as v4 is readable,
// so its known behaviour is pinned rather than forgotten:
//
//   - /pending 720: where anything under `/Annots` references a page, v4 reaches that page's content-stream
//     DICTIONARY, whose `/Length` a re-encode moves. Measured at the slice: 21 of 35 real-producer documents, exactly
//     those whose annotations reach a page. v5 hashes a reached page as its position.
//   - /pending 718: v4 hashes pdfcpu's bare join, so a fused page's digest moves when the repaired join is written
//     back. The generated corpus carries two such pages. v5 hashes `pdfread.PageContent`'s join.
//
// The law itself is asserted where it lives too: each page's decoded content, re-read from the written document, is
// the bytes the walk wrote.
func TestANoOpWalkKeepsTheDigest(t *testing.T) {
	gen := lawOneCorpus{name: "generated", docs: runCorpus(t)}
	for _, s := range []testpdf.JoinShape{testpdf.JoinRegular, testpdf.JoinComment, testpdf.JoinSafe} {
		pdf, _, err := testpdf.SplitContents("a divided page", s)
		if err != nil {
			t.Fatal(err)
		}
		gen.docs = append(gen.docs, runCorpusDoc{fmt.Sprintf("divided page, shape %d", s), pdf})
	}
	corpora := []lawOneCorpus{
		gen,
		externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers")),
	}
	var absent []string
	for _, corp := range corpora {
		if corp.absent != "" {
			absent = append(absent, corp.name+": "+corp.absent)
			continue
		}
		docs, pages, moved, held, encoding := 0, 0, 0, 0, 0
		for _, doc := range corp.docs {
			before, err := ContentDigest(doc.pdf)
			before4, err4 := ContentDigestAt(doc.pdf, legacyContentDigestVersion)
			if err == nil {
				err = err4
			}
			if err != nil {
				t.Logf("%s / %s: no digest (%v)", corp.name, doc.name, err)
				continue
			}
			out, rewritten, separated, err := noOpRewrite(t, doc.pdf)
			if err != nil {
				t.Logf("%s / %s: not rewritten (%v)", corp.name, doc.name, err)
				continue
			}
			bctx, berr := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
			actx, aerr := pdfread.Validated(out, model.NewDefaultConfiguration())
			if berr != nil || aerr != nil {
				t.Fatalf("%s / %s: re-read failed: %v / %v", corp.name, doc.name, berr, aerr)
			}
			reaches := annotsReachAPage(bctx)
			for p := 1; p <= bctx.PageCount; p++ {
				bd, _, _, _ := bctx.PageDict(p, false)
				ad, _, _, _ := actx.PageDict(p, false)
				if bd == nil || ad == nil {
					continue
				}
				bsrc, berr := pdfread.PageContent(bctx, bd, p)
				asrc, aerr := pdfread.PageContent(actx, ad, p)
				if (berr == nil) != (aerr == nil) || !bytes.Equal(bsrc, asrc) {
					t.Errorf("%s / %s page %d: the written page's content is not the content walked (%d → %d bytes)",
						corp.name, doc.name, p, len(bsrc), len(asrc))
				}
			}
			if rewritten == 0 {
				continue // a document with no page content has nothing to rewrite
			}
			if bytes.Equal(out, doc.pdf) {
				t.Errorf("%s / %s: %d page(s) rewritten and the file is byte-identical — the write did not happen",
					corp.name, doc.name, rewritten)
				continue
			}
			after, err := ContentDigest(out)
			after4, err4 := ContentDigestAt(out, legacyContentDigestVersion)
			if err == nil {
				err = err4
			}
			if err != nil {
				t.Errorf("%s / %s: the rewritten document has no digest: %v", corp.name, doc.name, err)
				continue
			}
			docs++
			pages += rewritten
			// Rule 5: the law, on every document.
			if after != before {
				t.Errorf("%s / %s: a no-op walk of %d page(s) moved ContentDigest %s → %s (rule %d; annotations reach "+
					"a page: %v; fused joins repaired: %d) — /pending 718 and 720 are closed by this rule",
					corp.name, doc.name, rewritten, before, after, ContentDigestVersion, reaches, separated)
			} else {
				held++
			}
			// Rule 4: the partition it was measured to have, which the legacy arm must keep.
			switch {
			case separated > 0 && after4 == before4:
				t.Errorf("%s / %s: rule 4 held over %d repaired fused page(s) — the legacy arm no longer hashes "+
					"pdfcpu's bare join, so it is not rule 4", corp.name, doc.name, separated)
			case separated > 0:
				moved++
			case reaches && after4 == before4:
				t.Errorf("%s / %s: rule 4 held though its annotations reach a page — the legacy arm no longer "+
					"expands a reached page, so it is not rule 4", corp.name, doc.name)
			case reaches:
				encoding++
			case after4 != before4:
				t.Errorf("%s / %s: a no-op walk moved rule 4's digest %s → %s, and no annotation reaches a page",
					corp.name, doc.name, before4, after4)
			}
		}
		if docs == 0 {
			t.Errorf("%s: not one document was rewritten and digested — the corpus exercised nothing", corp.name)
		}
		if corp.name == gen.name && moved != 2 {
			t.Errorf("generated: %d document(s) with a repaired join moved rule 4's digest, want the 2 fused fixtures", moved)
		}
		if held == 0 {
			t.Errorf("%s: no document held its digest — the law was never asserted", corp.name)
		}
		t.Logf("%s: %d documents, %d pages rewritten through setPageContent; rule %d held on %d; rule 4 moved on %d "+
			"whose annotations reach a page (/pending 720) and on %d with a repaired join (/pending 718)",
			corp.name, docs, pages, ContentDigestVersion, held, encoding, moved)
	}
	if len(absent) > 0 {
		t.Logf("NOTE (a narrower population, not a pass over it): %s", strings.Join(absent, "; "))
	}
}

// largestRealPage is the largest decoded page content in the real-producer corpus, or nil when it is absent.
func largestRealPage(tb testing.TB) (name string, src []byte) {
	root := os.Getenv("NIB_UA_PRODUCERS")
	if root == "" {
		root = filepath.Join(homeDir(), "nib", "producers")
	}
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".pdf") {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		ctx, verr := pdfread.Validated(b, model.NewDefaultConfiguration())
		if verr != nil {
			return nil
		}
		for i := 1; i <= ctx.PageCount; i++ {
			pd, _, _, perr := ctx.PageDict(i, false)
			if perr != nil || pd == nil {
				continue
			}
			if c, cerr := pdfread.PageContent(ctx, pd, i); cerr == nil && len(c) > len(src) {
				rel, _ := filepath.Rel(root, p)
				name, src = fmt.Sprintf("%s p%d", filepath.ToSlash(rel), i), c
			}
		}
		return nil
	})
	return name, src
}

// BenchmarkAWalkOfTheLargestRealPage — `PLAN-text-reflow.md` P05.S02: the cost of a walk, measured on a real page
// rather than estimated. Tokenize + write back, the whole of what a no-op walk does to a page's content. The figure
// is recorded in the plan with its population, machine and date. Absent the corpus it skips and says why.
func BenchmarkAWalkOfTheLargestRealPage(b *testing.B) {
	name, src := largestRealPage(b)
	if src == nil {
		b.Skip("SKIP (not a measurement): the real-producer corpus is absent; set NIB_UA_PRODUCERS")
	}
	b.Logf("%s: %d bytes decoded", name, len(src))
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := contentstream.WriteTokens(src, contentstream.Tokenize(src))
		if err != nil || len(out) != len(src) {
			b.Fatalf("the walk did not round-trip: %v", err)
		}
	}
}
