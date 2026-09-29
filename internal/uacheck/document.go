package uacheck

import (
	"fmt"
	"nib/internal/fontcode"
	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Document is a document opened once for every rule to read.
//
// # Why the rules share one open
//
// Each rule could open the bytes itself, and eighteen rules would then be eighteen
// `ReadValidateAndOptimize` calls over the same file. Measured elsewhere in this repo, that call is
// the expensive part of every operation in `internal/pdfops`. More importantly they would be
// eighteen possibly-different readings: `ReadValidateAndOptimize` normalises as it reads, so two
// rules could legitimately disagree about the same document and nothing would say which was right.
type Document struct {
	// Ctx is the parsed document. Rules read it; nothing mutates it — a checker that edited what it
	// was inspecting would report on a document the user does not have.
	Ctx *model.Context
	// Catalog is the root dictionary, resolved once because nearly every rule wants it.
	Catalog types.Dict

	// ptAns memoises parentTreeEntry's lookups by key, and ptParsed each parent-tree node's /Limits, /Nums and /Kids
	// by dictionary (`dictID`). ptLoop is the first loop a lookup met (veraPDF reports nothing), ptUnknown the first
	// lookup nib stopped short of, and ptSpent the read budget's refusal once it is spent (RR3-1, RR3-2).
	ptAns     map[int]ptAnswer
	ptParsed  map[uintptr]*ptNode
	ptLoop    string
	ptUnknown string
	ptSpent   string
	// nodes is every structure element reached from the root, built on first use by structNodes, and
	// nodesErr is why part of the tree was not read, when part was not.
	nodes     []structNode
	nodesErr  string
	nodesDone bool
	// nodeEntries and ptNodes are what `structNodes` and `parentTreeEntry` read — `/K` entries, repeats included, and
	// parent-tree node reads across every lookup — kept so a test can assert the stimulus of their bounds (R3-8, RR3-1).
	nodeEntries int
	ptNodes     int
	// roles memoises standardType's walk of the /RoleMap, keyed by every name on the path it followed —
	// the chain is followed to its end now (`/pending 507`), so a document with a long chain and many
	// elements would otherwise re-walk it once per element.
	roles map[string]roleResolution
	// tables memoises each table's layout (`rules_table.go`), keyed by the Table's dictionary (`dictID`), so an
	// inline table is laid out once, like one written as its own object.
	tables map[uintptr]*tableLayout
	// circular memoises roleMapCircular's walk, keyed by the element's own /S (P03.S01).
	circular map[string]bool
	// content is every page's classified drawing operators, built on first use by contentEvents.
	content []contentEvent
	// mcSubjects is every marked-content sequence the same walk closed, which is a different population
	// from `content`: a sequence enclosing no drawing operator is still a subject of 7.1 t1/t2 and
	// 7.2 t30-t32, and a drawing operator inside none is still a subject of 7.1 t3.
	mcSubjects []mcSubject
	// rootReach memoises `reachesStructTreeRoot` per element — `taggedContent` asks it once per drawing
	// operator and once per closed sequence, so without it one deep `/P` chain is climbed per event.
	rootReach map[uintptr]bool
	// langWalked is every tiling pattern and Type 3 glyph procedure the lang-only walk has already read,
	// keyed by the stream dictionary's identity — see `enterLangOnly` for why once per stream and not once
	// per use.
	langWalked map[uintptr]bool
	// charProcsRead is every Type 3 `/CharProcs` dictionary `enterType3` has handed whole to the lang-only walk,
	// and charProcAsks what re-enumerating one has cost against `maxCharProcAsks` (R3-1).
	charProcsRead map[uintptr]bool
	charProcAsks  int
	// streams numbers the content streams the walk has entered, so a frame knows which one it was opened
	// in. `inheritedLang` stops at that boundary where `parentsTags` and the struct parent cross it.
	streams int
	// contentErr is why content could not be read, or not all of it, when it could not.
	contentErr  string
	contentDone bool
	// contentFinished is set when the walk RETURNS; contentDone when it starts. A panic leaves only the second.
	contentFinished bool
	// formWalks counts the form XObjects the content walk has entered, and contentOver records that it spent
	// its budget (`overBudget`).
	formWalks   int
	contentOps  int
	contentOver bool
	// contentBytes is every byte the content walk has walked, counted at each walk and so once per DRAW — what
	// `maxContentBytes` bounds, because a stream of whitespace spends no operator (`/pending 721`).
	contentBytes int
	// decoded is each form XObject's and appearance stream's decoded content by object number, so a form drawn N
	// times is decoded once (`decodedContent`); streamDecodes counts the decodes that door performed.
	decoded       map[int][]byte
	streamDecodes int
	// glyphs is every distinct (font, code) a text-showing operator draws — veraPDF's `Glyph` (`glyphs.go`) —
	// gathered by the same walk; glyphSeen dedupes it, glyphFonts reads each font once, and glyphCodes counts
	// the codes read against `maxGlyphCodes`.
	glyphs     []glyph
	glyphSeen  map[glyphKey]int
	glyphFonts map[any]*glyphFont
	// ttList is the used TrueType fonts (`trueTypeFonts`), built once; ttByDict finds one by its dictionary.
	ttList   []*ttFont
	ttByDict map[uintptr]*ttFont
	ttErr    string
	ttDone   bool
	// ttUnresolved is the first used font that did not resolve — a refusal the TrueType clauses give only where no
	// font nib did read fails; ttPrograms is each program stream's parse, once, and ttReads their shared budget.
	ttUnresolved string
	ttPrograms   map[uintptr]trueTypeProgram
	ttReads      int
	cidReads     map[uintptr]cidRead
	cidMaps      map[uintptr]*fontcode.CIDMap // each embedded CMap stream's code-to-CID mappings, parsed once
	// cidWidths is each CIDFont's /W and /DW, read once however many Type 0 fonts share it, and cidWEntries what they
	// cost against `maxCIDWEntries`; cidGIDMaps each /CIDToGIDMap stream, decoded once; openTypeCFF each /FontFile3
	// /OpenType stream's "CFF " search, once (the P07 phase close, R2-3 and R2-1).
	cidWidths   map[uintptr]*cidWidthTable
	cidWEntries int
	cidGIDMaps  map[uintptr]cidGIDRead
	// cidSets is each subset-named descendant's 7.21.4.2 t2 answer, read once however many Type 0 fonts share it;
	// cidSetJudged counts the /CIDSet reads that reached the program's population (RR1-3).
	cidSets      map[uintptr]cidSetRead
	cidSetJudged int
	// cmapMalformedRead is each embedded CMap stream's "malformed" answer for 7.21.3.3 t2, parsed once; the parses are
	// counted in cmapMalformedParses (RR1-4).
	cmapMalformedRead   map[uintptr]bool
	cmapMalformedParses int
	openTypeCFF         map[uintptr]openTypeRead
	// drawnUnrecorded is every font a pattern or Type 3 procedure draws — glyphs recorded, font events not.
	drawnUnrecorded map[uintptr]bool
	glyphCodes      int
	// nothing is `reportsNothing`'s answer, computed once; t3Widths each Type 3 glyph procedure's width, read once.
	nothing     string
	nothingDone bool
	t3Widths    map[uintptr]t3Width
	// cidAsks is what the document's embedded CMaps have cost — entries parsed and mappings asked — against
	// `maxCIDAsks`, apart from `ttReads` so a large CMap cannot starve the TrueType clauses of their budget.
	cidAsks int
	// type1CReads is each Type1C program's reading per stream and subset-ness (P07.S05a), type1CThrows where veraPDF
	// throws reading it.
	type1CReads  map[type1CKey]type1CRead
	type1CThrows map[type1CKey]string
	// type1Reads is each /FontFile Type 1 program's reading, per stream (P07.S06).
	type1Reads map[uintptr]*type1Program
	// cffSpent and type1Spent are what the document's CFF and Type 1 programs have cost, all of them together — one
	// budget per program type, as `ttReads` is TrueType's.
	cffSpent   cffSpend
	type1Spent t1Spend
	// toUnicodes caches each /ToUnicode stream's parse, and toUnicodeBlocks is the range-index budget they share.
	toUnicodes      map[uintptr]*fontcode.ToUnicode
	toUnicodeBlocks int
	// toUnicodeChains is each /ToUnicode stream's reading WITH its /UseCMap chain linked (`toUnicodeCMap`), so fonts
	// sharing a stream share the chain; toUnicodeHops counts the streams that chain walk actually read.
	toUnicodeChains map[uintptr]toUnicodeChain
	toUnicodeHops   int
	// fontChainReads counts the Type 0 CMap chains the glyph door walked (`chainOf`): once per font, never per glyph.
	fontChainReads int
	glyphsOver     bool
	// kids memoises elementKids, keyed by the element's dictionary (`dictID`), and kidBudget is how many more
	// kids the document may expand before nib stops (`maxKidExpansion`).
	kids      map[uintptr]kidsResult
	kidBudget int
	// langs memoises parentLang's climb, keyed by the dictionary whose `/P` chain was followed (`dictID`), each
	// answer carrying how far above that dictionary it sits so a deeper climb is refused where a fresh one would be.
	langs map[uintptr]langClimb
	// mcLangCount is how many string `/Lang` values BDC property lists carried in the content walk, and mcLangBad
	// the first that failed the grammar (7.2 t29).
	mcLangCount int
	mcLangBad   *mcLang
	// raw is the file as given, kept for the one question the validated context cannot answer: whether pdfcpu's
	// validator dropped something (`hasInlineType3Font`). directType3 memoises that answer.
	raw         []byte
	directType3 *bool
	// xmp memoises readXMP.
	xmp     xmpFacts
	xmpDone bool
	// clipList is every media clip dictionary the document's actions reach, built on first use by
	// `mediaClips`; clipsErr is why the population may be short.
	clipList  []mediaClip
	clipsErr  string
	clipsDone bool
	// annotList is every annotation on every page, built on first use by `annots` — the ONE door
	// (ADR-009); annotsErr is why the population may be short, when it may be.
	annotList  []annotSubject
	annotsErr  string
	annotsDone bool
	// tableSlots is every grid slot the document's tables have asked for so far (`maxDocumentTableSlots`).
	tableSlots int64
	// specList is every file specification carrying an /EF, built on first use by `fileSpecs` — the ONE
	// door (ADR-009); specsErr is why the population may be short, when it may be.
	specList  []fileSpec
	specsErr  string
	specsDone bool
	// drawnForms is every form XObject the content walk actually entered — `7.20 t1`'s population,
	// which is what the document DRAWS rather than what it holds; drawnSeen dedups by identity.
	drawnForms []formXObject
	drawnSeen  map[uintptr]bool
	// formReaches is `7.20 t2`'s tally: per form XObject object number, every time veraPDF would build a
	// `PDXForm` for it. traversed is every stream object the walk has traversed the way veraPDF does, ONCE
	// per object key, and reachedAnnots every annotation already tallied — `retraversal` says why.
	formReaches   map[int]*formReach
	traversed     map[int]bool
	reachedAnnots map[int]bool
	// twinsOf memoises `formTwins` per object number; formsByLength and twinCompares are its index and budget.
	twinsOf map[int]int
	// drawsForms is every form XObject whose content draws another form (`walker.form`).
	drawsForms    map[int]bool
	formsByLength map[int64][]int
	twinCompares  int
}

// nextStream hands out the next content-stream number.
func (d *Document) nextStream() int {
	d.streams++
	return d.streams
}

// roleResolution is what one `/S` name resolves to through the role map: a standard structure type, or
// why nib could not follow the map to one. Exactly one of those two is set.
//
// **`circular` is a THIRD fact and it is independent of both** (`/pending 548`). Whether the walk revisits
// a name is not the same question as whether it can be typed, and veraPDF is what separates them: on
// `7.1 General/7.1-t06-fail-a.pdf` the role map is `<< /LI /LI >>` and veraPDF FAILS ua1 7.1-6 on the two
// `LI` elements — a self-map is a circular mapping — while the elements still type as `LI`, because a
// conforming reader recognises `LI` before it ever consults the map. Deriving one fact from the other
// either loses that failure or breaks three shipped rules over a document nothing is wrong with.
type roleResolution struct {
	standard   string
	unresolved string
}

// checkerConfig is the ONE pdfcpu configuration every read in this package uses (ADR-009): `open`, and
// the two unvalidated re-parses (`dynamicRenderFromRawFile`, and `rules_language.go`'s).
//
// **Every field that shapes a READ is pinned to pdfcpu's own built-in default**, because
// `NewDefaultConfiguration` otherwise takes them from the user's `$XDG_CONFIG_HOME/pdfcpu/config.yml` —
// and a checker whose answer depends on which machine asks is not a checker. Measured at P06.S05's review:
// `optimizeDuplicateContentStreams: true` there fused two pages' content streams and turned a keyed form
// drawn once on each from Fail to Pass (`TestThePdfcpuConfigOnThisMachineDoesNotMoveTheAnswer`). The P06
// phase-close review then found the pin covered that one field: `optimize: false` would switch off the
// form fusion `formTwins` exists to account for, and `validationMode` decides which files open at all.
func checkerConfig() *model.Configuration {
	conf := model.NewDefaultConfiguration()
	conf.Reader15 = true
	conf.DecodeAllStreams = false
	conf.ValidationMode = model.ValidationRelaxed
	conf.ValidateLinks = false
	conf.Optimize = true
	conf.OptimizeResourceDicts = true
	conf.OptimizeDuplicateContentStreams = false
	conf.Limits = model.DefaultResourceLimits()
	return conf
}

// open parses pdf for the rules to read.
//
// **A document that cannot be opened is an error, not a report full of failures.** A checker that
// answered "fails every clause" for a file it could not parse would be telling the user their
// document is inaccessible when what happened is that nib could not read it — two different facts,
// and the second is not the user's to fix.
func open(pdf []byte) (*Document, error) {
	if len(pdf) == 0 {
		return nil, fmt.Errorf("uacheck: no document to check")
	}
	conf := checkerConfig()
	// Through nib's one read door (`/pending 675`: a /UseCMap loop is refused before pdfcpu's validator recurses
	// on it and kills the process) and its optimize budget, refusing rather than skipping the pass
	// (`/pending 714`: the rules were measured against the optimized reading — see ReadOptimizedOrRefuse).
	ctx, err := pdfread.ReadOptimizedOrRefuse(pdf, conf)
	if err != nil {
		return nil, fmt.Errorf("uacheck: the document could not be read: %w", err)
	}
	cat, cerr := ctx.XRefTable.Catalog()
	if cerr != nil {
		return nil, fmt.Errorf("uacheck: the document has no catalog: %w", cerr)
	}
	return &Document{Ctx: ctx, Catalog: cat, raw: pdf}, nil
}

// declaresLang reports whether obj is a declared `/Lang`: a string, direct or indirect, literal or hex —
// and **present counts, even when empty**.
//
// **`/pending 489`, found on veraPDF's own corpus, in two halves.**
//   - **Encodings.** Every reader here cast to a direct `types.StringLiteral`, which is what nib and
//     LibreOffice write, so law 5's oracle never met anything else. The corpus stores
//     `/Lang <FEFF0045004E002D00550053>` and `/Lang 12 0 R` too, and each read as no language.
//   - **Presence, not content.** veraPDF's 7.2 t33 test is `gContainsCatalogLang` and its 7.2 t34 test
//     is `Lang != null` — the key being there. Its three corpus files `7.2-t29-fail-n/o/p.pdf` declare an
//     EMPTY `/Lang` (on the catalog, on a structure element, on marked content), and veraPDF passes t33
//     and t34 on all three while failing 7.2 t29, whose subject is the empty value. nib does not
//     implement t29, so reading "" as undeclared filed a t29 failure under t33 or t34.
//
// This is the checker's OWN door, not `pdfops`' — structure.go says why the checker keeps its readings
// independent of the writer's.
func (d *Document) declaresLang(obj types.Object) bool {
	_, ok := d.text(obj)
	return ok
}

// boolValue resolves a boolean that may be stored indirectly — `/Marked 42 0 R` with `42 0 obj true`
// is legal, veraPDF reads it as true, and a bare cast read it as absent (`/pending 489`).
func (d *Document) boolValue(obj types.Object) (value, ok bool) {
	if obj == nil {
		return false, false
	}
	b, err := d.Ctx.XRefTable.DereferenceBoolean(obj, model.V10)
	if err != nil || b == nil {
		return false, false
	}
	return b.Value(), true
}

// # The typed-value door — `/pending 496`
//
// Any PDF value may be stored as an indirect object, and a reader that casts a dictionary entry straight to
// `types.Integer` or calls pdfcpu's `NameEntry` sees `42 0 R` as the wrong type. Six rules did, and it cut
// both ways: `/DisplayDocTitle 9 0 R` failed 7.1 t10 on a document veraPDF passes, and a Figure whose `/S`
// was indirect was not a Figure, so its missing alternate text passed 7.3 t1. Every typed read in this
// package goes through these, and `TestTheRulesReadTypedValuesOnlyThroughTheDoor` refuses one that does not.

// intValue resolves an integer that may be stored indirectly.
func (d *Document) intValue(obj types.Object) (int, bool) {
	if obj == nil {
		return 0, false
	}
	i, err := d.Ctx.XRefTable.DereferenceInteger(obj)
	if err != nil || i == nil {
		return 0, false
	}
	return i.Value(), true
}

// name resolves a name that may be stored indirectly, or "" when obj is absent or not a name.
func (d *Document) name(obj types.Object) string {
	if obj == nil {
		return ""
	}
	n, err := d.Ctx.XRefTable.DereferenceName(obj, model.V10, nil)
	if err != nil {
		return ""
	}
	return n.Value()
}

// text resolves a string — literal or hex, direct or indirect — and reports whether obj was one.
//
// **A reference to an object that is not there is NOT one** (P04.S02's review, measured). pdfcpu's
// `DereferenceStringOrHexLiteral` answers `("", nil)` for a reference to a free object, which is
// indistinguishable here from an empty string that is really present — and veraPDF reads such a value as
// absent (`COSObject.getString` over a null base). Measured on 1.30.2 with a readable file whose `/Alt` is
// `99 0 R` and whose object 99 does not exist: veraPDF has no subject for 7.2 t22 and nib FAILED it, and with
// the same reference on `/Lang` veraPDF failed the clause and nib PASSED it — a false fail and a false pass
// from one missing check, in a file pdfcpu's validator does not refuse.
func (d *Document) text(obj types.Object) (string, bool) {
	if obj == nil {
		return "", false
	}
	if r, err := d.Ctx.Dereference(obj); err != nil || r == nil {
		return "", false
	}
	s, err := d.Ctx.XRefTable.DereferenceStringOrHexLiteral(obj, model.V10, nil)
	return s, err == nil
}

// nameOf reports the name a key holds and whether the key holds a NAME at all.
//
// `d.name` cannot tell an absent key from a present empty name (`/S /`), and for 7.18.8 t1 that is a false
// PASS: veraPDF's `getNameKeyStringValue` yields `""` for the empty name, the profile tests
// `structParentType == null`, and `"" != null` — so veraPDF FAILS a printer's mark in an element whose `/S`
// is empty. Measured 2026-09-23: pdfcpu accepts such a file, veraPDF fails it, and nib passed it.
func (d *Document) nameOf(o types.Object) (string, bool) {
	if o == nil {
		return "", false
	}
	r, err := d.Ctx.Dereference(o)
	if err != nil || r == nil {
		return "", false
	}
	n, ok := r.(types.Name)
	if !ok {
		return "", false
	}
	return n.Value(), true
}

// dict resolves obj to a dictionary, or nil.
func (d *Document) dict(obj types.Object) types.Dict {
	if obj == nil {
		return nil
	}
	res, err := d.Ctx.DereferenceDict(obj)
	if err != nil {
		return nil
	}
	return res
}
