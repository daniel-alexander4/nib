package mdpdf

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// Options are the choices a caller makes about one conversion. The zero value is what the package has always produced:
// US Letter, one-inch margins, no placeholders.
type Options struct {
	// PageSize is "letter" (the default), "a4" or "legal", portrait, in any case.
	PageSize string
	// Margin is the margin on all four sides in points; 0 is the default 72 (one inch). The content must stay at
	// least 144 points wide and 144 high.
	Margin float64
	// Vars turns placeholder substitution ON when it is non-nil: `{{key}}` in text is replaced by Vars[key], as
	// TEXT — never as Markdown, because it is put into the document after it has been parsed. A key with no value
	// is an error (*MissingPlaceholderError), never a literal `{{key}}`. `{{=key}}` is the escape and prints the
	// literal `{{key}}`. Fenced and indented code blocks are literal. A nil map leaves braces alone, as before.
	Vars map[string]string
}

// MissingPlaceholderError is a template that names a key no value was given for. It carries the key and never a value.
type MissingPlaceholderError struct{ Key string }

func (e *MissingPlaceholderError) Error() string {
	return fmt.Sprintf("the template uses {{%s}} and no value was given for %q", e.Key, e.Key)
}

// MalformedPlaceholderError is `{{…}}` text that is not a placeholder token (`{{first\_name}}`, `{{a b}}`). With
// placeholders on, braces that look like one and are not are an error and not a literal, for the same reason.
type MalformedPlaceholderError struct{ Token string }

func (e *MalformedPlaceholderError) Error() string {
	t := e.Token
	if len(t) > 60 {
		t = t[:60] + "…"
	}
	return fmt.Sprintf("%s looks like a placeholder and is not one: a key is letters, digits, _ . or -, with nothing else inside the braces (write {{=key}} to print the braces)", t)
}

// SplitPlaceholderError is a placeholder that formatting breaks in two (`{{na*me*}}`), which the substitution cannot
// see as one token. It is an error rather than a literal for the same reason a missing key is.
type SplitPlaceholderError struct{ Key string }

func (e *SplitPlaceholderError) Error() string {
	return fmt.Sprintf("the placeholder {{%s}} is broken up by formatting (emphasis, a link or an escape inside the braces); write it unbroken", e.Key)
}

// geom is one document's page geometry: the page, its margins, and the paper name pdfcpu is given. The three package
// constants above are the Letter case of it.
type geom struct {
	w, h, mx, my float64
	paper        string
}

var letterGeom = geom{pageW, pageH, marginX, marginY, "LetterP"}

func (g geom) contentW() float64 { return g.w - 2*g.mx }
func (g geom) top() float64      { return g.h - g.my }

// maxIndent bounds nested indentation to two thirds of the content width (see maxBlockIndent).
func (g geom) maxIndent() float64 { return g.contentW() * 2 / 3 }

var pageSizes = map[string]geom{
	"letter": {612, 792, 72, 72, "LetterP"},
	"a4":     {595, 842, 72, 72, "A4P"},
	"legal":  {612, 1008, 72, 72, "LegalP"},
}

// minContent is the smallest content box a margin may leave, in points.
const minContent = 144.0

// Validate reports a page size or margin the conversion would refuse, before any conversion work is done.
func (o Options) Validate() error {
	_, err := o.geometry()
	return err
}

func (o Options) geometry() (geom, error) {
	g := letterGeom
	if o.PageSize != "" {
		var ok bool
		if g, ok = pageSizes[strings.ToLower(o.PageSize)]; !ok {
			return g, fmt.Errorf("page size %q is not one of letter, a4, legal", o.PageSize)
		}
	}
	if o.Margin != 0 {
		if o.Margin < 0 || g.w-2*o.Margin < minContent || g.h-2*o.Margin < minContent {
			return g, fmt.Errorf("a margin of %g points leaves less than %g points of page to write on", o.Margin, minContent)
		}
		g.mx, g.my = o.Margin, o.Margin
	}
	return g, nil
}

var bracePairRE = regexp.MustCompile(`\{\{[^{}\n]*\}\}`)

var placeholderRE = regexp.MustCompile(`\{\{\s*(=?)([A-Za-z0-9_.-]+)\s*\}\}`)

// substituter replaces placeholders in the text nodes of a parsed document and finds the ones it cannot.
type substituter struct {
	vars map[string]string
	err  error
}

func (s *substituter) fail(e error) {
	if s.err == nil {
		s.err = e
	}
}

// text is one text node with its placeholders replaced.
func (s *substituter) text(in string) string {
	if s == nil || s.vars == nil || !strings.Contains(in, "{{") {
		return in
	}
	return placeholderRE.ReplaceAllStringFunc(in, func(m string) string {
		sub := placeholderRE.FindStringSubmatch(m)
		if sub[1] == "=" { // {{=key}} prints {{key}}
			return "{{" + sub[2] + "}}"
		}
		v, ok := s.vars[sub[2]]
		if !ok {
			s.fail(&MissingPlaceholderError{sub[2]})
			return m
		}
		return v
	})
}

// checkSplit looks for a placeholder that spans text nodes of one block: its plain text holds more of them than its
// nodes do one by one.
func (s *substituter) checkSplit(block ast.Node, src []byte) {
	if s == nil || s.vars == nil {
		return
	}
	var plain strings.Builder
	whole := 0
	var walk func(n ast.Node)
	walk = func(n ast.Node) {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch t := c.(type) {
			case *ast.Text:
				v := string(t.Segment.Value(src))
				plain.WriteString(v)
				whole += len(placeholderRE.FindAllString(v, -1))
			case *ast.String:
				plain.WriteString(string(t.Value))
				whole += len(placeholderRE.FindAllString(string(t.Value), -1))
			}
			walk(c)
		}
	}
	walk(block)
	for _, m := range bracePairRE.FindAllString(plain.String(), -1) {
		if !placeholderRE.MatchString(m) {
			s.fail(&MalformedPlaceholderError{m})
			return
		}
	}
	if p := placeholderRE.FindAllStringSubmatch(plain.String(), -1); len(p) > whole {
		// Name one that is not whole in any single node: the first beyond the whole ones is enough to say.
		s.fail(&SplitPlaceholderError{p[len(p)-1][2]})
	}
}
