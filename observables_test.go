package nib

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Every observable Go publishes has a reader, and the reader is NAMED.
//
// # The defect class
//
// `historyEvicted` was set, serialized, and asserted by six Go tests, and nothing read it —
// so ADR-003's "eviction is observable or it is not eviction" was undischarged while every
// test in the tree was green. `test/jsdom/published.test.mjs` closed that on the SERVER→CLIENT
// side and has paid twice since: `Record.version` (a doc comment matched instead of a read)
// and `verifyView` (a gate that was mandatory and unanswerable).
//
// **There was no Go-side equivalent**, and that is what this is. `udpmux.Stats`'s counters,
// `p2p.Channel`'s fields and `rendezvous.Stats` — which went from three fields to twelve in
// one slice, and is thirty now — were invisible to any scan in the tree.
//
// # Why this is not the seam-inventory graduation pass
//
// It is the defect class one level OUT from it, and P02's pass recorded the gap rather than
// papering over it: the graduation pass asks whether each inventory row's declared reader
// exists, and a field nobody entered into the inventory is invisible to a walk over the
// inventory. The inventory is written by the person who just added the field, which is
// exactly who cannot see the one they forgot. `rendezvous.Stats().Seeds` proved it — read by
// the live harness, and it had no inventory row until a sweep went looking.
//
// So the population here is DISCOVERED from source, never listed. A listed population
// reproduces the bug it is meant to catch.
//
// # What this scan can and cannot prove
//
// It proves a NAME is mentioned in a named file, with comments stripped. It does NOT prove
// the mention is a read of THIS shape: `.Seeds` in a reader satisfies `Stats.Seeds` even if
// the only `.Seeds` there belongs to something else. A green means "no field is obviously
// orphaned", not "every field is correctly consumed" — the failure it closes is
// `historyEvicted`'s, where the identifier appears nowhere at all, and that is the one that
// actually shipped. Saying so beats letting the next reader assume more.
//
// It is a pure source scan: no build, no boot, nothing to tear down.

// observablePackages are the packages whose exported data shapes leave their own package.
// Scanned WHOLE rather than file-by-file, so a shape added in a new file is in scope by
// default — `TestEveryTransportIsInTheTable` shipped blind to a third file for exactly the
// opposite reason.
var observablePackages = []string{
	"internal/ceremony",
	"internal/discovery",
	"internal/instance",
	"internal/ots",
	"internal/p2p",
	"internal/pdfops",
	"internal/rendezvous",
	"internal/sign",
	"internal/udpmux",
	"internal/vault",
}

