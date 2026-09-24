package uacheck

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfops"
)

// committedProposal tags pdf the way a person using the product does: the autotagger proposes, every
// element is accepted as proposed, and the commit writes it (`PLAN-accessibility.md` P08.S06). It is
// this package's "a tagged page" fixture since P08.S07 deleted `pdfops.TagAuthored`, the generic `/Div`
// emitter it replaced — and it is a product door, so a rule test built on it measures what a user
// can actually produce.
func committedProposal(t *testing.T, pdf []byte) []byte {
	t.Helper()
	prop, err := pdfops.ProposeTags(pdf)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if len(prop.Elements) == 0 {
		t.Fatal("setup: nothing was proposed, so the document would stay untagged and every tagged case would be the untagged one")
	}
	review := make([]pdfops.TagReview, len(prop.Elements))
	for i, e := range prop.Elements {
		review[i] = pdfops.TagReview{ID: e.ID, Role: e.Role, Text: e.Text}
	}
	out, err := pdfops.CommitTags(pdf, review)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	return out
}

// Mutation helpers for the rule tests — `PLAN-accessibility.md` P07.S02.
//
// Each one produces a document that breaks exactly one thing a rule reads, so a failing rule can be
// shown to fail for the reason it names rather than for any reason at all. They live here rather
// than being borrowed from `internal/pdfops`: `writeMutated` there is unexported, and exporting a
// test helper across packages to save fifteen lines is a dependency between two test suites.

// mutate reads pdf, applies fn, and writes it back — `writeMutated`'s shape, in this package.
func mutate(t *testing.T, pdf []byte, fn func(*model.Context) error) []byte {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := fn(ctx); err != nil {
		t.Fatalf("mutate: %v", err)
	}
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		t.Fatalf("write: %v", err)
	}
	return out.Bytes()
}

// dropMetadataKey removes the catalog's /Metadata entry.
func dropMetadataKey(pdf []byte) []byte {
	out, err := rawMutate(pdf, func(ctx *model.Context) error {
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		delete(cat, "Metadata")
		return nil
	})
	if err != nil {
		panic(err)
	}
	return out
}

// retypeMetadata returns a mutation that rewrites one name-valued key on the metadata stream dict.
func retypeMetadata(key, value string) func([]byte) []byte {
	return func(pdf []byte) []byte {
		out, err := rawMutate(pdf, func(ctx *model.Context) error {
			cat, cerr := ctx.XRefTable.Catalog()
			if cerr != nil {
				return cerr
			}
			sd, _, serr := ctx.DereferenceStreamDict(cat["Metadata"])
			if serr != nil || sd == nil {
				return serr
			}
			sd.Dict[key] = types.Name(value)
			return nil
		})
		if err != nil {
			panic(err)
		}
		return out
	}
}

// corruptMetadataXML replaces the packet with bytes that are not well-formed XML, leaving the stream
// dictionary itself correct — so the stream's existence and its parseability can be told apart.
func corruptMetadataXML(pdf []byte) []byte {
	out, err := rawMutate(pdf, func(ctx *model.Context) error {
		return replaceMetadataPacket(ctx,
			`<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/"><unclosed>`)
	})
	if err != nil {
		panic(err)
	}
	return out
}

// replaceMetadataPacket installs packet as the catalog's /Metadata, through the same idiom
// `pdfops.SetTitle` uses: a new stream built from the plain bytes, typed, ENCODED by pdfcpu, and
// referenced from the catalog.
//
// **The first version of these helpers edited the existing stream in place** — set `Content` and
// `Raw`, dropped `/Filter` — and the edit never landed: pdfcpu wrote the original compressed bytes
// with the filter gone, so the reader parsed zlib as XML. Both tests built on it went green or red
// for a reason unrelated to the case they named, and one of them — the unreadable-packet test — was
// passing on garbage rather than on the malformed XML it claimed to supply.
func replaceMetadataPacket(ctx *model.Context, packet string) error {
	cat, cerr := ctx.XRefTable.Catalog()
	if cerr != nil {
		return cerr
	}
	sd, err := ctx.NewStreamDictForBuf([]byte(packet))
	if err != nil {
		return err
	}
	sd.Dict["Type"] = types.Name("Metadata")
	sd.Dict["Subtype"] = types.Name("XML")
	if err := sd.Encode(); err != nil {
		return err
	}
	ref, err := ctx.IndRefForNewObject(*sd)
	if err != nil {
		return err
	}
	cat["Metadata"] = *ref
	return nil
}

// withASOnDefault puts `/AS` back on the default configuration — the state pdfcpu writes and
// `/pending 473` corrects.
func withASOnDefault(t *testing.T, pdf []byte) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		d, err := defaultConfig(ctx)
		if err != nil {
			return err
		}
		on, _ := ctx.DereferenceArray(d["ON"])
		d["AS"] = types.Array{types.Dict{
			"Category": types.Array{types.Name("View")},
			"Event":    types.Name("View"),
			"OCGs":     on,
		}}
		return nil
	})
}

// withoutNameOnDefault removes `/Name` from the default configuration.
func withoutNameOnDefault(t *testing.T, pdf []byte) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		d, err := defaultConfig(ctx)
		if err != nil {
			return err
		}
		delete(d, "Name")
		return nil
	})
}

// withBadConfigsEntry adds a SECOND configuration under `/Configs` carrying `/AS`, so the rules are
// driven against the half of the clause's population pdfcpu never writes.
func withBadConfigsEntry(t *testing.T, pdf []byte) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		ocp, oerr := ctx.DereferenceDict(cat["OCProperties"])
		if oerr != nil || ocp == nil {
			return oerr
		}
		// A usage-application dictionary requires /Category and /OCGs as well as /Event — pdfcpu's
		// own validator refuses it otherwise, which is how the first version of this fixture was
		// found to be invalid rather than merely non-conformant.
		on, _ := ctx.DereferenceArray(mustDefaultConfig(ctx)["ON"])
		alt, aerr := ctx.IndRefForNewObject(types.Dict{
			"Name": types.StringLiteral("An alternate configuration"),
			"AS": types.Array{types.Dict{
				"Category": types.Array{types.Name("View")},
				"Event":    types.Name("View"),
				"OCGs":     on,
			}},
		})
		if aerr != nil {
			return aerr
		}
		ocp["Configs"] = types.Array{*alt}
		return nil
	})
}

// defaultConfig resolves `/OCProperties /D`.
func defaultConfig(ctx *model.Context) (types.Dict, error) {
	cat, cerr := ctx.XRefTable.Catalog()
	if cerr != nil {
		return nil, cerr
	}
	ocp, oerr := ctx.DereferenceDict(cat["OCProperties"])
	if oerr != nil || ocp == nil {
		return nil, oerr
	}
	return ctx.DereferenceDict(ocp["D"])
}

// rawMutate is `mutate` for the helpers that cannot take a *testing.T because they are used as
// values in a table.
func rawMutate(pdf []byte, fn func(*model.Context) error) ([]byte, error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	if err := fn(ctx); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// mustDefaultConfig is defaultConfig for the call sites inside another mutation, where an error has
// nowhere to go but a nil map the caller already handles.
func mustDefaultConfig(ctx *model.Context) types.Dict {
	d, err := defaultConfig(ctx)
	if err != nil {
		return types.Dict{}
	}
	return d
}

// docWithMetadataKey builds a Document whose metadata stream dict has one name-valued key rewritten,
// WITHOUT going through `open` — which validates, and so cannot carry a document pdfcpu rejects.
//
// It exists for exactly two branches of `7.1 t8`, and its existence is the honest form of a
// limitation: the rule is right, the reader cannot reach it, and calling the rule directly says both
// things at once.
func docWithMetadataKey(t *testing.T, pdf []byte, key, value string) *Document {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	cat, cerr := ctx.XRefTable.Catalog()
	if cerr != nil {
		t.Fatal(cerr)
	}
	sd, _, serr := ctx.DereferenceStreamDict(cat["Metadata"])
	if serr != nil || sd == nil {
		t.Fatalf("the fixture has no metadata stream to mutate: %v", serr)
	}
	sd.Dict[key] = types.Name(value)
	return &Document{Ctx: ctx, Catalog: cat}
}

// withDisplayDocTitle sets the value of the catalog's ViewerPreferences /DisplayDocTitle.
func withDisplayDocTitle(t *testing.T, pdf []byte, v bool) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		prefs, perr := ctx.DereferenceDict(cat["ViewerPreferences"])
		if perr != nil || prefs == nil {
			return perr
		}
		prefs["DisplayDocTitle"] = types.Boolean(v)
		return nil
	})
}

