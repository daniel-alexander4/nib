package read

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"

	"nib/internal/docmodel"
	"nib/internal/pdfops"
)

// TestExportFidelityCorpus is the export oracle's corpus instrument (PLAN-import-export R1, /pending 945). It is not a
// tier and not a gate: it runs only when NIB_EXPORTFIDELITY_LIST names a LOCAL list of PDF paths (build/exportfidelity.sh
// sets it), because real documents are somebody's and are never committed. It prints COUNTS only — never a path, a word
// or a name.
//
// Two figures, both for a decision that was made unmeasured:
//
//   - completeness: per page, the non-space characters DocumentText reads (an independent walk of the page, with no
//     visibility rule) against those of the model's blocks, as a multiset — characters and not words, because a word is cut
//     differently by a run (kerning splits one, the grouping joins them) and a word multiset reports that as loss AND
//     as extra (the first run did: 82k lost against 95k extra). A character the model lacks is "lost"; the page is EXPLAINED when the document's
//     report names an exclusion (covered, hidden, off-page, unread, running header set aside) and UNEXPLAINED when it
//     names none. Words the model holds and the reference does not are "extra" (a tree's own text).
//   - order (941 #14): on each page the tree route took, whether the tree's word SEQUENCE differs from the proposal's
//     geometric one. Order is the author's call (D17), so this is the count of pages an order-sensitive check would move
//     to tree-incomplete, which is what decides between a weaker check and declining it.
func TestExportFidelityCorpus(t *testing.T) {
	list := os.Getenv("NIB_EXPORTFIDELITY_LIST")
	if list == "" {
		t.Skip("NIB_EXPORTFIDELITY_LIST not set: this is an instrument, run build/exportfidelity.sh")
	}
	raw, err := os.ReadFile(list)
	if err != nil {
		t.Fatal(err)
	}
	var tot struct {
		docs, unreadable, tagged, treeUnread, pages, treePages, orderDiffers                                     int
		refChars, lostChars, extraChars, explainedPages, unexplainedPages, extraOnlyPages, perfectPages, flagged int
		slowest                                                                                                  time.Duration
	}
	sources := map[string]int{}
	unexplainedCauses := map[string]int{}
	lostKinds := map[string]int{} // what the lost characters ARE, by class: a count, never a character
	var perPage []time.Duration
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r", ""), "\n") {
		path := strings.TrimSpace(line)
		if path == "" || strings.HasPrefix(path, "#") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			tot.unreadable++
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		start := time.Now()
		doc, err := Read(ctx, data, Options{})
		el := time.Since(start)
		if err != nil {
			cancel()
			tot.unreadable++
			continue
		}
		ref, rerr := pdfops.DocumentText(data)
		ed, eerr := pdfops.ExportRead(ctx, data, nil)
		tree, terr := pdfops.ReadStructure(data)
		cancel()
		if rerr != nil || eerr != nil {
			tot.unreadable++
			continue
		}
		tot.docs++
		sources[doc.Report.Source]++
		if doc.Report.Tagged {
			tot.tagged++
		}
		if doc.Report.TreeUnread {
			tot.treeUnread++
		}
		tot.flagged += len(doc.Report.FlaggedPages)
		if el > tot.slowest {
			tot.slowest = el
		}
		perPage = append(perPage, el/time.Duration(max(1, len(ref))))
		// A page is explained by ITS OWN counts: the document's totals explained seven pages whose loss had another cause
		// (/pending 949). Running furniture a tagged page's tree did not hold is counted for the document only.
		pageExcl := map[int]int{}
		for _, ep := range ed.Pages {
			v := ep.Visibility
			pageExcl[ep.Page] = v.Covered + v.MaybeCovered + v.HiddenText + v.HiddenLayer + v.OffPage + v.UnreadGlyphs + v.CoverUntested + v.ArtifactText + doc.Report.RunningSetAside
		}
		byPage := map[int]map[string]int{}
		for _, b := range doc.Blocks {
			m := byPage[b.Page]
			if m == nil {
				m = map[string]int{}
				byPage[b.Page] = m
			}
			b.Running = false // a running header is still in the file; words() leaves it out of the tree/page comparison, this is a count of what is written
			words([]docmodel.Block{b}, m)
		}
		for _, pt := range ref {
			tot.pages++
			want := map[string]int{}
			for _, r := range pt.Runs {
				for _, c := range strings.Join(strings.Fields(r.Text), "") {
					want[string(c)]++
				}
			}
			got := map[string]int{}
			for w, n := range byPage[pt.Page] {
				for _, c := range w {
					got[string(c)] += n
				}
			}
			lost, extra, total := 0, 0, 0
			for w, n := range want {
				total += n
				if g := got[w]; g < n {
					lost += n - g
					r := []rune(w)[0]
					switch {
					case unicode.IsLetter(r) && r < 0x250:
						lostKinds["latin-letter"] += n - g
					case unicode.IsLetter(r):
						lostKinds["other-letter"] += n - g
					case unicode.IsDigit(r):
						lostKinds["digit"] += n - g
					case unicode.IsPunct(r) || unicode.IsSymbol(r):
						lostKinds["punct-symbol"] += n - g
					default:
						lostKinds["other"] += n - g
					}
				}
			}
			for w, n := range got {
				if want[w] < n {
					extra += n - want[w]
				}
			}
			tot.refChars += total
			tot.lostChars += lost
			tot.extraChars += extra
			switch {
			case lost == 0 && extra == 0:
				tot.perfectPages++
			case lost == 0:
				tot.extraOnlyPages++ // the model holds more than the reference (list markers, the tree's own text); nothing is missing
			case pageExcl[pt.Page] > 0:
				tot.explainedPages++
			default:
				tot.unexplainedPages++
				// What the model itself said about this page, by cause: a count of causes, never of text.
				said := false
				for _, fp := range doc.Report.FlaggedPages {
					if fp.Page == pt.Page {
						unexplainedCauses[fp.Cause]++
						said = true
					}
				}
				for _, sp := range doc.Report.ScannedPages {
					if sp == pt.Page {
						unexplainedCauses["scanned-page"]++
						said = true
					}
				}
				if _, ok := doc.Report.Layout[pt.Page]; ok {
					unexplainedCauses["layout"]++
					said = true
				}
				if !said {
					unexplainedCauses["SAID-NOTHING"]++
				}
			}
		}
		if doc.Report.Tagged && terr == nil {
			tb, _ := treeBlocks(tree)
			for _, ep := range ed.Pages {
				prop := proposalBlocks(ep)
				if tb[ep.Page] == nil || !sameWords(tb[ep.Page], prop) || !hasText(prop) {
					continue
				}
				tot.treePages++
				if strings.Join(seq(tb[ep.Page]), " ") != strings.Join(seq(prop), " ") {
					tot.orderDiffers++
				}
			}
		}
	}
	sort.Slice(perPage, func(i, j int) bool { return perPage[i] < perPage[j] })
	med := time.Duration(0)
	if len(perPage) > 0 {
		med = perPage[len(perPage)/2]
	}
	fmt.Printf("EXPORTFIDELITY docs=%d unreadable=%d tagged=%d treeUnread=%d sources=%v flaggedPages=%d\n", tot.docs, tot.unreadable, tot.tagged, tot.treeUnread, sources, tot.flagged)
	fmt.Printf("EXPORTFIDELITY pages=%d perfect=%d extraOnly=%d lossExplained=%d UNEXPLAINEDLOSS=%d refChars=%d lostChars=%d extraChars=%d\n", tot.pages, tot.perfectPages, tot.extraOnlyPages, tot.explainedPages, tot.unexplainedPages, tot.refChars, tot.lostChars, tot.extraChars)
	if tot.refChars > 0 {
		fmt.Printf("EXPORTFIDELITY unexplainedLossPagesBySaidCause=%v\n", unexplainedCauses)
		fmt.Printf("EXPORTFIDELITY lostByClass=%v\n", lostKinds)
		fmt.Printf("EXPORTFIDELITY completeness=%.4f (1 - lost/ref)\n", 1-float64(tot.lostChars)/float64(tot.refChars))
	}
	fmt.Printf("EXPORTFIDELITY order: treePages=%d sequenceDiffers=%d (the pages an order-sensitive check would move to tree-incomplete)\n", tot.treePages, tot.orderDiffers)
	fmt.Printf("EXPORTFIDELITY time: slowest document %s, median per-reference-page-unit %s\n", tot.slowest.Round(time.Millisecond), med.Round(time.Microsecond))
}

// seq is a page's words in the order its blocks hold them.
func seq(blocks []docmodel.Block) []string {
	var out []string
	for _, b := range blocks {
		if b.Running {
			continue
		}
		var parts []string
		if b.Marker != "" {
			parts = append(parts, b.Marker)
		}
		parts = append(parts, b.Text())
		for _, r := range b.Rows {
			for _, c := range r {
				for _, p := range c.Paragraphs {
					parts = append(parts, plain(p))
				}
			}
		}
		out = append(out, strings.Fields(strings.Join(parts, " "))...)
	}
	return out
}
