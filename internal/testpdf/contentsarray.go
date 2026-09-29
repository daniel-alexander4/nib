package testpdf

import (
	"bytes"
	"errors"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
	"nib/internal/pdfread"
)

// JoinShape is how SplitContents divides a page's content between two streams.
type JoinShape int

const (
	// JoinRegular ends the first stream on `Tj` and begins the second on the next operator with no white-space
	// between them — legal, because a stream may end on any token boundary, and fused by a bare concatenation.
	JoinRegular JoinShape = iota
	// JoinComment ends the first stream in a `%` comment with no end-of-line, so a bare concatenation comments
	// out the second stream's first line.
	JoinComment
	// JoinSafe ends the first stream after the white-space that follows `Tj` — the shape every producer measured
	// writes, and one a bare concatenation already reads correctly.
	JoinSafe
)

// SplitContents is Text(s) with page 1's `/Contents` rewritten as an ARRAY of two streams, divided just after
// the first `Tj` in the given shape (ADR-056). It returns the document and the content the page means — the
// single stream it was divided from, which a correct join must tokenize identically (white-space aside).
func SplitContents(s string, shape JoinShape) (pdf, meant []byte, err error) {
	src, err := Text(s)
	if err != nil {
		return nil, nil, err
	}
	ctx, err := pdfread.Validated(src, model.NewDefaultConfiguration())
	if err != nil {
		return nil, nil, err
	}
	page, _, _, err := ctx.PageDict(1, false)
	if err != nil {
		return nil, nil, err
	}
	content, err := pdfread.PageContent(ctx, page, 1)
	if err != nil {
		return nil, nil, err
	}
	toks := contentstream.Tokenize(content)
	tj := -1
	for i, t := range toks {
		if t.Kind == contentstream.Operator && string(t.Bytes(content)) == "Tj" {
			tj = i
			break
		}
	}
	if tj < 0 || tj+2 >= len(toks) || toks[tj+1].Kind != contentstream.Whitespace {
		return nil, nil, errors.New("testpdf: the generated page has no `Tj` followed by white-space and a token")
	}
	tjEnd, next := toks[tj].End, toks[tj+2].Start
	var a, b []byte
	switch shape {
	case JoinRegular:
		a, b = content[:tjEnd], content[next:]
	case JoinComment:
		a, b = append(append([]byte{}, content[:tjEnd]...), " % a comment with no end of line"...), content[next:]
	default:
		a, b = content[:next], content[next:]
	}
	var arr types.Array
	for _, part := range [][]byte{a, b} {
		sd, err := ctx.NewStreamDictForBuf(append([]byte{}, part...))
		if err != nil {
			return nil, nil, err
		}
		if err := sd.Encode(); err != nil {
			return nil, nil, err
		}
		ref, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			return nil, nil, err
		}
		arr = append(arr, *ref)
	}
	page["Contents"] = arr
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		return nil, nil, err
	}
	return out.Bytes(), content, nil
}
