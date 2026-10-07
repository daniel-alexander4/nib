package pdfops

import (
	"bytes"
	"fmt"

	"nib/internal/contentstream"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"golang.org/x/text/unicode/bidi"
)

// The order a right-to-left OCR word is set in — ADR-097.
//
// # What was wrong
//
// A stamped word is set glyph after glyph from the left of its box. A Hebrew or Arabic word was handed over in
// READING order, so its first letter was the leftmost glyph — and the rightmost letter of the scanned word. Two things
// followed. Nib could not say where a letter of the word is, so a match on part of one takes the whole word (ADR-093).
// And every other reader had the word backwards: a PDF sets right-to-left text in the order it is SEEN, a reader
// turns it round to read it, and so poppler and pdf.js turned round a word that was already in reading order —
// measured, `pdftotext` and pdf.js's Find and copy both gave "םולש" for "שלום".
//
// # What is written
//
// The word in the reverse of reading order — the order every print PDF sets it in — so glyph i from the RIGHT is
// letter i, and inside `/ReversedChars BMC … EMC`, which is what ISO 32000-1 §14.8.2.3.3 provides for exactly this: a
// show string whose characters are in the reverse of reading order. The tag is how Nib's own reader knows this word
// from one stamped before, which is still in reading order and still taken whole.

// setOrder is the string to stamp for an OCR word, and whether it is the reverse of text.
//
// Reversed only where turning the whole word round is all a reader does to it: a word with a right-to-left letter
// and nothing that keeps its own direction inside one — a Latin letter, a digit of either kind — and no bracket,
// which a reader may or may not mirror. Any other word is set as it was, in reading order, and is read as before.
// Measured in poppler on the faces nib stamps in: every word this reverses came back in reading order, and
// "ב-2020" and "م2" set in reverse did not.
func setOrder(text string) (string, bool) {
	rtl := false
	for _, r := range text {
		p, _ := bidi.LookupRune(r)
		switch p.Class() {
		case bidi.R, bidi.AL:
			rtl = true
		case bidi.L, bidi.EN, bidi.AN:
			return text, false
		}
		if p.IsBracket() {
			return text, false
		}
	}
	if !rtl {
		return text, false
	}
	rs := []rune(text)
	for i, j := 0, len(rs)-1; i < j; i, j = i+1, j-1 {
		rs[i], rs[j] = rs[j], rs[i]
	}
	return string(rs), true
}

// markReversedChars brackets the content of the form a page draws as name in `/ReversedChars BMC … EMC`. The form
// is one word's, written by pdfcpu as a single balanced `q … Q`, so the bracket is around the whole of it.
func markReversedChars(ctx *model.Context, res types.Dict, name string) error {
	xobjs, err := ctx.DereferenceDict(res["XObject"])
	if err != nil || xobjs == nil {
		return fmt.Errorf("pdfops: the text layer's form %s is not in the page's resources", name)
	}
	ref, ok := xobjs[name].(types.IndirectRef)
	if !ok {
		return fmt.Errorf("pdfops: the text layer's form %s is not an indirect object", name)
	}
	sd, _, err := ctx.DereferenceStreamDict(ref)
	if err != nil || sd == nil {
		return fmt.Errorf("pdfops: the text layer's form %s does not read", name)
	}
	// Through the one door a form's bytes are read by (ADR-009), though this form is one pdfcpu wrote a moment ago.
	body := newFormWalkBudget(1).formContent(sd, ref)
	if body == nil {
		return fmt.Errorf("pdfops: the text layer's form %s does not decode", name)
	}
	open := []byte("/" + reversedCharsTag + " BMC ")
	if bytes.HasPrefix(bytes.TrimLeft(body, " \r\n\t"), open) {
		return nil
	}
	sd.Content = append(append(append([]byte{}, open...), body...), []byte(" EMC")...)
	if err := sd.Encode(); err != nil {
		return err
	}
	entry, found := ctx.FindTableEntryForIndRef(&ref)
	if !found || entry == nil {
		return fmt.Errorf("pdfops: the text layer's form %s has no entry to write back", name)
	}
	entry.Object = *sd
	return nil
}

// formDrawnAfter is the name of the form pdfcpu draws after a stamp's placement: `… cm /GS0 gs /Fm0 Do Q`. toks
// begin after the `cm`.
func formDrawnAfter(src []byte, toks []contentstream.Token) (string, bool) {
	name := ""
	for _, tk := range toks {
		if tk.Kind == contentstream.Whitespace {
			continue
		}
		s := string(tk.Bytes(src))
		if tk.Kind == contentstream.Operator {
			switch s {
			case "gs":
				continue
			case "Do":
				return name, name != ""
			}
			return "", false
		}
		name = ""
		if len(s) > 1 && s[0] == '/' {
			name = s[1:]
		}
	}
	return "", false
}
