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

// requestField is one exemption row: a form, query or JSON field a handler reads that no client
// sends.
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
//
// **/pending 477's extension spent the remaining headroom and did NOT raise the cap.** Adding the
// JSON carrier found `/api/session/arm address`, whose only caller in the whole tree is one
// in-package Go test — so the class is now 2 of 2. That is the ratchet working: a carrier that
// doubles the population is exactly when a cap is tempting to widen, and widening it would have
// turned the one number in this file that says *stop* into a number that follows the code.
const productGapCap = 2

// TestEveryRequestFieldAHandlerReadsIsOneSomeClientSends — /pending 447, extended to the JSON body
// carrier and to non-POST routes by /pending 477.
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
//
// # TWO carriers, and the boundary is declared rather than implied (/pending 477)
//
// The first version matched exactly three selectors — `FormValue`, `PostFormValue`, and `Get` on a
// `Query()` receiver — so its population was **form fields and query parameters**, and a handler
// that decoded a JSON body contributed nothing. **Probed, not inferred:** replacing `/api/ocr`'s
// `JSON.stringify({ lang, words })` in web/app.js with `JSON.stringify({})` left this test GREEN,
// re-run at v1.133.7. 28 routes and 79 fields — including the `block`/`para`/`line` that
// PLAN-accessibility P06.S06 had just wired up — sat outside a guard whose NAME claims the general
// rule, which is the same defect as the comment above: an enforcement cited and absent.
//
// So the JSON body is a second carrier now, with its own server scan and its own client scan.
// **The two carriers are kept apart on purpose.** Merging them would let an object key the client
// happens to use somewhere in a function excuse a FORM field of the same name, which weakens the
// half that already works to widen the half that did not exist.
//
// **What is still outside, named so the next author is not misled:**
//
//   - **JSON carried INSIDE a form field.** `json.Unmarshal([]byte(r.FormValue("rects")), &rects)`
//     — overlay.go, pages.go, form.go, outline.go, finalize.go. The OUTER field (`rects`, `fields`,
//     `outline`) is in the form population and IS checked; the inner objects' own keys are not.
//   - **Any caller that is not `web/app.js`** — the CLI, `internal/instance`, the tier-4/6 harness
//     scripts. That is what `declared` is for, and every row names its sender.
//   - **A body whose type this scan cannot resolve**: a map — `/api/profile` decodes
//     `map[string]string`, so it has no fields to check at all — or a named type outside
//     `internal/`. Both UNDER-report, which is the silent direction, and the stimulus floors below
//     are what stand in for that.
//
// # Every method, not just POST
//
// `serverReads` used to take only `POST /api/` registrations, so a query read on a GET route was
// outside the population too (/pending 477's AMEND). It now takes every method. Measured when it
// was widened: 35 GET routes contribute 5 (route, field) pairs and **every one of them is already
// sent**, so that half closed for the cost of the registration filter and no new exemption row.
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

		// ── the JSON body carrier (/pending 477) ─────────────────────────────────────────────
		"/api/handoff path": {cliOnly,
			"posted by the SECOND Nib process, not by any page: internal/instance/instance.go:260 " +
				"hands the file a relaunch was given to the instance already running. D20's " +
				"on-disk credential authorises this route and nothing else, so the web client " +
				"could not send it even if it had a reason to."},
		"/api/ceremony/deliver addresses": {harnessOnly,
			"build/pairrepro.sh:2112-2115 and :2995-2997 build `{'ceremony': …, 'addresses': …}` " +
				"for the decline round and the relay round. delivery.go:1376 calls it `an optional " +
				"hint per party fingerprint, for a caller that already knows` — the convener panel " +
				"does not, and browses instead."},
		"/api/ceremony/hop address": {harnessOnly,
			"build/ceremonyrepro.sh:767 posts `{\"ceremony\":…,\"address\":…}` on this route, and " +
				"the comment six lines above it says why: the no-address variant belongs to tier 4 " +
				"`--lan`, where an announcement can actually be heard. The panel sends a ceremony " +
				"id and lets the server find the party."},
		"/api/session/arm transport": {harnessOnly,
			"build/pairrepro.sh:1533 and build/ceremonyrepro.sh:409 force TCP or QUIC on this " +
				"route. Same reasoning as the two `transport` rows above it, on the receiving side " +
				"— the web client has no reason to choose a transport."},
		"/api/session/arm ceremony": {harnessOnly,
			"build/pairrepro.sh:2586 and :2591 drive D24's re-arm-from-a-stored-invitation, once " +
				"with an id this machine never accepted (expects 400) and once with a real one. " +
				"The panel re-arms by pasting the invitation again, which is the path armRequest " +
				"calls the manual one."},
		"/api/session/arm address": {productGap,
			"the ONLY caller in the tree is internal/server/session_test.go:1587, the forced-glare " +
				"case. **An in-package Go test is not a client for this guard's purposes** — every " +
				"field has one by construction, and accepting them would have excused `invitation`, " +
				"the defect this test was written after. session.go:2135 calls it `the manual tier " +
				"for the arm`, and no control and no harness script offers one.\n" +
				"**/pending 551 read it out, and the row is narrower than it looks.** The field is " +
				"honoured in exactly one branch — a QUIC arm carrying a ceremony — and the web " +
				"client never selects a transport, by the deliberate decision the `arm transport` " +
				"row above records. So wiring a control for this address alone would be inert: it " +
				"needs a transport choice the table says the client should not have. What 551 DID " +
				"fix is the half that needed nobody's decision — every arm that cannot honour the " +
				"field now REFUSES it (TestAnArmRefusesAnAddressItCannotUse) instead of answering " +
				"200 to a caller whose typed address was dropped on the floor. Whether the field is " +
				"then wired or removed is parked for Dan."},
	}

	serverPairs, npairs := serverReads(t)
	clientPairs, nclient := clientSends(t)
	serverJSON, untagged, njson := serverJSONReads(t)
	clientJSON, clientJSONFold, nclientJSON := clientJSONSends(t)

	// **Two stimulus floors per carrier, because the halves go blind in opposite directions.** A
	// server scan that reads nothing reports zero members — identical to a clean run, and the more
	// dangerous of the two. A client scan that reads nothing reports every field as unsent, which
	// is loud.
	//
	// **A second carrier needs its OWN floors or it can go blind while the first stays green**, and
	// the whole report would still look clean: the union of a live population and an empty one is
	// just the live one. Every number here is roughly half what the tree it was written against
	// actually holds (76 / 99 / 79 / 148 at v1.133.7), so ordinary growth never trips them and a
	// scan that stops resolving does.
	if npairs < 40 {
		t.Fatalf("the server scan found %d (route, field) pairs and this server has well over "+
			"fifty — it is not reading the handlers, and an empty report is what that looks like",
			npairs)
	}
	if nclient < 60 {
		t.Fatalf("the client scan found %d (route, field) pairs and web/app.js sends well over "+
			"eighty — every field would report as unsent", nclient)
	}
	if njson < 50 {
		t.Fatalf("the server JSON scan found %d (route, field) pairs and this server decodes a "+
			"request body on nearly thirty routes — the decode sites are not being found, or the "+
			"request types are no longer resolving, and either one reports NOTHING rather than "+
			"reporting wrongly", njson)
	}
	if nclientJSON < 90 {
		t.Fatalf("the client JSON scan found %d (route, key) pairs and web/app.js stringifies well "+
			"over a hundred — every JSON field would report as unsent", nclientJSON)
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
	for pair := range serverJSON {
		if clientJSON[pair] {
			continue
		}
		// An UNTAGGED exported field is matched case-insensitively, and that is not a nicety:
		// `handleOpenURL` decodes `struct{ URL string }` (sources.go:55) while the client sends
		// `{ url }`, and it works only because `encoding/json` falls back to a case-insensitive
		// match. A TAGGED field is compared exactly — a client sending `keypath` for
		// `json:"keyPath"` works at runtime and is still a typo somebody should be shown.
		if untagged[pair] {
			route, field, _ := strings.Cut(pair, " ")
			if clientJSONFold[route+" "+strings.ToLower(field)] {
				continue
			}
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

// serverPkg is one parsed Go package: its functions by name and its type declarations by name.
//
// The types are what /pending 477 needed and the original scan did not: a JSON body's fields live
// on a type, often in another package (`[]pdfops.Word`), and a scan that cannot follow the name
// reports the route as having no fields at all — silently.
type serverPkg struct {
	funcs map[string]*ast.FuncDecl
	types map[string]*ast.TypeSpec
}

// parseGoPkg parses one directory's non-test Go files.
//
// **A file that fails to parse is skipped rather than fatal**, for the imported packages only:
// build-tagged siblings under `internal/` parse fine, but a package this scan reaches by name is
// not a package this test is entitled to fail the build over. `internal/server` itself is fatal —
// see serverReads, where a missing server.go means no route is known at all.
func parseGoPkg(dir string) (*serverPkg, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	p := &serverPkg{funcs: map[string]*ast.FuncDecl{}, types: map[string]*ast.TypeSpec{}}
	fset := token.NewFileSet()
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if perr != nil {
			continue
		}
		for _, d := range f.Decls {
			switch dd := d.(type) {
			case *ast.FuncDecl:
				p.funcs[dd.Name.Name] = dd
			case *ast.GenDecl:
				for _, sp := range dd.Specs {
					if ts, ok := sp.(*ast.TypeSpec); ok {
						p.types[ts.Name.Name] = ts
					}
				}
			}
		}
	}
	return p, nil
}

// serverReads maps every /api/ route to the form and query fields its handler reads.
//
// **Parsed, never matched.** The reads are `*ast.CallExpr` nodes, so a field name in a comment or
// inside an unrelated string cannot register — which is the defect that made a bare-name prototype
// of this scan launder its own founding case.
//
// **A path can be registered on more than one METHOD, and eight of them are** — `/api/profile`,
// `/api/images`, `/api/ssh/keys`, `/api/outline`, `/api/ceremony/draft`, `/api/ceremony/hop`,
// `/api/identity/external`, `/api/images/{id}`. The route map therefore holds a SET of handlers
// per path and unions their reads; a `map[string]string` let the later registration overwrite the
// earlier, which drops a handler's whole field list without saying so.
func serverReads(t *testing.T) (map[string]bool, int) {
	t.Helper()
	funcs, routes := serverRoutes(t)
	pairs := map[string]bool{}
	n := 0
	for route, handlers := range routes {
		for handler := range handlers {
			for field := range fieldsRead(funcs, handler, map[string]bool{}, 0) {
				if !pairs[route+" "+field] {
					n++
				}
				pairs[route+" "+field] = true
			}
		}
	}
	return pairs, n
}

// serverRoutes parses internal/server and returns its functions plus every /api/ route's handlers.
var serverRoutes = func() func(*testing.T) (map[string]*ast.FuncDecl, map[string]map[string]bool) {
	var (
		cachedFuncs  map[string]*ast.FuncDecl
		cachedRoutes map[string]map[string]bool
	)
	return func(t *testing.T) (map[string]*ast.FuncDecl, map[string]map[string]bool) {
		t.Helper()
		if cachedFuncs != nil {
			return cachedFuncs, cachedRoutes
		}
		dir := filepath.Join("internal", "server")
		pkg, err := parseGoPkg(dir)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		muxFile, err := parser.ParseFile(fset, filepath.Join(dir, "server.go"), nil, 0)
		if err != nil {
			t.Fatalf("internal/server/server.go did not parse, so no route is known at all: %v", err)
		}
		routes := map[string]map[string]bool{}
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
			if !ok {
				return true
			}
			pattern := strings.Trim(lit.Value, `"`)
			method, path, hasMethod := strings.Cut(pattern, " ")
			if !hasMethod || method != strings.ToUpper(method) || !strings.HasPrefix(path, "/api/") {
				return true
			}
			// The handler may be wrapped — requirePublicLoopback(s.handleX), s.withVault(...) — so
			// take the last `handle*` selector anywhere in the argument.
			last := ""
			ast.Inspect(call.Args[1], func(m ast.Node) bool {
				if s2, ok := m.(*ast.SelectorExpr); ok && strings.HasPrefix(s2.Sel.Name, "handle") {
					last = s2.Sel.Name
				}
				return true
			})
			if last != "" {
				if routes[path] == nil {
					routes[path] = map[string]bool{}
				}
				routes[path][last] = true
			}
			return true
		})
		cachedFuncs, cachedRoutes = pkg.funcs, routes
		return cachedFuncs, cachedRoutes
	}
}()

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

