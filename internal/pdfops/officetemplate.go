package pdfops

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"nib/internal/ziputil"
	"nib/mdpdf"
)

// ErrTemplateFilledTooLarge: filling the placeholders would make one part of the document larger than a part may be.
// A value can be 64 KiB and a part can hold a million placeholders, so the product is bounded here and not trusted.
var ErrTemplateFilledTooLarge = errors.New("the filled template would be larger than Nib will build")

// UnwritableValueError is a value that holds a character an XML document cannot carry (a control character other
// than tab and newline, or a code point outside XML 1.0). It names the key and never the value (ADR-183).
type UnwritableValueError struct{ Key string }

func (e *UnwritableValueError) Error() string {
	return fmt.Sprintf("the value for %q holds a character a Word or OpenDocument file cannot carry", e.Key)
}

// SupportedTemplateExt is an Office extension whose `{{key}}` placeholders Nib fills before LibreOffice converts it.
func SupportedTemplateExt(ext string) bool {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "docx", "odt":
		return true
	}
	return false
}

// FillOfficeTemplate fills the `{{key}}` placeholders of a DOCX or ODT (RelaxTax B4, ADR-209).
//
// A placeholder is read from a PARAGRAPH's text, not from one run's: Word and LibreOffice routinely split `{{key}}`
// across runs or spans (a spell-check boundary, an edit session, a bold word), so a replace that read one text node
// at a time would miss the placeholders a person typed with formatting and leave the literal braces in a document
// that was supposed to be filled. The value is written into the node where the placeholder BEGINS, so it takes that
// run's formatting; the rest of the placeholder is cut out of the nodes that held it. This is the opposite of the
// Markdown path's refusal of a split placeholder, on purpose: in Markdown a split is a choice the author made with
// emphasis marks, in Word it is something the editor did without being asked.
//
// Everything else follows the Markdown path (mdpdf.Options.Vars): a value is TEXT, a key with no value is a
// *mdpdf.MissingPlaceholderError, braces that look like a placeholder and are not are a
// *mdpdf.MalformedPlaceholderError, `{{=key}}` prints the literal `{{key}}`, and no error carries a value.
// Only the parts that hold the document's text are read; every other member and every byte outside a replaced
// text node is carried through unchanged. A tab or line break in a value becomes the format's own tab and break.
func FillOfficeTemplate(data []byte, ext string, vars map[string]string) ([]byte, error) {
	kind := strings.ToLower(strings.TrimPrefix(ext, "."))
	if !SupportedTemplateExt(kind) {
		return nil, ErrOptionsNeedMarkdown
	}
	if vars == nil {
		return data, nil
	}
	f, _ := FormatForExt(kind)
	a, err := ziputil.Open(bytes.NewReader(data), int64(len(data)), packageLimits)
	if err != nil {
		return nil, packageRefusal(err, f)
	}
	entries := make([]ziputil.Entry, 0, len(a.Files()))
	for _, m := range a.Files() {
		if m.FileInfo().IsDir() {
			continue
		}
		// A part that holds text is capped as the package scan caps it; a picture or a font is carried whole under the
		// archive's own total (ziputil counts every byte Read produces against Limits.DecodedBytes).
		limit := int64(importDecodedBytes)
		if templatePart(kind, m.Name) {
			limit = packagePartCap
		}
		b, err := a.Read(m, limit)
		if err != nil {
			return nil, fmt.Errorf("%w: a part of the document could not be read", ErrArchiveRefused)
		}
		if templatePart(kind, m.Name) {
			if b, err = fillTemplatePart(b, kind, vars); err != nil {
				return nil, err
			}
		}
		// An ODF package's `mimetype` is stored first and uncompressed (its bytes sit at a fixed offset).
		entries = append(entries, ziputil.Entry{Name: m.Name, Data: b, Store: m.Method == 0 && m.Name == "mimetype"})
	}
	return ziputil.Write(entries)
}

var wordHeaderFooter = regexp.MustCompile(`^(header|footer)[0-9]*\.xml$`)

// templatePart is a member that holds the document's own text: the body, its headers and footers and its notes.
func templatePart(kind, name string) bool {
	if kind == "odt" {
		return name == "content.xml" || name == "styles.xml" // styles.xml holds the master pages' headers and footers
	}
	switch name {
	case "word/document.xml", "word/footnotes.xml", "word/endnotes.xml":
		return true
	}
	return path.Dir(name) == "word" && wordHeaderFooter.MatchString(path.Base(name))
}

