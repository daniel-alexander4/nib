package pdfops

import (
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// formWalkBudget bounds a content walk that follows form XObjects — `/pending 664`.
//
// **Depth does not bound the work.** A form that draws the next form ten times is walked ten times at
// every level, so six levels is a million walks from two kilobytes: measured 1.4 s (`formDrawCounts`),
// 0.9 s (`uncoveredDrawings`) and 1.2 s with 100,000 runs built (`readPageRuns`) at five levels, each ×10
// per level. `internal/uacheck` bounds its own walk the same way (`maxFormWalks`, `overBudget`); this is
// pdfops' one door for its three walkers, so a walker added later charges the same budget rather than
// inventing a fourth depth-only bound. It is also the walk's one DECODE door (`formContent`), so a form
// drawn N times is decoded once and nothing is decoded past the refusal (`/pending 742`).
//
// **Reaching it is an error, never a silent stop.** The forms past it were never read, so a count or a
// run list built from what was read would describe a page nib did not finish: each caller turns the error
// into a refusal (`uncoveredDrawings`, `readPageRuns`) or a defect (`structureCarriedCompletely`).
//
// The budget is per DOCUMENT where the walker reads the whole document, with an allowance per page, so a
// long ordinary document is not refused for being long while a document of many hostile pages still pays
// once: `newFormWalkBudget(ctx.PageCount)`. A walker that reads one page takes `newFormWalkBudget(1)`.
type formWalkBudget struct {
	walks, maxWalks int // form streams entered
	bytes, maxBytes int // form content bytes read, counted again every time a form is walked again
	over            string

	// decoded holds each form's decoded bytes by object number for the walk's life — `formContent`.
	decoded map[int][]byte
	decodes int // streams actually decoded, for the test that pins decode-once
}

const (
	// maxFormWalks is the base allowance of form walks, and formWalksPerPage what each page adds. An
	// ordinary page walks each form it draws once per `Do`.
	maxFormWalks     = 1 << 16
	formWalksPerPage = 1 << 10
	// maxFormBytes bounds the other way to amplify: one large form drawn many times, under the walk
	// budget and not under its byte count. Measured: the three walkers read form content at roughly
	// 13 MiB/s, so a page's worst case is ~1.5 s per walker. The per-page allowance is generous because a
	// long document whose every page draws one large background form reads that form once per page.
	maxFormBytes     = 1 << 24
	formBytesPerPage = 1 << 22
)

func newFormWalkBudget(pages int) *formWalkBudget {
	pages = max(pages, 1)
	return &formWalkBudget{maxWalks: maxFormWalks + formWalksPerPage*pages,
		maxBytes: maxFormBytes + formBytesPerPage*pages}
}

// enterForm charges one walk of a form whose content is n bytes, and reports whether the walk may
// proceed. Once it has said no it says no for the rest of the walk.
func (b *formWalkBudget) enterForm(n int) bool {
	if b.over != "" {
		return false
	}
	b.walks++
	b.bytes += n
	switch {
	case b.walks > b.maxWalks:
		b.over = fmt.Sprintf("the content enters form XObjects more than %d times (a form drawn inside forms "+
			"fans out); nib stops reading there, so what lies beyond was never read", b.maxWalks)
	case b.bytes > b.maxBytes:
		b.over = fmt.Sprintf("the content reads more than %d bytes of form XObjects, counting a form again each "+
			"time it is drawn; nib stops reading there, so what lies beyond was never read", b.maxBytes)
	}
	return b.over == ""
}

// formContent is the ONE door through which pdfops' form walkers read a form XObject's decoded bytes, and it
// decodes each object once per walk — `/pending 742`, the twin of uacheck's `decodedContent` (/pending 721).
//
// **Cached here because pdfcpu does not**: `DereferenceStreamDict` hands back a fresh copy of the stream
// dictionary at every call, so decoding it inflated the same form again at every `Do`. And the budget above
// is charged with the DECODED size, so the decode ran before any refusal and went on running after it:
// measured on one page drawing a 64 MiB flate form of spaces 80 times (a 66 KB file), `readPageRuns` took
// 6.6 s and `formDrawCounts` 13.4 s to refuse. Only the bytes are kept, never what a walk made of them — a
// form reads differently in each graphics state and marked-content stack it is drawn in, so every draw is
// still walked and still charged (`enterForm`).
//
// Once the budget has refused nothing is decoded at all: the walk is already an error, and a decode it can
// never use is the cost that ran on past the refusal. nil means "not read" — undecodable, or over budget.
// A direct stream (no object number) is decoded where it is met.
func (b *formWalkBudget) formContent(sd *types.StreamDict, obj types.Object) []byte {
	if b.over != "" {
		return nil
	}
	nr := -1
	if ir, ok := obj.(types.IndirectRef); ok {
		nr = ir.ObjectNumber.Value()
		if src, hit := b.decoded[nr]; hit {
			return src
		}
	}
	b.decodes++
	src := streamContent(sd)
	if nr >= 0 {
		if b.decoded == nil {
			b.decoded = map[int][]byte{}
		}
		b.decoded[nr] = src
	}
	return src
}

// err is the budget's verdict: nil, or why the walk stopped.
func (b *formWalkBudget) err() error {
	if b.over == "" {
		return nil
	}
	return fmt.Errorf("pdfops: %s", b.over)
}