// serverJSONReads maps every /api/ route to the JSON wire names its handler decodes from the
// request body (/pending 477). It returns the pairs, the subset whose Go field carried no `json`
// tag, and the pair count.
//
// **The decode site is found, and then the TYPE is resolved.** The site is
// `json.NewDecoder(io.LimitReader(r.Body, …)).Decode(&req)`, or a call to a package-level helper
// that does the same thing to one of its own parameters — `readJSON` at session.go:3354, which
// seven routes go through. **Those helpers are discovered by SHAPE, not by name**: a second
// `decodeBody` added next year is picked up, where a hardcoded name would take its routes out of
// the population and report nothing.
//
// **`r.Body`, and only `r.Body`.** `update.go:118` decodes a RESPONSE — GitHub's release JSON —
// through the identical call shape, and without that gate `/api/update/download` reported
// `tag_name`, `html_url`, `name`, `assets` and `browser_download_url` as request fields no client
// sends. A response walked in through the handler-following that makes the rest of this work.
func serverJSONReads(t *testing.T) (pairs, untagged map[string]bool, n int) {
	t.Helper()
	funcs, routes := serverRoutes(t)
	home := &serverPkg{funcs: funcs, types: map[string]*ast.TypeSpec{}}
	if pkg, err := parseGoPkg(filepath.Join("internal", "server")); err == nil {
		home.types = pkg.types
	}
	imported := map[string]*serverPkg{"server": home}

	helpers := jsonDecodeHelpers(funcs)
	pairs, untagged = map[string]bool{}, map[string]bool{}
	for route, handlers := range routes {
		fields, bare := map[string]bool{}, map[string]bool{}
		for h := range handlers {
			jsonFieldsRead(home, imported, helpers, h, map[string]bool{}, 0, fields, bare)
		}
		for f := range fields {
			if !pairs[route+" "+f] {
				n++
			}
			pairs[route+" "+f] = true
			if bare[f] {
				untagged[route+" "+f] = true
			}
		}
	}
	return pairs, untagged, n
}