var (
	bracePairRE   = regexp.MustCompile(`\{\{[^{}\n]*\}\}`)
	placeholderRE = regexp.MustCompile(`\{\{\s*(=?)([A-Za-z0-9_.-]+)\s*\}\}`)
)

// interrupts are the elements that sit BETWEEN two pieces of text without being text: a tab, a break, a field, a
// picture. A placeholder cannot run across one, so one is a NUL in the paragraph's text, which no key contains.
var interrupts = map[string]bool{
	"w:tab": true, "w:br": true, "w:cr": true, "w:fldChar": true, "w:instrText": true, "w:sym": true, "w:drawing": true,
	"w:pict": true, "w:object": true, "w:footnoteReference": true, "w:endnoteReference": true, "w:noBreakHyphen": true,
	"w:softHyphen": true, "w:ptab": true, "w:fldSimple": true,
	"text:tab": true, "text:line-break": true, "text:s": true, "draw:frame": true, "text:note": true,
	"text:page-number": true, "text:page-count": true, "text:bookmark-ref": true, "text:reference-ref": true,
}

type textNode struct {
	start, end           int    // the node's bytes in the part (a Word node starts at its `<w:t>` tag)
	plainStart, plainEnd int    // where its text sits in the paragraph's text
	text                 string // decoded
}

type paragraph struct {
	name  string
	text  strings.Builder
	nodes []textNode
}

type edit struct {
	start, end int
	repl       string
}

func qname(n xml.Name) string {
	if n.Space == "" {
		return n.Local
	}
	return n.Space + ":" + n.Local
}

