package cli

import (
	"fmt"
	"strings"
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
	// unrostered names the signers who claim this ceremony and are on no roster line. NAMED and
	// not counted, per this package's own rule: the reader's next action is about a person.
	unrostered []string
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
}

// ceremonyReportOf builds the report for one document.
func ceremonyReportOf(pdf []byte, st sign.Status, now time.Time) ceremonyReport {
	rec, err := ceremony.CheckRecord(pdf, now)
	if err != nil {
		// No record at all is the ordinary case and is not a finding. Anything else is.
		if strings.Contains(err.Error(), "no ceremony record") {
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
	// One proceeding is a property of the whole document: every valid signature committing to the
	// record this document carries. `Attestations` has already computed it per signature.
	out.oneProc = len(atts) > 0
	newerTag, versions := false, map[int]bool{}
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
			out.unrostered = append(out.unrostered, name)
		}
		// D32's two discriminators, ported clause-for-clause from the web client, which has had
		// them since P07 and which the CLI never grew. A signature this build cannot parse and a
		// signature that disagrees are different facts and want opposite advice.
		if a.TagVersion > p2p.AttestationTagVersion() {
			newerTag = true
		}
		if a.RosterVersion > 0 {
			versions[a.RosterVersion] = true
		}
	}
	switch {
	case newerTag:
		out.skew = "one or more signatures were written by a newer version of Nib, so this build " +
			"did not read their attestation — that is a version difference, not a disagreement"
	case len(versions) > 1:
		out.skew = "the signatures carry more than one record format, so this build cannot compare " +
			"them all — that is a version difference, not a disagreement"
	}
	for _, party := range rec.Roster {
		p := ceremonyParty{label: party.Label, fp: party.Fingerprint, signs: party.Signs}
		for _, a := range atts {
			if a.Valid && strings.EqualFold(a.Fingerprint, party.Fingerprint) {
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
// `claimed > 0` because `oneProc` is false on a document with no signatures at all. `skew == ""`
// because a version difference is not a disagreement, and without that clause `nib verify` was
// measured exiting 2 on a newer-tag document and on a record-format skew — under the README's own
// `nib verify contract.pdf && echo "signature intact"` idiom, where a non-zero exit breaks a
// script over something no party did wrong. `unreadable == ""` for `incomplete`'s stated reason.
func (c ceremonyReport) disagrees() bool {
	return c.present && c.unreadable == "" && c.claimed > 0 && c.skew == "" && !c.oneProc
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

// lines renders the human report, one line per output row.
func (c ceremonyReport) lines() []string {
	if !c.present {
		return nil
	}
	if c.unreadable != "" {
		return []string{"ceremony: this document carries a ceremony record Nib could not read — " + c.unreadable}
	}
	head := fmt.Sprintf("ceremony %s: %d of %d obliged signer(s) have signed", short12(c.id), c.signed, c.obliged)
	if c.intent != "" {
		head += fmt.Sprintf("\n  recital: %q", c.intent)
	}
	out := []string{head}
	if c.signed < c.obliged {
		out = append(out, fmt.Sprintf("INCOMPLETE — %d obliged party(ies) have not signed", c.obliged-c.signed))
	}
	// The proceeding verdict, and it is not the same question as completeness: a document can be
	// fully signed by parties who committed to DIFFERENT records.
	switch {
	case c.skew != "":
		out = append(out, "ceremony: "+c.skew)
	case c.claimed == 0:
		// Nothing claims this ceremony yet, so there is no agreement to report either way.
	case !c.oneProc:
		out = append(out, "signatures do NOT all commit to this document's ceremony")
	default:
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
		Complete: c.unreadable == "" && c.obliged > 0 && c.signed >= c.obliged &&
			!c.hasUnrostered(),
		OneProceeding: c.oneProc,
		Skew:          c.skew,
		Unrostered:    c.unrostered,
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
