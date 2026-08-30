package p2p

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The refusal enumeration, DERIVED from source rather than re-typed — /pending 315.
//
// # What was wrong
//
// The wire refusal codes had two "every code" tests and both were hand-written literal slices:
// `refusalwire_test.go`'s `all := []error{…}` and `internal/server/l3_test.go`'s twin. A slice
// that lists what it checks derives nothing: a sentinel omitted from the list is a sentinel the
// test never asks about, and the omission is invisible because the test still passes. The codes
// are an **append-only wire enumeration** — `session.go`'s const block says "Append only, and
// never renumber" — so this is a format with a hand-maintained conformance check, the shape
// ADR-009 exists against.
//
// It was not hypothetical. The grill that produced this guard ran its sentinel-coverage check by
// hand against HEAD and found three sentinels reaching the initiator as bare EOF:
// `ErrNoSignaturePages`, `ErrBlockOffThePage` and `ErrNoCeremonyIntent`, all raised inside
// `coSignExchange`, all with no code, all rendered as
// `502 co-signing did not complete: receive co-signed document: EOF` — a network fault inviting
// the retry a refusal must not invite. That is the P07.S03a defect, surviving in the three
// sentinels nobody had enumerated.
//
// # What it derives, and the one thing it deliberately does not
//
// The population comes from the **const block** — a third artifact, not from either switch, so a
// code missing from both is still visible. `wirePin` below is the exception and is hand-written
// ON PURPOSE: these are frozen wire values, so the map is a *record*, not maintenance. Nothing
// else can catch a renumbering — a build that reads code 3 as a new meaning keeps both switches
// symmetric, both lists valid and every other assertion green, while breaking the wire against
// every deployed peer.
//
// Modelled on `deadlines_test.go` and `listenercore_test.go`: discover the population, assert a
// floor whose Fatal names the blindness, then assert the rule.
func TestTheRefusalEnumerationIsDerivedFromSource(t *testing.T) {
	// wirePin is the frozen wire. **Adding a line here is the deliberate act of freezing a new
	// value**, which is why the guard requires it rather than deriving it: a value that a future
	// build may change is not a wire value at all.
	//
	// The identifiers are referenced, not quoted, so a rename fails to COMPILE this guard instead
	// of silently emptying its population — the failure `listenercore_test.go` guards against with
	// its discriminator.
	wirePin := map[string]int{
		"refuseNotYourTurn":        refuseNotYourTurn,
		"refuseNotInRoster":        refuseNotInRoster,
		"refusePrefixMismatch":     refusePrefixMismatch,
		"refusePrefixUnproven":     refusePrefixUnproven,
		"refuseProceedingMismatch": refuseProceedingMismatch,
		"refuseCeremonyComplete":   refuseCeremonyComplete,
		"refuseNotConnectedPeer":   refuseNotConnectedPeer,
		"refusePeerDoesNotAccept":  refusePeerDoesNotAccept,
		"refusePriorSignerCount":   refusePriorSignerCount,
		"refuseNoSignaturePages":   refuseNoSignaturePages,
		"refuseBlockOffThePage":    refuseBlockOffThePage,
		"refuseNoCeremonyIntent":   refuseNoCeremonyIntent,
		"refuseCeremonyEnded":      refuseCeremonyEnded,
		"refuseRosterMismatch":     refuseRosterMismatch,
	}
	// And the values they are pinned TO, written out separately from the identifiers above. The
	// two halves are compared below: the map above proves the names still exist, this one proves
	// the numbers have not moved.
	frozen := map[string]int{
		"refuseNotYourTurn": 1, "refuseNotInRoster": 2, "refusePrefixMismatch": 3,
		"refusePrefixUnproven": 4, "refuseProceedingMismatch": 5, "refuseCeremonyComplete": 6,
		"refuseNotConnectedPeer": 7, "refusePeerDoesNotAccept": 8, "refusePriorSignerCount": 9,
		"refuseNoSignaturePages": 10, "refuseBlockOffThePage": 11, "refuseNoCeremonyIntent": 12,
		// Frozen 2026-08-30 (P08.S04a). 14 is the older defect of the two: C17's roster mismatch
		// has reached the initiator as bare EOF since P07, and 13's new gate would otherwise have
		// shipped over it.
		"refuseCeremonyEnded": 13, "refuseRosterMismatch": 14,
	}

	fset := token.NewFileSet()
	src := parsePackage(t, fset)

	// ── The population: every `refuse*` constant, read from the const block ──────────────────
	codes := map[string]int{}
	for _, f := range src {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, s := range gd.Specs {
				vs, ok := s.(*ast.ValueSpec)
				if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
					continue
				}
				name := vs.Names[0].Name
				if !strings.HasPrefix(name, "refuse") {
					continue
				}
				lit, ok := vs.Values[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.INT {
					t.Fatalf("%s is not an integer literal — a refusal code computed from "+
						"something else is a wire value this guard cannot pin", name)
				}
				n, err := strconv.Atoi(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				codes[name] = n
			}
		}
	}
	// STIMULUS. A scan that read no const block agrees with every assertion below, because
	// comparing two empty sets always succeeds. The enumeration is append-only, so its size can
	// never legitimately shrink.
	if len(codes) < 14 {
		t.Fatalf("setup: found only %d refuse* constants — the const block has been renamed, "+
			"moved, or this scan is not reading it, and every check below is comparing empty "+
			"sets", len(codes))
	}

	// ── The anchor: what the AST read must equal what the compiler compiled ──────────────────
	for name, want := range wirePin {
		got, ok := codes[name]
		if !ok {
			t.Errorf("%s is pinned as a wire value but no longer exists in the const block", name)
			continue
		}
		if got != want {
			t.Errorf("this guard read %s = %d from source but the compiler says %d — the scan is "+
				"not reading the constants it thinks it is", name, got, want)
		}
	}
	// ── The freeze: "never renumber" ─────────────────────────────────────────────────────────
	for name, n := range codes {
		want, pinned := frozen[name]
		if !pinned {
			t.Errorf("%s = %d is a new refusal code and is not in this guard's frozen table. A "+
				"code is a value two builds must agree on: add it to `frozen` (and to `wirePin`) "+
				"as a deliberate freeze, and never reuse a retired number.", name, n)
			continue
		}
		if n != want {
			t.Errorf("%s is %d and the wire pin says %d. The const block's own doc says "+
				"\"Append only, and never renumber\" — a peer on the shipped build reads %d as "+
				"this refusal and %d as something else, and every other test in this package "+
				"stays green either way.", name, n, want, want, n)
		}
	}
	// Append-only in the other direction: no two codes share a number, and 0 is reserved for
	// "this error has no wire code" by `refusalCode`'s own contract.
	seen := map[int]string{}
	for _, name := range sortedKeys(codes) {
		n := codes[name]
		if n == 0 {
			t.Errorf("%s is 0, which refusalCode returns for an error that has NO code — a "+
				"sentinel mapped to it would be indistinguishable from one that is not a "+
				"refusal at all", name)
		}
		if prev, dup := seen[n]; dup {
			t.Errorf("%s and %s are both %d: one wire value, two meanings", prev, name, n)
		}
		seen[n] = name
	}

	// ── The two switches, read from source ───────────────────────────────────────────────────
	enc := switchPairs(t, src, "refusalCode")   // sentinel name -> code name
	dec := switchPairs(t, src, "errorForCode")  // code name -> sentinel name
	ack := switchSubjects(t, src, "refusalAck") // sentinel names reaching an ack byte directly

	// STIMULUS, and it is a BLINDNESS floor rather than a count.
	//
	// It was written as `< 12` first, and the red proof caught that: deleting one `refusalCode`
	// case — the live defect this guard exists for — tripped the floor and reported "the switch
	// walker has gone blind", masking the two assertions that name the actual sentinel. A floor
	// that fires on the condition the guard is for is a dead conjunct in front of the real check.
	// So this asks only whether the walker read anything at all; the per-code assertions below
	// say which code, and they are what must fire.
	if len(enc) == 0 || len(dec) == 0 {
		t.Fatalf("setup: refusalCode has %d case(s) and errorForCode %d — the switch walker has "+
			"gone blind and the symmetry check below means nothing", len(enc), len(dec))
	}

	// Every code is encodable, and every code is decodable.
	for name := range codes {
		if !containsValue(enc, name) {
			t.Errorf("%s is a refusal code that refusalCode can never RETURN. Nothing can emit "+
				"it, so it is a number reserved against a meaning no build produces.", name)
		}
		if _, ok := dec[name]; !ok {
			t.Errorf("%s is a refusal code that errorForCode does not decode. A peer emitting it "+
				"is told by this build's own initiator that the reason is one \"this version of "+
				"Nib does not know\" — about a code this version defines.", name)
		}
	}
	// The converse, which is what catches a bare literal or a constant from outside the block.
	for codeName := range dec {
		if _, ok := codes[codeName]; !ok {
			t.Errorf("errorForCode decodes %q, which is not a member of the refusal-code const "+
				"block — the encoder can never produce it, so this build decodes a value its own "+
				"enumeration does not define", codeName)
		}
	}
	for _, codeName := range enc {
		if _, ok := codes[codeName]; !ok {
			t.Errorf("refusalCode returns %q, which is not a member of the refusal-code const "+
				"block", codeName)
		}
	}
	// And the pairing: the two directions must agree about WHICH sentinel a code means.
	for sentinel, codeName := range enc {
		if back := dec[codeName]; back != sentinel {
			t.Errorf("refusalCode maps %s -> %s but errorForCode maps %s -> %s. One code, two "+
				"meanings: the refusing side names one thing and the initiator prints another.",
				sentinel, codeName, codeName, back)
		}
	}

	// ── Executable, not merely textual ───────────────────────────────────────────────────────
	// The checks above compare text to text. This one drives the derived population through the
	// live functions, so a switch that parses correctly and behaves differently is still caught.
	for name, n := range codes {
		if n < 0 || n > 255 {
			t.Errorf("%s = %d does not fit the single code byte the wire carries", name, n)
			continue
		}
		err := errorForCode(byte(n))
		if got := refusalCode(err); got != byte(n) {
			t.Errorf("code %d (%s) decodes to an error that re-encodes as %d — the round trip "+
				"through the live functions does not close", n, name, got)
		}
	}

	// ── Sentinel coverage: the check that found the live defect ──────────────────────────────
	//
	// Every exported error sentinel in this package either reaches the peer — through
	// `refusalCode`, or directly through one of `refusalAck`'s own ack-byte cases — or says at
	// its declaration why it does not. The exemption is a line, not a list kept elsewhere, so
	// the reason sits where the next reader of the sentinel will see it.
	sentinels := errSentinels(t, src)
	if len(sentinels) < 22 {
		t.Fatalf("setup: found only %d exported Err* sentinels in this package — the var scan is "+
			"blind and the coverage check below is vacuous", len(sentinels))
	}
	for _, s := range sentinels {
		if _, coded := enc[s.name]; coded {
			continue
		}
		if ack[s.name] {
			continue
		}
		if strings.Contains(s.doc, "No wire code:") {
			continue
		}
		t.Errorf("%s (%s) has no wire code, is not named in refusalAck, and carries no "+
			"\"No wire code:\" line saying why.\n"+
			"If it can be returned on the receive path, refusalAck writes NOTHING and the "+
			"initiator's readFrame gets bare EOF — rendered as \"502 co-signing did not complete: "+
			"receive co-signed document: EOF\", a network fault inviting the retry a refusal must "+
			"not invite. If it cannot reach the peer, say so at the declaration with a "+
			"\"No wire code: <reason>\" line.", s.name, s.pos)
	}

	t.Logf("refusal enumeration: %d code(s), %d encode case(s), %d decode case(s), "+
		"%d sentinel(s) checked", len(codes), len(enc), len(dec), len(sentinels))
}