// docWithViewerPref builds a Document with one ViewerPreferences entry rewritten, WITHOUT writing
// the file — the second place this is needed, and for a harder reason than `7.1 t8`'s.
//
// pdfcpu does not merely refuse a `/DisplayDocTitle` that is a name: it **panics** writing one. So
// the document cannot be produced at all, and the rule's type branch is reachable only by calling
// it. The branch is kept because nib is not the only thing that writes PDFs and a document from
// another producer can arrive in that state — which is exactly the population a checker is for.
func docWithViewerPref(t *testing.T, pdf []byte, key string, value types.Object) *Document {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	cat, cerr := ctx.XRefTable.Catalog()
	if cerr != nil {
		t.Fatal(cerr)
	}
	prefs, perr := ctx.DereferenceDict(cat["ViewerPreferences"])
	if perr != nil || prefs == nil {
		t.Fatalf("the fixture has no /ViewerPreferences to mutate: %v", perr)
	}
	prefs[key] = value
	return &Document{Ctx: ctx, Catalog: cat}
}

// withUAPart rewrites the metadata packet to declare a PDF/UA part, so a wrong value (`5 t2`) can be
// told from an absent identification (`5 t1`).
func withUAPart(t *testing.T, pdf []byte, part string) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		return replaceMetadataPacket(ctx, `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>`+
			`<x:xmpmeta xmlns:x="adobe:ns:meta/">`+
			`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`+
			`<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/" `+
			`xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/">`+
			`<dc:title><rdf:Alt><rdf:li xml:lang="x-default">A named document</rdf:li></rdf:Alt></dc:title>`+
			`<pdfuaid:part>`+part+`</pdfuaid:part>`+
			`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`)
	})
}

// withUAPrefix installs an identification whose `part`, `amd` and `corr` properties are each written
// under the given prefix, with that prefix bound to the identification namespace (P06.S01).
//
// **The binding is always the right URI and only the prefix moves**, because that is the whole content
// of `5 t3`/`t4`/`t5`: veraPDF finds the property by namespace and then refuses the prefix it was
// written under. A packet binding a different URI would be a different clause's fixture — the property
// would not be found at all, and the identification would not exist.
//
// This is the shape of veraPDF's own `5-t04-fail-a.pdf`, which binds BOTH `pdfuaia` and `pdfuaid` to
// the identification namespace: the second binding is what makes "which prefix maps to this URI"
// ambiguous and is why `parseXMP` reads raw prefixes rather than resolved ones.
func withUAPrefix(t *testing.T, pdf []byte, prefix string) []byte {
	// **The second binding only when the prefix is a foreign one.** Declaring `xmlns:pdfuaid` twice on
	// one element is a duplicate attribute and therefore not well-formed XML: measured, veraPDF then
	// reads NO packet at all and reports `5 t1` and `7.1 t9` failed with the rest of the family having
	// no subject — a fixture that looks like a prefix test and is really a broken-packet test.
	bindings := `xmlns:` + prefix + `="http://www.aiim.org/pdfua/ns/id/"`
	if prefix != "pdfuaid" {
		bindings = `xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/" ` + bindings
	}
	return mutate(t, pdf, func(ctx *model.Context) error {
		return replaceMetadataPacket(ctx, `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>`+
			`<x:xmpmeta xmlns:x="adobe:ns:meta/">`+
			`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`+
			`<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/" `+
			bindings+`>`+
			`<dc:title><rdf:Alt><rdf:li xml:lang="x-default">A named document</rdf:li></rdf:Alt></dc:title>`+
			`<`+prefix+`:part>1</`+prefix+`:part>`+
			`<`+prefix+`:amd>2014</`+prefix+`:amd>`+
			`<`+prefix+`:corr>0</`+prefix+`:corr>`+
			`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`)
	})
}

// withFileHeader rewrites the file's `%PDF-1.n` header to header, in place and without reparsing
// (P06.S01). `6.1 t1` is about the BYTES, so its fixture cannot go through pdfcpu: a mutation that
// rewrote the document would have the writer put a well-formed header back.
//
// **Length-preserving, so every xref offset stays true.** A shifted file would be refused by the
// reader before the clause ever ran, and the fixture would measure nothing.
func withFileHeader(t *testing.T, pdf []byte, header string) []byte {
	t.Helper()
	i := bytes.Index(pdf, []byte("%PDF-"))
	if i < 0 {
		t.Fatalf("the fixture has no %%PDF- header to rewrite")
	}
	end := i + bytes.IndexAny(pdf[i:], "\r\n")
	if end < i {
		t.Fatalf("the fixture's header line never ends")
	}
	if len(header) != end-i {
		t.Fatalf("header %q is %d bytes against the original's %d — rewriting it would shift every "+
			"xref offset in the file and the document would stop parsing", header, len(header), end-i)
	}
	out := append([]byte(nil), pdf...)
	copy(out[i:end], header)
	return out
}

// withUAProperty installs an identification whose ONLY property is the named one, under the required
// prefix (P06.S01) — the shape that separates "the identification exists" from "the part exists".
//
// veraPDF's `containsPDFUAIdentification` is true for any property in the namespace, whatever it is
// called, so `withUAProperty(t, pdf, "corr", "0")` produces a document that PASSES `5 t1` and FAILS
// `5 t2`. Both were measured on length-preserving mutations of the corpus file `5-t03-pass-a.pdf`
// before either rule was changed.
func withUAProperty(t *testing.T, pdf []byte, prop, value string) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		return replaceMetadataPacket(ctx, `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>`+
			`<x:xmpmeta xmlns:x="adobe:ns:meta/">`+
			`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`+
			`<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/" `+
			`xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/">`+
			`<dc:title><rdf:Alt><rdf:li xml:lang="x-default">A named document</rdf:li></rdf:Alt></dc:title>`+
			`<pdfuaid:`+prop+`>`+value+`</pdfuaid:`+prop+`>`+
			`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`)
	})
}

// withHeaderEOL replaces the single byte that ends the header line, leaving the header itself alone
// (P06.S01) — so "which bytes end the line" can be measured apart from "what the line says".
func withHeaderEOL(t *testing.T, pdf []byte, eol string) []byte {
	t.Helper()
	if len(eol) != 1 {
		t.Fatalf("withHeaderEOL rewrites ONE byte; %q is %d", eol, len(eol))
	}
	i := bytes.Index(pdf, []byte("%PDF-"))
	if i < 0 {
		t.Fatalf("the fixture has no %%PDF- header")
	}
	at := i + bytes.IndexAny(pdf[i:], "\r\n")
	if at < i {
		t.Fatalf("the fixture's header line never ends")
	}
	out := append([]byte(nil), pdf...)
	out[at] = eol[0]
	return out
}

// withPacket installs a packet whose `dc:` prefix is bound to dcURI, so a namespace-by-URI reader can
// be told from a prefix-trusting one.
func withPacket(t *testing.T, pdf []byte, dcURI string) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		return replaceMetadataPacket(ctx, `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>`+
			`<x:xmpmeta xmlns:x="adobe:ns:meta/">`+
			`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`+
			`<rdf:Description rdf:about="" xmlns:dc="`+dcURI+`">`+
			`<dc:title><rdf:Alt><rdf:li xml:lang="x-default">A named document</rdf:li></rdf:Alt></dc:title>`+
			`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`)
	})
}

// langOnEveryElement sets /Lang on every structure element reachable from the root's /K, and on
// nothing else — so a pass can only come from the tree walk, never from the catalog.
func langOnEveryElement(t *testing.T, pdf []byte, lang string) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		if _, has := cat["Lang"]; has {
			return fmt.Errorf("the fixture already declares a catalog /Lang, so a pass would prove nothing")
		}
		root, rerr := ctx.DereferenceDict(cat["StructTreeRoot"])
		if rerr != nil || root == nil {
			return fmt.Errorf("the fixture has no structure tree")
		}
		var walk func(o types.Object, depth int)
		set := 0
		walk = func(o types.Object, depth int) {
			if depth > 32 {
				return
			}
			if arr, err := ctx.DereferenceArray(o); err == nil && arr != nil {
				for _, k := range arr {
					walk(k, depth+1)
				}
				return
			}
			d, err := ctx.DereferenceDict(o)
			if err != nil || d == nil {
				return
			}
			if _, isElem := d["S"]; !isElem {
				return
			}
			d["Lang"] = types.StringLiteral(lang)
			set++
			walk(d["K"], depth+1)
		}
		walk(root["K"], 0)
		if set == 0 {
			return fmt.Errorf("no structure element was found to give a /Lang")
		}
		return nil
	})
}