// published names each discovered shape's readers: files that must mention every one of its
// fields. A shape's reader is frequently NOT the client — `udpmux.Stats` is read by the CLI
// and by tests, `ceremony.Record` by the code that parses it back.
var published = map[string][]string{
	// Derived from the tree, not guessed: for each shape, the files that actually mention
	// its fields with comments stripped. A first draft of this table was written from
	// memory and named four wrong files — the scan reporting a false orphan is worse than
	// no scan, so the table is evidence like everything else here.
	"udpmux.Stats":           {"internal/cli/rendezvous.go", "internal/udpmux/mux.go"},
	"rendezvous.Stats":       {"internal/cli/rendezvous.go", "internal/rendezvous/dht.go"},
	"rendezvous.SelfAddress": {"internal/rendezvous/selfaddr.go", "internal/cli/rendezvous.go"},
	"discovery.Stats":        {"internal/cli/discover.go"},
	// **Declared with its DEFINING file as the only reader, which is the honest answer and is
	// deliberately weak (P07.S06).** `SignatureWidgets` has no product caller: it is the positive
	// control D25's placement clause asks for, answering "was this block DRAWN, and where" without
	// a rasteriser. Naming a guard here would be worse than naming nothing, because this scan's
	// standing rule is that tests do not count — "a counter a test asserts and no human ever sees
	// is exactly the shape this scan exists to find". So every field is parked in `unreadKnown`
	// beneath, where it stays visible instead of passing.
	"pdfops.SignatureWidget": {"internal/pdfops/attachments.go"},
	// **`discovery.Seen` is back, and the reason it was removed dissolved on being measured
	// (/pending 284, 2026-08-27).** It was named here and had **never been discovered**: `Seen`
	// embeds `Announcement`, and `discoverObservables` treated an embed as "fields this scan
	// cannot see" and dropped the whole shape — so the entry claimed coverage it never had, which
	// this file's own `published`-key validation caught on its first run.
	//
	// It was removed rather than repaired because repairing it meant changing the DISCOVERER,
	// *"which would newly discover shapes across ten packages"*. **Counted: it discovers exactly
	// one.** Of 56 exported data shapes across all ten scanned packages, `Seen` is the only one
	// that embeds anything at all. The blast radius the fix was waiting on was one shape and one
	// field.
	//
	// The discoverer now records embeds instead of dropping the shape, and admits a shape whose
	// embedded types are themselves discovered — `Seen`'s own field is `From`, and its embedded
	// half stays covered by the `Announcement` entry below rather than being repeated here. An
	// embed whose type is NOT discovered is reported by name, because a shape that vanishes from
	// the scan reads identically to one that publishes nothing.
	"discovery.Seen":         {"internal/server/discover.go"},
	"discovery.Announcement": {"internal/discovery/announce.go", "internal/server/discover.go"},
	"p2p.Channel":            {"internal/p2p/verify.go", "internal/p2p/channel.go"},
	"p2p.SignerAttestation":  {"web/app.js", "internal/server/cosign.go", "internal/ceremony/record.go"},
	"p2p.Placement":          {"internal/p2p/cosign.go", "internal/sign/identity.go"},
	// L3's roster entry (P07.S03). Both fields are read by the predicate itself and both are
	// WRITTEN by the server, from the invitation it matched against the record at arm time —
	// which is why this is primitives rather than a `ceremony.Record`: `p2p` cannot import
	// `ceremony` (a production import cycle since P07.S02a), and it does not need to.
	//
	// **`p2p.Roster` is deliberately absent, and it is a gap rather than a decision.** This scan
	// discovers shapes RETURNED by exported functions, and `Roster` is only ever a parameter —
	// `NextContributor` returns a `RosterEntry`. So its `Commitment` field is covered by nothing
	// here. Named rather than left as a silent hole in a scan that reads as exhaustive.
	"p2p.RosterEntry": {"internal/p2p/l3.go", "internal/server/ceremonyid.go"},
	"ceremony.Record": {"internal/ceremony/record.go", "internal/server/ceremonynet.go"},
	// **Parked as "no reader yet" mid-slice, and the parking was STALE IN ITS OWN COMMIT.**
	// The convene route landed in the same slice and reads every field of both shapes, so the
	// honest entry is a reader rather than an exclusion. Found at the slice's diff review,
	// which noted the sharper half: as parked, DELETING the route would have left this scan
	// green — the exact failure the file exists to prevent.
	"ceremony.Convened": {"internal/server/convene.go"},
	// **`ceremony.Stored` is the listing's row (P08.S03), and its reader is the route.**
	// Every field is rendered: `ID` and `State` name the entry, `Reason` is the sentence a
	// degraded one shows, and `Intent`/`Expires`/`Roster` are what a panel draws for a healthy
	// one. `handleCeremonies` returns the slice verbatim, so the reader is the whole of it.
	//
	// Worth recording that this scan is what noticed the type at all: it was added, wired and
	// tested, and the guard failed the root package on the same commit — which is the shape the
	// file exists for, arriving on its own subject rather than on a regression.
	"ceremony.Stored": {"internal/ceremony/mirror.go", "internal/server/convene.go"},
	// **`ceremony.Termination` is the convener's signed end state (P08.S04b), and its reader is
	// `ReadStored`** — which folds it into `Stored.Ended` for the listing route. The object itself
	// is deliberately NOT rendered anywhere: a surface that showed the signature would invite a
	// reader to treat its ABSENCE as "still live", and this object cannot bind a convener, so
	// absence means unknown. What a user sees is the word, through `Stored.Ended`.
	//
	// Delivery to other parties is S05's, which is why this is inert today — the strongest
	// argument for the S04a/S04b cut being where it is.
	"ceremony.Termination":     {"internal/ceremony/mirror.go"},
	"vault.CeremonySecret":     {"internal/server/convene.go", "internal/vault/vault.go"},
	"ceremony.CandidateRecord": {"internal/ceremony/candidate.go", "internal/server/ceremonynet.go"},
	"ceremony.Invitation":      {"internal/ceremony/invitation.go", "internal/server/ceremonyid.go"},
	"ceremony.Party":           {"internal/ceremony/record.go", "internal/ceremony/invitation.go"},
	"ceremony.Endpoint":        {"internal/server/ceremonynet.go", "internal/ceremony/candidate.go"},
	"instance.Record":          {"internal/instance/instance.go", "internal/server/handoff.go"},
	"ots.VerifyResult":         {"internal/server/timestamp.go"},
	"sign.Status":              {"web/app.js", "internal/server/server.go"},
	"pdfops.AttachmentInfo":    {"internal/cli/commands.go", "internal/server/attachments.go", "web/app.js"},
	"pdfops.OutlineItem":       {"internal/cli/commands.go", "web/app.js"},
	"pdfops.ScanReport":        {"internal/server/scan.go", "web/app.js"},
	"pdfops.SplitPart":         {"internal/server/export.go", "internal/cli/commands.go"},
	"vault.KeyInfo":            {"internal/server/keys.go", "web/app.js"},
	"vault.PinnedPeer":         {"internal/server/peers.go", "internal/vault/vault.go"},
	"vault.Settings":           {"internal/server/settings.go", "internal/vault/vault.go"},
	"vault.Image":              {"internal/server/images.go", "internal/vault/vault.go"},
	"vault.ExternalSigner":     {"internal/server/keys.go", "internal/vault/vault.go"},
	"vault.Slot":               {"internal/vault/vault.go", "internal/server/keys.go"},
}