// parsePackage parses every non-test source file of this package. The population is DISCOVERED,
// never listed — `goroutines_test.go`'s rule, for its reason: a file added later is invisible
// exactly the way a listed one is not.
func parsePackage(t *testing.T, fset *token.FileSet) []*ast.File {
	t.Helper()
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var out []*ast.File
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, filepath.Base(n), nil, parser.ParseComments)
		if perr != nil {
			t.Fatalf("%s: %v", n, perr)
		}
		out = append(out, f)
	}
	if len(out) < 5 {
		t.Fatalf("setup: parsed only %d source file(s) in this package", len(out))
	}
	return out
}

// funcBodyAST finds a top-level function by name across the parsed files.
func funcBodyAST(t *testing.T, src []*ast.File, name string) *ast.BlockStmt {
	t.Helper()
	for _, f := range src {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if ok && fd.Recv == nil && fd.Name.Name == name && fd.Body != nil {
				return fd.Body
			}
		}
	}
	t.Fatalf("setup: %s not found — this guard is pinned to a function that no longer exists", name)
	return nil
}

// switchPairs reads a `switch` whose every case maps one identifier to one returned identifier.
// It handles both shapes in play: `switch { case errors.Is(err, ErrX): return codeY }` and
// `switch code { case codeY: return ErrX }`. The returned map is keyed by the case's subject.
func switchPairs(t *testing.T, src []*ast.File, fn string) map[string]string {
	t.Helper()
	out := map[string]string{}
	ast.Inspect(funcBodyAST(t, src, fn), func(n ast.Node) bool {
		cc, ok := n.(*ast.CaseClause)
		if !ok || len(cc.List) == 0 || len(cc.Body) == 0 {
			return true
		}
		ret, ok := cc.Body[0].(*ast.ReturnStmt)
		if !ok || len(ret.Results) == 0 {
			return true
		}
		id, ok := ret.Results[0].(*ast.Ident)
		if !ok {
			return true // the ErrRefusedUnknown fallthrough is a fmt.Errorf, not an ident
		}
		for _, e := range cc.List {
			if s := caseSubject(e); s != "" {
				out[s] = id.Name
			}
		}
		return true
	})
	return out
}

