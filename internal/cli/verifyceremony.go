package cli

import (
	"errors"
	"fmt"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/sign"
)

// The ceremony half of `nib verify` (P07.S10, C11's CLI clause).
//
// # Why the CLI and not only the app
//
// The plan's words: "the CLI is the surface a dispute actually uses". A stranger handed a signed
// PDF and told to check it with Nib ran `nib verify` and got `valid (9 signer(s))` — true, and
// silent about the only questions a nine-party instrument raises: who was supposed to sign, did
// they, and is this one proceeding or several documents wearing one roster.
//
// # Why this reads the DOCUMENT's own record, where the L3 gate must not
//
// `ceremonyid.go` states the opposing rule for the gate — it reads "the record the party verified
// at arm time" and never the one the document carries, because a gate reading the document's own
// record answers its own question. That reasoning is about ADMITTING a contribution: the party
// deciding whether to sign holds an invitation to check the document against.
//
// A third-party verifier holds nothing. There is no invitation, no prior knowledge, and no
// out-of-band anything — the document is the entire input. So reading its record is not a
// weakened version of the gate's check; it is a different question with a different answer, and
// the report is worded to say only what the document can support.
type ceremonyReport struct {
	// present is false for an ordinary co-signed or finalized document, which has no ceremony and
	// must not be described as having an empty one.
	present bool
	// unreadable carries the reason a record was found and could not be checked — a version skew,
	// a bad signature on the record, an expired proceeding. Reported rather than swallowed: "this
	// document carries a ceremony record Nib could not read" and "this document has no ceremony"
	// are different facts about a document somebody is relying on.
	unreadable string

	id      string
	intent  string
	obliged int
	signed  int
	oneProc bool
	parties []ceremonyParty

	// claimed is how many valid signatures carry a ceremony commitment at all (/pending 324).
	//
	// **It gates the proceeding verdict**, because `oneProc` is false on a document with ZERO
	// signatures — so without this, a convened-but-unsigned document printed "signatures do NOT
	// all commit to this document's ceremony" about no signatures at all.
	claimed int
	// skew is set when the disagreement is a VERSION difference rather than a disagreement
	// (/pending 324). The web client has carried these two discriminators since D32 and the CLI
	// never had them, so it printed the flat accusation at a user whose counterparty had merely
	// updated Nib. That mattered the moment `oneProc` reached the exit code: measured, the naive
	// predicate exits 2 on a newer-tag document and on a record-format skew, where every party
	// agreed.
	skew string
	// contradicted is set when a signature this build COULD read does not commit to the document's
	// record (/pending 809).
	//
	// **A skew excuses the signatures it describes, never the document.** `skew` used to switch the
	// whole proceeding check off, and the tag that sets it is text the signer typed: one signature
	// saying `[NibCoSign:2]` excused every other signature on the file, including a stranger's that
	// committed to nothing. So the excuse is per signature — an unread tag, or a record format other
	// than the one the comparison is made in — and every valid signature it does not cover is held
	// to `Proceeding.Commits`, the same test `markOneProceeding` applies.
	contradicted bool
	// unrostered names the signers who claim this ceremony and are on no roster line. NAMED and
	// not counted, per this package's own rule: the reader's next action is about a person.
	unrostered []string
	// endState and endSource are what THIS MACHINE knows about how the proceeding ended, and are
	// empty on any machine that was not party to it. See `localEnd` — they are deliberately not
	// facts about the document.
	endState, endSource string
}

type ceremonyParty struct {
	label   string
	fp      string
	signs   bool
	didSign bool
}

// ceremonyReportJSON is the --json shape. Separate from the text rendering because a script
// consuming it should never have to parse a sentence.
type ceremonyReportJSON struct {
	ID            string   `json:"id,omitempty"`
	Unreadable    string   `json:"unreadable,omitempty"`
	Intent        string   `json:"intent,omitempty"`
	Obliged       int      `json:"obliged"`
	Signed        int      `json:"signed"`
	Complete      bool     `json:"complete"`
	OneProceeding bool     `json:"oneProceeding"`
	Missing       []string `json:"missing,omitempty"`
	// Skew distinguishes "this build cannot read those signatures" from "these people did not
	// agree" (/pending 324) — D32's rule, which the web has had and the CLI had not.
	Skew string `json:"skew,omitempty"`
	// Unrostered names signers who claim this ceremony and are on no roster line (/pending 324).
	Unrostered []string `json:"unrostered,omitempty"`
	// LocalEnd is how the proceeding ended ACCORDING TO THIS MACHINE, absent when it holds no
	// record (P08.S09). **Nested under its own key rather than flattened beside `complete`**, so a
	// script cannot read it as a property of the document: the two are different kinds of claim
	// and the same file yields no `localEnd` at all on a machine that was not party to it.
	LocalEnd *localEndJSON `json:"localEnd,omitempty"`
}

