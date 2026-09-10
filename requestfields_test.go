package nib

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// requestField is one exemption row: a form or query field a handler reads that no client sends.
type requestField struct {
	category string // harnessOnly | cliOnly | productGap
	reason   string
}

const (
	harnessOnly = "harness-only"
	cliOnly     = "cli-only"
	productGap  = "product-gap"
	// sharedHelper is a FOURTH category the deepdive's design did not have, and it exists because
	// this scan follows helpers where that count did not. A field can be read inside one function
	// serving several routes — `runHopDial` reads `transport` for both `/api/ceremony/hop` and
	// `/api/session/initiate` — so it is genuinely sent on one route and genuinely not on another,
	// and neither of the first three categories says that truthfully. Its reason must NAME the
	// sibling route, which is what keeps it a claim rather than a shrug, and it is deliberately
	// left uncapped: the cap's job is to stop `product-gap` growing quietly, and a shared read is
	// not a gap in the product.
	sharedHelper = "shared-helper"
)

// productGapCap is what stops this class growing quietly, and it is the whole reason the rows carry
// a CATEGORY rather than just a reason (/pending 447).
//
// A harness-only or cli-only field is a caller that genuinely exists and can be named. A
// `product-gap` is a field the product was supposed to send and does not — which is exactly
// `invitation`, the defect this guard was written after. Naming a harness sender alone would NOT
// have caught it: `-F invitation=` appears in the harness seven times, so it would have passed as
// harness-only forever. The cap is the part that makes a third one a decision rather than a row.
const productGapCap = 2

