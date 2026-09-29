package pdfops

import "fmt"

// formWalkBudget bounds a content walk that follows form XObjects — `/pending 664`.
//
// **Depth does not bound the work.** A form that draws the next form ten times is walked ten times at
// every level, so six levels is a million walks from two kilobytes: measured 1.4 s (`formDrawCounts`),
// 0.9 s (`uncoveredDrawings`) and 1.2 s with 100,000 runs built (`readPageRuns`) at five levels, each ×10
// per level. `internal/uacheck` bounds its own walk the same way (`maxFormWalks`, `overBudget`); this is
// pdfops' one door for its three walkers, so a walker added later charges the same budget rather than
// inventing a fourth depth-only bound.
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

// err is the budget's verdict: nil, or why the walk stopped.
func (b *formWalkBudget) err() error {
	if b.over == "" {
		return nil
	}
	return fmt.Errorf("pdfops: %s", b.over)
}