// alternateTextWithNoLanguage puts `keys` on the first structure element reachable from the root that has no
// `/Lang` of its own, on a document that declares no language anywhere — the failed half of 7.2 t21, t22 and t23
// (P04.S02).
//
// **It reaches the failing half only, and that is the whole of its job** (the slice's review): the committed
// proposal carries exactly ONE structure element, so there is no second element on this document to exercise the
// clauses' passing checks. Every other corpus document does that, which is what `notYetReachable`'s both-ways
// requirement is measuring.
func alternateTextWithNoLanguage(t *testing.T, pdf []byte, keys ...string) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		if _, has := cat["Lang"]; has {
			return fmt.Errorf("the fixture declares a catalog /Lang, so the clause could not fail on it")
		}
		root, rerr := ctx.DereferenceDict(cat["StructTreeRoot"])
		if rerr != nil || root == nil {
			return fmt.Errorf("the fixture has no structure tree")
		}
		// Descends through both `/K` arrays and element kids, so a base document whose top-level element
		// already declares a `/Lang` still yields one below it rather than erroring.
		var first func(o types.Object, depth int) types.Dict
		first = func(o types.Object, depth int) types.Dict {
			if depth > 32 {
				return nil
			}
			if arr, err := ctx.DereferenceArray(o); err == nil && arr != nil {
				for _, k := range arr {
					if d := first(k, depth+1); d != nil {
						return d
					}
				}
				return nil
			}
			d, err := ctx.DereferenceDict(o)
			if err != nil || d == nil {
				return nil
			}
			if _, isElem := d["S"]; !isElem {
				return nil
			}
			if _, has := d["Lang"]; !has {
				return d
			}
			return first(d["K"], depth+1)
		}
		el := first(root["K"], 0)
		if el == nil {
			return fmt.Errorf("no structure element without a /Lang was found to carry the alternate text")
		}
		for _, k := range keys {
			el[k] = types.StringLiteral("text with no determinable language")
		}
		return nil
	})
}

// addMediaClip attaches a Screen annotation to page 1 whose `/A` is a Rendition action carrying one media
// clip, with `ct` and `alt` written verbatim — the only route to 7.18.6.2's halves, since **nib plays no media
// and writes no clip**: the phase's four other annotation subtypes at least have a product door for one half,
// and this one has none at all.
func addMediaClip(t *testing.T, pdf []byte, ct, alt string) []byte {
	t.Helper()
	return mutate(t, pdf, func(ctx *model.Context) error {
		page, _, _, err := ctx.PageDict(1, false)
		if err != nil {
			return err
		}
		file, ferr := ctx.IndRefForNewObject(types.Dict{
			"Type": types.Name("Filespec"),
			"F":    types.StringLiteral("clip.mp3"),
			"UF":   types.StringLiteral("clip.mp3"),
		})
		if ferr != nil {
			return ferr
		}
		clip := types.Dict{"Type": types.Name("MediaClip"), "S": types.Name("MCD"), "D": *file}
		if ct != "" {
			clip["CT"] = types.StringLiteral(ct)
		}
		if alt != nil2 {
			clip["Alt"] = altArray(alt)
		}
		action, aerr := ctx.IndRefForNewObject(types.Dict{
			"Type": types.Name("Action"),
			"S":    types.Name("Rendition"),
			"R":    types.Dict{"Type": types.Name("Rendition"), "S": types.Name("MR"), "C": clip},
		})
		if aerr != nil {
			return aerr
		}
		annot, rerr := ctx.IndRefForNewObject(types.Dict{
			"Type":     types.Name("Annot"),
			"Subtype":  types.Name("Screen"),
			"Rect":     types.NewNumberArray(10, 10, 30, 30),
			"F":        types.Integer(4),
			"Contents": types.StringLiteral("a media clip"),
			"A":        *action,
		})
		if rerr != nil {
			return rerr
		}
		annots, derr := ctx.DereferenceArray(page["Annots"])
		if derr != nil {
			return fmt.Errorf("the fixture's /Annots does not resolve to an array: %v", derr)
		}
		page["Annots"] = append(annots, *annot)
		// A page carrying an annotation must declare its tab order (7.18.3 t1), or this mutation would break a
		// clause it has nothing to do with.
		page["Tabs"] = types.Name("S")
		return nil
	})
}

// altArray turns "en|a clip" into the language/description pairs `/Alt` holds, splitting on "|" so a caller can
// write a malformed shape (an odd count) as easily as a well-formed one.
func altArray(spec string) types.Array {
	out := types.Array{}
	for _, part := range strings.Split(spec, "|") {
		out = append(out, types.StringLiteral(part))
	}
	return out
}

// nil2 is the sentinel meaning "write no /Alt at all", distinct from an empty array.
const nil2 = "\x00none"

// withoutTabs drops `/Tabs` from every page — 7.18.3 t1's failing half, which no product door reaches
// because `setStructureTabOrder` writes `/Tabs /S` on every page that carries an annotation (`form.go:184`)
// and `AddNotes` calls it whether or not the document is tagged.
func withoutTabs(t *testing.T, pdf []byte) []byte {
	t.Helper()
	return mutate(t, pdf, func(ctx *model.Context) error {
		dropped := 0
		for p := 1; p <= ctx.PageCount; p++ {
			page, _, _, err := ctx.PageDict(p, false)
			if err != nil {
				return err
			}
			if _, had := page["Tabs"]; had {
				delete(page, "Tabs")
				dropped++
			}
		}
		if dropped == 0 {
			return fmt.Errorf("the fixture declares no /Tabs to drop, so this mutation changes nothing")
		}
		return nil
	})
}

// addAnnotation puts one annotation of `subtype` on page 1, for the clauses P05.S02 checks over
// subtypes **nib writes nothing of**: Link, TrapNet and PrinterMark. `nib office`'s Markdown conversion
// emits no `/Annots` at all (measured), `AddNotes` writes only `/Text` and form authoring only `/Widget`,
// so neither half of 7.18.5 t1/t2, 7.18.2 t1 or 7.18.8 t1 is reachable through a product door — and the
// veraPDF corpus reaches only 7.18.8's failing half, its one 7.18.2 file being unreadable to pdfcpu.
//
// `tag` empty leaves the annotation with no `/StructParent`; otherwise a structure element with that tag
// is added to the tree, the parent tree gains a row naming it, and the annotation points at that row.
// Every annotation gets an `/AP` because **pdfcpu requires one on a PrinterMark** (`validateAnnotationDict`
// → `validateAnnotationDictPrinterMark`); on a TrapNet it is surplus, whose one REQUIRED entry is `/F`
// (measured both ways — a TrapNet with `/F` and no `/AP` reads fine and nib and veraPDF agree on it, which is
// also what `corpusUnreadable`'s row for the corpus TrapNet file says). pdfcpu additionally requires a TrapNet
// to be the LAST entry of `/Annots`; this helper appends, so a caller that inserts elsewhere will be refused
// opaquely.
func addAnnotation(t *testing.T, pdf []byte, subtype string, entries types.Dict, tag string) []byte {
	t.Helper()
	return mutate(t, pdf, func(ctx *model.Context) error {
		page, _, _, err := ctx.PageDict(1, false)
		if err != nil {
			return err
		}
		sd, serr := ctx.NewStreamDictForBuf(nil)
		if serr != nil {
			return serr
		}
		sd.Dict["Type"] = types.Name("XObject")
		sd.Dict["Subtype"] = types.Name("Form")
		sd.Dict["BBox"] = types.NewNumberArray(0, 0, 10, 10)
		if eerr := sd.Encode(); eerr != nil {
			return eerr
		}
		ap, aerr := ctx.IndRefForNewObject(*sd)
		if aerr != nil {
			return aerr
		}
		annot := types.Dict{
			"Type":    types.Name("Annot"),
			"Subtype": types.Name(subtype),
			"Rect":    types.NewNumberArray(10, 10, 30, 30),
			"F":       types.Integer(4),
			"AP":      types.Dict{"N": *ap},
		}
		for k, v := range entries {
			annot[k] = v
		}
		aref, rerr := ctx.IndRefForNewObject(annot)
		if rerr != nil {
			return rerr
		}
		if tag != "" {
			cat, cerr := ctx.XRefTable.Catalog()
			if cerr != nil {
				return cerr
			}
			rootRef, ok := cat["StructTreeRoot"].(types.IndirectRef)
			if !ok {
				return fmt.Errorf("the fixture's /StructTreeRoot is not an indirect reference")
			}
			root, derr := ctx.DereferenceDict(cat["StructTreeRoot"])
			if derr != nil || root == nil {
				return fmt.Errorf("the fixture has no structure tree to add an element to")
			}
			elem, eerr := ctx.IndRefForNewObject(types.Dict{
				"Type": types.Name("StructElem"),
				"S":    types.Name(tag),
				"P":    rootRef,
				"K":    types.Dict{"Type": types.Name("OBJR"), "Obj": *aref},
			})
			if eerr != nil {
				return eerr
			}
			kids, kerr := ctx.DereferenceArray(root["K"])
			if kerr != nil {
				return kerr
			}
			root["K"] = append(kids, *elem)
			// The parent tree's next free key, so the row cannot collide with one the document already has.
			pt, perr := ctx.DereferenceDict(root["ParentTree"])
			if perr != nil || pt == nil {
				return fmt.Errorf("the fixture's parent tree does not resolve")
			}
			// **The key's PRESENCE is checked separately from its value.** `DereferenceArray` answers
			// `(nil, nil)` for an absent key, so testing only the error let a `/Kids`-form parent tree through
			// as "a flat /Nums with no rows" — `next` would compute to 0 and collide with the key the `/Kids`
			// side already holds. No caller passes such a tree today; the guard could not have said so.
			if _, flat := pt["Nums"]; !flat {
				return fmt.Errorf("the fixture's parent tree has no flat /Nums; this helper cannot pick a free key in a /Kids tree")
			}
			nums, nerr := ctx.DereferenceArray(pt["Nums"])
			if nerr != nil {
				return fmt.Errorf("the fixture's parent tree is not a flat /Nums: %v", nerr)
			}
			next := 0
			for i := 0; i+1 < len(nums); i += 2 {
				if k, ok := nums[i].(types.Integer); ok && k.Value() >= next {
					next = k.Value() + 1
				}
			}
			pt["Nums"] = append(nums, types.Integer(next), *elem)
			annot["StructParent"] = types.Integer(next)
			// The annotation is now reachable from the structure tree as well as from the page, and pdfcpu's
			// validator walks both; marking the object valid keeps the second walk from re-validating a
			// dictionary it is already inside. Only this branch creates that second path.
			if uerr := ctx.SetValid(*aref); uerr != nil {
				return uerr
			}
		}
		annots, aerr2 := ctx.DereferenceArray(page["Annots"])
		if aerr2 != nil {
			// A dropped error here would silently discard the page's existing annotations.
			return fmt.Errorf("the fixture's /Annots does not resolve to an array: %v", aerr2)
		}
		page["Annots"] = append(annots, *aref)
		return nil
	})
}