// TestEveryRequestFieldAHandlerReadsIsOneSomeClientSends — /pending 447.
//
// # The finding was a FALSE CLAIM, not a missing test
//
// `internal/server/cosign.go` said of adding an `invitation` parameter: *"Adding the parameter here
// now would be a field no client fills, which is the shape this repo's reader scans exist to
// refuse."* **No scan refused it.** `observables_test.go` cannot see a multipart form field at all —
// there is no Go struct for it to discover — and for the JSON request shapes its matcher passes on
// coincidental name collisions: `armRequest.Transport` on `ceremony.TransportQUIC`, and
// `armRequest.Address` on `st.address` in web/app.js, which is the arm RESPONSE's address, on the
// same route, travelling the other way. The author knew the rule, cited the enforcement, and the
// enforcement was absent.
//
// # Why it is (route, field) and not field
//
// Because `r.FormValue` reads the QUERY STRING too, and this client uses both carriers — `?method=`,
// `?engine=gs`, `?overwrite=1`, `?format=` — and sends `overwrite` and `format` as query on one
// route and as form appends on another. A carrier-blind, route-blind scan was measured at a **44%
// false-positive rate**, 4 flags of 9. Route-correlated, the report is three rows.
//
// # What it already caught, before it was finished
//
// `/api/pages` `size` and `color`, on a tree where /pending 448 was marked closed. That item added
// both keys to `pageNumGo`'s options object and stopped there; `pageOp` appends a named list and
// dropped them, so every page-number stamp stayed 11pt black and 448's own guard was satisfied one
// function short of the wire. Fixed at v1.128.95.
func TestEveryRequestFieldAHandlerReadsIsOneSomeClientSends(t *testing.T) {
	declared := map[string]requestField{
		"/api/session/send transport": {harnessOnly,
			"build/pairrepro.sh passes `-F transport=` so a run can force one transport. ADR-010's " +
				"own lesson is three lines below it in that file: the harness used to pass it to " +
				"BOTH sides, which configured tier 4 past the disagreement it exists to find."},
		"/api/session/initiate invitation": {cliOnly,
			"`invitationRoster` for the CLI and harness callers that do send one — build/pairrepro.sh " +
				"passes `-F invitation=`. The web client posts none, and /pending 436 closed by " +
				"giving the convener a dial that does not need one rather than by adding the field."},
		"/api/session/initiate transport": {harnessOnly,
			"build/pairrepro.sh line 1062 passes `-F transport=$transport` on this route, so a run " +
				"can force TCP or QUIC. The web client never does — it has no reason to choose."},
		"/api/ceremony/hop transport": {sharedHelper,
			"read by `runHopDial`, which serves this route AND /api/session/initiate. The harness " +
				"passes it on that sibling and deliberately not here — build/ceremonyrepro.sh says " +
				"so in its own words: 'NO INVITATION and no transport in the request'."},
		"/api/form/fill-csv nameCol": {productGap,
			"internal/server/form.go says so at the line: `nameCol \"\" → FillFormCSV names each " +
				"output row-NNN; a column picker can set it later`. No control offers one yet."},
	}

	serverPairs, npairs := serverReads(t)
	clientPairs, nclient := clientSends(t)

	// **Two stimulus floors, because the halves go blind in opposite directions.** A server scan
	// that reads nothing reports zero members — identical to a clean run, and the more dangerous of
	// the two. A client scan that reads nothing reports every field as unsent, which is loud.
	if npairs < 40 {
		t.Fatalf("the server scan found %d (route, field) pairs and this server has well over "+
			"fifty — it is not reading the handlers, and an empty report is what that looks like",
			npairs)
	}
	if nclient < 60 {
		t.Fatalf("the client scan found %d (route, field) pairs and web/app.js sends well over "+
			"eighty — every field would report as unsent", nclient)
	}

	var members []string
	live := map[string]bool{}
	for pair := range serverPairs {
		if clientPairs[pair] {
			continue
		}
		live[pair] = true
		if _, ok := declared[pair]; !ok {
			members = append(members, pair)
		}
	}
	sort.Strings(members)
	if len(members) > 0 {
		t.Errorf("%d request field(s) are read by a handler and sent by no client (/pending 447). "+
			"Either send them, stop reading them, or add each to `declared` with a category and a "+
			"reason — %q naming the harness file that sends it, %q, or %q, which is capped at %d.\n  %s",
			len(members), harnessOnly, cliOnly, productGap, productGapCap, strings.Join(members, "\n  "))
	}

	// The exemption table is itself a laundering surface, so it is checked in both directions and
	// on its own contents. Three of these four arms are mirrored from observables_test.go; the
	// category and its cap are new, and are what this class needs.
	gaps := 0
	var stale, unreasoned []string
	for pair, row := range declared {
		switch row.category {
		case harnessOnly, cliOnly:
		case sharedHelper:
			// The reason has to name the sibling route the field IS sent on, or the category is a
			// way of saying "some other route probably sends it" with nothing behind it.
			if !strings.Contains(row.reason, "/api/") {
				t.Errorf("%s is categorised %q and its reason names no sibling route. A shared read "+
					"is only benign if the field is genuinely sent SOMEWHERE through the same "+
					"helper; say which route that is", pair, sharedHelper)
			}
		case productGap:
			gaps++
		default:
			t.Errorf("%s carries category %q, which is not one of %q, %q, %q, %q",
				pair, row.category, harnessOnly, cliOnly, productGap, sharedHelper)
		}
		if len(strings.TrimSpace(row.reason)) < 40 {
			unreasoned = append(unreasoned, pair)
		}
		if !live[pair] {
			stale = append(stale, pair)
		}
	}
	sort.Strings(stale)
	sort.Strings(unreasoned)
	if gaps > productGapCap {
		t.Errorf("%d rows are categorised %q and the cap is %d. A third one is a decision about "+
			"the product, not a row: either the field gets a client, or the handler stops reading "+
			"it. The cap exists because naming a harness sender alone would never have caught "+
			"`invitation`, which has seven of them", gaps, productGap, productGapCap)
	}
	if len(unreasoned) > 0 {
		t.Errorf("%d exemption(s) carry no real reason. A row without one is a field nobody can "+
			"check:\n  %s", len(unreasoned), strings.Join(unreasoned, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("%d exemption(s) no longer describe anything: the field gained a client sender, or "+
			"the handler stopped reading it, or the route is gone. Remove the row — an exemption "+
			"for something that is fine is a reason nobody can check.\n  %s",
			len(stale), strings.Join(stale, "\n  "))
	}
}

// serverReads maps every POST route to the form and query fields its handler reads.
//
// **Parsed, never matched.** The reads are `*ast.CallExpr` nodes, so a field name in a comment or
// inside an unrelated string cannot register — which is the defect that made a bare-name prototype
// of this scan launder its own founding case.
func serverReads(t *testing.T) (map[string]bool, int) {
	t.Helper()
	dir := filepath.Join("internal", "server")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	funcs := map[string]*ast.FuncDecl{}
	var muxFile *ast.File
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", e.Name(), perr)
		}
		if e.Name() == "server.go" {
			muxFile = f
		}
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok {
				funcs[fd.Name.Name] = fd
			}
		}
	}
	if muxFile == nil {
		t.Fatal("internal/server/server.go did not parse, so no route is known at all")
	}

	routes := map[string]string{}
	ast.Inspect(muxFile, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "HandleFunc" {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || !strings.HasPrefix(strings.Trim(lit.Value, `"`), "POST /api/") {
			return true
		}
		route := strings.TrimPrefix(strings.Trim(lit.Value, `"`), "POST ")
		// The handler may be wrapped — requirePublicLoopback(s.handleX), s.withVault(...) — so take
		// the last `handle*` selector anywhere in the argument.
		last := ""
		ast.Inspect(call.Args[1], func(m ast.Node) bool {
			if s2, ok := m.(*ast.SelectorExpr); ok && strings.HasPrefix(s2.Sel.Name, "handle") {
				last = s2.Sel.Name
			}
			return true
		})
		if last != "" {
			routes[route] = last
		}
		return true
	})

	pairs := map[string]bool{}
	n := 0
	for route, handler := range routes {
		for field := range fieldsRead(funcs, handler, map[string]bool{}, 0) {
			pairs[route+" "+field] = true
			n++
		}
	}
	return pairs, n
}