func fillTemplatePart(b []byte, kind string, vars map[string]string) ([]byte, error) {
	budget := packagePartCap // what the values may still add to this part: a value is 64 KiB and a part may name it a million times
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = true
	d.Entity = nil
	var (
		stack   []string
		sawText bool // a paragraph's text node was read; none with braces in the bytes means the markup was not recognised
		paras   []*paragraph
		edits   []edit
		tStart  int
		prefix  string // the namespace prefix a break or tab is written with
	)
	isPara := func(n string) bool {
		if kind == "odt" {
			return n == "text:p" || n == "text:h"
		}
		return n == "w:p"
	}
	for {
		before := int(d.InputOffset())
		tok, err := d.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: a part of the document is not well-formed", ErrCorruptDocument)
		}
		after := int(d.InputOffset())
		switch t := tok.(type) {
		case xml.StartElement:
			n := qname(t.Name)
			switch {
			case isPara(n):
				paras = append(paras, &paragraph{name: n})
				prefix = t.Name.Space
			case len(paras) > 0 && interrupts[n]:
				paras[len(paras)-1].text.WriteByte(0)
			}
			if kind == "docx" && n == "w:t" {
				tStart, prefix = before, t.Name.Space
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if len(paras) > 0 && isPara(qname(t.Name)) {
				p := paras[len(paras)-1]
				paras = paras[:len(paras)-1]
				es, err := fillParagraph(p, kind, prefix, vars, &budget)
				if err != nil {
					return nil, err
				}
				edits = append(edits, es...)
			}
		case xml.CharData:
			if len(paras) == 0 || len(stack) == 0 || (kind == "docx" && stack[len(stack)-1] != "w:t") {
				continue
			}
			sawText = true
			p := paras[len(paras)-1]
			start := before
			if kind == "docx" {
				start = tStart
			}
			n := textNode{start: start, end: after, plainStart: p.text.Len(), text: string(t)}
			p.text.WriteString(n.text)
			n.plainEnd = p.text.Len()
			p.nodes = append(p.nodes, n)
		}
	}
	if len(edits) == 0 {
		// Braces in a part where no text was found are not "nothing to fill": the markup is not the one Word or
		// LibreOffice writes (a default namespace, a different prefix), and handing back a "filled" document with its
		// literal placeholders is the failure this function exists to prevent.
		if !sawText && bytes.Contains(b, []byte("{{")) {
			return nil, fmt.Errorf("%w: the text of a part of the document could not be found to fill", ErrCorruptDocument)
		}
		return b, nil
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var out bytes.Buffer
	cur := 0
	for _, e := range edits {
		out.Write(b[cur:e.start])
		out.WriteString(e.repl)
		cur = e.end
		if out.Len() > packagePartCap {
			return nil, ErrTemplateFilledTooLarge
		}
	}
	out.Write(b[cur:])
	if out.Len() > packagePartCap {
		return nil, ErrTemplateFilledTooLarge
	}
	return out.Bytes(), nil
}

// fillParagraph reads one paragraph's placeholders from its joined text and returns the edits that put the values in.
func fillParagraph(p *paragraph, kind, prefix string, vars map[string]string, budget *int) ([]edit, error) {
	text := p.text.String()
	if !strings.Contains(text, "{{") {
		return nil, nil
	}
	for _, m := range bracePairRE.FindAllString(text, -1) {
		if !placeholderRE.MatchString(m) {
			return nil, &mdpdf.MalformedPlaceholderError{Token: strings.ReplaceAll(m, "\x00", "")}
		}
	}
	type hit struct {
		start, end int
		repl       string
	}
	var hits []hit
	for _, ix := range placeholderRE.FindAllStringSubmatchIndex(text, -1) {
		key := text[ix[4]:ix[5]]
		h := hit{start: ix[0], end: ix[1]}
		if text[ix[2]:ix[3]] == "=" { // {{=key}} prints {{key}}
			h.repl = "{{" + key + "}}"
		} else {
			v, ok := vars[key]
			if !ok {
				return nil, &mdpdf.MissingPlaceholderError{Key: key}
			}
			if !xmlText(v) {
				return nil, &UnwritableValueError{Key: key}
			}
			h.repl = v
		}
		// Counted BEFORE anything is built: the refusal must cost less than the thing it refuses.
		if *budget -= len(h.repl); *budget < 0 {
			return nil, ErrTemplateFilledTooLarge
		}
		hits = append(hits, h)
	}
	var edits []edit
	for _, n := range p.nodes {
		var sb strings.Builder
		changed := false
		cur := n.plainStart
		for _, h := range hits {
			if h.end <= n.plainStart || h.start >= n.plainEnd {
				continue
			}
			changed = true
			if h.start > cur {
				sb.WriteString(text[cur:h.start])
			}
			if h.start >= n.plainStart { // the node where the placeholder begins carries the value
				sb.WriteString(h.repl)
			}
			cur = min(h.end, n.plainEnd)
		}
		if !changed {
			continue
		}
		if cur < n.plainEnd {
			sb.WriteString(text[cur:n.plainEnd])
		}
		edits = append(edits, edit{n.start, n.end, serialize(kind, prefix, sb.String())})
	}
	return edits, nil
}

// xmlText reports whether s can be written into an XML 1.0 document.
func xmlText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
		case r >= 0x20 && r <= 0xD7FF, r >= 0xE000 && r <= 0xFFFD, r >= 0x10000 && r <= 0x10FFFF:
		default:
			return false
		}
	}
	return true
}

var xmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// serialize writes a text node's new text: escaped, with a tab or line break as the format's own element.
func serialize(kind, prefix, s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	if kind == "docx" {
		// A Word text element drops leading and trailing space unless it says otherwise, and a value may have both.
		var sb strings.Builder
		sb.WriteString("<" + prefix + `:t xml:space="preserve">`)
		for _, r := range s {
			switch r {
			case '\n':
				sb.WriteString("</" + prefix + ":t><" + prefix + ":br/><" + prefix + `:t xml:space="preserve">`)
			case '\t':
				sb.WriteString("</" + prefix + ":t><" + prefix + ":tab/><" + prefix + `:t xml:space="preserve">`)
			default:
				sb.WriteString(xmlEscaper.Replace(string(r)))
			}
		}
		return sb.String()
	}
	var sb strings.Builder
	space := true // a space at the start of a node or after another space would collapse, so it is `text:s`
	run := 0
	flush := func() {
		if run > 0 {
			sb.WriteString("<" + prefix + ":s " + prefix + `:c="` + strconv.Itoa(run) + `"/>`)
			run = 0
		}
	}
	for _, r := range s {
		if r == ' ' {
			if space {
				run++
			} else {
				sb.WriteByte(' ')
				space = true
			}
			continue
		}
		flush()
		space = false
		switch r {
		case '\n':
			sb.WriteString("<" + prefix + ":line-break/>")
		case '\t':
			sb.WriteString("<" + prefix + ":tab/>")
		default:
			sb.WriteString(xmlEscaper.Replace(string(r)))
		}
	}
	flush()
	return sb.String()
}