// excluded shapes, each with its reason. An UNEXPLAINED entry here is how a genuinely
// unread observable gets parked and forgotten, which is the failure this file exists for.
var excluded = map[string]string{}

// unreadKnown are fields this scan found published and NOT read, kept VISIBLE rather than
// excluded. An exclusion says "not this scan's business"; these are exactly its business —
// each is a `historyEvicted` — so they carry a reason and, where there is one, the follow-up.
// Deleting an entry is how one gets fixed; a NEW unread field cannot be parked without
// somebody writing a line, which is the intended cost.
var unreadKnown = map[string]string{
	// **`pdfops.SignatureWidget`, P07.S06, and it is entered here the day it is written.**
	//
	// `SignatureWidgets` is the positive control D25's placement clause asks for: it answers "was
	// this block DRAWN, and where" without a rasteriser, because a check on placement ARITHMETIC
	// cannot distinguish "placed correctly" from "not placed at all" — both leave a valid document
	// and sign.Verify reports an invisible signature exactly as it reports a visible one.
	//
	// Its only reader is a GUARD, and this scan's own standing rule is that tests do not count:
	// "a counter a test asserts and no human ever sees is exactly the shape this scan exists to
	// find". That rule is right and it is not suspended here — so rather than claim a reader, the
	// three fields are parked visibly. The honest state is that nothing in the product surfaces
	// where a block was drawn.
	//
	// Deleting these is what a UI that reports "your block is on page 6 of this document" does,
	// and P07.S07 is the slice that renders block content.
	"pdfops.SignatureWidget.Page":  "read by the P07.S06 placement guard only; no product surface reports where a block landed",
	"pdfops.SignatureWidget.Rect":  "read by the P07.S06 placement guard only; no product surface reports where a block landed",
	"pdfops.SignatureWidget.HasAP": "read by the P07.S06 placement guard only; nothing tells a user their block is blank",

	// The node-cache trio. DELIBERATELY unprinted, and `internal/cli/rendezvous.go` says so
	// at the line: this command uses a SCRATCH directory, so Loaded is always 0 and
	// CacheRejected always false, and "printing them as if they were findings reads as a
	// broken cache where there is none"; Saved is written by the deferred Close AFTER the
	// output. What the command prints instead is `realCacheLine()` — the cache a real
	// ceremony will use.
	//
	// Named here rather than excluded, because "no production reader" is still true of them
	// and the day a non-scratch caller appears they should be printed. Their only readers
	// are TESTS, and tests deliberately do not count: `historyEvicted` was "set, serialized,
	// and asserted by six Go tests, and nothing read it". A counter a test asserts and no
	// human ever sees is exactly the shape this scan exists to find.
	"rendezvous.Stats.Loaded":        "deliberate — see internal/cli/rendezvous.go's scratch-directory comment",
	"rendezvous.Stats.Saved":         "deliberate — written by the deferred Close, after the output is printed",
	"rendezvous.Stats.CacheRejected": "deliberate — always false against a scratch directory",

	// **The termination object's three verification fields, parked visibly (P08.S04b).**
	//
	// `Version`, `ConvenerCert` and `Sig` are consumed by `Termination.Verify` — which is in the
	// DEFINING package, and this scan's own standing rule is that a field consumed by the code
	// that sets it has no consumer at the far end. That rule is right, and naming
	// `termination.go` as their reader would be exactly the laundering it exists to catch.
	//
	// **Their real far end is another party's machine, and S05 is the slice that puts them there.**
	// Until the delivery round exists this object never leaves the convener's disk, so the honest
	// state is: written, verifiable, and read by nobody who did not write it. `State` alone has a
	// reader today, through `Stored.Ended`.
	//
	// Delete these three when S05 delivers a termination and a receiving party verifies one — that
	// is the moment the fields acquire the consumer they were designed for.
	"ceremony.Termination.Version":      "no far-end reader until S05 delivers the object; Verify is in the defining package",
	"ceremony.Termination.ConvenerCert": "no far-end reader until S05 delivers the object; Verify is in the defining package",
	"ceremony.Termination.Sig":          "no far-end reader until S05 delivers the object; Verify is in the defining package",

	// **Parked HONESTLY rather than passing silently (2026-08-24, P07.S02).** This scan's
	// declared readers for `ceremony.Party` are `record.go` and `invitation.go` — both inside
	// the DEFINING package — so any new field satisfies it the moment the producer mentions
	// it once. Measured at the P07.S02 grill: `Capacity` added to `Party` and to
	// `rosterPreimage` passes this scan with **no display reader anywhere**.
	//
	// So `Capacity` is entered here deliberately. It IS published, it IS committed, and
	// nothing renders it yet — which is the true state, and it would otherwise have shipped
	// as a green. Deleting this line is what P07.S07 does when a signature block renders it
	// (C19). The same limitation applies to `Record`/`Invitation` and is stated in the
	// package doc below rather than repeated per field.
	"ceremony.Party.Capacity": "published and committed at P07.S02; the block renderer that " +
		"displays it is P07.S07 (C19). Delete this line then.",
}