// fieldsRead collects the request fields one function reads, following same-package helpers.
//
// **Two levels deep and prefix-gated, which is a stated approximation.** Handlers routinely park the
// read in a helper (`parseX(r)`), and a scan that stopped at the handler body would miss those and
// under-report — the silent direction. Going deeper, or following every callee, drags in shared
// utilities and attributes their reads to every route that touches them, which over-reports on the
// LOUD side. Two levels is where this repo's handlers actually put them.
func fieldsRead(funcs map[string]*ast.FuncDecl, name string, seen map[string]bool, depth int) map[string]bool {
	out := map[string]bool{}
	fd := funcs[name]
	if fd == nil || fd.Body == nil || seen[name] || depth > 2 {
		return out
	}
	seen[name] = true
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch sel.Sel.Name {
		case "FormValue", "PostFormValue", "Get":
			// **`Get` only on a `Query()` receiver.** `r.Header.Get("X-Nib-Doc")` is the same
			// selector name and is not a request FIELD — it is ADR-004's pinning header, which
			// `apiFetch` attaches to every call rather than any route sending it, and which
			// `pinning.test.mjs` already polices. Without this the report is forty rows of one
			// mechanism.
			if sel.Sel.Name == "Get" && !isQueryReceiver(sel.X) {
				return true
			}
			if len(call.Args) == 1 {
				if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if f := strings.Trim(lit.Value, `"`); f != "" && !strings.Contains(f, " ") {
						out[f] = true
					}
				}
			}
		default:
			for f := range fieldsRead(funcs, sel.Sel.Name, seen, depth+1) {
				out[f] = true
			}
		}
		return true
	})
	// ADR-004's other half: `docFor` falls back to a `doc` query parameter for the one caller that
	// is not `apiFetch` — pdf.js fetching `/api/pdf` — and it is read on the path of every route
	// that resolves a document. It is a mechanism, not a field of any one request, and its own ADR
	// says not to "fix" it into uniformity.
	delete(out, "doc")
	// A bare call to a package-level helper, `parseX(r)` rather than `s.parseX(r)`.
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, ok := call.Fun.(*ast.Ident); ok {
			for f := range fieldsRead(funcs, id.Name, seen, depth+1) {
				out[f] = true
			}
		}
		return true
	})
	return out
}

var (
	jsAPIFetch = regexp.MustCompile(`apiFetch\(`)
	jsRouteLit = regexp.MustCompile("[`'\"](/api/[^`'\"?]*)")
	jsQueryLit = regexp.MustCompile("[`'\"](/api/[^`'\"]*)\\?([^`'\"]*)")
	jsAppend   = regexp.MustCompile(`\.append\(\s*['"]([^'"]+)['"]`)
	jsURLEnc   = regexp.MustCompile(`['"]([a-zA-Z][\w-]*)=`)
)

