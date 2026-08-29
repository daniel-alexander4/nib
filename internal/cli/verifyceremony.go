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
	for _, a := range atts {
		if !a.OneProceeding {
			out.oneProc = false
		}
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
	if !c.oneProc {
		out = append(out, "signatures do NOT all commit to this document's ceremony")
	} else {
		out = append(out, "every signature commits to this document's ceremony")
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
		Complete:      c.unreadable == "" && c.obliged > 0 && c.signed >= c.obliged,
		OneProceeding: c.oneProc,
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