// localEndJSON carries the end state and, always beside it, where it came from.
type localEndJSON struct {
	State  string `json:"state"`
	Source string `json:"source"`
}

// ceremonyReportOf builds the report for one document.
func ceremonyReportOf(pdf []byte, st sign.Status, now time.Time) ceremonyReport {
	rec, err := ceremony.CheckRecord(pdf, now)
	if err != nil {
		// No record at all is the ordinary case and is not a finding. Anything else is.
		// By identity, not by text (`/pending 504`): `ErrNoRecord` is the sentinel `Extract` returns,
		// and a reworded message would otherwise turn every ordinary document into an unreadable one.
		if errors.Is(err, ceremony.ErrNoRecord) {
			return ceremonyReport{}
		}
		return ceremonyReport{present: true, unreadable: err.Error()}
	}
	proc := ceremony.ProceedingOf(pdf, now)
	atts := p2p.Attestations(st, proc)
	signed, obliged := p2p.Completeness(atts, proc)

	out := ceremonyReport{
		present: true,
		id:      rec.ID,
		intent:  rec.Intent,
		obliged: obliged,
		signed:  signed,
	}
	out.endState, out.endSource = localEnd(rec)
	// One proceeding is a property of the whole document: every valid signature committing to the
	// record this document carries. `Attestations` has already computed it per signature.
	out.oneProc = len(atts) > 0
	newerTag, versions := false, map[int]bool{}
	for _, a := range atts {
		if a.RosterVersion > 0 {
			versions[a.RosterVersion] = true
		}
	}
	// A record-format skew exists only BETWEEN signatures (the D32 branch, as it was): one format
	// throughout is compared as it always was, so a document whose every signature names a format
	// other than its record's still disagrees rather than being excused wholesale.
	formatSkew := len(versions) > 1
	for _, a := range atts {
		if !a.OneProceeding {
			out.oneProc = false
		}
		if a.Valid && a.RosterHash != "" {
			out.claimed++
		}
		if a.Unrostered {
			name := a.Signer
			if name == "" {
				name = short12(a.Fingerprint)
			}
			// A signature nib could not attribute has no fingerprint to shorten (ADR-051), and an
			// empty entry in this list reads as a blank rather than as a fact. Say which it is.
			if name == "" {
				name = "an unidentified signer"
			}
			out.unrostered = append(out.unrostered, name)
		}
		// D32's two discriminators, ported clause-for-clause from the web client, which has had
		// them since P07 and which the CLI never grew. A signature this build cannot parse and a
		// signature that disagrees are different facts and want opposite advice.
		//
		// **Each excuses its OWN signature and nothing else** (/pending 809) — see `contradicted`.
		unread := a.UnreadTag()
		otherFormat := formatSkew && a.RosterVersion > 0 && a.RosterVersion != rec.Version
		if unread {
			newerTag = true
		}
		if !unread && !otherFormat && a.Valid && !proc.Commits(a) {
			out.contradicted = true
		}
	}
	switch {
	case newerTag:
		out.skew = "one or more signatures were written by a newer version of Nib, so this build " +
			"did not read their attestation — that is a version difference, not a disagreement"
	case formatSkew:
		out.skew = "the signatures carry more than one record format, so this build cannot compare " +
			"them all — that is a version difference, not a disagreement"
	}
	for _, party := range rec.Roster {
		p := ceremonyParty{label: party.Label, fp: party.Fingerprint, signs: party.Signs}
		for _, a := range atts {
			// An empty fingerprint on either side matches nothing (ADR-051): `p2p.SameParty`, the
			// door `p2p.Completeness` counts through, so the tick and the count cannot disagree.
			if a.Valid && p2p.SameParty(a.Fingerprint, party.Fingerprint) {
				p.didSign = true
				break
			}
		}
		out.parties = append(out.parties, p)
	}
	return out
}