// clientSends maps every route web/app.js posts to the fields it sends on it.
//
// **Attributed by the ENCLOSING FUNCTION, found by brace matching, not by enumerating named
// functions.** Two shapes this file uses constantly are invisible to a name walk: an arrow function
// assigned to a property (`els.saveAsGo.onclick = async () => {`), and a handler registered inline.
// Enumerating names put /api/write's `name` and `overwrite` in the report, and both are sent.
//
// **The parameter list is walked before the body brace is taken**, because `pageOp(op, extra = {})`
// puts a `{` inside its own parameters — the exact defect `scanUnpinned` records having shipped
// with, where it read `{}` as the whole body of the function driving twenty document operations.
func clientSends(t *testing.T) (map[string]bool, int) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("web", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	src := stripJSComments(string(b))

	// Every brace-matched block in the file, so the enclosing chain of any offset is available.
	type span struct{ open, close int }
	var blocks []span
	var stack []int
	for i, c := range src {
		switch c {
		case '{':
			stack = append(stack, i)
		case '}':
			if len(stack) > 0 {
				o := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				blocks = append(blocks, span{o, i})
			}
		}
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].open < blocks[j].open })

	isFuncOpen := func(o int) bool {
		j := o - 1
		for j >= 0 && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n') {
			j--
		}
		if j < 1 {
			return false
		}
		return src[j] == ')' || src[j-1:j+1] == "=>"
	}

	pairs := map[string]bool{}
	n := 0
	for _, m := range jsAPIFetch.FindAllStringIndex(src, -1) {
		args := matchedParen(src, m[1]-1)
		routes := map[string]bool{}
		for _, r := range jsRouteLit.FindAllStringSubmatch(args, -1) {
			routes[r[1]] = true
		}
		if len(routes) == 0 {
			continue
		}
		body := ""
		for _, bl := range blocks { // outermost first
			if bl.open < m[0] && m[0] < bl.close && isFuncOpen(bl.open) {
				body = src[bl.open : bl.close+1]
				break
			}
		}
		if body == "" {
			body = args
		}
		fields := map[string]bool{}
		for _, a := range jsAppend.FindAllStringSubmatch(body, -1) {
			fields[a[1]] = true
		}
		for _, u := range jsURLEnc.FindAllStringSubmatch(body, -1) {
			fields[u[1]] = true
		}
		for _, q := range jsQueryLit.FindAllStringSubmatch(args, -1) {
			for _, kv := range strings.Split(q[2], "&") {
				k := strings.TrimSpace(strings.SplitN(kv, "=", 2)[0])
				if k != "" && !strings.ContainsAny(k, " ${}+") {
					fields[k] = true
				}
			}
		}
		for r := range routes {
			for f := range fields {
				if !pairs[r+" "+f] {
					n++
				}
				pairs[r+" "+f] = true
			}
		}
	}
	return pairs, n
}

// isQueryReceiver reports whether a selector's receiver is `…Query()`, so `Query().Get("x")` counts
// as a request field and `Header.Get("x")` does not.
func isQueryReceiver(x ast.Expr) bool {
	call, ok := x.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Query"
}

// matchedParen returns the call's argument text, from its opening paren to the matching close.
func matchedParen(src string, lp int) string {
	d := 0
	for j := lp; j < len(src); j++ {
		switch src[j] {
		case '(':
			d++
		case ')':
			d--
			if d == 0 {
				return src[lp : j+1]
			}
		}
	}
	return src[lp:]
}

// stripJSComments removes line and block comments while leaving string and template literals whole,
// so a route or field named in prose cannot register as a send.
func stripJSComments(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		if c == '"' || c == '\'' || c == '`' {
			j := i + 1
			for j < len(s) {
				if s[j] == '\\' {
					j += 2
					continue
				}
				if s[j] == c {
					break
				}
				j++
			}
			if j >= len(s) {
				j = len(s) - 1
			}
			b.WriteString(s[i : j+1])
			i = j + 1
			continue
		}
		if strings.HasPrefix(s[i:], "//") {
			j := strings.IndexByte(s[i:], '\n')
			if j < 0 {
				j = len(s) - i
			}
			b.WriteString(strings.Repeat(" ", j))
			i += j
			continue
		}
		if strings.HasPrefix(s[i:], "/*") {
			j := strings.Index(s[i:], "*/")
			if j < 0 {
				j = len(s) - i
			} else {
				j += 2
			}
			b.WriteString(strings.Repeat(" ", j))
			i += j
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}