// jsonDecodeHelpers finds the package-level functions that decode the request body into a
// parameter — the `readJSON(r, &req)` shape, where the TYPE is at the call site, not here.
func jsonDecodeHelpers(funcs map[string]*ast.FuncDecl) map[string]bool {
	out := map[string]bool{}
	for name, fd := range funcs {
		if fd.Body == nil || fd.Type.Params == nil {
			continue
		}
		params := map[string]bool{}
		for _, p := range fd.Type.Params.List {
			for _, pn := range p.Names {
				params[pn.Name] = true
			}
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Decode" || !isJSONDecoderOfRequest(sel.X) {
				return true
			}
			for _, a := range call.Args {
				if id, ok := a.(*ast.Ident); ok && params[id.Name] {
					out[name] = true
				}
			}
			return true
		})
	}
	return out
}

// jsonFieldsRead collects the JSON body fields one function decodes, following same-package helpers
// exactly as `fieldsRead` does — two levels, for the reason stated there.
func jsonFieldsRead(home *serverPkg, imported map[string]*serverPkg, helpers map[string]bool,
	name string, seen map[string]bool, depth int, out, untagged map[string]bool) {
	fd := home.funcs[name]
	if fd == nil || fd.Body == nil || seen[name] || depth > 2 {
		return
	}
	seen[name] = true
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		target, callee := "", ""
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			callee = fun.Sel.Name
			if fun.Sel.Name == "Decode" && isJSONDecoderOfRequest(fun.X) {
				target = pointerArg(call.Args)
			}
		case *ast.Ident:
			callee = fun.Name
		}
		if helpers[callee] {
			target = pointerArg(call.Args)
		}
		if target != "" {
			if te := varType(fd, target); te != nil {
				structFields(home, imported, te, 0, out, untagged)
			}
		}
		if callee != "" {
			jsonFieldsRead(home, imported, helpers, callee, seen, depth+1, out, untagged)
		}
		return true
	})
}

