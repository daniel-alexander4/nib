package pdfops

import (
	"fmt"
	"strconv"
	"strings"
)

// PageSpans turns a page-split request into an ordered list of pdfcpu page
// selections, one per output file. mode "every" chunks the n-page document into
// runs of `every` pages; mode "ranges" parses a comma/space-separated list where
// each token ("4-8" or "5") becomes one file. It is the single source of truth
// for split-range parsing, shared by the server's folder split and the CLI.
func PageSpans(mode, every, ranges string, n int) ([]string, error) {
	switch mode {
	case "every":
		k, err := strconv.Atoi(strings.TrimSpace(every))
		if err != nil || k < 1 {
			return nil, fmt.Errorf("pages-per-file must be a positive whole number")
		}
		var spans []string
		for from := 1; from <= n; from += k {
			thru := from + k - 1
			if thru > n {
				thru = n
			}
			spans = append(spans, spanLabel(from, thru))
		}
		return spans, nil
	case "ranges":
		var spans []string
		for _, tok := range strings.FieldsFunc(ranges, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\n' || r == '\t'
		}) {
			from, thru, err := parseSpan(tok, n)
			if err != nil {
				return nil, err
			}
			spans = append(spans, spanLabel(from, thru))
		}
		if len(spans) == 0 {
			return nil, fmt.Errorf("enter at least one page range, e.g. 1-3, 4-8")
		}
		return spans, nil
	default:
		return nil, fmt.Errorf("unknown split mode")
	}
}

// parseSpan parses one range token ("A-B" or "A") and bounds it to 1..n.
func parseSpan(tok string, n int) (from, thru int, err error) {
	lo, hi, isRange := strings.Cut(strings.TrimSpace(tok), "-")
	from, err = strconv.Atoi(strings.TrimSpace(lo))
	if err != nil {
		return 0, 0, fmt.Errorf("bad page range %q", tok)
	}
	thru = from
	if isRange {
		if thru, err = strconv.Atoi(strings.TrimSpace(hi)); err != nil {
			return 0, 0, fmt.Errorf("bad page range %q", tok)
		}
	}
	if from < 1 || thru < from || thru > n {
		return 0, 0, fmt.Errorf("page range %q is outside 1-%d", tok, n)
	}
	return from, thru, nil
}

// spanLabel is the pdfcpu selection (and base filename) for a span: "5" for a
// single page, "4-8" for a run.
func spanLabel(from, thru int) string {
	if from == thru {
		return strconv.Itoa(from)
	}
	return fmt.Sprintf("%d-%d", from, thru)
}