// incomplete reports whether an obliged party has not signed. It is what makes `nib verify` exit
// non-zero on an unfinished ceremony, so it deliberately does NOT fire on an unreadable record —
// that is a different failure with a different sentence, and conflating them would tell a user
// with a version skew that somebody refused to sign.
func (c ceremonyReport) incomplete() bool {
	return c.present && c.unreadable == "" && c.obliged > 0 && c.signed < c.obliged
}

// disagrees reports signatures that name DIFFERENT proceedings — and every clause in it was earned
// by measurement (/pending 324).
//
// `claimed > 0 || skew != ""` because nothing on a document no signature claims has disagreed
// with anything — an unread newer tag counts, being an attestation this build could not read
// rather than none. A version difference is not a disagreement, and without excusing it `nib
// verify` was measured exiting 2 on a newer-tag document and on a record-format skew — under the
// README's own `nib verify contract.pdf && echo "signature intact"` idiom, where a non-zero exit
// breaks a script over something no party did wrong. **But the excuse is per signature**
// (`contradicted`, /pending 809): it used to be `skew == ""` here, which let one signer's typed tag
// silence the check for every signature on the document. `unreadable == ""` for `incomplete`'s
// stated reason.
func (c ceremonyReport) disagrees() bool {
	return c.present && c.unreadable == "" && (c.claimed > 0 || c.skew != "") && c.contradicted
}

// hasUnrostered reports a signature claiming this ceremony from someone the roster does not name.
func (c ceremonyReport) hasUnrostered() bool { return len(c.unrostered) > 0 }

// refuses is the ONE door between a ceremony verdict and the exit code (ADR-009).
//
// Three predicates, one call site. Before this, only `incomplete` reached `commands.go`: a document
// whose signatures named different ceremonies printed exactly that in the text output and **exited
// 0**, and so did one carrying a signature from outside the roster. The CLI was the surface where
// the machine-readable channel disagreed with the human one — which is the divergence `AddedAfter`
// was added to this same condition to close, recorded three paragraphs down from it.
func (c ceremonyReport) refuses() bool {
	return c.incomplete() || c.disagrees() || c.hasUnrostered()
}

// complete is `--json`'s `complete`: `refuses()` negated, over a ceremony there is something to be
// complete about (/pending 649). It was its own conjunction, which folded in `hasUnrostered()` and
// dropped `disagrees()` — a document whose signatures named different proceedings exited 2 and
// told a script `"complete": true`. Built FROM the door, so a predicate added to `refuses()`
// reaches both channels or neither.
func (c ceremonyReport) complete() bool {
	return c.present && c.unreadable == "" && c.obliged > 0 && !c.refuses()
}

// lines renders the human report, one line per output row. A party's label, a signer's name and the
// unreadable cause are the document's own text, so the caller prints each row through `termText`
// and no row may carry a newline of its own.
func (c ceremonyReport) lines() []string {
	if !c.present {
		return nil
	}
	if c.unreadable != "" {
		return []string{"ceremony: this document carries a ceremony record Nib could not read — " + c.unreadable}
	}
	head := fmt.Sprintf("ceremony %s: %d of %d obliged signer(s) have signed", short12(c.id), c.signed, c.obliged)
	out := []string{head}
	if c.intent != "" {
		// Its own row, not a "\n" inside the head: the caller puts every row through `termText`,
		// which escapes a newline (/pending 727).
		out = append(out, fmt.Sprintf("  recital: %q", c.intent))
	}
	if c.signed < c.obliged {
		out = append(out, fmt.Sprintf("INCOMPLETE — %d obliged party(ies) have not signed", c.obliged-c.signed))
	}
	// The proceeding verdict, and it is not the same question as completeness: a document can be
	// fully signed by parties who committed to DIFFERENT records.
	//
	// The skew is said FIRST and no longer INSTEAD (/pending 809): it excuses the signatures it
	// describes, so a disagreement among the others is still one and is printed beside it.
	if c.skew != "" {
		out = append(out, "ceremony: "+c.skew)
	}
	switch {
	case c.disagrees():
		out = append(out, "signatures do NOT all commit to this document's ceremony")
	case c.claimed == 0:
		// Nothing this build can read claims this ceremony, so there is no agreement to report.
	case c.skew != "":
		// Only what was read can be vouched for, and the line says so rather than "every".
		out = append(out, "every signature this build could read commits to this document's ceremony")
	case c.oneProc:
		out = append(out, "every signature commits to this document's ceremony")
	}
	for _, name := range c.unrostered {
		out = append(out, fmt.Sprintf("UNROSTERED — %s signed claiming this ceremony and is not "+
			"on its roster", name))
	}
	for _, p := range c.parties {
		mark, role := "·", "not obliged to sign"
		switch {
		case p.didSign:
			mark, role = "✓", "signed"
		case p.signs:
			mark, role = "✗", "HAS NOT SIGNED"
		}
		name := p.label
		if name == "" {
			name = short12(p.fp)
		}
		out = append(out, fmt.Sprintf("%s %-28s %s", mark, name, role))
	}
	// **The end state goes LAST and under a heading that disclaims the document.** Everything
	// above is derived from the bytes in hand; this is read from this machine's disk, and on
	// another machine the same file produces none of it. Printed inline with the rest, a reader
	// deciding whether to rely on the document would take it for something the document says.
	//
	// The source is on its own line and is never omitted, because "declined" is worth acting on
	// and "who says so" is what decides how much. A signed termination and this machine's own
	// note are both useful and are not the same evidence.
	if c.endState != "" {
		out = append(out,
			"",
			"how this ceremony ended — from THIS MACHINE's records, not from the document:",
			"  "+c.endState,
			"  ("+c.endSource+")",
			"  The document itself does not record how its proceeding ended; a Nib PDF proves",
			"  only that its signatures are intact and who among the roster has signed.")
	} else if c.endSource != "" {
		out = append(out, "", "how this ceremony ended: "+c.endSource)
	}
	return out
}