// isJSONDecoderOfRequest reports whether an expression is a `json.NewDecoder(… r.Body …)` chain.
func isJSONDecoderOfRequest(e ast.Expr) bool {
	decoder, body := false, false
	ast.Inspect(e, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name == "NewDecoder" {
			decoder = true
		}
		if sel.Sel.Name == "Body" {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "r" {
				body = true
			}
		}
		return true
	})
	return decoder && body
}

// pointerArg returns the name of the first `&x` argument, which is the decode target.
func pointerArg(args []ast.Expr) string {
	for _, a := range args {
		if u, ok := a.(*ast.UnaryExpr); ok && u.Op == token.AND {
			if id, ok := u.X.(*ast.Ident); ok {
				return id.Name
			}
		}
	}
	return ""
}

// varType finds the declared type of a local, from `var x T` or `x := T{…}`.
func varType(fn *ast.FuncDecl, name string) ast.Expr {
	var found ast.Expr
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.DeclStmt:
			gd, ok := s.Decl.(*ast.GenDecl)
			if !ok {
				return true
			}
			for _, sp := range gd.Specs {
				vs, ok := sp.(*ast.ValueSpec)
				if !ok || vs.Type == nil {
					continue
				}
				for _, nm := range vs.Names {
					if nm.Name == name {
						found = vs.Type
					}
				}
			}
		case *ast.AssignStmt:
			if s.Tok != token.DEFINE || len(s.Lhs) != 1 || len(s.Rhs) != 1 {
				return true
			}
			if id, ok := s.Lhs[0].(*ast.Ident); !ok || id.Name != name {
				return true
			}
			if cl, ok := s.Rhs[0].(*ast.CompositeLit); ok {
				found = cl.Type
			}
		}
		return true
	})
	return found
}

