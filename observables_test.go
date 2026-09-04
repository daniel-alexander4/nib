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
	"internal/server",
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

// jsonPublishedPackages are the packages whose shapes leave as JSON rather than as Go types, and
// for which the "exported" test is therefore the WRONG proxy (/pending 347).
//
// # Why the proxy fails exactly here
//
// Everywhere else, "an exported struct returned by an exported function" is a good stand-in for
// "this shape leaves its package" — the only way another package can see it is through the type
// system. `internal/server` is the one package where that is false: its shapes are serialised by
// `writeJSON` straight to the browser, so nothing needs to import them and **they are all
// unexported**. The package has exactly ONE exported struct in the whole tree — `Server` itself,
// which carries no json tags.
//
// **That made this the one package the scan could not see into, and it is the one that publishes
// the most.** Measured 2026-09-01: 54 unexported json-tagged structs, 186 fields, including every
// shape the web client reads — `sessionStatus`, `pendingView`, `docResponse`, `statusResponse`,
// `noticeView`. Adding the package to the list above WITHOUT this relaxation passes green and
// discovers nothing, which is worse than the gap: a package in the list reads as covered.
//
// Three things relax for these packages, each because the export rule is standing in for something
// that is not true here: the TYPE may be unexported, an unexported FIELD is skipped rather than
// disqualifying the whole shape (an unexported field cannot be serialised, so it is not published
// and its presence says nothing), and the shape need not be RETURNED by an exported function
// (nothing returns these; `writeJSON` consumes them in place).
var jsonPublishedPackages = map[string]bool{"internal/server": true}

