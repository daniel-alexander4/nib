package pdfops

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

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