// structFields enumerates a request type's JSON wire names, following named types into other
// `internal/` packages.
//
// **Four levels of nesting, and the case that needs them is the one that found this gap.**
// `/api/ocr` decodes `[]pdfops.Word`, so `block`, `para` and `line` — the three fields
// PLAN-accessibility P06.S06 wired up — are one type and one package away from the handler. A
// scan that stopped at the handler's own struct would have reported the route as fully sent while
// the client sent nothing at all, which is exactly what the unextended guard did.
//
// **A map's KEYS are not fields.** `/api/profile` decodes `map[string]string`, so it contributes
// nothing and cannot: the wire names are the user's own form-field names. Stated rather than
// silently empty, because an empty answer here and a broken resolver look identical.
//
// **A type outside `internal/` is not followed**, which under-reports on the silent side. Nothing
// in this server's request bodies is one today; if that changes, the JSON stimulus floor is what
// notices the population shrinking.
func structFields(home *serverPkg, imported map[string]*serverPkg, expr ast.Expr, depth int, out, untagged map[string]bool) {
	if expr == nil || depth > 4 {
		return
	}
	switch t := expr.(type) {
	case *ast.StarExpr:
		structFields(home, imported, t.X, depth, out, untagged)
	case *ast.ArrayType:
		structFields(home, imported, t.Elt, depth, out, untagged)
	case *ast.MapType:
		structFields(home, imported, t.Value, depth, out, untagged)
	case *ast.Ident:
		if ts, ok := home.types[t.Name]; ok {
			structFields(home, imported, ts.Type, depth+1, out, untagged)
		}
	case *ast.SelectorExpr:
		id, ok := t.X.(*ast.Ident)
		if !ok {
			return
		}
		pkg, seen := imported[id.Name]
		if !seen {
			pkg, _ = parseGoPkg(filepath.Join("internal", id.Name))
			imported[id.Name] = pkg
		}
		if pkg == nil {
			return
		}
		if ts, ok := pkg.types[t.Sel.Name]; ok {
			structFields(pkg, imported, ts.Type, depth+1, out, untagged)
		}
	case *ast.StructType:
		for _, f := range t.Fields.List {
			if len(f.Names) == 0 { // embedded: its fields are inlined on the wire
				structFields(home, imported, f.Type, depth, out, untagged)
				continue
			}
			if !ast.IsExported(f.Names[0].Name) {
				continue
			}
			name, tagged := f.Names[0].Name, false
			if f.Tag != nil {
				raw := strings.Trim(f.Tag.Value, "`")
				if i := strings.Index(raw, `json:"`); i >= 0 {
					if j := strings.Index(raw[i+6:], `"`); j >= 0 {
						v, _, _ := strings.Cut(raw[i+6:i+6+j], ",")
						if v == "-" {
							continue // never on the wire
						}
						if v != "" {
							name, tagged = v, true
						}
					}
				}
			}
			out[name] = true
			if !tagged {
				untagged[name] = true
			}
			structFields(home, imported, f.Type, depth+1, out, untagged)
		}
	}
}

