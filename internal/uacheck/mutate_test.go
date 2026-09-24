package uacheck

import (
	"bytes"
	"fmt"
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

// widgetMutation breaks one link of a described form's widget ↔ Form-element linkage.
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