func TestEveryPublishedObservableHasANamedReader(t *testing.T) {
	shapes := discoverObservables(t)

	// STIMULUS, and it is not decoration: without it every check below is equally true of a
	// scan that discovered nothing — which is how a guard reports full coverage of zero.
	if len(shapes) < 20 {
		t.Fatalf("discovered %d published observables; this tree has at least twenty across "+
			"%d packages. The discovery is broken, so everything below passes on almost "+
			"nothing.", len(shapes), len(observablePackages))
	}

	readerCache := map[string]string{}
	readerSrc := func(t *testing.T, p string) string {
		if s, ok := readerCache[p]; ok {
			return s
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("reader %s named by the table does not exist: %v", p, err)
		}
		s := codeOnly(string(b))
		readerCache[p] = s
		return s
	}

	for _, name := range sortedKeys(shapes) {
		sh := shapes[name]
		if why, ok := excluded[name]; ok {
			if why == "" {
				t.Errorf("%s is excluded with no reason. An unexplained exclusion is how a "+
					"real unread observable gets parked and forgotten.", name)
			}
			continue
		}
		readers, ok := published[name]
		if !ok {
			// **The half that closes a CLASS rather than an instance.** A shape nobody
			// entered is invisible to a walk over entries — the P05 lesson, and the reason
			// the population above is discovered rather than listed.
			t.Errorf("%s (%s, %d field(s)) is published by an exported function and is in "+
				"neither the table nor the exclusions. Every observable that leaves its "+
				"package needs a named reader, or it is a field somebody set and nobody "+
				"was ever told.", name, sh.file, len(sh.fields))
			continue
		}
		for _, f := range sh.fields {
			if _, known := unreadKnown[name+"."+f]; known {
				continue
			}
			found := false
			for _, r := range readers {
				src := readerSrc(t, r)
				if strings.Contains(src, "."+f) {
					found = true
					break
				}
				if jt := sh.tag[f]; jt != "" && strings.Contains(src, "."+jt) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s.%s (%s) is published and no named reader mentions it: %v. "+
					"A produced-and-never-consumed field is not a nit — the whole point of "+
					"publishing it is that somebody is told.", name, f, sh.file, readers)
			}
		}
	}

	// An entry in unreadKnown for a field that no longer exists silently covers the next
	// field given that name. Same rule as the transport table's exemptions.
	for k := range unreadKnown {
		i := strings.LastIndex(k, ".")
		if i < 0 {
			t.Errorf("unreadKnown key %q is not <type>.<field>", k)
			continue
		}
		sh, ok := shapes[k[:i]]
		if !ok {
			t.Errorf("unreadKnown names %q but %s is no longer a published observable", k, k[:i])
			continue
		}
		if !contains(sh.fields, k[i+1:]) {
			t.Errorf("unreadKnown names %q but that field no longer exists. A parked entry "+
				"for a deleted field is a hole waiting for the next field of that name.", k)
		}
	}
	// The same, one level up, for exclusions.
	for k := range excluded {
		if _, ok := shapes[k]; !ok {
			t.Errorf("%s is excluded but is no longer a published observable", k)
		}
	}
	// **And for `published` itself, which was the one table nothing validated (2026-08-24,
	// P07.S02).** `unreadKnown` and `excluded` are both checked against the discovered set;
	// `published` was not, so a shape that stopped being discovered simply vanished from the
	// scan. Measured at the grill: adding ONE UNEXPORTED field to `ceremony.Party` drops it
	// out of discoverObservables entirely — the count went 26 → 25 and the run PASSED, with
	// nothing anywhere saying `ceremony.Party` was no longer covered.
	//
	// The stimulus floor below is `len(shapes) < 20`, so six shapes could have disappeared
	// before anything noticed. A coverage gate that silently stops covering something is the
	// vacuous green applied to itself.
	for name := range published {
		if _, ok := shapes[name]; !ok {
			t.Errorf("`published` names %q and it was NOT discovered as a published observable. "+
				"Either the type was renamed or deleted — in which case remove the entry — or it "+
				"stopped being discoverable (an unexported field will do it), in which case this "+
				"scan has quietly stopped covering a shape it claims to cover.", name)
		}
	}
	t.Logf("scanned %d published observables across %d packages", len(shapes), len(observablePackages))
}