// jsonShapeReaders is the reader set for every shape from a jsonPublishedPackage, declared ONCE
// rather than as one `published` entry per shape.
//
// Fifty-four near-identical entries all naming the web client would be bookkeeping that drifts, and
// the assertion that matters is per-FIELD anyway — "does the client mention this field" — which is
// unchanged. The CLI is included because several server shapes are consumed there instead.
//
// **Request shapes are covered by the same rule and that is deliberate.** For a request type the
// client WRITES the field rather than reading it, and the check — does the reader source mention
// `.field` or `.jsonTag` — answers both directions. A request field nothing sets is as much a
// field nobody was ever told about as a response field nothing reads.
var jsonShapeReaders = []string{
	"web/app.js",
	"internal/cli/commands.go",
	"internal/cli/rendezvous.go",
	"internal/cli/discover.go",
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
	// **Discovered only from 2026-09-01, by being EMBEDDED (/pending 347).** `WatermarkStyle` is
	// exported and json-tagged and is only ever a parameter — `StampWatermark(pdf, text, st)` —
	// so the "returned by an exported function" filter had never admitted it, and its four fields
	// reached the wire inside `server.watermarkParam` with nothing checking they were consumed.
	"pdfops.WatermarkStyle": {"web/app.js"},
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
	// **`ceremony.Receipt` is the close-out's local record (P08.S06), and its reader is the same
	// route.** All three fields are rendered: `Ceremony` identifies the entry, `State` is the word
	// a user reads, and `ObservedAt` is what the list is ordered by and the date shown against it.
	// `handleCeremonies` carries `ListEnded`'s slice verbatim in `ceremoniesResponse.Ended`.
	//
	// **This scan is what turned the type into something with a reader.** It was first written
	// with a `ReadReceipt` that nothing called — `CheckDocument`'s defect exactly, a door built
	// and wired to nothing — and the guard failed the root package on the commit that added it.
	// The honest fix was a reader and not an exclusion: the receipt exists so a user can find the
	// contribution the prune preserved, and a receipt no surface shows preserves it in secret.
	"ceremony.Receipt": {"internal/ceremony/closeout.go", "internal/server/convene.go"},
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
	// ── internal/server, entered the day the scan could first see it (/pending 347) ──────────
	//
	// **Eleven of these are one fact, not eleven**: P06 has not built the ceremony surface. Named
	// search, 2026-09-01: `/api/ceremony/convene`, `/api/ceremony/accept` and the ceremonies list
	// route have **zero** references in `web/app.js`. So every field on a convene or accept shape
	// is unread because there is no client flow to read or set it — which is a schedule fact about
	// P06, not a field somebody forgot. They are parked BY NAME rather than covered by a blanket
	// exclusion, so that when P06 lands, each one fails here until its surface actually uses it.
	//
	// That is the whole point of parking them individually: a wildcard would go green the moment
	// the routes were called at all, whether or not the fields reached a user.
	"server.acceptedParty.Capacity":        "P06: no accept surface — /api/ceremony/accept has zero references in web/app.js",
	"server.acceptedParty.Convener":        "P06: no accept surface",
	"server.acceptedParty.Signs":           "P06: no accept surface",
	"server.ceremoniesResponse.Ceremonies": "P06: the ceremonies list has no surface (P08.S03 built the route; nothing calls it)",
	"server.ceremoniesResponse.Primary":    "P06: the ceremonies list has no surface",
	"server.conveneInvite.Signs":           "P06: no convene surface — /api/ceremony/convene has zero references in web/app.js",
	"server.convenePartyRequest.Capacity":  "P06: no convene surface, so nothing SETS this request field",
	"server.convenePartyRequest.Signs":     "P06: no convene surface, so nothing SETS this request field",
	"server.conveneRequest.ConvenerSigns":  "P06: no convene surface, so nothing SETS this request field",
	"server.conveneResponse.Invites":       "P06: no convene surface",
	"server.conveneResponse.Warnings":      "P06: no convene surface — and this is the one to wire FIRST when it lands: it carries the sitting warning P08.S05b computes, which is the only place a convener is told their deadline is tight",
	"server.lanHeardResponse.WindowMs":     "/pending 23: the discovery counters have a reader and no user-facing surface shows them. Same gap, same item.",

	// **Two were real, were filed rather than parked, and are now CLOSED** — /pending 349 (the D19
	// diagnosis gained a reader) and /pending 350 (the field was deleted). Both entries are gone
	// from this map, which is what an item's close is supposed to look like here; the two arms
	// below are what make leaving one behind a failure rather than a quiet lie.
	// **`sessionStatus.Diagnosis` and `diagnosisView.Cause` were here and are GONE — read since
	// /pending 349 (v1.117.309).** `reflectDiagnosis` renders the summary in the wait view, puts
	// the detail behind a disclosure per D19's presentation pin, and branches on the cause for
	// tone. The entries are removed rather than reworded, which is what makes an item's close
	// visible to this file — and the arm below is what makes leaving them a failure.
	// **`diagnosisResponse.Cause` is NOT parked, and the reason is a stated limit of this scan.**
	// It is a different shape from `diagnosisView` and carries an identically named field with an
	// identical tag, and this scan proves only that "a NAME is mentioned in a named file" — so
	// `d.cause` in `reflectDiagnosis`, which reads the arm-side view, satisfies both. A park here
	// could never be enforced and would sit forever looking like coverage.
	//
	// The honest state, recorded where it can be acted on rather than in a map this scan cannot
	// police: the standalone route is not called from the client at all (`grep -c "api/diagnos"
	// web/app.js` returns 0), while its own doc says "a client (and P06's ceremony panel) reads"
	// it. That is P06's surface, and it is /pending 268's neighbourhood rather than a field defect.
	"server.lanHeardResponse.Heard": "/pending 23: with WindowMs above — the discovery counters have a reader and no user-facing surface shows them",
	// **`updateResponse.Managed` was parked here and the FIELD is gone (/pending 350,
	// v1.117.309)** — consumed by `assetURL` inside its own handler before the response was
	// written, and read by nothing at the far end. It is a local now, so there is nothing on the
	// wire to read. Its removal from this map was not optional: the arm below refuses a park for a
	// field that no longer exists, because such an entry is a hole waiting for the next field of
	// that name — and that arm is what caught this one.

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
	// **Reason corrected 2026-09-01: its own deletion condition HAS been met, and the entry stays
	// anyway.** It said "Deleting this line is what P07.S07 does when a signature block renders
	// it (C19)" — and P07.S07 shipped, `AppearanceLines` emits `Capacity: <capacity>`, and
	// /pending 286 now bounds it. What has not changed is the thing this map actually asserts:
	// the renderer is `internal/p2p`, which is not a declared reader of `ceremony.Party`, so no
	// reader named here consumes the field. The entry is accurate about that and the old sentence
	// was not.
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

	// **A PER-PACKAGE floor for the JSON-published packages, and it exists because the obvious
	// fix to /pending 347 was a vacuous green.** Adding `internal/server` to `observablePackages`
	// and changing nothing else makes this test pass while discovering **zero** shapes from it —
	// the package is unexported throughout, so the export test drops every one. A package in the
	// list then reads as covered, to this scan, to the graduation pass that dispositions its rows,
	// and to the next person who greps the list. That is worse than the gap it was meant to close.
	//
	// The global floor above cannot see it: 33 shapes from ten other packages clears "at least
	// twenty" comfortably. Only a floor that names the package can.
	for pkg := range jsonPublishedPackages {
		short := pkg[strings.LastIndex(pkg, "/")+1:]
		n := 0
		for name := range shapes {
			if strings.HasPrefix(name, short+".") {
				n++
			}
		}
		if n < 20 {
			t.Fatalf("discovered %d shape(s) from %s, which publishes dozens of JSON bodies to "+
				"the web client. A JSON-publishing package in the list that discovers nothing "+
				"READS AS COVERED and is worse than leaving it out — see jsonPublishedPackages.",
				n, pkg)
		}
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
		if !ok && strings.HasPrefix(name, "server.") {
			readers, ok = jsonShapeReaders, true
		}
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
				// **A JavaScript reader is also matched on the BARE tag, and the reason is the
				// language rather than laxity (/pending 347).** `.field` is how a RESPONSE is
				// consumed, but a REQUEST is built as an object literal, and the client writes
				// them as `{ angle: n }` or with shorthand `{ fingerprint, intent }` — neither of
				// which contains `.angle` or `.intent` anywhere. Held to the strict form, the
				// scan reported every request field unread while the client set all of them.
				//
				// **Keyed on the READER's language, not on the shape's package**, which is the
				// second cut at this: the first asked whether the shape came from the JSON-
				// publishing package, and `pdfops.WatermarkStyle` — which reaches the client
				// inside a server shape — was a false finding under it. What decides the idiom is
				// the file doing the reading.
				//
				// This stays within what the scan claims for itself one comment up: it proves a
				// NAME is mentioned in a named file, and a green means "no field is obviously
				// orphaned" rather than "correctly consumed".
				if jt := sh.tag[f]; jt != "" && strings.HasSuffix(r, ".js") && mentionsWord(src, jt) {
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

	// **An entry that has since been FIXED must be deleted, and until /pending 349 nothing said
	// so.** `test/jsdom/published.test.mjs` has had this arm since it was written — it is what
	// told this sweep that `sessionStatus.diagnosis` had gained a reader — and the Go side, which
	// is the scan that covers `internal/server` at all, had only the missing-field arm below.
	//
	// The two failures are opposite and both matter. A park for a field that no longer EXISTS
	// silently covers the next field given that name; a park for a field that is now READ makes
	// the list stop describing anything, and quietly re-parks the field the day somebody deletes
	// its reader. The second is how a closed item's fix becomes invisible.
	{
		var fixed []string
		for k, why := range unreadKnown {
			// **Scoped to parks that name a tracked item, and the scope is the honest one.** A
			// park saying "/pending N keeps this unread" is a CLAIM ABOUT A DEFECT, and when the
			// item closes the entry has to go or the list stops describing anything. A park
			// saying "no product surface reports where a block landed" is a JUDGEMENT, and this
			// arm cannot adjudicate it: the matcher is deliberately loose — it accepts a bare
			// word in a JavaScript reader, and it accepts a mention in the shape's own defining
			// package — so run over design parks it reports fixes that are not fixes. Measured on
			// first run: three flagged, and two were `pdfops.SignatureWidget.Page` and
			// `ceremony.Party.Capacity`, both correct as they stand.
			//
			// A loose matcher is safe in the direction the main loop uses it (a false match means
			// "not obviously orphaned", which under-reports) and unsafe in this one, where a
			// false match tells you to delete a legitimate entry.
			if !strings.Contains(why, "/pending") {
				continue
			}
			dot := strings.LastIndex(k, ".")
			if dot < 0 {
				continue
			}
			sh, ok := shapes[k[:dot]]
			if !ok {
				continue
			}
			f := k[dot+1:]
			readers, ok := published[k[:dot]]
			if !ok && strings.HasPrefix(k, "server.") {
				readers = jsonShapeReaders
			}
			for _, r := range readers {
				src := readerSrc(t, r)
				if strings.Contains(src, "."+f) ||
					(sh.tag[f] != "" && strings.Contains(src, "."+sh.tag[f])) ||
					(sh.tag[f] != "" && strings.HasSuffix(r, ".js") && mentionsWord(src, sh.tag[f])) {
					fixed = append(fixed, k)
					break
				}
			}
		}
		sort.Strings(fixed)
		if len(fixed) > 0 {
			t.Errorf("these are parked in unreadKnown and now HAVE a reader — delete their "+
				"entries, or the list stops describing anything and silently re-parks each "+
				"field the day its reader is removed: %v", fixed)
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
	type embedCheck struct{ owner, embed string }
	var embedChecks []embedCheck
	// Every candidate, including those the "returned by an exported function" filter drops — see
	// the promotion in the deferred pass below.
	allCands := map[string]observable{}
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
					if !ok || st.Fields == nil {
						return true
					}
					// See jsonPublishedPackages: where a shape leaves as JSON, being unexported
					// is the normal case rather than a sign it stays home.
					if !d.Name.IsExported() && !jsonPublishedPackages[pkg] {
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
							} else if !jsonPublishedPackages[pkg] {
								// An unexported field cannot be serialised, so in a
								// JSON-publishing package it is not published and its presence
								// says nothing about the shape. Elsewhere it means the scan
								// cannot see the whole shape, which is a reason to skip it.
								unexported = true
							}
						}
					}
					// In a JSON-publishing package the json TAG is the evidence of publication:
					// an unexported struct with no tags is an internal record, not an observable.
					keep := len(fields) > 0 && !unexported
					if jsonPublishedPackages[pkg] {
						keep = len(tags) > 0
					}
					if keep {
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
			allCands[short+"."+name] = observable{file: c.file, fields: c.fields, tag: c.tag}
		}
		for name, c := range cands {
			// Nothing RETURNS a JSON-published shape — `writeJSON` consumes it in place — so the
			// return test would drop every one of them and report the package as clean.
			if !returned[name] && !jsonPublishedPackages[pkg] {
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
			// **The embed check is DEFERRED to after every package, and the reason is a real
			// miss (/pending 347).** It used to ask whether the embedded type was discovered in
			// THIS package, which is the wrong question for a cross-package embed:
			// `server.attestationView` embeds `p2p.SignerAttestation` and
			// `server.watermarkParam` embeds `pdfops.WatermarkStyle`, both of which are
			// discovered — under their own package's key. Asked per package, both read as
			// "covered by nothing" and the scan reported two false findings the moment
			// `internal/server` entered it. Asked once at the end, against everything discovered,
			// it answers what it means to.
			for _, e := range c.embeds {
				embedChecks = append(embedChecks, embedCheck{owner: short + "." + name, embed: e})
			}
			out[short+"."+name] = observable{file: c.file, fields: c.fields, tag: c.tag}
		}
	}
	// The deferred embed validation — see the comment at the append above. An embed whose type is
	// discovered ANYWHERE is covered by that shape's own entry; one discovered nowhere is reported
	// by name rather than dropped quietly, because a shape that vanishes takes its coverage with
	// it and reads identically to a shape that publishes nothing.
	for _, ec := range embedChecks {
		covered := false
		for full := range out {
			if full[strings.LastIndex(full, ".")+1:] == ec.embed {
				covered = true
				break
			}
		}
		// **A type EMBEDDED by a published shape is itself published, whatever the return
		// filter says (/pending 347).** `pdfops.WatermarkStyle` is exported and json-tagged and
		// is only ever a PARAMETER — `StampWatermark(pdf, text, st)` — so "returned by an
		// exported function" drops it. It still reaches the wire, inside `server.watermarkParam`,
		// and its four fields are as published as any other. Promoting it here is the rule the
		// return filter is a proxy for, applied where the proxy is wrong.
		if !covered {
			if c, ok := allCands[ec.embed]; ok {
				out[ec.embed] = c
				covered = true
			}
			for full, c := range allCands {
				if full[strings.LastIndex(full, ".")+1:] == ec.embed {
					out[full] = c
					covered = true
					break
				}
			}
		}
		if !covered {
			t.Errorf("%s embeds %s, which this scan does not discover — so %s's fields are "+
				"covered by nothing. Give %s an entry, or record here why it cannot have one.",
				ec.owner, ec.embed, ec.embed, ec.embed)
		}
	}
	return out
}

// mentionsWord reports whether src contains tok as a whole identifier, so `id` does not match
// inside `docId` and `lines` does not match inside `linesUsed`.
func mentionsWord(src, tok string) bool {
	isWord := func(b byte) bool {
		return b == '_' || b == '$' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
	}
	for i := 0; ; {
		j := strings.Index(src[i:], tok)
		if j < 0 {
			return false
		}
		j += i
		before := j == 0 || !isWord(src[j-1])
		after := j+len(tok) >= len(src) || !isWord(src[j+len(tok)])
		if before && after {
			return true
		}
		i = j + 1
	}
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