// annotMutation breaks the first NON-widget annotation of a document, for the halves of 7.18.1 t1 and
// t2 that no product door reaches: `pdfops.AddNotes` writes a described note with `/Contents`, which is
// both clauses' passing half, and nothing nib ships writes an undescribed or untagged annotation.
func annotMutation(t *testing.T, pdf []byte, kind string) []byte {
	t.Helper()
	return mutate(t, pdf, func(ctx *model.Context) error {
		for p := 1; p <= ctx.PageCount; p++ {
			page, _, _, err := ctx.PageDict(p, false)
			if err != nil {
				return err
			}
			annots, _ := ctx.DereferenceArray(page["Annots"])
			for _, a := range annots {
				ad, derr := ctx.DereferenceDict(a)
				if derr != nil || ad == nil {
					continue
				}
				if sub := ad.NameEntry("Subtype"); sub == nil || *sub == "Widget" {
					continue
				}
				switch kind {
				case "drop-structparent":
					if _, ok := ad["StructParent"]; !ok {
						return fmt.Errorf("the fixture's annotation has no /StructParent to drop")
					}
					delete(ad, "StructParent")
				case "drop-contents":
					if _, ok := ad["Contents"]; !ok {
						return fmt.Errorf("the fixture's annotation has no /Contents to drop")
					}
					delete(ad, "Contents")
				default:
					return fmt.Errorf("unknown annotation mutation %q", kind)
				}
				return nil
			}
		}
		return fmt.Errorf("the fixture has no non-widget annotation to break")
	})
}

func widgetMutation(t *testing.T, pdf []byte, kind string) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		page, _, _, err := ctx.PageDict(1, false)
		if err != nil {
			return err
		}
		annots, _ := ctx.DereferenceArray(page["Annots"])
		for _, a := range annots {
			ad, derr := ctx.DereferenceDict(a)
			if derr != nil || ad == nil {
				continue
			}
			if sub := ad.NameEntry("Subtype"); sub == nil || *sub != "Widget" {
				continue
			}
			sp, ok := ad["StructParent"].(types.Integer)
			if !ok {
				return fmt.Errorf("the fixture's widget has no /StructParent to break")
			}
			if kind == "drop-structparent" {
				delete(ad, "StructParent")
				return nil
			}
			if kind == "contents-lang-on-ancestor" {
				// P04.S03's DECISIVE fixture, and the one that measures the slice's finding rather than
				// asserting it. The annotation carries `/Contents`; the element its `/StructParent` names
				// declares no `/Lang`; that element's `/P` ancestor DOES. veraPDF's `GFPDAnnot.getLang`
				// reads only the named element's own `/Lang` — there is no `/P` climb, unlike 7.2 t21-t23
				// — so this must FAIL. If veraPDF passes it, the no-climb reading is wrong and the oracle
				// says so here rather than in production.
				ad["Contents"] = types.StringLiteral("a note whose language only an ancestor declares")
				cat, _ := ctx.XRefTable.Catalog()
				root, _ := ctx.DereferenceDict(cat["StructTreeRoot"])
				pt, _ := ctx.DereferenceDict(root["ParentTree"])
				nums, _ := ctx.DereferenceArray(pt["Nums"])
				for i := 0; i+1 < len(nums); i += 2 {
					if k, ok := nums[i].(types.Integer); !ok || k.Value() != sp.Value() {
						continue
					}
					elem, eerr := ctx.DereferenceDict(nums[i+1])
					if eerr != nil || elem == nil {
						return fmt.Errorf("the parent tree entry does not resolve")
					}
					delete(elem, "Lang")
					parent, perr := ctx.DereferenceDict(elem["P"])
					if perr != nil || parent == nil {
						return fmt.Errorf("the named element has no /P ancestor to carry the language")
					}
					parent["Lang"] = types.StringLiteral("en-US")
					return nil
				}
				return fmt.Errorf("the widget's key %d is not in a flat /Nums", sp.Value())
			}
			if kind == "contents-lang-on-element" {
				// The near control for the ancestor case: the named element declares its OWN /Lang, which
				// is the one source t24 accepts besides the catalog.
				ad["Contents"] = types.StringLiteral("a note whose language its own element declares")
				cat, _ := ctx.XRefTable.Catalog()
				root, _ := ctx.DereferenceDict(cat["StructTreeRoot"])
				pt, _ := ctx.DereferenceDict(root["ParentTree"])
				nums, _ := ctx.DereferenceArray(pt["Nums"])
				for i := 0; i+1 < len(nums); i += 2 {
					if k, ok := nums[i].(types.Integer); !ok || k.Value() != sp.Value() {
						continue
					}
					elem, eerr := ctx.DereferenceDict(nums[i+1])
					if eerr != nil || elem == nil {
						return fmt.Errorf("the parent tree entry does not resolve")
					}
					elem["Lang"] = types.StringLiteral("en-US")
					return nil
				}
				return fmt.Errorf("the widget's key %d is not in a flat /Nums", sp.Value())
			}
			if kind == "contents-no-lang" {
				// P04.S03's fail fixture for ua1 7.2 t24. The annotation gains a `/Contents`, which is
				// what makes it a subject at all, and the element its `/StructParent` names loses any
				// `/Lang` — so the only remaining source of a language is the catalog, which this
				// fixture does not carry. veraPDF is what says whether that is a failure; nib is
				// compared against its answer.
				ad["Contents"] = types.StringLiteral("a note whose language nothing determines")
				cat, _ := ctx.XRefTable.Catalog()
				root, _ := ctx.DereferenceDict(cat["StructTreeRoot"])
				pt, _ := ctx.DereferenceDict(root["ParentTree"])
				nums, _ := ctx.DereferenceArray(pt["Nums"])
				for i := 0; i+1 < len(nums); i += 2 {
					if k, ok := nums[i].(types.Integer); !ok || k.Value() != sp.Value() {
						continue
					}
					elem, eerr := ctx.DereferenceDict(nums[i+1])
					if eerr != nil || elem == nil {
						return fmt.Errorf("the parent tree entry does not resolve")
					}
					delete(elem, "Lang")
					return nil
				}
				return fmt.Errorf("the widget's key %d is not in a flat /Nums", sp.Value())
			}
			cat, _ := ctx.XRefTable.Catalog()
			root, _ := ctx.DereferenceDict(cat["StructTreeRoot"])
			pt, _ := ctx.DereferenceDict(root["ParentTree"])
			nums, _ := ctx.DereferenceArray(pt["Nums"])
			for i := 0; i+1 < len(nums); i += 2 {
				if k, ok := nums[i].(types.Integer); !ok || k.Value() != sp.Value() {
					continue
				}
				elem, eerr := ctx.DereferenceDict(nums[i+1])
				if eerr != nil || elem == nil {
					return fmt.Errorf("the parent tree entry does not resolve")
				}
				switch kind {
				case "retype-div":
					elem["S"] = types.Name("Div")
				case "drop-objr":
					elem["K"] = types.Array{}
				case "rolemap-form":
					elem["S"] = types.Name("MyField")
					rm, _ := ctx.DereferenceDict(root["RoleMap"])
					if rm == nil {
						rm = types.Dict{}
					}
					rm["MyField"] = types.Name("Form")
					root["RoleMap"] = rm
				}
				return nil
			}
			return fmt.Errorf("the widget's key %d is not in a flat /Nums", sp.Value())
		}
		return fmt.Errorf("no widget found")
	})
}