// switchSubjects reads only the case subjects of a switch — used for `refusalAck`, whose cases
// return a composite literal rather than a bare identifier, so `switchPairs` cannot read them.
func switchSubjects(t *testing.T, src []*ast.File, fn string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	ast.Inspect(funcBodyAST(t, src, fn), func(n ast.Node) bool {
		cc, ok := n.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, e := range cc.List {
			if s := caseSubject(e); s != "" {
				out[s] = true
			}
		}
		return true
	})
	return out
}

// caseSubject returns the identifier a case clause is about: the second argument of
// `errors.Is(err, X)`, or a bare identifier for a tagged switch.
//
// **A literal is reported as its own text rather than skipped**, and that is the whole reason
// this function has a BasicLit arm. The first draft returned "" for anything that was not an
// identifier, so `case 42:` in `errorForCode` was invisible — and the red proof written to catch
// exactly that (a decode case for a code the const block does not define) passed GREEN. Returning
// the literal makes it fail the const-block membership check below with the number in the message,
// which is the answer the reader needs.
func caseSubject(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.BasicLit:
		return v.Value
	case *ast.CallExpr:
		sel, ok := v.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Is" || len(v.Args) != 2 {
			return ""
		}
		switch a := v.Args[1].(type) {
		case *ast.Ident:
			return a.Name
		case *ast.BasicLit:
			return a.Value
		}
	}
	return ""
}

type sentinel struct{ name, doc, pos string }

// errSentinels finds every package-level `Err*` variable and the doc comment attached to it,
// whether it is declared alone or inside a grouped var block.
func errSentinels(t *testing.T, src []*ast.File) []sentinel {
	t.Helper()
	var out []sentinel
	for _, f := range src {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, s := range gd.Specs {
				vs, ok := s.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, n := range vs.Names {
					if !strings.HasPrefix(n.Name, "Err") {
						continue
					}
					// The group's own doc is used ONLY for a single-spec declaration. A
					// grouped block's doc comment describes the block, so reading it per
					// member would let one "No wire code:" line at the top exempt every
					// sentinel underneath it — an exemption nobody wrote for the sentinel
					// it would silently cover.
					doc := ""
					if vs.Doc != nil {
						doc = vs.Doc.Text()
					} else if gd.Doc != nil && !gd.Lparen.IsValid() {
						doc = gd.Doc.Text()
					}
					out = append(out, sentinel{name: n.Name, doc: doc, pos: f.Name.Name})
				}
			}
		}
	}
	return out
}

func containsValue(m map[string]string, v string) bool {
	for _, got := range m {
		if got == v {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