type observable struct {
	file   string
	fields []string
	// tag is the json name per field, where it has one. A JS reader sees `oneProceeding`,
	// never `OneProceeding`, so a scan that only ever looked for the Go name would report
	// every client-rendered field as an orphan — and the reader would learn to ignore it.
	tag map[string]string
}

// discoverObservables finds every exported struct that (a) has at least one exported field,
// (b) has NO unexported fields, and (c) is returned by an exported function or method.
//
// (b) is what separates an observable from a HANDLE. `udpmux.Mux`, `rendezvous.Server`,
// `p2p.Conn` and `vault.Vault` are all returned by exported constructors and none of them is
// a published fact — they are objects with methods, carrying unexported state. A struct whose
// every field is exported is carrying data outward, which is the thing that can be published
// and unread.
func discoverObservables(t *testing.T) map[string]observable {
	t.Helper()
	out := map[string]observable{}
	for _, pkg := range observablePackages {
		files, err := filepath.Glob(filepath.Join(pkg, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		type cand struct {
			file   string
			fields []string
			tag    map[string]string
			// embeds names each embedded type. An embed carries fields this scan cannot
			// attribute to THIS shape; what it must not do is make the shape vanish.
			embeds []string
		}
		cands := map[string]cand{}
		returned := map[string]bool{}
		short := pkg[strings.LastIndex(pkg, "/")+1:]
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			af, perr := parser.ParseFile(fset, f, nil, 0)
			if perr != nil {
				t.Fatal(perr)
			}
			ast.Inspect(af, func(n ast.Node) bool {
				switch d := n.(type) {
				case *ast.TypeSpec:
					st, ok := d.Type.(*ast.StructType)
					if !ok || !d.Name.IsExported() || st.Fields == nil {
						return true
					}
					var fields []string
					var embeds []string
					tags := map[string]string{}
					unexported := false
					for _, fl := range st.Fields.List {
						if len(fl.Names) == 0 {
							// **An embed is RECORDED, not a reason to drop the shape (/pending 284).**
							//
							// This used to set `unexported`, which discarded the whole struct: an
							// embed carries fields the scan cannot attribute here, so the shape was
							// treated as unscannable. `discovery.Seen` embeds `Announcement`, so it
							// had never been discovered at all, and the `published` entry naming its
							// readers claimed coverage it never had.
							//
							// The entry filed against this asked how many OTHER shapes embed and said
							// nobody had counted. Counted: across all ten scanned packages, of 56
							// exported data shapes, **exactly one embeds** — `Seen` — and the type it
							// embeds is itself discovered. So the blast radius the entry was waiting
							// on is one shape and one field.
							embeds = append(embeds, embedName(fl.Type))
							continue
						}
						jt := ""
						if fl.Tag != nil {
							if k := strings.Index(fl.Tag.Value, `json:"`); k >= 0 {
								rest := fl.Tag.Value[k+6:]
								jt = rest[:strings.IndexAny(rest+`"`, `",`)]
							}
						}
						for _, nm := range fl.Names {
							if nm.IsExported() {
								fields = append(fields, nm.Name)
								if jt != "" && jt != "-" {
									tags[nm.Name] = jt
								}
							} else {
								unexported = true
							}
						}
					}
					if len(fields) > 0 && !unexported {
						cands[d.Name.Name] = cand{file: f, fields: fields, tag: tags, embeds: embeds}
					}
				case *ast.FuncDecl:
					if !d.Name.IsExported() || d.Type.Results == nil {
						return true
					}
					for _, r := range d.Type.Results.List {
						for _, id := range typeIdents(r.Type) {
							returned[id] = true
						}
					}
				}
				return true
			})
		}
		for name, c := range cands {
			if !returned[name] {
				continue
			}
			// **An embed is fine when the embedded type is ITSELF covered, and a finding when it
			// is not.** `Seen`'s own field is `From`; its embedded half is covered through the
			// separate `discovery.Announcement` entry, so walking into the embed here would make
			// `Seen`'s reader repeat every field `Announcement` already requires.
			//
			// What must not happen is the old behaviour — silently dropping the shape — because a
			// shape that vanishes from the scan takes its coverage with it and looks exactly like
			// a shape that was never published. So an embed whose type is not discovered is
			// reported BY NAME rather than dropped quietly.
			missing := ""
			for _, e := range c.embeds {
				if _, known := cands[e]; !known {
					missing = e
					break
				}
			}
			if missing != "" {
				t.Errorf("%s.%s embeds %s, which this scan does not discover — so %s's fields are "+
					"covered by nothing and the shape would once have been dropped whole, which "+
					"reads identically to a shape that publishes nothing. Give %s an entry, or "+
					"record here why it cannot have one.", short, name, missing, missing, missing)
				continue
			}
			out[short+"."+name] = observable{file: c.file, fields: c.fields, tag: c.tag}
		}
	}
	return out
}

// embedName is the bare type name of an embedded field — `Announcement`, `p2p.Channel`'s
// `Channel`, `*Foo`'s `Foo`. Only the name is needed: the question is whether the scan discovered
// a shape by that name in the same package.
func embedName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return embedName(v.X)
	case *ast.SelectorExpr:
		return v.Sel.Name
	}
	return "?"
}