// langOnTopLevelOnly sets /Lang on the structure root's DIRECT children only, and fails the fixture
// if none of them owns an MCID directly — the case must need the ancestor walk to pass.
func langOnTopLevelOnly(t *testing.T, pdf []byte, lang string) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		root, rerr := ctx.DereferenceDict(cat["StructTreeRoot"])
		if rerr != nil || root == nil {
			return fmt.Errorf("the fixture has no structure tree")
		}
		kids := []types.Object{root["K"]}
		if arr, err := ctx.DereferenceArray(root["K"]); err == nil && arr != nil {
			kids = arr
		}
		nested := false
		for _, k := range kids {
			d, err := ctx.DereferenceDict(k)
			if err != nil || d == nil {
				continue
			}
			d["Lang"] = types.StringLiteral(lang)
			// Does this top-level element hold a child ELEMENT (rather than only MCIDs)?
			if ka, err := ctx.DereferenceArray(d["K"]); err == nil {
				for _, c := range ka {
					if cd, cerr := ctx.DereferenceDict(c); cerr == nil && cd != nil {
						if _, isElem := cd["S"]; isElem {
							nested = true
						}
					}
				}
			}
		}
		if !nested {
			return fmt.Errorf("setup: no top-level element has a child element, so a pass would not " +
				"prove the ancestor walk")
		}
		return nil
	})
}

// markedFalse keeps the /MarkInfo dictionary and sets /Marked false, so presence and value are told
// apart.
func markedFalse(t *testing.T, pdf []byte) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		mi, err := ctx.DereferenceDict(cat["MarkInfo"])
		if err != nil || mi == nil {
			return fmt.Errorf("the fixture has no /MarkInfo to alter")
		}
		mi["Marked"] = types.Boolean(false)
		return nil
	})
}

// pageContentEmptied replaces page 1's content with a path, so the only text left is in annotation
// appearances.
func pageContentEmptied(t *testing.T, pdf []byte) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		page, _, _, err := ctx.PageDict(1, false)
		if err != nil {
			return err
		}
		sd, err := ctx.NewStreamDictForBuf([]byte("0 0 m 10 10 l S"))
		if err != nil {
			return err
		}
		if err := sd.Encode(); err != nil {
			return err
		}
		ref, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			return err
		}
		page["Contents"] = *ref
		return nil
	})
}

// cidSetMode is which /CIDSet withCIDSet writes.
type cidSetMode int

const (
	// cidExact is every maxp glyph slot and nothing else — what veraPDF passes.
	cidExact cidSetMode = iota
	// cidPartial is only the first byte's eight CIDs — an under-claim.
	cidPartial
	// cidPadded is every slot plus the unused bits of the last byte — an over-claim.
	cidPadded
)

// withCIDSet gives every TrueType font descriptor a /CIDSet of the given shape.
//
// **Its first version wrote 0xFF into every byte for the "full" case**, which also set the unused
// bits of the last byte — an over-claim — so the fixture meant to pass was one veraPDF FAILS, and
// nib's rule, checking only coverage, passed it. The law 5 check caught the disagreement.
func withCIDSet(t *testing.T, pdf []byte, mode cidSetMode) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		added := 0
		for _, e := range ctx.XRefTable.Table {
			if e == nil {
				continue
			}
			d, ok := e.Object.(types.Dict)
			if !ok {
				continue
			}
			if ty := d.NameEntry("Type"); ty == nil || *ty != "FontDescriptor" {
				continue
			}
			ff, _, _ := ctx.DereferenceStreamDict(d["FontFile2"])
			if ff == nil || ff.Decode() != nil {
				continue
			}
			n, err := trueTypeGlyphCount(ff.Content)
			if err != nil {
				return err
			}
			set := make([]byte, (n+7)/8)
			for cid := 0; cid < len(set)*8; cid++ {
				on := false
				switch mode {
				case cidExact:
					on = cid < n
				case cidPartial:
					on = cid < 8
				case cidPadded:
					on = true
				}
				if on {
					set[cid/8] |= 0x80 >> uint(cid%8)
				}
			}
			if mode == cidPadded && n%8 == 0 {
				return fmt.Errorf("setup: numGlyphs %d is a multiple of 8, so padding bits do not exist", n)
			}
			sd, err := ctx.NewStreamDictForBuf(set)
			if err != nil {
				return err
			}
			if err := sd.Encode(); err != nil {
				return err
			}
			ref, err := ctx.IndRefForNewObject(*sd)
			if err != nil {
				return err
			}
			d["CIDSet"] = *ref
			added++
		}
		if added == 0 {
			return fmt.Errorf("the fixture has no TrueType font descriptor to give a /CIDSet")
		}
		return nil
	})
}

// withoutDCTitle keeps a metadata packet and removes its dc:title, so 7.1 t9 has a subject and fails
// — the one state of the fifteen clauses the S05 corpus did not reach until this fixture existed.
func withoutDCTitle(t *testing.T, pdf []byte) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		return replaceMetadataPacket(ctx, `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>`+
			`<x:xmpmeta xmlns:x="adobe:ns:meta/">`+
			`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`+
			`<rdf:Description rdf:about="" xmlns:xmp="http://ns.adobe.com/xap/1.0/">`+
			`<xmp:CreateDate>2026-09-14T00:00:00Z</xmp:CreateDate>`+
			`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`)
	})
}

// withRoleMapCycle installs a `/Loopy → /Ringy → /Loopy` role map on the structure tree root, and when
// `used` is true retypes exactly one structure element to `/Loopy` — `/pending 548`.
//
// **The two halves are a pair and neither is redundant.** veraPDF's 7.1-6 object is `PDStructElem`, so the
// used case is the one it fails and the unused case is one it passes with the same dictionary in the file.
// A document-scoped reading of the clause fails the unused case, and nothing else in the corpus tells the
// two readings apart.
func withRoleMapCycle(t *testing.T, pdf []byte, used bool) []byte {
	leaf := ""
	if used {
		leaf = "Loopy"
	}
	return withRoleMapOnLeaf(t, pdf, types.Dict{"Loopy": types.Name("Ringy"), "Ringy": types.Name("Loopy")}, leaf)
}

// withRoleMapOnLeaf installs roleMap on the structure root and, when leafType is not empty, retypes the
// first element below the root's own child to it — the one shape every role-map oracle document shares
// (7.1 t5, t6, t7). No product door writes a /RoleMap, so each is a mutation named for what it declares.
func withRoleMapOnLeaf(t *testing.T, pdf []byte, roleMap types.Dict, leafType string) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		root, rerr := ctx.DereferenceDict(cat["StructTreeRoot"])
		if rerr != nil || root == nil {
			return fmt.Errorf("the fixture has no structure tree, so a role map has nowhere to live")
		}
		if _, has := root["RoleMap"]; has {
			return fmt.Errorf("the fixture already carries a /RoleMap, so this mutation would be editing one rather than installing it")
		}
		root["RoleMap"] = roleMap
		if leafType == "" {
			return nil
		}
		// Retype the first element the walk meets at depth 1 or below. **Which element that is depends on the
		// root's `/K`**: a single `Document` reference puts the root's child at depth 0 and retypes its first kid,
		// but an ARRAY `/K` puts the root's own children at depth 1, so the first of them is retyped — on the
		// Markdown oracle fixture (root `/K [25 0 R 26 0 R 27 0 R]`, no Document element) that is the heading,
		// which is why 7.4.2 t1 sits in `knownCannotCheck`. (P03's phase close corrected this comment, which
		// said the root's own child was always skipped; veraPDF agrees with every verdict either way.)
		var retyped types.Dict
		var walk func(o types.Object, depth int)
		walk = func(o types.Object, depth int) {
			if retyped != nil || depth > 32 {
				return
			}
			if arr, err := ctx.DereferenceArray(o); err == nil && arr != nil {
				for _, k := range arr {
					walk(k, depth+1)
				}
				return
			}
			d, err := ctx.DereferenceDict(o)
			if err != nil || d == nil {
				return
			}
			if _, isElem := d["S"]; !isElem {
				return
			}
			if depth > 0 {
				retyped = d
				d["S"] = types.Name(leafType)
				return
			}
			walk(d["K"], depth+1)
		}
		walk(root["K"], 0)
		if retyped == nil {
			return fmt.Errorf("no structure element below the root's own child was found to retype")
		}
		return nil
	})
}

