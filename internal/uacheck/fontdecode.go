package uacheck

import (
	"errors"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// The font-program and CMap streams' decode ceilings (/pending 807 R6).
//
// Nine readers — /ToUnicode, an embedded CMap read three ways, TrueType, OpenType, CFF, Type 1 and a Type 3
// glyph procedure — called a bare `sd.Decode()`, so each stream inflated to pdfcpu's 512 MiB default and was
// held in `sd.Content` for the whole check; at Flate's ~1000:1 a 520 KB stream reaches that, and nothing bounded
// how many. ADR-009's shape: `decodeWithin` was already the package's door for a capped decode and these sites did
// not use it.
//
// **A stream** is bounded at 64 MiB: the largest real embedded programs — whole CJK faces — run to a few tens of
// MiB, and a CMap is far smaller. **The document** is bounded at 256 MiB across all of them, the way
// `maxContentBytes` bounds the content walk: a font is usually shared, and each stream is decoded once here.
// Past either, the reader that asked says why and the rule it feeds cannot check — never a pass.
const (
	maxFontStreamDecoded = 64 << 20
	maxFontBytesDecoded  = 256 << 20
)

// decodeFontStream is the ONE decode of a font program, CMap or glyph procedure: it answers "" when sd.Content
// holds the stream, and otherwise why it does not. `what` names the stream for that sentence.
//
// A stream already decoded (by this door or any other) is not decoded again. A refusal is remembered, so a stream
// many fonts name is inflated toward its ceiling once.
//
// **Remembered by the stream, not by the copy** (`/pending 730`): `DereferenceStreamDict` returns a fresh copy of
// the stream dictionary to every caller, so a decode written into one copy's `sd.Content` is not in the next. The
// copies share their `Dict`, which is what `dictID` keys, so the bytes decoded once are handed to every later copy.
func (d *Document) decodeFontStream(sd *types.StreamDict, what string) string {
	if sd.Content != nil {
		return ""
	}
	key := dictID(sd.Dict)
	if why, failed := d.fontDecodeFailed[key]; failed {
		return why
	}
	if b, done := d.fontDecodedContent[key]; done {
		sd.Content = b
		return ""
	}
	d.fontDecodes++
	limit := min(maxFontStreamDecoded, maxFontBytesDecoded-d.fontDecoded)
	why := ""
	if err := pdfread.DecodeWithin(sd, int64(limit)); err != nil {
		switch {
		case !errors.Is(err, pdfread.ErrDecodeLimit):
			why = what + " could not be decoded: " + err.Error()
		case limit < maxFontStreamDecoded:
			why = fmt.Sprintf("%s was not decoded: the document's font programs and CMaps have used %d of the %d "+
				"bytes nib decodes for them, and nib stops there", what, d.fontDecoded, maxFontBytesDecoded)
		default:
			why = fmt.Sprintf("%s decodes past %d bytes, and nib stopped decoding it there", what, maxFontStreamDecoded)
		}
		sd.Content = nil
		if d.fontDecodeFailed == nil {
			d.fontDecodeFailed = map[uintptr]string{}
		}
		d.fontDecodeFailed[key] = why
		return why
	}
	d.fontDecoded += len(sd.Content)
	if d.fontDecodedContent == nil {
		d.fontDecodedContent = map[uintptr][]byte{}
	}
	d.fontDecodedContent[key] = sd.Content
	return ""
}