// json renders the machine shape, or nil for a document with no ceremony — so `ceremony` is
// absent rather than an object full of zeros, which a script would read as a ceremony of nobody.
func (c ceremonyReport) json() *ceremonyReportJSON {
	if !c.present {
		return nil
	}
	out := &ceremonyReportJSON{
		ID: c.id, Unreadable: c.unreadable, Intent: c.intent,
		Obliged: c.obliged, Signed: c.signed,
		Complete:      c.complete(),
		OneProceeding: c.oneProc,
		Skew:          c.skew,
		Unrostered:    c.unrostered,
	}
	if c.endState != "" {
		out.LocalEnd = &localEndJSON{State: c.endState, Source: c.endSource}
	}
	for _, p := range c.parties {
		if p.signs && !p.didSign {
			name := p.label
			if name == "" {
				name = p.fp
			}
			out.Missing = append(out.Missing, name)
		}
	}
	return out
}

func short12(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12] + "…"
}

// ── The end state, which the DOCUMENT does not carry (P08.S09, D28) ──────────────────────────

// localEnd is what THIS MACHINE knows about how a ceremony ended.
//
// **The document does not carry this and never will.** D25 forbids a structural write after a
// signature, so nothing can be added to a finished PDF to say how its proceeding ended; and D28's
// four end states include two — expired and abandoned — that nobody can sign, because the party who
// would attest to them is precisely the one that stopped answering. So completeness is the only
// proceeding-level fact a Nib PDF proves about itself, and everything below is read from the disk
// of whoever is running `nib verify`.
//
// **Which is exactly why it is reported under its own heading.** A line saying "declined" beside
// lines derived from the document's own bytes invites a reader — and a reader here is often the
// person deciding whether to rely on the document — to treat it as something the document says. It
// is not. On another machine the same file produces no such line at all.
//
// Two sources, in this order, and the order is their standing. A `Termination` is the convener's
// SIGNED statement and is verified against the record from the document in hand — never against
// the `record.json` beside it, which a planted pair would satisfy against itself. A `Receipt` is
// this machine's own unattested note, and it is the only artifact that can carry `expired` or
// `abandoned`. Both are absent on a machine that was not party to the ceremony, which is the
// ordinary case and reports nothing.
//
// The folder is `ceremony.StoreRoot`'s, the one door the server writes through too (/pending 712
// R6-5). **Where there is no home directory the answer is "not looked up", said, never silence**:
// the server then writes under a relative `nib`, so an empty answer here would read as "this
// machine has no record" on the one machine whose records went somewhere else.
func localEnd(rec ceremony.Record) (state, source string) {
	root, err := ceremony.StoreRoot()
	if err != nil {
		return "", "this machine's records were not looked up: " + err.Error()
	}
	if t, terr := ceremony.ReadTermination(root, rec); terr == nil {
		return t.State, "signed by the convener and checked against this document's own record"
	}
	if r, rerr := ceremony.ReadReceipt(root, rec.ID); rerr == nil {
		return r.State, "this machine's own note, made when it saw the proceeding end — not signed"
	}
	return "", ""
}