// withPacketBody installs a metadata packet whose rdf:Description holds body, with the dc and xmp
// namespaces bound — the shapes law 5's guard measured 7.2 t33 against.
func withPacketBody(t *testing.T, pdf []byte, body string) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		return replaceMetadataPacket(ctx, `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>`+
			`<x:xmpmeta xmlns:x="adobe:ns:meta/">`+
			`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`+
			`<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:xmp="http://ns.adobe.com/xap/1.0/">`+
			body+`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`)
	})
}

// withRawPacket installs an arbitrary metadata packet verbatim, well-formed or not (P06.S01) — so a
// fixture can break ONE property of the XML and leave the rest of it intact.
//
// `corruptMetadataXML` truncates, which breaks several things at once; this writes exactly what it is
// given. `replaceMetadataPacket` asserts the bytes landed, which is what stops a fixture like this
// from silently measuring zlib garbage instead of the shape it meant to supply.
func withRawPacket(t *testing.T, pdf []byte, packet string) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		return replaceMetadataPacket(ctx, packet)
	})
}

// withUADefaultNamespace writes the identification under the DEFAULT namespace, so its properties
// carry no prefix at all (P06.S01) — the `null` half of `5 t3`/`t4`/`t5`, which veraPDF passes.
func withUADefaultNamespace(t *testing.T, pdf []byte) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		return replaceMetadataPacket(ctx, `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>`+
			`<x:xmpmeta xmlns:x="adobe:ns:meta/">`+
			`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`+
			`<rdf:Description rdf:about="" xmlns="http://www.aiim.org/pdfua/ns/id/">`+
			`<part>1</part><amd>2014</amd><corr>0</corr>`+
			`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`)
	})
}

// withUARootBinding declares the identification's prefix on the packet's OUTERMOST element rather
// than on `rdf:Description` (P06.S01), which is where every other fixture here declares it.
//
// It is legal XML and it is the case a scope stack that starts one frame too high cannot see: with
// the binding on the root, the prefix resolves to nothing and the whole identification reads as
// absent. The prefix is a foreign one so the expected answer is a definite Fail — a reader that
// missed the binding would answer NotApplicable instead.
func withUARootBinding(t *testing.T, pdf []byte) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		return replaceMetadataPacket(ctx, `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>`+
			`<x:xmpmeta xmlns:x="adobe:ns:meta/" xmlns:pdfuaia="http://www.aiim.org/pdfua/ns/id/">`+
			`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`+
			`<rdf:Description rdf:about="">`+
			`<pdfuaia:part>1</pdfuaia:part>`+
			`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`)
	})
}

// withUAAttributes writes the identification in RDF/XML's ABBREVIATED syntax — the three properties
// as attributes on `rdf:Description` rather than as child elements (P06.S01).
//
// This is ordinary XMP, not an exotic shape, and veraPDF reads it: measured on a length-preserving
// mutation of `5-t03-pass-a.pdf`, an attribute-form identification passes all five clause-5 tests
// there while nib — reading start elements only — failed `5 t1` and found no subject for the rest.
func withUAAttributes(t *testing.T, pdf []byte, prefix, part string) []byte {
	bindings := `xmlns:` + prefix + `="http://www.aiim.org/pdfua/ns/id/"`
	if prefix != "pdfuaid" {
		bindings = `xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/" ` + bindings
	}
	return mutate(t, pdf, func(ctx *model.Context) error {
		return replaceMetadataPacket(ctx, `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>`+
			`<x:xmpmeta xmlns:x="adobe:ns:meta/">`+
			`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`+
			`<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/" `+bindings+` `+
			prefix+`:part="`+part+`" `+prefix+`:amd="2014" `+prefix+`:corr="0">`+
			`<dc:title><rdf:Alt><rdf:li xml:lang="x-default">A named document</rdf:li></rdf:Alt></dc:title>`+
			`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`)
	})
}

// withUnprefixedPartAttribute writes `part="1"` with NO prefix (P06.S01). Under XML Namespaces an
// unprefixed attribute is in no namespace at all — not even the element's default one — so it names
// no property of the identification and must not create a subject for the clause-5 family.
func withUnprefixedPartAttribute(t *testing.T, pdf []byte) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		return replaceMetadataPacket(ctx, `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>`+
			`<x:xmpmeta xmlns:x="adobe:ns:meta/">`+
			`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`+
			`<rdf:Description rdf:about="" xmlns="http://www.aiim.org/pdfua/ns/id/" part="1">`+
			`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`)
	})
}

// withOneUAPropertyWronglyPrefixed writes all three identification properties, with exactly ONE of
// them under a foreign prefix (P06.S01) — the fixture that tells `5 t3`, `5 t4` and `5 t5` apart.
func withOneUAPropertyWronglyPrefixed(t *testing.T, pdf []byte, wrong string) []byte {
	prefixOf := func(prop string) string {
		if prop == wrong {
			return "pdfuaia"
		}
		return "pdfuaid"
	}
	body := ""
	for _, p := range []struct{ name, value string }{{"part", "1"}, {"amd", "2014"}, {"corr", "0"}} {
		q := prefixOf(p.name)
		body += `<` + q + `:` + p.name + `>` + p.value + `</` + q + `:` + p.name + `>`
	}
	return mutate(t, pdf, func(ctx *model.Context) error {
		return replaceMetadataPacket(ctx, `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>`+
			`<x:xmpmeta xmlns:x="adobe:ns:meta/">`+
			`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`+
			`<rdf:Description rdf:about="" `+
			`xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/" `+
			`xmlns:pdfuaia="http://www.aiim.org/pdfua/ns/id/">`+
			body+
			`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`)
	})
}

// withSuspects sets `/MarkInfo /Suspects` (P06.S02) — the only way to 7.1 t4's failing half, since
// no nib door writes the key and the autotagger has no notion of unreliable tagging to record.
func withSuspects(t *testing.T, pdf []byte, v bool) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		mi, derr := ctx.DereferenceDict(cat["MarkInfo"])
		if derr != nil {
			return derr
		}
		if mi == nil {
			mi = types.Dict{}
			cat["MarkInfo"] = mi
		}
		mi["Suspects"] = types.Boolean(v)
		return nil
	})
}

// withDynamicXFA installs an AcroForm whose XFA config packet declares `dynamicRender required`
// (P06.S02) — 7.15 t1's failing half. Nib writes no XFA at all, so this is the only route.
//
// **The element is written the way veraPDF's own fixture writes it**, with the newline before the
// closing angle bracket, because that is the serialisation a `bytes.Contains` search misses and the
// rule is parsed rather than searched for exactly that reason.
func withDynamicXFA(t *testing.T, pdf []byte) []byte {
	return withXFAConfig(t, pdf, "<config xmlns=\"http://www.xfa.org/schema/xci/3.0/\"><acrobat>"+
		"<acrobat7><dynamicRender\n>required</dynamicRender\n></acrobat7></acrobat></config>")
}

// withSpecKey rewrites one key on every file specification carrying an embedded file (P06.S02):
// `to == nil` deletes it, otherwise it is set. Splitting 7.11 t1's conjunction needs all four.
func withSpecKey(t *testing.T, pdf []byte, key string, to types.Object) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		touched := 0
		for nr := range ctx.XRefTable.Table {
			o, err := ctx.Dereference(types.IndirectRef{ObjectNumber: types.Integer(nr)})
			if err != nil {
				continue
			}
			dict, ok := o.(types.Dict)
			if !ok {
				continue
			}
			if _, hasEF := dict["EF"]; !hasEF {
				continue
			}
			// **Counts a CHANGE, not a specification.** Counting the specs it found meant the delete
			// case reported success on a document whose specs never carried the key — returning the
			// UNMUTATED document while claiming, in the error string below, to prevent exactly that.
			if to == nil {
				if _, had := dict[key]; !had {
					continue
				}
				delete(dict, key)
			} else {
				dict[key] = to
			}
			touched++
		}
		if touched == 0 {
			return fmt.Errorf("no file specification with an /EF changed, so this fixture IS the " +
				"unmutated document and the case it names is never reached")
		}
		return nil
	})
}

