package pdfread

import (
	"errors"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/filter"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// MaxPageContentBytes bounds a page's decoded content — every stream of its `/Contents`, counted at every
// reference — and the page content pdfcpu's optimize pass decodes over a whole document (`Unaffordable`).
// It is ADR-005's ceiling on a whole open document and pdfcpu's own per-stream decode ceiling
// (`filter.DefaultMaxDecodeBytes`), and the checker's content budget (`uacheck.maxContentBytes`) is the same
// figure, so a walk has one number rather than three (`/pending 748`).
const MaxPageContentBytes = 512 << 20

// ErrDecodeLimit is `DecodeWithin`'s refusal: the stream decodes to more than its caller's limit, and nib
// stopped decoding it there.
var ErrDecodeLimit = errors.New("the stream decodes to more than nib will read")

// DecodeWithin is nib's one capped decode: it decodes sd, refusing (ErrDecodeLimit) once its DECODED size passes
// limit, and on success leaves the whole stream in sd.Content.
//
// **The cap is applied DURING the decode, not after it** — `/pending 748`. Every budget nib keeps is charged
// with a stream's decoded size, and charging it after `sd.Decode()` returns bounds what nib parses and nothing
// about what it inflated: a few hundred kilobytes of flate inflate to pdfcpu's own ceiling (512 MiB), which
// measured ~1 s and a ~1.8 GiB peak heap per stream, before a 20 MiB budget could say no. pdfcpu's
// `DecodeWithLimit` (v0.13.0) applies the limit to EVERY stage of a filter chain: Flate, LZW, RunLength, ASCII85
// and CCITTFax stop while they stream, ASCIIHex and DCT refuse from the size they would produce. A stage that
// buffers its input first (ASCIIHex, ASCII85, DCT) buffers bytes the previous stage already capped, or the raw
// stream, which ADR-005 bounds. The length check after it covers what the limit cannot see: a stream decoded
// earlier, and an unfiltered stream, whose raw bytes pdfcpu hands back unmeasured.
//
// A limit below one byte refuses any non-empty stream (pdfcpu reads a limit of 0 as "its default", so it is
// never passed through). The peak is still the decode's own buffer growth, up to about twice the limit.
func DecodeWithin(sd *types.StreamDict, limit int64) error {
	if err := sd.DecodeWithLimit(max(limit, 1)); err != nil {
		if errors.Is(err, filter.ErrDecodeLimitExceeded) {
			return decodeLimitError(limit)
		}
		return err
	}
	if int64(len(sd.Content)) > limit {
		return decodeLimitError(limit)
	}
	return nil
}

func decodeLimitError(limit int64) error {
	return fmt.Errorf("%w: it decodes past %d bytes, and nib stopped decoding it there", ErrDecodeLimit, max(limit, 0))
}
