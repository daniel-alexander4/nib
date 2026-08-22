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
	"udpmux.Stats":             {"internal/cli/rendezvous.go", "internal/udpmux/mux.go"},
	"rendezvous.Stats":         {"internal/cli/rendezvous.go", "internal/rendezvous/dht.go"},
	"rendezvous.SelfAddress":   {"internal/rendezvous/selfaddr.go", "internal/cli/rendezvous.go"},
	"discovery.Stats":          {"internal/cli/discover.go"},
	"discovery.Seen":           {"internal/cli/discover.go", "internal/server/discover.go"},
	"discovery.Announcement":   {"internal/discovery/announce.go", "internal/server/discover.go"},
	"p2p.Channel":              {"internal/p2p/verify.go", "internal/p2p/channel.go"},
	"p2p.SignerAttestation":    {"web/app.js", "internal/server/cosign.go", "internal/ceremony/record.go"},
	"p2p.Placement":            {"internal/p2p/cosign.go", "internal/sign/identity.go"},
	"ceremony.Record":          {"internal/ceremony/record.go", "internal/server/ceremonynet.go"},
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
	"ceremony.Party.Name": "PUBLISHED AND READ BY NOBODY — and written by nobody either. " +
		"This scan's first find, on its first run. `Party.Name` is the six-word display name " +
		"(D3), declared, JSON-serialized and documented at length; the only occurrence of the " +
		"identifier in internal/ceremony is inside a comment, which codeOnly strips, and the " +
		"one production roster builder (internal/cli/rendezvous.go) does not set it. " +
		"It matters because P01's D21 criterion requires \"an invitation whose name and " +
		"fingerprint disagree\" to be REFUSED, and MatchesRecord compares Fingerprint and " +
		"Signs and never Name — so nothing can refuse it. The hazard is latent only because " +
		"no name is ever set; it opens the moment P07 populates one for display. " +
		"Two fixes, and they differ in what D21's criterion can be driven against, so the " +
		"choice is Dan's and is parked rather than guessed: DELETE the field (it is a pure " +
		"function of Fingerprint, so storing it is a second statement of one fact that can " +
		"disagree — and it sits outside every preimage, so nothing signed moves), or POPULATE " +
		"it at the door and check it with pairing.Matches. Filed as its own pending item.",

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
					tags := map[string]string{}
					unexported := false
					for _, fl := range st.Fields.List {
						if len(fl.Names) == 0 {
							unexported = true // an embed carries fields this scan cannot see
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
						cands[d.Name.Name] = cand{file: f, fields: fields, tag: tags}
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
			if returned[name] {
				out[short+"."+name] = observable{file: c.file, fields: c.fields, tag: c.tag}
			}
		}
	}
	return out
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