var (
	jsAPIFetch = regexp.MustCompile(`apiFetch\(`)
	jsAnyFetch = regexp.MustCompile(`\b(?:apiFetch|fetch)\(`)
	jsRouteLit = regexp.MustCompile("[`'\"](/api/[^`'\"?]*)")
	jsQueryLit = regexp.MustCompile("[`'\"](/api/[^`'\"]*)\\?([^`'\"]*)")
	jsAppend   = regexp.MustCompile(`\.append\(\s*['"]([^'"]+)['"]`)
	jsURLEnc   = regexp.MustCompile(`['"]([a-zA-Z][\w-]*)=`)
	jsStringif = regexp.MustCompile(`JSON\.stringify\(`)
	jsBareArg  = regexp.MustCompile(`^\(\s*([A-Za-z_$][\w$]*)\s*\)$`)
)

// jsSource reads web/app.js with its comments blanked, and indexes its function bodies.
type jsSource struct {
	src    string
	blocks []struct{ open, close int }
}

// readJSSource brace-matches every block in the file, so the enclosing chain of any offset is
// available.
func readJSSource(t *testing.T) *jsSource {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("web", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	j := &jsSource{src: stripJSComments(string(b))}
	var stack []int
	for i, c := range j.src {
		switch c {
		case '{':
			stack = append(stack, i)
		case '}':
			if len(stack) > 0 {
				o := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				j.blocks = append(j.blocks, struct{ open, close int }{o, i})
			}
		}
	}
	sort.Slice(j.blocks, func(a, b int) bool { return j.blocks[a].open < j.blocks[b].open })
	return j
}

// enclosingFunc returns the span of the innermost-outermost function body containing off, or
// (-1, -1).
//
// **The parameter list is walked before the body brace is taken**, because `pageOp(op, extra = {})`
// puts a `{` inside its own parameters — the exact defect `scanUnpinned` records having shipped
// with, where it read `{}` as the whole body of the function driving twenty document operations.
func (j *jsSource) enclosingFunc(off int) (int, int) {
	for _, bl := range j.blocks { // outermost first
		if bl.open < off && off < bl.close && j.isFuncOpen(bl.open) {
			return bl.open, bl.close
		}
	}
	return -1, -1
}

func (j *jsSource) isFuncOpen(o int) bool {
	s := j.src
	i := o - 1
	for i >= 0 && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n') {
		i--
	}
	if i < 1 {
		return false
	}
	return s[i] == ')' || s[i-1:i+1] == "=>"
}

