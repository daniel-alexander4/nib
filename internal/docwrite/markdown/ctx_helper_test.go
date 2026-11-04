package markdown

import (
	"context"

	"nib/internal/docmodel"
)

// writeOK is Write with a live context, for the tests that are not about cancellation.
func writeOK(d docmodel.Document) []byte {
	b, err := Write(context.Background(), d)
	if err != nil {
		panic(err)
	}
	return b
}