// withBareFileSpec adds a typed file specification carrying NO embedded file (P06.S02) — the shape
// that is a PASSING check rather than an absent subject, and the one nib used to miss.
//
// **It hangs off an annotation's `/FS` rather than an `/AF` array, and that is forced rather than
// chosen.** A spec reached only from `/AF` does not survive a pdfcpu write: measured, the `/AF` key is
// written and the specification object it names is dropped, leaving a dangling reference
// (`/pending 655`). A fixture built that way has no subject at all, so it would assert nothing — the
// first version of this helper did exactly that and the test failed for the wrong reason.
func withBareFileSpec(t *testing.T, pdf []byte) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		annot, err := ctx.IndRefForNewObject(types.Dict{
			"Type":    types.Name("Annot"),
			"Subtype": types.Name("FileAttachment"),
			"Rect":    types.NewNumberArray(50, 50, 70, 70),
			"F":       types.Integer(4),
			"FS": types.Dict{
				"Type": types.Name("Filespec"),
				"F":    types.StringLiteral("elsewhere.csv"),
			},
		})
		if err != nil {
			return err
		}
		page, _, _, perr := ctx.PageDict(1, false)
		if perr != nil {
			return perr
		}
		page["Annots"] = types.Array{*annot}
		return nil
	})
}

// withStaticXFA installs an XFA form whose `dynamicRender` is `forbidden` (P06.S02) — 7.15 t1's
// passing half with a subject present, which is a different answer from having no form at all.
func withStaticXFA(t *testing.T, pdf []byte) []byte {
	return withXFAConfig(t, pdf, "<config xmlns=\"http://www.xfa.org/schema/xci/3.0/\"><acrobat>"+
		"<acrobat7><dynamicRender\n>forbidden</dynamicRender\n></acrobat7></acrobat></config>")
}

// withXFAConfig is `withDynamicXFA`'s door, taking the config packet verbatim.
func withXFAConfig(t *testing.T, pdf []byte, config string) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		sd, err := ctx.NewStreamDictForBuf([]byte(config))
		if err != nil {
			return err
		}
		if err := sd.Encode(); err != nil {
			return err
		}
		ref, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			return err
		}
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		form, err := ctx.IndRefForNewObject(types.Dict{
			"Fields": types.Array{},
			"XFA":    types.Array{types.StringLiteral("config"), *ref},
		})
		if err != nil {
			return err
		}
		cat["AcroForm"] = *form
		return nil
	})
}

// withXFAPackets installs an AcroForm whose `/XFA` array holds the given packets in order, each under
// a name (P06.S02) — so a packet nib cannot read can be put BEFORE the one that answers.
func withXFAPackets(t *testing.T, pdf []byte, packets ...string) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		arr := types.Array{}
		for i, p := range packets {
			sd, err := ctx.NewStreamDictForBuf([]byte(p))
			if err != nil {
				return err
			}
			if err := sd.Encode(); err != nil {
				return err
			}
			ref, err := ctx.IndRefForNewObject(*sd)
			if err != nil {
				return err
			}
			arr = append(arr, types.StringLiteral(fmt.Sprintf("packet%d", i)), *ref)
		}
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		form, err := ctx.IndRefForNewObject(types.Dict{"Fields": types.Array{}, "XFA": arr})
		if err != nil {
			return err
		}
		cat["AcroForm"] = *form
		return nil
	})
}

// withEncryption encrypts pdf with an OWNER password only, under the given permissions (P06.S03).
//
// **The user password is empty on purpose.** `pdfops.Encrypt` refuses an empty one — it sets the same
// secret as both user and owner so `RemovePassword` reverses it exactly — but an oracle document has to
// be readable by veraPDF, which is run without a password. An owner-only restriction is the shape the
// corpus's own `7.16-t01-fail-a.pdf` uses, and veraPDF reads both of these: measured, it reports
// `passedChecks="1"` for the permissive one and `failedChecks="1"` for the restrictive one.
//
// **`model.PermissionsNone` yields `/P = -3901`, which is exactly what `pdfops.Encrypt` writes today**
// — it never sets `conf.Permissions` at all. That is the coupling P06 declared at its open, and it is
// why this clause fails nib's own protected output.
func withEncryption(t *testing.T, pdf []byte, perms model.PermissionFlags) []byte {
	t.Helper()
	conf := model.NewAESConfiguration("", "owner", 256)
	conf.Permissions = perms
	var out bytes.Buffer
	if err := api.Encrypt(bytes.NewReader(pdf), &out, conf); err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return out.Bytes()
}

// withReferenceXObject adds a form XObject carrying `/Ref` to page 1's resources (P06.S03) —
// 7.20 t1's failing half. Nib writes no reference XObject, so there is no product door to it.
func withReferenceXObject(t *testing.T, pdf []byte) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		sd, err := ctx.NewStreamDictForBuf([]byte(""))
		if err != nil {
			return err
		}
		sd.Dict["Type"] = types.Name("XObject")
		sd.Dict["Subtype"] = types.Name("Form")
		sd.Dict["BBox"] = types.NewNumberArray(0, 0, 10, 10)
		sd.Dict["Ref"] = types.Dict{
			"F":    types.Dict{"Type": types.Name("Filespec"), "F": types.StringLiteral("external.pdf")},
			"Page": types.Integer(0),
		}
		if err := sd.Encode(); err != nil {
			return err
		}
		ref, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			return err
		}
		page, _, _, perr := ctx.PageDict(1, false)
		if perr != nil {
			return perr
		}
		res, _ := ctx.DereferenceDict(page["Resources"])
		if res == nil {
			res = types.Dict{}
			page["Resources"] = res
		}
		xo, _ := ctx.DereferenceDict(res["XObject"])
		if xo == nil {
			xo = types.Dict{}
			res["XObject"] = xo
		}
		xo["Xref0"] = *ref
		// **The form must be DRAWN, or it is not a subject at all.** Measured: veraPDF reports
		// 0 passed / 0 failed for a form that sits in a page's resources and is never drawn, and FAILS
		// the same form once the content stream draws it. The first version of this helper omitted the
		// operator, and the oracle caught it — nib failed a document veraPDF found no subject in.
		cs, cerr := ctx.PageContent(page, 1)
		if cerr != nil {
			return cerr
		}
		drawn, derr := ctx.NewStreamDictForBuf(append(append([]byte{}, cs...), []byte("\nq /Xref0 Do Q\n")...))
		if derr != nil {
			return derr
		}
		if eerr := drawn.Encode(); eerr != nil {
			return eerr
		}
		cref, rerr := ctx.IndRefForNewObject(*drawn)
		if rerr != nil {
			return rerr
		}
		page["Contents"] = *cref
		return nil
	})
}

// withUndrawnReferenceXObject puts a `/Ref`-carrying form in page 1's resources and does NOT draw it
// (P06.S03) — the shape veraPDF reports no subject for.
func withUndrawnReferenceXObject(t *testing.T, pdf []byte) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		sd, err := ctx.NewStreamDictForBuf([]byte(""))
		if err != nil {
			return err
		}
		sd.Dict["Type"] = types.Name("XObject")
		sd.Dict["Subtype"] = types.Name("Form")
		sd.Dict["BBox"] = types.NewNumberArray(0, 0, 10, 10)
		sd.Dict["Ref"] = types.Dict{
			"F":    types.Dict{"Type": types.Name("Filespec"), "F": types.StringLiteral("external.pdf")},
			"Page": types.Integer(0),
		}
		if err := sd.Encode(); err != nil {
			return err
		}
		ref, rerr := ctx.IndRefForNewObject(*sd)
		if rerr != nil {
			return rerr
		}
		page, _, _, perr := ctx.PageDict(1, false)
		if perr != nil {
			return perr
		}
		res, _ := ctx.DereferenceDict(page["Resources"])
		if res == nil {
			res = types.Dict{}
			page["Resources"] = res
		}
		xo, _ := ctx.DereferenceDict(res["XObject"])
		if xo == nil {
			xo = types.Dict{}
			res["XObject"] = xo
		}
		xo["Unused0"] = *ref
		return nil
	})
}