// clientSends maps every route web/app.js posts to the FORM and QUERY fields it sends on it.
//
// **Attributed by the ENCLOSING FUNCTION, found by brace matching, not by enumerating named
// functions.** Two shapes this file uses constantly are invisible to a name walk: an arrow function
// assigned to a property (`els.saveAsGo.onclick = async () => {`), and a handler registered inline.
// Enumerating names put /api/write's `name` and `overwrite` in the report, and both are sent.
//
// **This half reads `apiFetch(` only, and that is deliberate** (/pending 477). Its JSON twin below
// also reads bare `fetch(`, because the pre-unlock SSH routes have no CSRF token to attach and call
// `fetch` directly. Widening THIS scan to match would let those sites excuse form fields too —
// a client population is only ever allowed to grow the carrier it was measured on.
func clientSends(t *testing.T) (map[string]bool, int) {
	t.Helper()
	j := readJSSource(t)
	src := j.src

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
		if open, close := j.enclosingFunc(m[0]); open >= 0 {
			body = src[open : close+1]
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

// clientJSONSends maps every route web/app.js sends a JSON body to, to the object keys it sends
// (/pending 477). The second return value is the same set lower-cased, for the untagged fields
// `encoding/json` matches case-insensitively.
//
// **It reads bare `fetch(` as well as `apiFetch(`.** Four SSH routes run pre-unlock, before a CSRF
// token exists, so they call `fetch` directly (app.js:285, 637, 676) — and an apiFetch-only
// population reported all eleven of their fields as unsent. Where such a call takes its URL from a
// variable (`const url = authState === 'migrate' ? '/api/ssh/migrate' : '/api/ssh/enroll'`), the
// route literals are taken from the enclosing function instead; that over-attributes on a function
// naming two routes, which is what enroll and migrate genuinely are — one body, two doors.
//
// **Three shapes, because the body is rarely a literal at the call site.**
//
//  1. The keys of `JSON.stringify({ … })` itself.
//  2. Every object literal in the enclosing function that is OUTSIDE any fetch call's arguments.
//     That is where a nested element is built: `/api/ocr`'s words are
//     `words.push({ page, text, rect, block, para, line })`, twenty lines above the send. The
//     fetch's own options object is excluded by the masking, or `method`, `headers`, `body` and
//     `docId` would count as fields every route sends.
//  3. When the stringified value is a bare identifier: the `x.key =` assignments made on it
//     (`handleEnroll` builds `body.mode` and `body.keyPath` by assignment, never as a literal),
//     and — if the enclosing function can be called by name — the object literals its CALLERS
//     pass. `saveSettings(body)` is called seven times with a different single-key object each
//     time, and without the caller step all seven reported. One level only, gated on the argument
//     being a bare identifier: the client-side twin of `fieldsRead`'s two-level cap, and for its
//     reason.
//
// **What the shapes cost.** Harvesting every literal in a function attributes unrelated keys to the
// route, and that is the SILENT direction — a server field named `data` or `label` could be excused
// by a destructuring pattern in the same function. It is accepted here for the same reason
// `clientSends` accepts every `.append` in the function: the alternative is a scan that
// under-reports sends, which turns the exemption table into the API. Measured on the tree it was
// written against: 148 (route, key) pairs against 79 server fields, and the six that report are
// each traceable to a named non-web caller.
func clientJSONSends(t *testing.T) (pairs, folded map[string]bool, n int) {
	t.Helper()
	j := readJSSource(t)
	src := j.src

	// Every fetch call's arguments, blanked, so shape 2 cannot read the options object.
	masked := []byte(src)
	for _, m := range jsAnyFetch.FindAllStringIndex(src, -1) {
		lp := m[1] - 1
		for k := lp; k < lp+len(matchedParen(src, lp)) && k < len(masked); k++ {
			if masked[k] != '\n' {
				masked[k] = ' '
			}
		}
	}
	outside := string(masked)

	pairs, folded = map[string]bool{}, map[string]bool{}
	for _, m := range jsAnyFetch.FindAllStringIndex(src, -1) {
		lp := m[1] - 1
		args := matchedParen(src, lp)
		open, close := j.enclosingFunc(m[0])
		routes := map[string]bool{}
		for _, r := range jsRouteLit.FindAllStringSubmatch(args, -1) {
			routes[r[1]] = true
		}
		if len(routes) == 0 && open >= 0 {
			for _, r := range jsRouteLit.FindAllStringSubmatch(outside[open:close+1], -1) {
				routes[r[1]] = true
			}
		}
		if len(routes) == 0 {
			continue
		}
		fields := map[string]bool{}
		for _, s := range jsStringif.FindAllStringIndex(args, -1) {
			stringified := matchedParen(args, s[1]-1)
			jsObjectKeys(stringified, fields) // shape 1
			bare := jsBareArg.FindStringSubmatch(strings.TrimSpace(stringified))
			if bare == nil || open < 0 {
				continue
			}
			// shape 3a: `body.mode = …`
			assigned := regexp.MustCompile(`\b` + regexp.QuoteMeta(bare[1]) + `\.([A-Za-z_$][\w$]*)\s*=[^=]`)
			for _, a := range assigned.FindAllStringSubmatch(outside[open:close+1], -1) {
				fields[a[1]] = true
			}
			// shape 3b: what this function's callers pass
			if name := j.callableName(open); name != "" {
				callSite := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\(`)
				for _, cs := range callSite.FindAllStringIndex(src, -1) {
					if cs[0] >= open && cs[0] <= close {
						continue // the declaration itself
					}
					jsObjectKeys(matchedParen(src, cs[1]-1), fields)
				}
			}
		}
		if open >= 0 {
			jsObjectKeys(outside[open:close+1], fields) // shape 2
		}
		for r := range routes {
			for f := range fields {
				if !pairs[r+" "+f] {
					n++
				}
				pairs[r+" "+f] = true
				folded[r+" "+strings.ToLower(f)] = true
			}
		}
	}
	return pairs, folded, n
}

// callableName returns the name a function whose body opens at `open` can be CALLED by — `function
// saveSettings(…)`, `const addKey = async (…)` — or "" for the anonymous shapes this file is full
// of (`els.x.onclick = async () => {`), where there is no call site to follow.
func (j *jsSource) callableName(open int) string {
	s := j.src
	i := open - 1
	skipWS := func(k int) int {
		for k >= 0 && (s[k] == ' ' || s[k] == '\t' || s[k] == '\n') {
			k--
		}
		return k
	}
	i = skipWS(i)
	if i >= 1 && s[i] == '>' && s[i-1] == '=' {
		i = skipWS(i - 2)
	}
	if i < 0 || s[i] != ')' {
		return ""
	}
	depth := 0
	for ; i >= 0; i-- {
		if s[i] == ')' {
			depth++
		} else if s[i] == '(' {
			if depth--; depth == 0 {
				break
			}
		}
	}
	if i < 0 {
		return ""
	}
	end := i
	for end > 0 && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	start := end
	for start > 0 && isJSIdentByte(s[start-1]) {
		start--
	}
	switch name := s[start:end]; name {
	// `async (…) => {` and the statement keywords are not names anything calls.
	case "", "function", "async", "if", "for", "while", "switch", "catch", "return":
		return ""
	default:
		return name
	}
}

func isJSIdentByte(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// jsObjectKeys collects the property names of every object literal in src.
//
// **An object literal is told from a BLOCK by its first token**, which is the whole difficulty: a
// regex for `name:` reads `case x:`, a label, and the middle of every ternary as keys, and each one
// it invents is a field the report stops naming. After a key is taken, the value is skipped to the
// next comma at that depth, so `{ a: x ? y : z }` yields `a` and not `y`.
//
// Strings are stepped over whole, so a colon inside one cannot open a key; a quoted key
// (`{ 'content-type': … }`) is taken, since that is how a wire name with a hyphen has to be written.
func jsObjectKeys(src string, out map[string]bool) {
	type frame struct{ obj, key bool }
	var st []frame
	top := func() *frame {
		if len(st) == 0 {
			return nil
		}
		return &st[len(st)-1]
	}
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '\'' || c == '"' || c == '`':
			end := skipJSString(src, i)
			if f := top(); f != nil && f.obj && f.key {
				if k := skipJSSpace(src, end+1); k < len(src) && src[k] == ':' {
					out[src[i+1:end]] = true
					f.key = false
					i = k + 1
					continue
				}
			}
			i = end + 1
			continue
		case c == '{':
			st = append(st, frame{obj: opensObjectLiteral(src, i), key: true})
			i++
			continue
		case c == '(' || c == '[':
			st = append(st, frame{})
			i++
			continue
		case c == '}' || c == ')' || c == ']':
			if len(st) > 0 {
				st = st[:len(st)-1]
			}
			i++
			continue
		}
		f := top()
		if f == nil || !f.obj {
			i++
			continue
		}
		if c == ',' {
			f.key = true
			i++
			continue
		}
		if !f.key || !isJSIdentByte(c) {
			i++
			continue
		}
		end := i
		for end < len(src) && isJSIdentByte(src[end]) {
			end++
		}
		name := src[i:end]
		k := skipJSSpace(src, end)
		switch {
		case k < len(src) && src[k] == ':':
			out[name] = true
			f.key = false
			i = k + 1
		case k < len(src) && (src[k] == ',' || src[k] == '}'):
			out[name] = true // `{ lang, words }` — shorthand
			i = k
		default:
			f.key = false
			i = end
		}
	}
}

// opensObjectLiteral reports whether the `{` at i begins an object literal rather than a block.
func opensObjectLiteral(src string, i int) bool {
	j := skipJSSpace(src, i+1)
	if j >= len(src) {
		return false
	}
	if src[j] == '}' || strings.HasPrefix(src[j:], "...") {
		return true
	}
	k := j
	if src[k] == '\'' || src[k] == '"' {
		k = skipJSString(src, k) + 1
	} else {
		for k < len(src) && isJSIdentByte(src[k]) {
			k++
		}
		if k == j {
			return false
		}
	}
	k = skipJSSpace(src, k)
	return k < len(src) && (src[k] == ':' || src[k] == ',' || src[k] == '}')
}

// skipJSString returns the index of the quote that closes the one at i.
func skipJSString(s string, i int) int {
	q := s[i]
	for j := i + 1; j < len(s); j++ {
		if s[j] == '\\' {
			j++
			continue
		}
		if s[j] == q {
			return j
		}
	}
	return len(s) - 1
}

func skipJSSpace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
		i++
	}
	return i
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