// typeIdents pulls the bare type names out of a result expression, through pointers, slices,
// maps and arrays — a `map[string]Invitation` publishes Invitation just as a bare return does.
func typeIdents(e ast.Expr) []string {
	switch v := e.(type) {
	case *ast.Ident:
		return []string{v.Name}
	case *ast.StarExpr:
		return typeIdents(v.X)
	case *ast.ArrayType:
		return typeIdents(v.Elt)
	case *ast.MapType:
		return append(typeIdents(v.Key), typeIdents(v.Value)...)
	case *ast.SelectorExpr:
		return []string{v.Sel.Name}
	}
	return nil
}

// codeOnly strips comments before anything looks for a read.
//
// **A mention is not a read**, and without this the check is satisfied by prose ABOUT the
// field. Measured on the JS side: `Record.version` was declared read, the read was deleted,
// and the scan still passed — because a doc comment said "see Record.Version". Two guards in
// this repo have now had that hole; this one is born without it.
func codeOnly(src string) string {
	var b strings.Builder
	inBlock := false
	for _, line := range strings.Split(src, "\n") {
		for inBlock {
			i := strings.Index(line, "*/")
			if i < 0 {
				line = ""
				break
			}
			line = line[i+2:]
			inBlock = false
		}
		if i := strings.Index(line, "/*"); i >= 0 {
			if j := strings.Index(line[i:], "*/"); j >= 0 {
				line = line[:i] + line[i+j+2:]
			} else {
				line = line[:i]
				inBlock = true
			}
		}
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func sortedKeys(m map[string]observable) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