// withRefOnAppearance puts `/Ref` on the NORMAL appearance stream of every annotation (P06.S03).
//
// **It stamps `/AP /N` through the annotation door, not every form in the file.** The first version
// walked the xref table and stamped any stream whose `/Subtype` is `Form`, so a Fail could have come
// from a page-content form rather than an appearance — the test's whole claim. Nothing asserted the
// difference, and the fixture passed only because `AuthorForm`'s output happens to draw no other form.
func withRefOnAppearance(t *testing.T, pdf []byte) []byte {
	return mutate(t, pdf, func(ctx *model.Context) error {
		touched := 0
		for p := 1; ; p++ {
			page, _, _, perr := ctx.PageDict(p, false)
			if perr != nil || page == nil {
				break
			}
			annots, _ := ctx.DereferenceArray(page["Annots"])
			for _, ao := range annots {
				ad, derr := ctx.DereferenceDict(ao)
				if derr != nil || ad == nil {
					continue
				}
				ap, aerr := ctx.DereferenceDict(ad["AP"])
				if aerr != nil || ap == nil {
					continue
				}
				sd, _, serr := ctx.DereferenceStreamDict(ap["N"])
				if serr != nil || sd == nil {
					continue
				}
				sd.Dict["Ref"] = types.Dict{
					"F":    types.Dict{"Type": types.Name("Filespec"), "F": types.StringLiteral("external.pdf")},
					"Page": types.Integer(0),
				}
				if ir, ok := ap["N"].(types.IndirectRef); ok {
					ctx.XRefTable.Table[ir.ObjectNumber.Value()].Object = *sd
				}
				touched++
			}
		}
		if touched == 0 {
			return fmt.Errorf("no annotation carried an /AP /N stream to stamp, so this fixture is the " +
				"unmutated document")
		}
		return nil
	})
}

// openEncrypted opens a password-protected document for a rule to read (P06.S03). `open` uses the
// default configuration, which has no password, so a protected document cannot go through it.
func openEncrypted(t *testing.T, pdf []byte, password string) (*Document, string) {
	t.Helper()
	conf := model.NewDefaultConfiguration()
	conf.UserPW, conf.OwnerPW = password, password
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), conf)
	if err != nil {
		return nil, err.Error()
	}
	cat, cerr := ctx.XRefTable.Catalog()
	if cerr != nil {
		return nil, cerr.Error()
	}
	return &Document{Ctx: ctx, Catalog: cat, raw: pdf}, ""
}

// withFormulaParagraph retypes the document's first addressable paragraph to `/Formula` through the
// structure editor's own door, optionally giving it alternate text (P06.S04).
//
// **This is the construction that used to BE the documented counterexample.** Until `7.7 t1` shipped,
// a Markdown conversion whose paragraph was retyped `/Formula` with no alternate text passed every
// clause nib checked and failed veraPDF; `counterexample_test.go` built exactly this and asserted it.
// Now that the checker catches it, the same construction is what puts both halves of the clause in
// front of the oracle — the example did not disappear, it became a test.
func withFormulaParagraph(t *testing.T, pdf []byte, alt string) []byte {
	t.Helper()
	tree, err := pdfops.ReadStructure(pdf)
	if err != nil {
		t.Fatalf("read structure: %v", err)
	}
	para := 0
	for _, e := range tree.Elements {
		if e.Standard == "P" && e.ID > 0 {
			para = e.ID
			break
		}
	}
	if para == 0 {
		t.Fatal("setup: no addressable paragraph to retype, so the fixture is the unmutated document")
	}
	edits := []pdfops.StructureEdit{{Kind: "retype", Element: para, Value: "Formula", Index: -1}}
	if alt != "" {
		edits = append(edits, pdfops.StructureEdit{Kind: "alt", Element: para, Value: alt, Index: -1})
	}
	out, err := pdfops.EditStructure(pdf, edits)
	if err != nil {
		t.Fatalf("retype to Formula: %v", err)
	}
	return out
}

// withSheetFormFused makes an n-up document's second sheet draw its FIRST sheet's first form in place of its
// own — the shape pdfcpu's optimize pass produced before P02.S02 made the carry un-fuse it (ADR-038): one
// form carrying `/StructParents`, drawn on two sheets. 7.20 t2's failing half.
func withSheetFormFused(t *testing.T, pdf []byte) []byte {
	t.Helper()
	sheetForms := func(ctx *model.Context, p int) types.Dict {
		page, _, _, err := ctx.PageDict(p, false)
		if err != nil || page == nil {
			t.Fatalf("setup: sheet %d does not resolve: %v", p, err)
		}
		res, _ := ctx.DereferenceDict(page["Resources"])
		xo, _ := ctx.DereferenceDict(res["XObject"])
		if len(xo) == 0 {
			t.Fatalf("setup: sheet %d draws no form XObject, so there is nothing to fuse", p)
		}
		return xo
	}
	first := func(d types.Dict) string {
		keys := make([]string, 0, len(d))
		for k := range d {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys[0]
	}
	return mutate(t, pdf, func(ctx *model.Context) error {
		if ctx.PageCount < 2 {
			t.Fatalf("setup: the n-up document has %d sheet(s); fusing needs two", ctx.PageCount)
		}
		one, two := sheetForms(ctx, 1), sheetForms(ctx, 2)
		// Stimulus before response: the two sheets' forms must start out DIFFERENT objects, or the
		// document is already fused and this helper measures nothing.
		if one[first(one)] == two[first(two)] {
			t.Fatalf("setup: sheets 1 and 2 already draw the same form %v", one[first(one)])
		}
		// And the form grafted must carry the key, or the second sheet draws an unkeyed form twice and
		// 7.20 t2 has nothing to fail.
		if sd, _, err := ctx.DereferenceStreamDict(one[first(one)]); err != nil || sd == nil || sd.Dict["StructParents"] == nil {
			t.Fatalf("setup: sheet 1's form %v carries no /StructParents, so drawing it twice fails nothing", one[first(one)])
		}
		two[first(two)] = one[first(one)]
		return nil
	})
}

// withCIDFontType2 is a Type 0 font over a CIDFontType2 whose program is nib's own embedded LiberationMono, with or
// without a `/CIDToGIDMap` (P07.S01) — the shape pdfcpu's validator keeps, unlike the corpus's `7.21.3.2-t01-fail-a`.
func withCIDFontType2(t *testing.T, withMap bool) []byte {
	t.Helper()
	prog, err := os.ReadFile("../pdfops/fonts/LiberationMono-BoldItalic.ttf")
	if err != nil {
		t.Fatalf("setup: the TrueType program nib ships is missing: %v", err)
	}
	return withCIDFontType2Program(t, prog, withMap)
}

// withCIDFontType2Program is withCIDFontType2 over an arbitrary program — bytes neither nib nor veraPDF may parse.
func withCIDFontType2Program(t *testing.T, prog []byte, withMap bool) []byte {
	t.Helper()
	pdf := buildPDF(type0Doc("/Identity-H", "/CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >>", nil))
	return mutate(t, pdf, func(ctx *model.Context) error {
		sd, err := ctx.NewStreamDictForBuf(prog)
		if err != nil {
			return err
		}
		if err := sd.Encode(); err != nil {
			return err
		}
		ref, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			return err
		}
		cid, err := ctx.DereferenceDict(types.IndirectRef{ObjectNumber: 11})
		if err != nil || cid == nil {
			t.Fatalf("setup: the CIDFont does not resolve: %v", err)
		}
		cid["Subtype"] = types.Name("CIDFontType2")
		if withMap {
			cid["CIDToGIDMap"] = types.Name("Identity")
		}
		desc, err := ctx.DereferenceDict(types.IndirectRef{ObjectNumber: 12})
		if err != nil || desc == nil {
			t.Fatalf("setup: the font descriptor does not resolve: %v", err)
		}
		desc["FontFile2"] = *ref
		return nil
	})
}

// asFontFile3 moves a CIDFont's `/FontFile2` program to `/FontFile3` with the given `/Subtype` (P07.S01's review).
func asFontFile3(t *testing.T, pdf []byte, subtype string) []byte {
	t.Helper()
	return mutate(t, pdf, func(ctx *model.Context) error {
		desc, err := ctx.DereferenceDict(types.IndirectRef{ObjectNumber: 12})
		if err != nil || desc == nil {
			t.Fatalf("setup: the font descriptor does not resolve: %v", err)
		}
		ref, has := desc["FontFile2"]
		if !has {
			t.Fatal("setup: the descriptor has no /FontFile2 to move")
		}
		sd, _, err := ctx.DereferenceStreamDict(ref)
		if err != nil || sd == nil {
			t.Fatalf("setup: /FontFile2 is not a stream: %v", err)
		}
		sd.Dict["Subtype"] = types.Name(subtype)
		delete(desc, "FontFile2")
		desc["FontFile3"] = ref
		return nil
	})
}
