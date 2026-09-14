package uacheck

import (
	"bytes"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

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

// withUAPart rewrites the metadata packet to declare a PDF/UA part, so `5 t1`'s wrong-value branch
// can be told from its absent-value branch.
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
